// redditoauth_test.go exercises Reddit OAuth credential resolution, API User-Agent selection, token
// request shape, token error handling, and the token cache's caching/concurrency behaviour via a
// stubbed fetcher.do. No network call.

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// updateRedditGolden regenerates testdata/reddit-thread-golden.md from the
// current formatter's output instead of comparing against it. It is never
// set in CI; a human sets it deliberately, via
// `go -C plugins/prowler test -run TestFormatRedditThread_GoldenOAuthOutput . -args -update-golden`,
// after confirming a formatter change is intended.
var updateRedditGolden = flag.Bool("update-golden", false, "regenerate testdata/reddit-thread-golden.md from the current formatter's output")

// tokenResponse builds a canned *http.Response for the token endpoint,
// with the given status and body.
func tokenResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// stringSlicesEqual reports whether a and b contain the same elements in
// the same order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRedditCredentials(t *testing.T) {
	tests := []struct {
		name         string
		clientID     string
		clientSecret string
		wantMissing  []string
	}{
		{"neither_set", "", "", []string{redditClientIDEnv, redditClientSecretEnv}},
		{"only_id_set", "an-id", "", []string{redditClientSecretEnv}},
		{"only_secret_set", "", "a-secret", []string{redditClientIDEnv}},
		{"both_set", "an-id", "a-secret", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(redditClientIDEnv, tt.clientID)
			t.Setenv(redditClientSecretEnv, tt.clientSecret)
			redditTokens.reset()
			t.Cleanup(redditTokens.reset)

			gotID, gotSecret, gotMissing := redditCredentials()
			if gotID != tt.clientID {
				t.Errorf("redditCredentials() clientID = %q; want %q", gotID, tt.clientID)
			}
			if gotSecret != tt.clientSecret {
				t.Errorf("redditCredentials() clientSecret = %q; want %q", gotSecret, tt.clientSecret)
			}
			if !stringSlicesEqual(gotMissing, tt.wantMissing) {
				t.Errorf("redditCredentials() missing = %v; want %v", gotMissing, tt.wantMissing)
			}
		})
	}
}

func TestRedditAPIUserAgent(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv(redditUserAgentEnv, "")
		got := redditAPIUserAgent()
		if got != defaultRedditAPIUserAgent {
			t.Errorf("redditAPIUserAgent() = %q; want %q", got, defaultRedditAPIUserAgent)
		}
		if got == browserUA {
			t.Errorf("redditAPIUserAgent() = %q; must never equal browserUA", got)
		}
	})

	t.Run("env_override", func(t *testing.T) {
		t.Setenv(redditUserAgentEnv, "my-custom-agent/2.0")
		got := redditAPIUserAgent()
		if got != "my-custom-agent/2.0" {
			t.Errorf("redditAPIUserAgent() = %q; want %q", got, "my-custom-agent/2.0")
		}
		if got == browserUA {
			t.Errorf("redditAPIUserAgent() = %q; must never equal browserUA", got)
		}
	})
}

func TestRedditTokenRequestShape(t *testing.T) {
	const clientID = "test-client-id"
	const clientSecret = "test-client-secret"

	var gotReq *http.Request
	var gotBody string
	f := fetcher{do: func(req *http.Request) (*http.Response, error) {
		gotReq = req
		b, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		gotBody = string(b)
		return tokenResponse(200, `{"access_token":"abc123","expires_in":3600}`), nil
	}}

	_, _, err := requestRedditToken(context.Background(), f, clientID, clientSecret)
	if err != nil {
		t.Fatalf("requestRedditToken() error = %v; want nil", err)
	}

	if gotReq.Method != http.MethodPost {
		t.Errorf("requestRedditToken() method = %q; want %q", gotReq.Method, http.MethodPost)
	}
	if gotReq.URL.String() != redditTokenURL {
		t.Errorf("requestRedditToken() URL = %q; want %q", gotReq.URL.String(), redditTokenURL)
	}
	if got := gotReq.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Errorf("requestRedditToken() Content-Type = %q; want %q", got, "application/x-www-form-urlencoded")
	}
	if got := gotReq.Header.Get("User-Agent"); got != defaultRedditAPIUserAgent {
		t.Errorf("requestRedditToken() User-Agent = %q; want %q", got, defaultRedditAPIUserAgent)
	}
	if gotBody != "grant_type=client_credentials" {
		t.Errorf("requestRedditToken() body = %q; want %q", gotBody, "grant_type=client_credentials")
	}

	auth := gotReq.Header.Get("Authorization")
	const prefix = "Basic "
	if !strings.HasPrefix(auth, prefix) {
		t.Fatalf("requestRedditToken() Authorization = %q; want prefix %q", auth, prefix)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, prefix))
	if err != nil {
		t.Fatalf("decode Authorization payload: %v", err)
	}
	if want := clientID + ":" + clientSecret; string(decoded) != want {
		t.Errorf("requestRedditToken() decoded Authorization = %q; want %q", decoded, want)
	}
}

