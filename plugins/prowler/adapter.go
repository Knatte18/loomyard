// adapter.go defines the pluggable site-adapter mechanism that fetchPage
// consults before falling back to the generic static-fetch/Readability/
// browser cascade. Each adapter owns a higher-fidelity strategy for one
// site (e.g. Reddit's old.reddit.com HTML, Hacker News' Algolia API) so
// site-specific quirks live in their own file rather than as special-cased
// branches inside fetch.go.

package main

import "context"

// siteAdapter is a higher-fidelity fetch strategy for one site, tried by
// fetchPage before the generic cascade. Matches reports whether url belongs
// to this adapter's site. Fetch attempts the adapter's strategy and reports
// handled=false when it could not produce usable content — for example, the
// site's dedicated endpoint failed or returned something this adapter does
// not know how to interpret. A matched-but-unhandled URL falls through to
// the generic HTML cascade rather than being tried against another adapter:
// fetchPage stops scanning at the first adapter whose Matches returns true.
type siteAdapter interface {
	// Matches reports whether url belongs to this adapter's site.
	Matches(url string) bool

	// Fetch attempts this adapter's dedicated strategy for url via f (the
	// injectable transport seam). handled=false means the strategy could
	// not produce usable content and the caller should fall through to the
	// generic cascade instead of treating out as the final result.
	Fetch(ctx context.Context, f fetcher, url string) (out string, handled bool)
}

// defaultAdapters returns the production site-adapter set, in the order
// fetchPage consults them: Reddit first, then Hacker News. newFetcher wires
// this into every real fetcher; tests construct their own adapter slices
// (including nil, or stub adapters) to exercise the routing logic in
// isolation.
func defaultAdapters() []siteAdapter {
	return []siteAdapter{redditAdapter{}, hackerNewsAdapter{}}
}
