// fetcher.go defines the injection seam every fetch code path is written
// against, so the whole static-fetch/Reddit/browser cascade can be unit
// tested with stubbed transports instead of real network or Chrome calls.

package main

import (
	"context"
	"net/http"
)

// fetcher bundles the two side-effecting operations the fetch cascade needs:
// issuing an HTTP request and driving a headless-browser fallback. Production
// code wires both fields to real implementations via newFetcher; tests wire
// them to stubs so the cascade logic runs deterministically with no network
// or Chrome dependency.
//
// A zero fetcher is not valid — both fields must be set before use.
type fetcher struct {
	// do performs the raw HTTP transport. Callers apply their own headers to
	// the request before invoking it; do itself adds nothing.
	do func(*http.Request) (*http.Response, error)

	// browser drives a headless-Chrome fallback fetch of url and reports
	// whether it produced usable content (the empty string with false means
	// the browser fallback was unavailable or failed).
	browser func(ctx context.Context, url string) (string, bool)
}