func TestRedditTokenErrors(t *testing.T) {
	const clientID = "fake-client-id"
	const clientSecret = "fake-client-secret"

	assertErrorHidesCreds := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("requestRedditToken() error = nil; want non-nil")
		}
		msg := err.Error()
		if strings.Contains(msg, clientID) {
			t.Errorf("requestRedditToken() error = %q; must not contain the client id", msg)
		}
		if strings.Contains(msg, clientSecret) {
			t.Errorf("requestRedditToken() error = %q; must not contain the client secret", msg)
		}
	}

	t.Run("status_401", func(t *testing.T) {
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			return tokenResponse(401, `{"error":"invalid_grant"}`), nil
		}}
		_, _, err := requestRedditToken(context.Background(), f, clientID, clientSecret)
		assertErrorHidesCreds(t, err)
	})

	t.Run("malformed_json", func(t *testing.T) {
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			return tokenResponse(200, `not json`), nil
		}}
		_, _, err := requestRedditToken(context.Background(), f, clientID, clientSecret)
		assertErrorHidesCreds(t, err)
	})

	t.Run("empty_access_token", func(t *testing.T) {
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			return tokenResponse(200, `{"access_token":"","expires_in":3600}`), nil
		}}
		_, _, err := requestRedditToken(context.Background(), f, clientID, clientSecret)
		assertErrorHidesCreds(t, err)
	})
}

func TestRedditTokenCaching(t *testing.T) {
	t.Run("sequential_reuse_and_expiry", func(t *testing.T) {
		redditTokens.reset()
		t.Cleanup(redditTokens.reset)
		realTimeNow := timeNow
		t.Cleanup(func() { timeNow = realTimeNow })

		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		timeNow = func() time.Time { return now }

		var requestCount int
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			requestCount++
			return tokenResponse(200, `{"access_token":"tok-1","expires_in":3600}`), nil
		}}

		tok1, err := redditTokens.get(context.Background(), f, "id", "secret")
		if err != nil {
			t.Fatalf("get() error = %v; want nil", err)
		}
		tok2, err := redditTokens.get(context.Background(), f, "id", "secret")
		if err != nil {
			t.Fatalf("get() error = %v; want nil", err)
		}
		if tok1 != "tok-1" || tok2 != "tok-1" {
			t.Errorf("get() tokens = %q, %q; want both %q", tok1, tok2, "tok-1")
		}
		if requestCount != 1 {
			t.Errorf("token request count = %d; want 1 after two sequential get() calls", requestCount)
		}

		// Advance past expiresAt (now + 3600s - 60s margin) to force a
		// second request.
		now = now.Add(1 * time.Hour)
		tok3, err := redditTokens.get(context.Background(), f, "id", "secret")
		if err != nil {
			t.Fatalf("get() error = %v; want nil", err)
		}
		if tok3 != "tok-1" {
			t.Errorf("get() token = %q; want %q", tok3, "tok-1")
		}
		if requestCount != 2 {
			t.Errorf("token request count = %d; want 2 after timeNow advanced past expiry", requestCount)
		}
	})

	t.Run("concurrent_get_issues_one_request", func(t *testing.T) {
		redditTokens.reset()
		t.Cleanup(redditTokens.reset)
		realTimeNow := timeNow
		t.Cleanup(func() { timeNow = realTimeNow })
		timeNow = time.Now

		var mu sync.Mutex
		var requestCount int
		f := fetcher{do: func(*http.Request) (*http.Response, error) {
			mu.Lock()
			requestCount++
			mu.Unlock()
			return tokenResponse(200, `{"access_token":"tok-concurrent","expires_in":3600}`), nil
		}}

		const goroutines = 20
		var wg sync.WaitGroup
		errs := make(chan error, goroutines)
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := redditTokens.get(context.Background(), f, "id", "secret"); err != nil {
					errs <- fmt.Errorf("goroutine get() error: %w", err)
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}

		mu.Lock()
		got := requestCount
		mu.Unlock()
		if got != 1 {
			t.Errorf("token request count = %d; want exactly 1 across %d concurrent get() calls", got, goroutines)
		}
	})
}

