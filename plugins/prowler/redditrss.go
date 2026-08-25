// redditrss.go implements Reddit's unauthenticated ".rss" read tier: URL canonicalisation, per-IP
// pacing, Atom parsing, and markdown rendering. It is the tier that needs no credentials and no app
// registration -- unlike redditoauth.go's tier 1, it works for every reader with no setup at all.

package main

import (
	"fmt"
	"net/url"
	"strings"
)

// redditRSSHost is the host every canonical .rss URL is built against, mirroring
// redditOAuthHost's role on the OAuth tier. Matches in reddit.go accepts bare, www, and old
// hosts alike; normalising every one of them onto this single host keeps error strings and
// fixtures singular rather than forking three ways.
const redditRSSHost = "www.reddit.com"

// redditRSSURL rewrites rawURL into the equivalent unauthenticated .rss feed URL: scheme
// forced to https, host normalised to redditRSSHost, any incoming query or fragment
// discarded, and the path made to end in exactly one ".rss" suffix. It returns an error when
// rawURL does not parse or has an empty path, mirroring redditOAuthURL's error shape in
// redditoauth.go.
//
// The path rewrite strips an optional trailing "/", then a trailing ".rss", then another
// optional trailing "/" -- in that order -- before re-appending "/.rss". Each optional strip
// tolerates a stray slash on either side of the ".rss" suffix, so any already-canonical form
// (with or without that slash) collapses back to the bare resource path first. This is what
// makes the function idempotent: feeding its own output back in returns that output
// unchanged. Appending "/.rss" without first stripping an existing ".rss" suffix would instead
// turn an already-".rss" path into "/.rss/.rss" on a second pass.
func redditRSSURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse reddit URL %q: %w", rawURL, err)
	}
	if u.Path == "" {
		return "", fmt.Errorf("reddit URL %q has no path", rawURL)
	}

	u.Scheme = "https"
	u.Host = redditRSSHost
	u.RawQuery = ""
	u.Fragment = ""

	path := strings.TrimSuffix(u.Path, "/")
	path = strings.TrimSuffix(path, ".rss")
	path = strings.TrimSuffix(path, "/")
	u.Path = path + "/.rss"

	return u.String(), nil
}
