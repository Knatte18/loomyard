// reddit.go implements the Reddit site adapter: Reddit hard-blocks scraping of its ordinary www
// HTML pages but does not gate old.reddit.com's legacy, server-rendered markup nearly as
// aggressively, so redditAdapter fetches that page directly rather than the modern SPA shell.

package main

import (
	"context"
	"regexp"
)

// redditHostPattern matches Reddit URLs across its three host forms
// (bare, www, and old.reddit.com).
var redditHostPattern = regexp.MustCompile(`^https?://(www\.|old\.)?reddit\.com`)

// redditHostReplace captures scheme and host for rewriting to old.reddit.com.
var redditHostReplace = regexp.MustCompile(`^(https?://)(www\.|old\.)?reddit\.com`)

// maxTopComments bounds top-level comments included when formatting
// threads, and (in redditoauth.go's formatRedditThread) also bounds the
// replies rendered under each top-level comment. Used by the Hacker News
// adapter and the Reddit OAuth adapter.
const maxTopComments = 20

// toOldRedditURL rewrites a Reddit URL to its old.reddit.com equivalent.
// No-op when already old.reddit.com.
func toOldRedditURL(rawURL string) string {
	return redditHostReplace.ReplaceAllString(rawURL, "${1}old.reddit.com")
}

// redditAdapter is the siteAdapter for Reddit: it delegates entirely to
// fetchOldRedditHTML's old.reddit.com HTML strategy.
type redditAdapter struct{}

// Matches reports whether url points at Reddit, across its bare, www, and old.reddit.com host
// forms.
func (redditAdapter) Matches(url string) bool {
	return redditHostPattern.MatchString(url)
}

// Fetch retrieves url's old.reddit.com equivalent and formats it into markdown.
// Reports handled=false when request fails or content is insufficient.
func (redditAdapter) Fetch(ctx context.Context, f fetcher, url string) (string, bool) {
	return fetchOldRedditHTML(ctx, f, url)
}
