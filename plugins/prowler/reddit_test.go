// reddit_test.go exercises Reddit URL matching, the old.reddit.com host rewrite, and
// redditAdapter.Fetch's success/failure branches via a stubbed fetcher.do.
// No network.

package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRedditAdapterMatches(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"bare_reddit", "https://reddit.com/r/golang", true},
		{"www_reddit", "https://www.reddit.com/r/golang", true},
		{"old_reddit", "https://old.reddit.com/r/golang", true},
		{"http_scheme", "http://reddit.com/r/golang", true},
		{"non_reddit", "https://example.com/r/golang", false},
		{"lookalike_host", "https://notreddit.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (redditAdapter{}).Matches(tt.url); got != tt.want {
				t.Errorf("redditAdapter{}.Matches(%q) = %v; want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestToOldRedditURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"bare_reddit", "https://reddit.com/r/golang/comments/abc/post", "https://old.reddit.com/r/golang/comments/abc/post"},
		{"www_reddit", "https://www.reddit.com/r/golang/comments/abc/post", "https://old.reddit.com/r/golang/comments/abc/post"},
		{"already_old_reddit", "https://old.reddit.com/r/golang/comments/abc/post", "https://old.reddit.com/r/golang/comments/abc/post"},
		{"http_scheme", "http://reddit.com/r/golang", "http://old.reddit.com/r/golang"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toOldRedditURL(tt.url); got != tt.want {
				t.Errorf("toOldRedditURL(%q) = %q; want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestRedditAdapterFetch(t *testing.T) {
	const url = "https://www.reddit.com/r/golang/comments/abc/some_post"
	const oldURL = "https://old.reddit.com/r/golang/comments/abc/some_post"

	t.Run("handled_true_with_body_on_success", func(t *testing.T) {
		f := stubResponses(t, map[string]*http.Response{
			oldURL: htmlResponse(redditLikeHTMLWithComments),
		}, nil)

		out, handled := (redditAdapter{}).Fetch(context.Background(), f, url)
		if !handled {
			t.Fatalf("redditAdapter{}.Fetch() handled = false; want true")
		}
		if !strings.Contains(out, "original self-post text") {
			t.Errorf("redditAdapter{}.Fetch() out = %q; want the post text", out)
		}
		if !strings.Contains(out, "First commenter's opinion") {
			t.Errorf("redditAdapter{}.Fetch() out = %q; want the comment text", out)
		}
	})

	t.Run("handled_false_when_fetchOldRedditHTML_fails", func(t *testing.T) {
		respond := func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("blocked"))}, nil
		}
		f := fetcher{do: respond, doNoRedirect: respond}

		out, handled := (redditAdapter{}).Fetch(context.Background(), f, url)
		if handled {
			t.Fatalf("redditAdapter{}.Fetch() handled = true, out = %q; want false", out)
		}
	})
}
