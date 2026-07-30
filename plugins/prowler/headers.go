// headers.go defines the browser-identity HTTP headers and the shared client
// used by every static fetch, so requests present as a real browser instead
// of a bot-blockable default Go client.

package main

import (
	"net/http"
	"time"
)

// browserUA is the User-Agent string presented on every static HTTP fetch.
// It mirrors a recent desktop Chrome build so bot-detection heuristics that
// key off User-Agent see an ordinary browser, not a scripted client.
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// defaultHeaders returns the full header set applied to every static (non-Reddit)
// fetch request. The set mirrors a real browser's request headers closely enough
// to pass simple bot-detection checks that key off User-Agent/Accept alone.
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

// httpClient is the shared transport for every static fetch in this package.
// It follows redirects using the stdlib default policy and bounds total request
// time to ~60s so one unresponsive host cannot stall a whole multi-URL run.
var httpClient = &http.Client{
	Timeout: 60 * time.Second,
}