// testdata/reddit-thread.json is hand-authored rather than captured from a
// live response, because oauth.reddit.com cannot be read without
// credentials that do not exist yet at this point in the plan; batch 4's
// live integration test is what validates the real response shape. It is
// structurally faithful to Reddit's documented comments-endpoint Listing
// shape: a two-element array of [post listing, comments listing].
func TestFormatRedditThread(t *testing.T) {
	data := readTestdataFile(t, "reddit-thread.json")

	var listings []redditListing
	if err := json.Unmarshal([]byte(data), &listings); err != nil {
		t.Fatalf("json.Unmarshal(reddit-thread.json) error = %v", err)
	}

	out, err := formatRedditThread(listings, "https://oauth.reddit.com/r/golang/comments/abc123")
	if err != nil {
		t.Fatalf("formatRedditThread() error = %v; want nil", err)
	}

	if !strings.Contains(out, "What is the idiomatic way to handle errors in Go?") {
		t.Errorf("formatRedditThread() out missing title:\n%s", out)
	}
	if !strings.Contains(out, "I've been writing Go for a few months") {
		t.Errorf("formatRedditThread() out missing selftext:\n%s", out)
	}

	if topLevelCount := strings.Count(out, "** ("); topLevelCount != maxTopComments {
		t.Errorf("formatRedditThread() top-level comment count = %d; want %d", topLevelCount, maxTopComments)
	}
	// c20 and c21 fall beyond the cap; the "more" child sits before them in
	// the fixture, so if it were mistakenly counted toward the cap it would
	// have displaced a real comment earlier than c20 -- these two absences
	// together prove the more child contributed nothing and the cap landed
	// in the right place.
	if strings.Contains(out, "top_author_20") || strings.Contains(out, "top_author_21") {
		t.Errorf("formatRedditThread() out contains a comment beyond maxTopComments:\n%s", out)
	}

	if replyCount := strings.Count(out, "> **u/capreply"); replyCount != maxTopComments {
		t.Errorf("formatRedditThread() reply count for the over-capped comment = %d; want %d", replyCount, maxTopComments)
	}

	if strings.Contains(out, "A reply to a reply") {
		t.Errorf("formatRedditThread() out contains a reply's own nested reply, which must not render:\n%s", out)
	}

	t.Run("zero_comment_thread", func(t *testing.T) {
		const fixture = `[{"kind":"Listing","data":{"children":[{"kind":"t3","data":{"title":"A quiet post","selftext":"Nobody has replied yet.","author":"solo","subreddit":"golang","score":1,"url":"","replies":""}}]}}]`
		var listings []redditListing
		if err := json.Unmarshal([]byte(fixture), &listings); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		out, err := formatRedditThread(listings, "https://oauth.reddit.com/r/golang/comments/quiet")
		if err != nil {
			t.Fatalf("formatRedditThread() error = %v; want nil", err)
		}
		if out == "" {
			t.Error("formatRedditThread() out is empty; want a non-empty result")
		}
		if strings.Contains(out, "## Top Comments") {
			t.Errorf("formatRedditThread() out contains a Top Comments heading with zero comments:\n%s", out)
		}
	})
}

