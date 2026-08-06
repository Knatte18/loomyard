// fetcher.go defines the injection seam every fetch code path is written
// against, so the whole static-fetch/Reddit/browser cascade can be unit
// tested with stubbed transports instead of real network or Chrome calls.

package main

import (
	"context"
	"net/http"
)

// fetcher bundles the two side-effecting operations the fetch cascade needs:
// issuing HTTP requests and driving a headless-browser fallback. Both fields
// must be set before use.
type fetcher struct {
	// do performs the raw HTTP transport.
	do func(*http.Request) (*http.Response, error)

	// browser drives a headless-Chrome fallback fetch and reports whether it
	// produced usable content (empty string with false means unavailable/failed).
	browser func(ctx context.Context, url string) (string, bool)

	// adapters are the site-specific fetch strategies fetchPage tries before
	// falling back to the generic cascade. Nil or empty slice is valid.
	adapters []siteAdapter
}
