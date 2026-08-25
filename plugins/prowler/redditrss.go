// redditrss.go implements Reddit's unauthenticated ".rss" read tier: URL canonicalisation, per-IP
// pacing, Atom parsing, and markdown rendering. It is the tier that needs no credentials and no app
// registration -- unlike redditoauth.go's tier 1, it works for every reader with no setup at all.

package main

import (
	"encoding/xml"
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

// redditAtomCategory decodes an Atom <category> element's "term" attribute -- on the
// feed-level element this is the subreddit name.
type redditAtomCategory struct {
	Term string `xml:"term,attr"`
}

// redditAtomAuthor decodes an Atom <author> element's <name> child, the only part of it this
// tier reads.
type redditAtomAuthor struct {
	Name string `xml:"name"`
}

// redditAtomLink decodes an Atom <link> element's "href" attribute, the only part of it this
// tier reads.
type redditAtomLink struct {
	Href string `xml:"href,attr"`
}

// redditAtomFeed decodes an unauthenticated Reddit .rss feed: its title, its feed-level
// <category term=…> (the subreddit name), and every entry in document order.
type redditAtomFeed struct {
	Title    string             `xml:"title"`
	Category redditAtomCategory `xml:"category"`
	Entries  []redditAtomEntry  `xml:"entry"`
}

// redditAtomEntry decodes one Atom <entry>: its title, id, author, HTML content, and its own
// link.
//
// <id> carries Reddit's fullname with its kind prefix -- "t3_" for a post, "t1_" for a
// comment -- and is this tier's only kind discriminator, mirroring redditChild.Kind on the
// OAuth side. Unmapped elements such as media:thumbnail, <updated>, and <published> are
// ignored by encoding/xml and need no fields here.
type redditAtomEntry struct {
	Title   string           `xml:"title"`
	ID      string           `xml:"id"`
	Author  redditAtomAuthor `xml:"author"`
	Content string           `xml:"content"`
	Link    redditAtomLink   `xml:"link"`
}

// parseRedditFeed unmarshals an unauthenticated .rss response body into a redditAtomFeed,
// wrapping any decode failure with the payload's own context.
func parseRedditFeed(body []byte) (redditAtomFeed, error) {
	var feed redditAtomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return redditAtomFeed{}, fmt.Errorf("decode reddit atom feed: %w", err)
	}
	return feed, nil
}

// redditRSSAuthor trims a leading "/u/" or "u/" from an Atom author name. The feed emits
// "/u/username", while formatRedditThread's own rendering already prefixes "u/" itself; left
// untrimmed, a rendered author would read "u//u/username".
func redditRSSAuthor(name string) string {
	name = strings.TrimPrefix(name, "/u/")
	name = strings.TrimPrefix(name, "u/")
	return name
}

// redditRSSSCOffMarker and redditRSSSCOnMarker bound the actual post/comment body within a
// Reddit Atom <content> payload -- everything outside them is Reddit's own rendering
// scaffolding (a thumbnail table, a "submitted by … [link] … [comments]" trailer), never the
// author's own text.
const (
	redditRSSSCOffMarker = "<!-- SC_OFF -->"
	redditRSSSCOnMarker  = "<!-- SC_ON -->"
)

// redditRSSBody returns redditHTMLToMarkdown applied to the span of content strictly between
// redditRSSSCOffMarker and redditRSSSCOnMarker, and the empty string when either marker is
// absent.
//
// It never falls back to the whole content: a link post carries no markers at all, and its
// entire content is a thumbnail <table> plus a "submitted by … [link] … [comments]" trailer --
// a whole-content fallback would render that scaffolding as the post's body. This is the
// common case for link posts in this tier's fixtures, not a rare edge case.
func redditRSSBody(content string) string {
	start := strings.Index(content, redditRSSSCOffMarker)
	if start == -1 {
		return ""
	}
	start += len(redditRSSSCOffMarker)

	end := strings.Index(content[start:], redditRSSSCOnMarker)
	if end == -1 {
		return ""
	}

	return redditHTMLToMarkdown(content[start : start+end])
}

// redditRSSLinkURL parses content with goquery and returns the href of the anchor whose
// trimmed text is exactly "[link]", when that href differs from permalink -- otherwise the
// empty string. The permalink comparison is what stops a self-post's "[comments]"-only
// trailer from rendering as a Link: line pointing back at the page the reader is already on.
func redditRSSLinkURL(content, permalink string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + content + "</div>"))
	if err != nil {
		return ""
	}

	var href string
	doc.Find("a").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if strings.TrimSpace(s.Text()) != "[link]" {
			return true
		}
		href, _ = s.Attr("href")
		return false
	})

	if href == "" || href == permalink {
		return ""
	}
	return href
}

// redditPostFromFeed maps feed onto the tier-neutral redditPost representation: the thread
// mapping for this tier. It returns an error when feed.Entries is empty, and an error naming
// the offending id when the first entry's <id> does not start with "t3_" -- a response
// carrying no post is a tier failure rather than an empty document.
//
// Flat is set to true explicitly, never inferred: the feed carries every comment as a sibling
// with no parent reference of any kind, so depth is genuinely unrecoverable and
// "## Top Comments" would be a false claim about entries that are not necessarily top-level.
//
// It never truncates -- the maxTopComments cap lives in formatRedditThread alone, exactly as
// on the OAuth side.
func redditPostFromFeed(feed redditAtomFeed, sourceURL string) (redditPost, error) {
	if len(feed.Entries) == 0 {
		return redditPost{}, fmt.Errorf("reddit atom feed %q has no entries", sourceURL)
	}

	first := feed.Entries[0]
	if !strings.HasPrefix(first.ID, "t3_") {
		return redditPost{}, fmt.Errorf("reddit atom feed %q: first entry id %q is not a post (t3_)", sourceURL, first.ID)
	}

	post := redditPost{
		Title:     first.Title,
		Subreddit: feed.Category.Term,
		Author:    redditRSSAuthor(first.Author.Name),
		Score:     nil,
		Selftext:  redditRSSBody(first.Content),
		URL:       redditRSSLinkURL(first.Content, first.Link.Href),
		Flat:      true,
	}

	for _, entry := range feed.Entries[1:] {
		if !strings.HasPrefix(entry.ID, "t1_") {
			continue
		}
		// An entry whose body comes back empty is skipped rather than rendered as a blank
		// comment -- redditRSSBody returns "" when the SC_OFF/SC_ON markers are absent.
		body := redditRSSBody(entry.Content)
		if body == "" {
			continue
		}
		post.Comments = append(post.Comments, redditComment{
			Author: redditRSSAuthor(entry.Author.Name),
			Score:  nil,
			Body:   body,
		})
	}

	return post, nil
}

// formatRedditListing renders feed as markdown for the non-thread case -- a subreddit or user
// feed with no single post to anchor a redditPost mapping to. It renders an H1 from the feed
// title, a "Source:" line pointing at sourceURL (the caller's original URL, never the derived
// .rss URL, exactly as on the thread branch), and one bullet per entry giving its title,
// author, and link.
func formatRedditListing(feed redditAtomFeed, sourceURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", feed.Title)
	fmt.Fprintf(&b, "Source: %s\n\n", sourceURL)

	for _, entry := range feed.Entries {
		fmt.Fprintf(&b, "- %s by u/%s: %s\n", entry.Title, redditRSSAuthor(entry.Author.Name), entry.Link.Href)
	}

	return strings.TrimRight(b.String(), "\n")
}
