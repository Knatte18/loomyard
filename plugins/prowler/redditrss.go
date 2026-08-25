// redditrss.go implements Reddit's unauthenticated ".rss" read tier: URL canonicalisation, per-IP
// pacing, Atom parsing, and markdown rendering. It is the tier that needs no credentials and no app
// registration -- unlike redditoauth.go's tier 1, it works for every reader with no setup at all.

package main

import (
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
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

// redditRSSBaseURL is the base every root-relative href in a Reddit Atom <content> payload is
// resolved against. Reddit emits anchors such as "/r/golang" and "/u/name" with no scheme or
// host at all, and a link rendered without one would be unopenable outside reddit.com.
var redditRSSBaseURL = &url.URL{Scheme: "https", Host: redditRSSHost}

// redditHTMLToMarkdown converts one Reddit Atom <content> payload -- HTML, unlike the OAuth
// tier's Reddit-markdown bodies -- into markdown, then delegates whitespace normalisation and
// tag stripping to the existing htmlToText.
//
// A bare htmlToText call is not enough for this tier: htmlToText is built on goquery's
// .Text(), which discards every href and every block boundary. A comment written
// "[the docs](https://example.com)" would arrive as the bare words "the docs" with the URL
// gone -- and links are the substance of the use case this tier exists for -- and a
// five-paragraph post would arrive as one run-on line. redditHTMLToMarkdown rewrites anchors
// into markdown links and block boundaries into newlines before handing off to htmlToText, so
// both survive; htmlToText itself is untouched, since the generic fetch cascade and the Hacker
// News adapter both depend on its current link-and-block-dropping behaviour.
func redditHTMLToMarkdown(fragment string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + fragment + "</div>"))
	if err != nil {
		return htmlToText(fragment)
	}

	// Anchors become markdown links: "[text](resolved-href)", collapsing to the bare URL
	// when the anchor's text already equals its resolved href, and to the text alone when
	// href is empty.
	doc.Find("a").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		text := strings.TrimSpace(s.Text())

		replacement := text
		if href != "" {
			resolved := redditResolveHref(href)
			if text == resolved {
				replacement = resolved
			} else {
				replacement = "[" + text + "](" + resolved + ")"
			}
		}
		s.ReplaceWithHtml(html.EscapeString(replacement))
	})

	// Block boundaries become newlines, so htmlToText's normalizeWhitespace has something to
	// collapse instead of a single run-on line.
	doc.Find("br").Each(func(_ int, s *goquery.Selection) {
		s.ReplaceWithHtml("\n\n")
	})
	doc.Find("p, blockquote").Each(func(_ int, s *goquery.Selection) {
		s.AppendHtml("\n\n")
	})
	doc.Find("li").Each(func(_ int, s *goquery.Selection) {
		s.PrependHtml("- ")
		s.AppendHtml("\n")
	})

	rewritten, err := doc.Find("div").First().Html()
	if err != nil {
		return htmlToText(fragment)
	}
	return htmlToText(rewritten)
}

// redditResolveHref resolves href against redditRSSBaseURL when it is root-relative, and
// returns it unchanged when it fails to parse -- a best-effort literal is more useful than an
// empty link.
func redditResolveHref(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	return redditRSSBaseURL.ResolveReference(u).String()
}
