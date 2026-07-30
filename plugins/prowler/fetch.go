// fetch.go implements the static-fetch extraction cascade: Reddit special-case
// first, then a plain HTTP GET run through Readability, falling back to raw
// body text and finally a headless-browser render when nothing else yields
// usable content. This is the same degrade-gracefully shape weblens uses.

package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/andybalholm/brotli"
	readability "github.com/go-shiori/go-readability"
)

// errUnsupportedContentEncoding is returned by decodeContentEncoding for a
// Content-Encoding value it has no decoder for. gzip, deflate, and br
// (Brotli, via the pure-Go andybalholm/brotli decoder — the standard
// library has none) all decode locally; anything else falls back to this
// sentinel. It is a distinct sentinel — rather than a generic decode error —
// so fetchPage can route around it to the browser fallback instead of
// surfacing it as a hard fetch failure: a real browser's network stack
// decodes encodings this function doesn't recognize, so the content is
// still recoverable via that path.
var errUnsupportedContentEncoding = errors.New("unsupported Content-Encoding")

// minUsableTextLen is the character-count threshold below which extracted
// text is considered too thin to be the article itself (e.g. a cookie
// banner or loading skeleton rather than the real content) and worth
// attempting a further fallback for.
const minUsableTextLen = 100

// scriptStyleNoscriptBlock matches <script>/<style>/<noscript> elements
// (including their contents) so they can be stripped before running
// Readability or the body-text fallback — their contents are never part of
// the readable article and can otherwise pollute extracted text.
var scriptStyleNoscriptBlock = regexp.MustCompile(`(?is)<(script|style|noscript)\b[^>]*>.*?</(script|style|noscript)>`)

// errorResult formats a fetch failure into the "# Error fetching <url>"
// markdown shape shared by every failure branch of both fetchPage and
// fetchReddit (reddit.go), so the prefix is written once rather than
// hand-rolled at each of their several error-return points.
func errorResult(url, detail string) string {
	return "# Error fetching " + url + "\n\n" + detail
}

// fetchPage fetches url and extracts its readable content, trying — in
// order — the Reddit JSON special-case, static HTML plus Readability, raw
// body text, and finally a headless-browser render. Every step degrades to
// the next rather than failing the whole fetch, matching weblens' resilient
// behavior: a bot-blocked or JS-only page should still yield the best
// content available rather than an empty result. f is the injectable
// transport/browser seam, so this whole cascade is unit-testable without
// real network or Chrome access.
func fetchPage(ctx context.Context, f fetcher, url string) string {
	// Reddit hard-blocks scraping of its HTML pages but leaves the JSON API
	// open, so a Reddit URL gets a dedicated, higher-fidelity path first.
	if isRedditUrl(url) {
		if out, handled := fetchReddit(ctx, f, url); handled {
			return out
		}
		// A Reddit-shaped URL that didn't yield a JSON response (e.g. a
		// search page) falls through to the ordinary static path below.
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		// An unparseable URL can never be sent, so there is nothing for
		// f.do to attempt — report it exactly like a transport failure.
		return errorResult(url, err.Error())
	}
	for key, values := range defaultHeaders() {
		req.Header[key] = values
	}

	resp, err := f.do(req)
	if err != nil {
		return errorResult(url, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errorResult(url, "HTTP "+strconv.Itoa(resp.StatusCode))
	}

	compressedBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return errorResult(url, err.Error())
	}
	// defaultHeaders() sends an explicit Accept-Encoding, which disables
	// http.Transport's own transparent gzip decoding (it only auto-decodes
	// when the caller has NOT set that header itself) — so a compressed
	// response must be decoded here or every downstream step sees garbage
	// bytes instead of HTML.
	rawHTML, err := decodeContentEncoding(compressedBody, resp.Header.Get("Content-Encoding"))
	if errors.Is(err, errUnsupportedContentEncoding) {
		// We cannot statically decode this encoding, so the compressed bytes
		// are useless to Readability/stripToBodyText — go straight to the
		// browser fallback rather than feeding either of them garbage.
		if browserText, ok := f.browser(ctx, url); ok && browserText != "" {
			return browserText
		}
		return "# " + url + "\n\nCould not extract readable content from this page."
	}
	if err != nil {
		return errorResult(url, err.Error())
	}
	cleaned := scriptStyleNoscriptBlock.ReplaceAll(rawHTML, nil)

	// "Succeeds" means both no error AND some non-empty text was extracted:
	// go-readability's own fallback logic returns a nil error even when it
	// finds nothing at all (e.g. the whole page's text sits inside a
	// display:none container its visibility check excludes), so a literal
	// err==nil is not by itself evidence that extraction produced anything
	// usable to threshold-check.
	article, readabilityErr := readability.FromReader(bytes.NewReader(cleaned), req.URL)
	if readabilityErr == nil && article.TextContent != "" {
		if len(article.TextContent) >= minUsableTextLen {
			return "# " + article.Title + "\n\nSource: " + url + "\n\n" + article.TextContent
		}
		// Readability parsed *something*, but too little to be confident
		// it's the real article (e.g. it landed on a cookie-notice div) —
		// try the heavier browser fallback before settling for the short
		// result.
		if browserText, ok := f.browser(ctx, url); ok && browserText != "" {
			return browserText
		}
		return "# " + article.Title + "\n\nSource: " + url + "\n\n" + article.TextContent
	}

	// Readability found no article structure at all; fall back to the raw
	// page's body text, which is often still readable for simply-formatted
	// pages Readability's heuristics don't recognize as an "article".
	if bodyText := stripToBodyText(string(rawHTML)); len(bodyText) > minUsableTextLen {
		return "# " + url + "\n\n" + bodyText
	}

	if browserText, ok := f.browser(ctx, url); ok && browserText != "" {
		return browserText
	}

	return "# " + url + "\n\nCould not extract readable content from this page."
}

