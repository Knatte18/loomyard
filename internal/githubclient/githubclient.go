// githubclient.go constructs the authenticated *github.Client this package hands to every consumer:
// New and NewWithBaseURL, plus the request timeout they share.
// See doc.go for the package's full design record -- resolution chain, cache, authenticating
// transport, and the GitHub surface consumers need.

package githubclient

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-github/v75/github"
)

// clientTimeout bounds the whole HTTP round trip via request context, covering
// both original attempt and 401 replay.
const clientTimeout = 30 * time.Second

// New returns an authenticated *github.Client against the real GitHub API, with credentials
// resolved lazily and non-blockingly via authRT.
func New() (*github.Client, error) {
	return NewWithBaseURL("", nil)
}

// NewWithBaseURL returns an authenticated *github.Client at baseURL with the given http.Client's
// Transport, useful for tests pointing at httptest servers.
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
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	client.BaseURL = u

	return client, nil
}
