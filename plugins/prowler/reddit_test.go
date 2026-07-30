// reddit_test.go exercises Reddit URL classification, JSON transforms, and
// formatting purely over in-memory fixtures, plus fetchReddit's branches via
// a stubbed fetcher.do. No network.

package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestIsRedditUrl(t *testing.T) {
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
			if got := isRedditUrl(tt.url); got != tt.want {
				t.Errorf("isRedditUrl(%q) = %v; want %v", tt.url, got, tt.want)
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

func TestToRedditJsonUrl(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"trailing_slash", "https://reddit.com/r/golang/", "https://reddit.com/r/golang.json"},
		{"no_trailing_slash", "https://reddit.com/r/golang", "https://reddit.com/r/golang.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toRedditJsonUrl(tt.url); got != tt.want {
				t.Errorf("toRedditJsonUrl(%q) = %q; want %q", tt.url, got, tt.want)
			}
		})
	}
}

const redditPostFixture = `[
  {"kind":"Listing","data":{"children":[{"kind":"t3","data":{
    "title":"Test Title","subreddit":"golang","score":42,"num_comments":7,
    "selftext":"Hello self text","url":"https://example.com/post"}}]}},
  {"kind":"Listing","data":{"children":[
    {"kind":"t1","data":{"author":"alice","score":5,"body":"Nice post"}},
    {"kind":"more","data":{}}
  ]}}
]`

const redditPostFixtureNoSelftext = `[
  {"kind":"Listing","data":{"children":[{"kind":"t3","data":{
    "title":"Link Post","subreddit":"golang","score":1,"num_comments":0,
    "url":"https://example.com/target"}}]}},
  {"kind":"Listing","data":{"children":[]}}
]`

const redditPostFixtureEmptyChildren = `[
  {"kind":"Listing","data":{"children":[]}},
  {"kind":"Listing","data":{"children":[]}}
]`

func TestFormatRedditPost(t *testing.T) {
	t.Run("title_subreddit_score_comments_and_selftext", func(t *testing.T) {
		got := formatRedditPost([]byte(redditPostFixture))
		for _, want := range []string{
			"# Test Title",
			"r/golang | 42 points | 7 comments",
			"Hello self text",
			"## Top Comments",
			"**alice** (5 pts):\nNice post",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("formatRedditPost() = %q; want it to contain %q", got, want)
			}
		}
		if strings.Contains(got, "more") {
			t.Errorf("formatRedditPost() included a non-t1 comment thing: %q", got)
		}
	})

	t.Run("no_selftext_renders_link", func(t *testing.T) {
		got := formatRedditPost([]byte(redditPostFixtureNoSelftext))
		want := "Link: https://example.com/target"
		if !strings.Contains(got, want) {
			t.Errorf("formatRedditPost() = %q; want it to contain %q", got, want)
		}
		if strings.Contains(got, "## Top Comments") {
			t.Errorf("formatRedditPost() = %q; want no comments section", got)
		}
	})

	t.Run("empty_children_returns_empty", func(t *testing.T) {
		if got := formatRedditPost([]byte(redditPostFixtureEmptyChildren)); got != "" {
			t.Errorf("formatRedditPost() = %q; want \"\"", got)
		}
	})
}

const redditSubredditFixture = `{"kind":"Listing","data":{"children":[
  {"kind":"t3","data":{"title":"Post One","subreddit":"golang","score":10,"num_comments":3,"selftext":"Some short text"}},
  {"kind":"t3","data":{"title":"Post Two","subreddit":"golang","score":20,"num_comments":5}}
]}}`

const redditSubredditFixtureEmpty = `{"kind":"Listing","data":{"children":[]}}`

func TestFormatRedditSubreddit(t *testing.T) {
	t.Run("renders_listing", func(t *testing.T) {
		got := formatRedditSubreddit([]byte(redditSubredditFixture))
		for _, want := range []string{
			"# r/golang",
			"- **Post One** (10 pts, 3 comments) Some short text",
			"- **Post Two** (20 pts, 5 comments)",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("formatRedditSubreddit() = %q; want it to contain %q", got, want)
			}
		}
	})

	t.Run("empty_returns_empty", func(t *testing.T) {
		if got := formatRedditSubreddit([]byte(redditSubredditFixtureEmpty)); got != "" {
			t.Errorf("formatRedditSubreddit() = %q; want \"\"", got)
		}
	})
}

