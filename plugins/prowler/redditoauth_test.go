// redditoauth_test.go exercises Reddit OAuth credential resolution, API User-Agent selection, token
// request shape, token error handling, and the token cache's caching/concurrency behaviour via a
// stubbed fetcher.do. No network call.

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

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