// fetchOldRedditHTML is fetchReddit's fallback for when Reddit's JSON API
// itself is unreachable (e.g. a 403 from network-level bot protection): it
// retries the same post/subreddit against old.reddit.com's plain,
// server-rendered HTML page — which Reddit does not appear to gate as
// aggressively as its JSON API or its www SPA shell (the SPA shell serves a
// JS-driven "please wait" verification page to non-browser clients, which
// old.reddit.com's legacy markup does not).
//
// Deliberately skips Readability (unlike the ordinary static-fetch cascade
// in fetchPage): Readability's whole job is to find and keep only the
// single highest-scoring "article" block and discard the rest as chrome —
// but on a Reddit thread the comments are a separate, lower-scoring block
// from the self-post text, so Readability silently drops exactly the
// content a reader most wants (verified empirically: it returned only the
// post body, none of an 18-comment thread present in the same page's raw
// HTML). stripToBodyText's cruder script/style/nav/header/footer strip
// keeps everything else, comments included, which is what this fallback
// needs. It reports ok=false when the fallback request itself fails or
// yields too little content to be useful, so the caller falls back to the
// original JSON-API error instead of a near-empty result.
func fetchOldRedditHTML(ctx context.Context, f fetcher, url string) (out string, ok bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, toOldRedditURL(url), nil)
	if err != nil {
		return "", false
	}
	for key, values := range defaultHeaders() {
		req.Header[key] = values
	}

	resp, err := f.do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false
	}

	compressedBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false
	}
	rawHTML, err := decodeContentEncoding(compressedBody, resp.Header.Get("Content-Encoding"))
	if err != nil {
		// Includes errUnsupportedContentEncoding: this fallback has no
		// browser tier to route around it, so an undecodable encoding is
		// simply a failed fallback attempt.
		return "", false
	}

	if bodyText := stripToBodyText(string(rawHTML)); len(bodyText) >= minUsableTextLen {
		return "# " + url + "\n\n" + bodyText, true
	}

	return "", false
}

// decodeContentEncoding decompresses body according to the response's
// Content-Encoding header. gzip and deflate are handled directly (the two
// formats the standard library supports); an empty or "identity" encoding is
// returned unchanged; any other non-empty value (most notably "br"/Brotli,
// which weblens' Node runtime decodes natively but Go's standard library
// cannot) yields errUnsupportedContentEncoding so the caller can route
// around it instead of treating the still-compressed bytes as HTML.
func decodeContentEncoding(body []byte, contentEncoding string) ([]byte, error) {
	switch contentEncoding {
	case "", "identity":
		return body, nil
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	case "deflate":
		reader := flate.NewReader(bytes.NewReader(body))
		defer reader.Close()
		return io.ReadAll(reader)
	case "br":
		return io.ReadAll(brotli.NewReader(bytes.NewReader(body)))
	default:
		return nil, errUnsupportedContentEncoding
	}
}
