// headers.go defines the browser-identity HTTP headers and the shared client used by every static
// fetch, so requests present as a real browser instead of a bot-blockable default Go client.

package main

import (
	"net/http"
	"time"
)

// browserUA is the User-Agent string presented on every static HTTP fetch,
// designed to pass bot-detection checks.
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// defaultHeaders returns the header set for every static HTTP fetch,
// designed to pass simple bot-detection checks.
func defaultHeaders() http.Header {
	h := http.Header{}
	h.Set("User-Agent", browserUA)
	h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	h.Set("Accept-Language", "en-US,en;q=0.9,nb;q=0.8")
	h.Set("Accept-Encoding", "gzip, deflate, br")
	h.Set("Cache-Control", "no-cache")
	h.Set("DNT", "1")
	return h
}

// httpClient is the shared transport for static fetches, with a ~60s timeout
// to prevent stalling on unresponsive hosts. It follows redirects, which is
// correct for the generic cascade -- do not change that behaviour here; a
// fetch path that needs to observe a redirect instead of following it uses
// noRedirectHTTPClient.
var httpClient = &http.Client{
	Timeout: 60 * time.Second,
}

// noRedirectHTTPClient is the shared transport for fetches that must observe
// a 3xx response instead of following it, such as old.reddit.com's login
// redirect. Its CheckRedirect returns http.ErrUseLastResponse so Do returns
// the 3xx response itself rather than the page the redirect points at.
var noRedirectHTTPClient = &http.Client{
	Timeout: 60 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}
