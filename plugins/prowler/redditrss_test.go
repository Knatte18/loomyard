// redditrss_test.go exercises the RSS tier offline, no network: URL canonicalisation (this
// file), HTML-to-markdown conversion, and Atom parsing/mapping are all added here card by
// card as batch 2 proceeds.

package main

import (
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
