// redditrss_test.go exercises the RSS tier offline, no network: URL canonicalisation (this
// file), HTML-to-markdown conversion, and Atom parsing/mapping are all added here card by
// card as batch 2 proceeds.

package main

import (
	"fmt"
	"html"
	"os"
	"strings"
	"testing"
)

// TestRedditRSSURL covers redditRSSURL's host/scheme normalisation, query/fragment
// stripping, and idempotent ".rss" suffixing across Reddit's three host forms and both
// slash-terminated and unterminated paths.
func TestRedditRSSURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "bare_host",
			in:   "https://reddit.com/r/golang",
			want: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name: "www_host",
			in:   "https://www.reddit.com/r/golang",
			want: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name: "old_host",
			in:   "https://old.reddit.com/r/golang",
			want: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name: "http_scheme_forced_to_https",
			in:   "http://www.reddit.com/r/golang",
			want: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name: "path_with_trailing_slash",
			in:   "https://www.reddit.com/r/golang/",
			want: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name: "path_without_trailing_slash",
			in:   "https://www.reddit.com/r/golang",
			want: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name: "query_string_dropped",
			in:   "https://www.reddit.com/r/golang?sort=new",
			want: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name: "fragment_dropped",
			in:   "https://www.reddit.com/r/golang#comments",
			want: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name: "already_rss_with_trailing_slash_unchanged",
			in:   "https://www.reddit.com/r/golang/.rss/",
			want: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name: "already_rss_without_trailing_slash_unchanged",
			in:   "https://www.reddit.com/r/golang/.rss",
			want: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name: "subreddit_path",
			in:   "https://www.reddit.com/r/golang/",
			want: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name:    "empty_path_yields_error",
			in:      "https://www.reddit.com",
			wantErr: true,
		},
		{
			name:    "unparseable_url_yields_error",
			in:      "://bad",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := redditRSSURL(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("redditRSSURL(%q) error = nil; want non-nil", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("redditRSSURL(%q) error = %v; want nil", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("redditRSSURL(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}

	// The success cases above are also the idempotence corpus: feeding each one's own
	// output back through redditRSSURL must return that output unchanged, proving the
	// ".rss"-stripping step in redditRSSURL prevents "/.rss/.rss" on a second pass.
	t.Run("idempotence", func(t *testing.T) {
		for _, tt := range tests {
			if tt.wantErr {
				continue
			}
			t.Run(tt.name, func(t *testing.T) {
				first, err := redditRSSURL(tt.in)
				if err != nil {
					t.Fatalf("redditRSSURL(%q) error = %v; want nil", tt.in, err)
				}
				second, err := redditRSSURL(first)
				if err != nil {
					t.Fatalf("redditRSSURL(%q) error = %v; want nil", first, err)
				}
				if second != first {
					t.Errorf("redditRSSURL(%q) = %q; want %q (idempotent)", first, second, first)
				}
			})
		}
	})
}

// TestRedditHTMLToMarkdown covers redditHTMLToMarkdown's three rewrites -- anchors to
// markdown links, block boundaries to newlines -- and proves htmlToText itself is untouched.
func TestRedditHTMLToMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		fragment string
		want     string
	}{
		{
			name:     "absolute_href_anchor_becomes_markdown_link",
			fragment: `<p>See <a href="https://example.com/docs">the docs</a> for more.</p>`,
			want:     "[the docs](https://example.com/docs)",
		},
		{
			name:     "relative_href_absolutized",
			fragment: `<p>Check out <a href="/r/golang">/r/golang</a>.</p>`,
			want:     "[/r/golang](https://www.reddit.com/r/golang)",
		},
		{
			name:     "anchor_text_equal_to_href_renders_bare_url",
			fragment: `<p>Website: <a href="https://pingularity.dev">https://pingularity.dev</a></p>`,
			want:     "https://pingularity.dev",
		},
		{
			name:     "li_items_render_as_bullets",
			fragment: `<ul><li>first item</li><li>second item</li></ul>`,
			want:     "- first item\n- second item",
		},
		{
			name:     "nested_markup_inside_anchor_text",
			fragment: `<p><a href="https://example.com"><strong>bold</strong> text</a></p>`,
			want:     "[bold text](https://example.com)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redditHTMLToMarkdown(tt.fragment)
			if !strings.Contains(got, tt.want) {
				t.Errorf("redditHTMLToMarkdown(%q) = %q; want it to contain %q", tt.fragment, got, tt.want)
			}
		})
	}

	t.Run("paragraph_and_br_produce_blank_line_breaks", func(t *testing.T) {
		got := redditHTMLToMarkdown(`<p>first paragraph</p><p>second paragraph</p>`)
		if !strings.Contains(got, "first paragraph\n\nsecond paragraph") {
			t.Errorf("redditHTMLToMarkdown() = %q; want a blank-line break between paragraphs", got)
		}

		got = redditHTMLToMarkdown(`line one<br>line two`)
		if !strings.Contains(got, "line one\n\nline two") {
			t.Errorf("redditHTMLToMarkdown() = %q; want a blank-line break at <br>", got)
		}
	})

	t.Run("empty_href_renders_text_alone", func(t *testing.T) {
		got := redditHTMLToMarkdown(`<p>an <a href="">anchor with no href</a> here</p>`)
		if !strings.Contains(got, "anchor with no href") {
			t.Errorf("redditHTMLToMarkdown() = %q; want the anchor's bare text", got)
		}
		if strings.Contains(got, "[") || strings.Contains(got, "]") {
			t.Errorf("redditHTMLToMarkdown() = %q; want no markdown link brackets for an empty href", got)
		}
	})

	// Regression: a real comment body pulled from the committed thread fixture must still
	// carry its external link after conversion.
	t.Run("real_fixture_comment_keeps_its_external_link", func(t *testing.T) {
		data, err := os.ReadFile("testdata/reddit-thread.rss")
		if err != nil {
			t.Fatalf("os.ReadFile(reddit-thread.rss) error = %v", err)
		}
		// The fixture is raw Atom XML, so its <content> payload is entity-escaped; unescape
		// once before hunting for the literal SC_OFF/SC_ON markers. Several entries in this
		// fixture carry their own SC_OFF/SC_ON span, so scan all of them for the one
		// containing the known external link rather than assuming it is the first.
		text := html.UnescapeString(string(data))
		var fragment string
		for searchFrom := 0; ; {
			start := strings.Index(text[searchFrom:], "<!-- SC_OFF -->")
			if start == -1 {
				break
			}
			start += searchFrom
			end := strings.Index(text[start:], "<!-- SC_ON -->")
			if end == -1 {
				break
			}
			end += start + len("<!-- SC_ON -->")
			span := text[start:end]
			if strings.Contains(span, "pingularity.dev") {
				fragment = span
				break
			}
			searchFrom = end
		}
		if fragment == "" {
			t.Fatalf("testdata/reddit-thread.rss: expected fixture setup changed -- no SC_OFF/SC_ON span containing pingularity.dev found")
		}

		got := redditHTMLToMarkdown(fragment)
		if !strings.Contains(got, "https://pingularity.dev") {
			t.Errorf("redditHTMLToMarkdown() on the real fixture comment = %q; want it to still contain https://pingularity.dev", got)
		}
	})

	// Proves the shared helper is provably untouched: htmlToText's own behaviour on an
	// anchor-bearing fragment is unchanged -- the URL is still dropped.
	t.Run("htmlToText_itself_still_drops_the_href", func(t *testing.T) {
		got := htmlToText(`<p>See <a href="https://example.com/docs">the docs</a> for more.</p>`)
		if strings.Contains(got, "https://example.com/docs") {
			t.Errorf("htmlToText() = %q; want the href still dropped (htmlToText must stay untouched)", got)
		}
		if !strings.Contains(got, "the docs") {
			t.Errorf("htmlToText() = %q; want the anchor's text preserved", got)
		}
	})
}

// parseRedditFeedTestdata reads and parses a testdata Atom fixture, failing the test on
// either error.
func parseRedditFeedTestdata(t *testing.T, name string) redditAtomFeed {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", name, err)
	}
	feed, err := parseRedditFeed(data)
	if err != nil {
		t.Fatalf("parseRedditFeed(%q) error = %v; want nil", name, err)
	}
	return feed
}