// newTestResponse builds a minimal *http.Response suitable for handing back
// from a stubbed fetcher.do.
func newTestResponse(status int, contentType, body string) *http.Response {
	resp := &http.Response{
		StatusCode: status,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	if contentType != "" {
		resp.Header.Set("Content-Type", contentType)
	}
	return resp
}

func TestFetchReddit(t *testing.T) {
	const url = "https://reddit.com/r/golang/comments/abc123/some_post/"

	t.Run("transport_error", func(t *testing.T) {
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		}}
		out, handled := fetchReddit(context.Background(), f, url)
		if !handled {
			t.Fatalf("fetchReddit() handled = false; want true")
		}
		if !strings.HasPrefix(out, "# Error fetching "+url) || !strings.Contains(out, "boom") {
			t.Errorf("fetchReddit() out = %q; want an error message containing url and \"boom\"", out)
		}
	})

	t.Run("non_2xx_status_and_old_reddit_fallback_also_fails", func(t *testing.T) {
		// Every request (both the .json call and the old.reddit.com HTML
		// fallback fetchReddit now attempts) gets the same 404, so the
		// fallback fails too and the original JSON-API error surfaces.
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			return newTestResponse(404, "application/json", ""), nil
		}}
		out, handled := fetchReddit(context.Background(), f, url)
		if !handled {
			t.Fatalf("fetchReddit() handled = false; want true")
		}
		if !strings.Contains(out, "HTTP 404") {
			t.Errorf("fetchReddit() out = %q; want it to contain \"HTTP 404\"", out)
		}
	})

	t.Run("non_2xx_status_falls_back_to_old_reddit_html", func(t *testing.T) {
		f := stubResponses(t, map[string]*http.Response{
			toRedditJsonUrl(url): newTestResponse(403, "text/html", "blocked"),
			toOldRedditURL(url):  htmlResponse(readableArticleHTML),
		}, func(context.Context, string) (string, bool) {
			t.Fatal("browser fallback should not be invoked; the old.reddit.com HTML fallback is not tiered that deep")
			return "", false
		})

		out, handled := fetchReddit(context.Background(), f, url)
		if !handled {
			t.Fatalf("fetchReddit() handled = false; want true")
		}
		if !strings.Contains(out, "genuinely readable article content") {
			t.Errorf("fetchReddit() out = %q; want the old.reddit.com fallback's article body", out)
		}
	})

	t.Run("non_json_content_type_falls_through", func(t *testing.T) {
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			return newTestResponse(200, "text/html", "<html></html>"), nil
		}}
		out, handled := fetchReddit(context.Background(), f, url)
		if handled {
			t.Fatalf("fetchReddit() handled = true; want false")
		}
		if out != "" {
			t.Errorf("fetchReddit() out = %q; want \"\"", out)
		}
	})

	t.Run("json_content_type_unparseable_body_falls_through", func(t *testing.T) {
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			return newTestResponse(200, "application/json", "not json"), nil
		}}
		out, handled := fetchReddit(context.Background(), f, url)
		if handled {
			t.Fatalf("fetchReddit() handled = true; want false")
		}
		if out != "" {
			t.Errorf("fetchReddit() out = %q; want \"\"", out)
		}
	})

	t.Run("parseable_but_empty_payload", func(t *testing.T) {
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			return newTestResponse(200, "application/json", redditPostFixtureEmptyChildren), nil
		}}
		out, handled := fetchReddit(context.Background(), f, url)
		if !handled {
			t.Fatalf("fetchReddit() handled = false; want true")
		}
		if !strings.Contains(out, "Could not parse Reddit") {
			t.Errorf("fetchReddit() out = %q; want it to contain \"Could not parse Reddit\"", out)
		}
	})

	t.Run("malformed_url_reports_error_without_calling_do", func(t *testing.T) {
		calledDo := false
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			calledDo = true
			return nil, nil
		}}
		// A control character in the URL makes http.NewRequestWithContext's
		// internal url.Parse fail before any network call would be attempted.
		out, handled := fetchReddit(context.Background(), f, "https://reddit.com/r/\x7f")
		if !handled {
			t.Fatalf("fetchReddit() handled = false; want true")
		}
		if calledDo {
			t.Errorf("fetchReddit() called f.do for a malformed URL; want it skipped")
		}
		if !strings.HasPrefix(out, "# Error fetching ") {
			t.Errorf("fetchReddit() out = %q; want an error message prefix", out)
		}
	})
}
