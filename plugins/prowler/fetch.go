// fetch.go implements the static-fetch extraction cascade: Reddit special-case
// first, then a plain HTTP GET run through Readability, falling back to raw
// body text and finally a headless-browser render when nothing else yields
// usable content. This is the same degrade-gracefully shape weblens uses.

package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"regexp"
	"strconv"

	readability "github.com/go-shiori/go-readability"
)

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
		return "# Error fetching " + url + "\n\n" + err.Error()
	}
	for key, values := range defaultHeaders() {
		req.Header[key] = values
	}

	resp, err := f.do(req)
	if err != nil {
		return "# Error fetching " + url + "\n\n" + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "# Error fetching " + url + "\n\nHTTP " + strconv.Itoa(resp.StatusCode)
	}

	rawHTML, err := io.ReadAll(resp.Body)
	if err != nil {
		return "# Error fetching " + url + "\n\n" + err.Error()
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