// TestFormatRedditThread_GoldenOAuthOutput compares formatRedditThread's
// current output for testdata/reddit-thread.json against the byte-for-byte
// golden file committed at testdata/reddit-thread-golden.md. This is the
// regression that proves batch 2's refactor onto the tier-neutral
// representation changes nothing observable on the OAuth path: the golden
// file is captured before that refactor and must still match after it.
func TestFormatRedditThread_GoldenOAuthOutput(t *testing.T) {
	data := readTestdataFile(t, "reddit-thread.json")

	var listings []redditListing
	if err := json.Unmarshal([]byte(data), &listings); err != nil {
		t.Fatalf("json.Unmarshal(reddit-thread.json) error = %v", err)
	}

	const sourceURL = "https://www.reddit.com/r/golang/comments/abc123/idiomatic_errors/"
	const goldenPath = "testdata/reddit-thread-golden.md"

	out, err := formatRedditThread(listings, sourceURL)
	if err != nil {
		t.Fatalf("formatRedditThread() error = %v; want nil", err)
	}

	if *updateRedditGolden {
		if err := os.WriteFile(goldenPath, []byte(out), 0o644); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", goldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", goldenPath, err)
	}

	if out != string(want) {
		t.Errorf("formatRedditThread() output does not match golden file %s; want the two byte-for-byte identical.\n"+
			"If this difference is intended, re-run with -args -update-golden after confirming it.\ngot:\n%s\nwant:\n%s",
			goldenPath, out, want)
	}
}

func TestRedditOAuthURL(t *testing.T) {
	const wantPath = "/r/golang/comments/abc123/some_title/"
	const wantQuery = "raw_json=1&limit=100&depth=2"

	tests := []struct {
		name   string
		rawURL string
	}{
		{"bare_host", "https://reddit.com" + wantPath},
		{"www_host", "https://www.reddit.com" + wantPath},
		{"old_host", "https://old.reddit.com" + wantPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := redditOAuthURL(tt.rawURL)
			if err != nil {
				t.Fatalf("redditOAuthURL(%q) error = %v; want nil", tt.rawURL, err)
			}
			u, parseErr := url.Parse(got)
			if parseErr != nil {
				t.Fatalf("url.Parse(%q) error = %v", got, parseErr)
			}
			if u.Host != redditOAuthHost {
				t.Errorf("redditOAuthURL(%q) host = %q; want %q", tt.rawURL, u.Host, redditOAuthHost)
			}
			if u.Path != wantPath {
				t.Errorf("redditOAuthURL(%q) path = %q; want %q", tt.rawURL, u.Path, wantPath)
			}
			if u.RawQuery != wantQuery {
				t.Errorf("redditOAuthURL(%q) query = %q; want %q", tt.rawURL, u.RawQuery, wantQuery)
			}
		})
	}

	t.Run("incoming_query_discarded", func(t *testing.T) {
		got, err := redditOAuthURL("https://www.reddit.com" + wantPath + "?sort=top&foo=bar")
		if err != nil {
			t.Fatalf("redditOAuthURL() error = %v; want nil", err)
		}
		if strings.Contains(got, "sort=top") || strings.Contains(got, "foo=bar") {
			t.Errorf("redditOAuthURL() = %q; want the incoming query discarded", got)
		}
	})

	t.Run("unparseable_input_yields_error", func(t *testing.T) {
		if _, err := redditOAuthURL("http://[::1"); err == nil {
			t.Fatal("redditOAuthURL() error = nil; want non-nil for an unparseable URL")
		}
	})

	t.Run("empty_path_yields_error", func(t *testing.T) {
		if _, err := redditOAuthURL("https://reddit.com"); err == nil {
			t.Fatal("redditOAuthURL() error = nil; want non-nil for a URL with no path")
		}
	})
}