// TestParseRedditFeed covers parseRedditFeed's decoding of the feed-level category and every
// entry's id, against the real committed thread fixture.
func TestParseRedditFeed(t *testing.T) {
	feed := parseRedditFeedTestdata(t, "reddit-thread.rss")

	if len(feed.Entries) != 5 {
		t.Fatalf("parseRedditFeed() len(Entries) = %d; want 5", len(feed.Entries))
	}
	if got, want := feed.Entries[0].ID, "t3_1vxc255"; got != want {
		t.Errorf("parseRedditFeed() Entries[0].ID = %q; want %q", got, want)
	}
	if got, want := feed.Category.Term, "golang"; got != want {
		t.Errorf("parseRedditFeed() Category.Term = %q; want %q", got, want)
	}
	for i, entry := range feed.Entries[1:] {
		if !strings.HasPrefix(entry.ID, "t1_") {
			t.Errorf("parseRedditFeed() Entries[%d].ID = %q; want a t1_ prefix", i+1, entry.ID)
		}
	}
}

// TestRedditPostFromFeed covers redditPostFromFeed's mapping of a full thread feed onto
// redditPost, against the real committed thread fixture.
func TestRedditPostFromFeed(t *testing.T) {
	feed := parseRedditFeedTestdata(t, "reddit-thread.rss")

	post, err := redditPostFromFeed(feed, "https://www.reddit.com/r/golang/comments/1vxc255/small_projects/")
	if err != nil {
		t.Fatalf("redditPostFromFeed() error = %v; want nil", err)
	}

	if got, want := post.Title, "Small Projects"; got != want {
		t.Errorf("redditPostFromFeed() Title = %q; want %q", got, want)
	}
	if got, want := post.Subreddit, "golang"; got != want {
		t.Errorf("redditPostFromFeed() Subreddit = %q; want %q", got, want)
	}
	if got, want := post.Author, "AutoModerator"; got != want {
		t.Errorf("redditPostFromFeed() Author = %q; want %q", got, want)
	}
	if post.Score != nil {
		t.Errorf("redditPostFromFeed() Score = %v; want nil", *post.Score)
	}
	if !strings.Contains(post.Selftext, "This is the weekly thread for Small Projects.") {
		t.Errorf("redditPostFromFeed() Selftext = %q; want it to contain the post's real body text", post.Selftext)
	}
	for _, scaffold := range []string{"submitted by", "[link]", "[comments]"} {
		if strings.Contains(post.Selftext, scaffold) {
			t.Errorf("redditPostFromFeed() Selftext = %q; want no trailer text %q", post.Selftext, scaffold)
		}
	}
	if !post.Flat {
		t.Error("redditPostFromFeed() Flat = false; want true")
	}

	if got, want := len(post.Comments), 4; got != want {
		t.Fatalf("redditPostFromFeed() len(Comments) = %d; want %d", got, want)
	}
	wantComments := []struct {
		author string
	}{
		{"cansofgrease"},
		{"realPanditJi"},
		{"SovereignZ3r0"},
		{"mrehanabbasi"},
	}
	for i, want := range wantComments {
		c := post.Comments[i]
		if c.Author != want.author {
			t.Errorf("redditPostFromFeed() Comments[%d].Author = %q; want %q", i, c.Author, want.author)
		}
		if c.Body == "" {
			t.Errorf("redditPostFromFeed() Comments[%d].Body is empty; want non-empty", i)
		}
		if c.Score != nil {
			t.Errorf("redditPostFromFeed() Comments[%d].Score = %v; want nil", i, *c.Score)
		}
		if c.Replies != nil {
			t.Errorf("redditPostFromFeed() Comments[%d].Replies = %v; want nil", i, c.Replies)
		}
	}
}

