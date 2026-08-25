// fetch.go implements the static-fetch extraction cascade: a matching site adapter first, then a
// plain HTTP GET run through Readability, falling back to raw body text and finally a
// headless-browser render when nothing else yields usable content.
// This is the same degrade-gracefully shape weblens uses.

package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/andybalholm/brotli"
	readability "github.com/go-shiori/go-readability"
)

// errUnsupportedContentEncoding is returned by decodeContentEncoding for
// unsupported encodings, allowing fetchPage to route to browser fallback.
var errUnsupportedContentEncoding = errors.New("unsupported Content-Encoding")

// minUsableTextLen is the character-count threshold; below this, extracted
// text is often chrome rather than article content.
const minUsableTextLen = 100

// scriptStyleNoscriptBlock matches <script>/<style>/<noscript> elements
// (including their contents) so they can be stripped before running
// Readability or the body-text fallback — their contents are never part of
// the readable article and can otherwise pollute extracted text.
var scriptStyleNoscriptBlock = regexp.MustCompile(`(?is)<(script|style|noscript)\b[^>]*>.*?</(script|style|noscript)>`)

// errorResult formats fetch failures into the standard "# Error fetching
// <url>" markdown format.
func errorResult(url, detail string) string {
	return "# Error fetching " + url + "\n\n" + detail
}

// browserFallback drives f's headless-browser fallback and, when it reports
// success with non-empty text, additionally runs that text through
// looksLikeBlockPage before trusting it. This exists because a headless
// render can land on the same bot-challenge/block page a static fetch would,
// and the browser's own success/failure signal has no way to distinguish a
// rendered wall from rendered content.
func browserFallback(ctx context.Context, f fetcher, url string) (string, bool) {
	text, ok := f.browser(ctx, url)
	if !ok || text == "" {
		return "", false
	}
	if _, blocked := looksLikeBlockPage(text); blocked {
		return "", false
	}
	return text, true
}

// fetchPage fetches url and extracts readable content, trying site adapters
// then HTML+Readability, body text, and browser render, cascading gracefully.
// f bundles injectable transport and browser for testability.
func fetchPage(ctx context.Context, f fetcher, url string) string {
	for _, adapter := range f.adapters {
		if !adapter.Matches(url) {
			continue
		}
		if out, handled := adapter.Fetch(ctx, f, url); handled {
			return out
		}
		break
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
		if browserText, ok := browserFallback(ctx, f, url); ok {
			return browserText
		}
		return "# " + url + "\n\nCould not extract readable content from this page."
	}
	if err != nil {
		return errorResult(url, err.Error())
	}

	// Check the raw static HTML for a bot wall before spending Readability
	// or the body-text fallback on it: both would happily "succeed" on a
	// challenge page's chrome text, since it comfortably clears
	// minUsableTextLen. A real headless browser can still legitimately
	// clear some challenges on non-Reddit sites, so route straight to it
	// rather than failing outright.
	if reason, blocked := looksLikeBlockPage(string(rawHTML)); blocked {
		if browserText, ok := browserFallback(ctx, f, url); ok {
			return browserText
		}
		return errorResult(url, "blocked: "+reason)
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
		if browserText, ok := browserFallback(ctx, f, url); ok {
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

	if browserText, ok := browserFallback(ctx, f, url); ok {
		return browserText
	}

	return "# " + url + "\n\nCould not extract readable content from this page."
}

// fetchOldRedditHTML fetches Reddit URLs from old.reddit.com's HTML, which
// deliberately skips Readability to preserve comments that Readability
// would drop. Anonymous access to old.reddit.com is now login-gated: the
// request is sent through f.doNoRedirect so a login redirect is observed
// rather than followed, and the returned error names the specific reason a
// fetch failed -- a redirect to Reddit's login page, a non-2xx status, a
// transport failure, an undecodable Content-Encoding, a decoded page that
// looksLikeBlockPage flags, or too little extracted content -- rather than
// collapsing every failure into a bare false.
func fetchOldRedditHTML(ctx context.Context, f fetcher, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, toOldRedditURL(url), nil)
	if err != nil {
		return "", fmt.Errorf("build old.reddit.com request: %w", err)
	}
	for key, values := range defaultHeaders() {
		req.Header[key] = values
	}

	resp, err := f.doNoRedirect(req)
	if err != nil {
		return "", fmt.Errorf("old.reddit.com request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// old.reddit.com now redirects anonymous readers to a login page
		// instead of serving the requested content -- this is the defect
		// this card fixes, since the shared (redirect-following) client
		// previously followed this and reported the login page as content.
		return "", fmt.Errorf("old.reddit.com redirected (login-gated), status %d, Location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("old.reddit.com request returned status %d", resp.StatusCode)
	}

	compressedBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read old.reddit.com response: %w", err)
	}
	rawHTML, err := decodeContentEncoding(compressedBody, resp.Header.Get("Content-Encoding"))
	if err != nil {
		// Includes errUnsupportedContentEncoding: this fallback has no
		// browser tier to route around it, so an undecodable encoding is
		// simply a failed fallback attempt.
		return "", fmt.Errorf("decode old.reddit.com response: %w", err)
	}

	if reason, blocked := looksLikeBlockPage(string(rawHTML)); blocked {
		return "", fmt.Errorf("old.reddit.com response looked like a wall (%s)", reason)
	}

	bodyText := stripToBodyText(string(rawHTML))
	if len(bodyText) < minUsableTextLen {
		return "", fmt.Errorf("old.reddit.com response yielded too little content")
	}

	return "# " + url + "\n\n" + bodyText, nil
}

// decodeContentEncoding decompresses body according to Content-Encoding.
// Handles gzip, deflate, and Brotli; returns errUnsupportedContentEncoding
// for others so the caller can route to browser fallback.
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