func TestFetchRedditOAuthThread(t *testing.T) {
	const rawURL = "https://www.reddit.com/r/golang/comments/abc123/some_title/"

	t.Run("happy_path", func(t *testing.T) {
		t.Setenv(redditClientIDEnv, "id")
		t.Setenv(redditClientSecretEnv, "secret")
		redditTokens.reset()
		t.Cleanup(redditTokens.reset)

		threadJSON := readTestdataFile(t, "reddit-thread.json")
		wantThreadURL, err := redditOAuthURL(rawURL)
		if err != nil {
			t.Fatalf("redditOAuthURL() error = %v", err)
		}

		var threadReq *http.Request
		f := fetcher{do: func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case redditTokenURL:
				return tokenResponse(200, `{"access_token":"tok-happy","expires_in":3600}`), nil
			case wantThreadURL:
				threadReq = req
				return htmlResponse(threadJSON), nil
			default:
				t.Fatalf("unexpected request URL: %s", req.URL.String())
				return nil, nil
			}
		}}

		out, err := fetchRedditOAuthThread(context.Background(), f, rawURL)
		if err != nil {
			t.Fatalf("fetchRedditOAuthThread() error = %v; want nil", err)
		}
		if threadReq == nil {
			t.Fatal("thread request was never issued")
		}
		if got := threadReq.Header.Get("Authorization"); got != "bearer tok-happy" {
			t.Errorf("thread request Authorization = %q; want %q", got, "bearer tok-happy")
		}
		if got := threadReq.Header.Get("User-Agent"); got == browserUA {
			t.Errorf("thread request User-Agent = %q; must not be browserUA", got)
		}
		if !strings.Contains(out, "What is the idiomatic way to handle errors in Go?") {
			t.Errorf("fetchRedditOAuthThread() out missing post title:\n%s", out)
		}
	})

	t.Run("missing_credentials", func(t *testing.T) {
		t.Setenv(redditClientIDEnv, "")
		t.Setenv(redditClientSecretEnv, "")
		redditTokens.reset()
		t.Cleanup(redditTokens.reset)

		f := fetcher{do: func(req *http.Request) (*http.Response, error) {
			t.Fatal("transport must not be invoked when credentials are missing")
			return nil, nil
		}}

		_, err := fetchRedditOAuthThread(context.Background(), f, rawURL)
		if err == nil {
			t.Fatal("fetchRedditOAuthThread() error = nil; want non-nil")
		}
		if !strings.Contains(err.Error(), redditClientIDEnv) || !strings.Contains(err.Error(), redditClientSecretEnv) {
			t.Errorf("fetchRedditOAuthThread() error = %q; want it to name both %s and %s", err, redditClientIDEnv, redditClientSecretEnv)
		}
	})

	t.Run("403_block_page", func(t *testing.T) {
		t.Setenv(redditClientIDEnv, "id")
		t.Setenv(redditClientSecretEnv, "secret")
		redditTokens.reset()
		t.Cleanup(redditTokens.reset)

		blockHTML := readTestdataFile(t, "reddit-block-page.html")
		wantReason, blocked := looksLikeBlockPage(blockHTML)
		if !blocked {
			t.Fatal("fixture reddit-block-page.html does not trip looksLikeBlockPage")
		}

		f := fetcher{do: func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == redditTokenURL {
				return tokenResponse(200, `{"access_token":"tok-403","expires_in":3600}`), nil
			}
			return &http.Response{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(blockHTML))}, nil
		}}

		_, err := fetchRedditOAuthThread(context.Background(), f, rawURL)
		if err == nil {
			t.Fatal("fetchRedditOAuthThread() error = nil; want non-nil")
		}
		if !strings.Contains(err.Error(), wantReason) {
			t.Errorf("fetchRedditOAuthThread() error = %q; want it to mention %q", err, wantReason)
		}
	})

	t.Run("200_with_block_page_body", func(t *testing.T) {
		t.Setenv(redditClientIDEnv, "id")
		t.Setenv(redditClientSecretEnv, "secret")
		redditTokens.reset()
		t.Cleanup(redditTokens.reset)

		blockHTML := readTestdataFile(t, "reddit-block-page.html")
		wantReason, blocked := looksLikeBlockPage(blockHTML)
		if !blocked {
			t.Fatal("fixture reddit-block-page.html does not trip looksLikeBlockPage")
		}

		f := fetcher{do: func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == redditTokenURL {
				return tokenResponse(200, `{"access_token":"tok-200wall","expires_in":3600}`), nil
			}
			return htmlResponse(blockHTML), nil
		}}

		_, err := fetchRedditOAuthThread(context.Background(), f, rawURL)
		if err == nil {
			t.Fatal("fetchRedditOAuthThread() error = nil; want non-nil")
		}
		if !strings.Contains(err.Error(), wantReason) {
			t.Errorf("fetchRedditOAuthThread() error = %q; want it to mention the wall reason %q rather than a JSON syntax error", err, wantReason)
		}
		if strings.Contains(strings.ToLower(err.Error()), "invalid character") {
			t.Errorf("fetchRedditOAuthThread() error = %q; want a wall reason, not a raw JSON decode error", err)
		}
	})
}
