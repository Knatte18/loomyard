// redditrss_test.go exercises the RSS tier offline, no network: URL canonicalisation (this
// file), HTML-to-markdown conversion, and Atom parsing/mapping are all added here card by
// card as batch 2 proceeds.

package main

import (
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
