// reddit.go implements the Reddit site adapter: Reddit hard-blocks scraping
// of its ordinary www HTML pages but does not gate old.reddit.com's legacy,
// server-rendered markup nearly as aggressively, so redditAdapter fetches
// that page directly rather than the modern SPA shell.

package main

import (
	"context"
	"regexp"
)

// redditHostPattern matches Reddit URLs across its three common host forms
// (bare, www, and old.reddit.com); all three serve the same content, so any
// of them is eligible for the old.reddit.com HTML strategy.
var redditHostPattern = regexp.MustCompile(`^https?://(www\.|old\.)?reddit\.com`)

// redditHostReplace captures the scheme separately from the host so
// toOldRedditURL can rewrite just the host segment, regardless of which of
// the three eligible forms (bare, www, old.) the input used.
var redditHostReplace = regexp.MustCompile(`^(https?://)(www\.|old\.)?reddit\.com`)

// maxTopComments bounds how many top-level comments an adapter includes when
// formatting a discussion thread into markdown, mirroring weblens' behavior
// of showing only the first ~20 rather than an entire, potentially huge,
// comment tree. Used by the Hacker News adapter; declared here alongside
// Reddit's other retained constants. redditAdapter keeps all comments
// unbounded via stripToBodyText.
const maxTopComments = 20

// toOldRedditURL rewrites a Reddit URL to its old.reddit.com equivalent,
// e.g. "https://www.reddit.com/r/foo" -> "https://old.reddit.com/r/foo". This
// is a no-op rewrite when the input is already old.reddit.com, which is
// harmless: redditAdapter always fetches the plain HTML page regardless of
// which of the three eligible host forms the input used.
func toOldRedditURL(rawURL string) string {
	return redditHostReplace.ReplaceAllString(rawURL, "${1}old.reddit.com")
}

// redditAdapter is the siteAdapter for Reddit: it delegates entirely to
// fetchOldRedditHTML's old.reddit.com HTML strategy.
type redditAdapter struct{}

// Matches reports whether url points at Reddit, across its bare, www, and
// old.reddit.com host forms.
func (redditAdapter) Matches(url string) bool {
	return redditHostPattern.MatchString(url)
}

// Fetch retrieves url's old.reddit.com equivalent and formats it into
// markdown. It reports handled=false when fetchOldRedditHTML's request
// fails or yields too little content to be useful, so fetchPage falls
// through to the generic HTML cascade instead of returning a near-empty
// result.
func (redditAdapter) Fetch(ctx context.Context, f fetcher, url string) (string, bool) {
	return fetchOldRedditHTML(ctx, f, url)
}
