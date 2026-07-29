// githubclient.go constructs the authenticated *github.Client this package
// hands to every consumer: New and NewWithBaseURL, plus the request timeout
// they share. See doc.go for the package's full design record -- resolution
// chain, cache, authenticating transport, and the GitHub surface consumers
// need.

package githubclient

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-github/v75/github"
)

// clientTimeout bounds the whole HTTP round trip performed by the returned
// client's underlying http.Client. It is enforced through the request
// context, so it covers the original attempt and a 401 replay TOGETHER --
// worst case 30s total, not 30s per attempt. It is not decoration:
// http.DefaultTransport sets no response timeout of its own, so without this
// a stalled connection would hang an autonomous lyx run indefinitely.
const clientTimeout = 30 * time.Second

// New returns a *github.Client authenticated against the real GitHub API at
// its default base URL. Credentials are resolved lazily and non-blockingly
// on each request via authRT (transport.go); construction itself never
// fails -- a missing or unusable token surfaces later, as a typed error from
// the first request that needs one.
func New() (*github.Client, error) {
	return NewWithBaseURL("", nil)
}

// NewWithBaseURL returns a *github.Client authenticated the same way as New,
// but pointed at baseURL (when non-empty) and delegating to httpClient's
// Transport (when non-nil) instead of http.DefaultTransport. This is the
// seam a test uses to point a real go-github client at an httptest server,
// without the client ever reaching real token resolution or the real API.
//
// Exactly one layer owns the Authorization header: authRT, installed here as
// the client's outermost transport. github.WithAuthToken is deliberately not
// used -- it captures a fixed token inside go-github's own transport
// wrapper, so combined with authRT the header would have two owners, and a
// 401 replay would re-send the stale token from that wrapper's closure,
// defeating the entire re-resolution authRT exists to provide.
func NewWithBaseURL(baseURL string, httpClient *http.Client) (*github.Client, error) {
	var inner http.RoundTripper
	if httpClient != nil {
		inner = httpClient.Transport
	}

	client := github.NewClient(&http.Client{
		Timeout:   clientTimeout,
		Transport: &authRT{transport: inner},
	})

	if baseURL == "" {
		return client, nil
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("githubclient: parse base URL %q: %w", baseURL, err)
	}
	// go-github's NewRequest requires a trailing slash on BaseURL.Path so it
	// can safely resolve relative API paths against it.
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	client.BaseURL = u

	return client, nil
}
