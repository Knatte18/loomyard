// reddit.go implements the Reddit site adapter as three tiers, tried in order and never falling
// through to the generic headless-browser cascade: the authenticated OAuth API when credentials are
// configured, then an anonymous old.reddit.com HTML fetch (now login-gated for anonymous readers,
// see fetchOldRedditHTML), and finally a markdown error naming why each attempted tier failed.

package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
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

// redditAdapter is the siteAdapter for Reddit: it tries the authenticated
// OAuth API, then an anonymous old.reddit.com HTML fetch, and always
// reports handled=true -- it never falls through to the generic
// headless-browser cascade, since a second headless request against a
// solvable-looking Reddit challenge has been measured to escalate it into a
// hard IP-level block rather than recover it.
type redditAdapter struct{}

// Matches reports whether url points at Reddit, across its bare, www, and old.reddit.com host
// forms.
func (redditAdapter) Matches(url string) bool {
	return redditHostPattern.MatchString(url)
}

// Fetch runs url through Reddit's three fetch tiers in order and always
// reports handled=true, so a Reddit URL never falls through to fetchPage's
// generic static-fetch/Readability/browser cascade -- in particular, f.browser
// is never called from this method, directly or indirectly.
//
// Tier 1 uses the authenticated OAuth API (fetchRedditOAuthThread) when
// redditCredentials reports no missing environment variables; when
// credentials are absent, this tier is skipped without issuing any request.
// Tier 2 falls back to an anonymous old.reddit.com HTML fetch
// (fetchOldRedditHTML), which Reddit currently login-gates for anonymous
// readers. When both tiers fail, Fetch returns an errorResult-formatted
// markdown error listing each attempted tier and its failure reason.
func (redditAdapter) Fetch(ctx context.Context, f fetcher, url string) (string, bool) {
	var attempts []string

	// redditCredentials is re-consulted here (rather than only inside
	// fetchRedditOAuthThread) so a missing-credentials skip can be recorded
	// without issuing any request at all.
	_, _, missing := redditCredentials()
	if len(missing) == 0 {
		out, err := fetchRedditOAuthThread(ctx, f, url)
		if err == nil {
			return out, true
		}
		attempts = append(attempts, fmt.Sprintf("- Tier 1 (Reddit OAuth API): %s", err))
	} else {
		attempts = append(attempts, fmt.Sprintf("- Tier 1 (Reddit OAuth API): skipped, missing environment variables: %s", strings.Join(missing, ", ")))
	}

	out, err := fetchOldRedditHTML(ctx, f, url)
	if err == nil {
		return out, true
	}
	attempts = append(attempts, fmt.Sprintf("- Tier 2 (old.reddit.com HTML): %s", err))

	return errorResult(url, strings.Join(attempts, "\n")), true
}