// findRedditAtomEntryByID locates the entry with the given id within a parsed feed's entries,
// failing the test if it is absent. Tests use this rather than a fixed index so a fixture
// reorder cannot silently break a test that is looking for a specific entry.
func findRedditAtomEntryByID(t *testing.T, feed redditAtomFeed, id string) redditAtomEntry {
	t.Helper()
	for _, entry := range feed.Entries {
		if entry.ID == id {
			return entry
		}
	}
	t.Fatalf("no entry with id %q found in feed", id)
	return redditAtomEntry{}
}

// TestRedditPostFromFeed_MarkerAbsentLinkPost covers the common case a whole-content fallback
// would get wrong: a link post's <content> carries no SC_OFF/SC_ON markers at all, since its
// entire content is a thumbnail table plus a "submitted by … [link] … [comments]" trailer.
func TestRedditPostFromFeed_MarkerAbsentLinkPost(t *testing.T) {
	listing := parseRedditFeedTestdata(t, "reddit-listing.rss")
	entry := findRedditAtomEntryByID(t, listing, "t3_1vx8uvc")

	feed := redditAtomFeed{Title: listing.Title, Category: listing.Category, Entries: []redditAtomEntry{entry}}
	post, err := redditPostFromFeed(feed, "https://www.reddit.com/r/golang/comments/1vx8uvc/excessive_nil_pointer_checks_in_go/")
	if err != nil {
		t.Fatalf("redditPostFromFeed() error = %v; want nil", err)
	}

	if post.Selftext != "" {
		t.Errorf("redditPostFromFeed() Selftext = %q; want empty (no SC_OFF/SC_ON markers)", post.Selftext)
	}
	const wantURL = "https://konradreiche.com/blog/excessive-nil-pointer-checks-in-go/"
	if post.URL != wantURL {
		t.Errorf("redditPostFromFeed() URL = %q; want %q (the external [link] href, not the permalink)", post.URL, wantURL)
	}

	out := formatRedditThread(post, "https://www.reddit.com/r/golang/comments/1vx8uvc/excessive_nil_pointer_checks_in_go/")
	if !strings.Contains(out, "Link: "+wantURL) {
		t.Errorf("formatRedditThread() out = %q; want a Link: line for %q", out, wantURL)
	}
	for _, scaffold := range []string{"submitted by", "[link]", "[comments]"} {
		if strings.Contains(out, scaffold) {
			t.Errorf("formatRedditThread() out = %q; want no trailer text %q", out, scaffold)
		}
	}
}

