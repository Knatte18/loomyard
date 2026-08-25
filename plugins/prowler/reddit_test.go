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

	// fatalBrowser fails the test the moment f.browser is invoked, proving
	// redditAdapter.Fetch never falls through to the generic
	// headless-browser cascade on any path exercised in this function.
	fatalBrowser := func(t *testing.T) func(context.Context, string) (string, bool) {
		return func(context.Context, string) (string, bool) {
			t.Fatal("redditAdapter.Fetch must never call f.browser")
			return "", false
		}
	}

	t.Run("credentials_absent_tier2_succeeds", func(t *testing.T) {
		t.Setenv(redditClientIDEnv, "")
		t.Setenv(redditClientSecretEnv, "")
		t.Cleanup(func() { redditTokens.reset() })

		f := stubResponses(t, map[string]*http.Response{
			oldURL: htmlResponse(redditLikeHTMLWithComments),
		}, fatalBrowser(t))

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

	t.Run("both_tiers_fail_reports_handled_true_naming_both_tiers", func(t *testing.T) {
		t.Setenv(redditClientIDEnv, "id")
		t.Setenv(redditClientSecretEnv, "secret")
		t.Cleanup(func() { redditTokens.reset() })
		redditTokens.reset()

		respond := func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == redditTokenURL {
				return tokenResponse(200, `{"access_token":"tok-both-fail","expires_in":3600}`), nil
			}
			// Both the OAuth thread request (oauth.reddit.com) and the
			// old.reddit.com request land on this same non-2xx branch.
			return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("boom"))}, nil
		}
		f := fetcher{do: respond, doNoRedirect: respond, browser: fatalBrowser(t)}

		out, handled := (redditAdapter{}).Fetch(context.Background(), f, url)
		if !handled {
			t.Fatalf("redditAdapter{}.Fetch() handled = false; want true even when every tier fails")
		}
		if !strings.HasPrefix(out, "# Error fetching ") {
			t.Errorf("redditAdapter{}.Fetch() out = %q; want it to start with \"# Error fetching \"", out)
		}
		if !strings.Contains(out, "Tier 1") {
			t.Errorf("redditAdapter{}.Fetch() out = %q; want it to name Tier 1", out)
		}
		if !strings.Contains(out, "Tier 2") {
			t.Errorf("redditAdapter{}.Fetch() out = %q; want it to name Tier 2", out)
		}
	})

	t.Run("credentials_absent_tier2_also_fails", func(t *testing.T) {
		t.Setenv(redditClientIDEnv, "")
		t.Setenv(redditClientSecretEnv, "")
		t.Cleanup(func() { redditTokens.reset() })

		var tokenRequestIssued bool
		respond := func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == redditTokenURL {
				tokenRequestIssued = true
			}
			return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("boom"))}, nil
		}
		f := fetcher{do: respond, doNoRedirect: respond, browser: fatalBrowser(t)}

		out, handled := (redditAdapter{}).Fetch(context.Background(), f, url)
		if !handled {
			t.Fatalf("redditAdapter{}.Fetch() handled = false; want true")
		}
		if tokenRequestIssued {
			t.Errorf("redditAdapter{}.Fetch() issued a token request despite missing credentials")
		}
		if !strings.Contains(out, redditClientIDEnv) || !strings.Contains(out, redditClientSecretEnv) {
			t.Errorf("redditAdapter{}.Fetch() out = %q; want it to name both %s and %s", out, redditClientIDEnv, redditClientSecretEnv)
		}
	})

	t.Run("credentials_present_oauth_tier_succeeds_tier2_never_requested", func(t *testing.T) {
		t.Setenv(redditClientIDEnv, "id")
		t.Setenv(redditClientSecretEnv, "secret")
		t.Cleanup(func() { redditTokens.reset() })
		redditTokens.reset()

		threadJSON := readTestdataFile(t, "reddit-thread.json")
		wantThreadURL, err := redditOAuthURL(url)
		if err != nil {
			t.Fatalf("redditOAuthURL() error = %v", err)
		}

		f := fetcher{
			do: func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case redditTokenURL:
					return tokenResponse(200, `{"access_token":"tok-oauth-succeeds","expires_in":3600}`), nil
				case wantThreadURL:
					return htmlResponse(threadJSON), nil
				default:
					t.Fatalf("unexpected request URL: %s", req.URL.String())
					return nil, nil
				}
			},
			doNoRedirect: func(*http.Request) (*http.Response, error) {
				t.Fatal("tier 2 (old.reddit.com) must not be requested when tier 1 succeeds")
				return nil, nil
			},
			browser: fatalBrowser(t),
		}

		out, handled := (redditAdapter{}).Fetch(context.Background(), f, url)
		if !handled {
			t.Fatalf("redditAdapter{}.Fetch() handled = false; want true")
		}
		if !strings.Contains(out, "What is the idiomatic way to handle errors in Go?") {
			t.Errorf("redditAdapter{}.Fetch() out = %q; want the OAuth thread's post title", out)
		}
	})

	t.Run("credentials_present_oauth_tier_fails_tier2_succeeds", func(t *testing.T) {
		t.Setenv(redditClientIDEnv, "id")
		t.Setenv(redditClientSecretEnv, "secret")
		t.Cleanup(func() { redditTokens.reset() })
		redditTokens.reset()

		f := fetcher{
			do: func(req *http.Request) (*http.Response, error) {
				if req.URL.String() == redditTokenURL {
					return tokenResponse(200, `{"access_token":"tok-oauth-fails","expires_in":3600}`), nil
				}
				return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("boom"))}, nil
			},
			doNoRedirect: func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != oldURL {
					t.Fatalf("unexpected doNoRedirect request URL: %s", req.URL.String())
				}
				return htmlResponse(redditLikeHTMLWithComments), nil
			},
			browser: fatalBrowser(t),
		}

		out, handled := (redditAdapter{}).Fetch(context.Background(), f, url)
		if !handled {
			t.Fatalf("redditAdapter{}.Fetch() handled = false; want true")
		}
		if !strings.Contains(out, "original self-post text") {
			t.Errorf("redditAdapter{}.Fetch() out = %q; want the tier-2 post text", out)
		}
		if !strings.Contains(out, "First commenter's opinion") {
			t.Errorf("redditAdapter{}.Fetch() out = %q; want the tier-2 comment text", out)
		}
	})
}