// TestRedditPostFromFeed_SelfPostURLSuppression covers the permalink comparison in
// redditRSSLinkURL: a self-post's [link] anchor points at its own permalink, so the mapped
// URL must be empty and formatRedditThread must render the selftext with no Link: line.
func TestRedditPostFromFeed_SelfPostURLSuppression(t *testing.T) {
	feed := parseRedditFeedTestdata(t, "reddit-thread.rss")
	const sourceURL = "https://www.reddit.com/r/golang/comments/1vxc255/small_projects/"

	post, err := redditPostFromFeed(feed, sourceURL)
	if err != nil {
		t.Fatalf("redditPostFromFeed() error = %v; want nil", err)
	}
	if post.URL != "" {
		t.Errorf("redditPostFromFeed() URL = %q; want empty (self-post [link] points at its own permalink)", post.URL)
	}

	out := formatRedditThread(post, sourceURL)
	if !strings.Contains(out, "This is the weekly thread for Small Projects.") {
		t.Errorf("formatRedditThread() out = %q; want the selftext rendered", out)
	}
	if strings.Contains(out, "Link:") {
		t.Errorf("formatRedditThread() out = %q; want no Link: line for a self-post", out)
	}
}

// TestRedditPostFromFeed_ZeroEntries covers the not-found fixture: a feed with no entries at
// all must yield an error, not a zero-value post.
func TestRedditPostFromFeed_ZeroEntries(t *testing.T) {
	feed := parseRedditFeedTestdata(t, "reddit-rss-notfound.rss")
	if len(feed.Entries) != 0 {
		t.Fatalf("parseRedditFeed(reddit-rss-notfound.rss) len(Entries) = %d; want 0", len(feed.Entries))
	}

	_, err := redditPostFromFeed(feed, "https://www.reddit.com/r/announcements/.rss")
	if err == nil {
		t.Fatal("redditPostFromFeed() error = nil; want non-nil for a zero-entry feed")
	}
}

// TestRedditPostFromFeed_NonT3FirstEntry covers redditPostFromFeed's kind-discriminator
// check: a first entry whose id does not start with "t3_" must yield an error naming that id.
func TestRedditPostFromFeed_NonT3FirstEntry(t *testing.T) {
	feed := redditAtomFeed{
		Title:    "some feed",
		Category: redditAtomCategory{Term: "golang"},
		Entries:  []redditAtomEntry{{ID: "t1_something", Title: "not a post"}},
	}

	_, err := redditPostFromFeed(feed, "https://www.reddit.com/r/golang/.rss")
	if err == nil {
		t.Fatal("redditPostFromFeed() error = nil; want non-nil when the first entry is not a t3_ post")
	}
	if !strings.Contains(err.Error(), "t1_something") {
		t.Errorf("redditPostFromFeed() error = %q; want it to name the offending id %q", err, "t1_something")
	}
}

// TestFormatRedditListing covers the non-thread rendering path, against the real committed
// listing fixture.
func TestFormatRedditListing(t *testing.T) {
	feed := parseRedditFeedTestdata(t, "reddit-listing.rss")
	const sourceURL = "https://www.reddit.com/r/golang/"

	out := formatRedditListing(feed, sourceURL)

	if !strings.Contains(out, "# "+feed.Title) {
		t.Errorf("formatRedditListing() out = %q; want an H1 with the feed title %q", out, feed.Title)
	}
	if !strings.Contains(out, "Source: "+sourceURL) {
		t.Errorf("formatRedditListing() out = %q; want a Source: line with %q", out, sourceURL)
	}
	for _, entry := range feed.Entries {
		want := fmt.Sprintf("- %s by u/%s: %s", entry.Title, redditRSSAuthor(entry.Author.Name), entry.Link.Href)
		if !strings.Contains(out, want) {
			t.Errorf("formatRedditListing() out missing bullet %q\nfull output:\n%s", want, out)
		}
	}
}
