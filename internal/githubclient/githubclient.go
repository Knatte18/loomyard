// githubclient.go constructs the authenticated *github.Client this package hands to every consumer:
// New and NewWithBaseURL, plus the request timeout they share.
// See doc.go for the package's full design record:
// resolution chain, cache, authenticating transport, and the GitHub surface consumers need.

package githubclient

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-github/v75/github"
)

// clientTimeout bounds the whole HTTP round trip, the original attempt and the 401 replay together.
// authRT applies it after the pause hold ends, so a hold does not use it up.
const clientTimeout = 30 * time.Second

// New returns an authenticated *github.Client against the real GitHub API.
// Credentials are resolved lazily and non-blockingly by authRT.
func New() (*github.Client, error) {
	return NewWithBaseURL("", nil)
}

// NewWithBaseURL returns an authenticated *github.Client that sends requests to baseURL.
// An empty baseURL keeps the real GitHub API.
// Its inner transport is httpClient's Transport, or the default when httpClient is nil.
// Tests use this to target httptest servers.
func NewWithBaseURL(baseURL string, httpClient *http.Client) (*github.Client, error) {
	var inner http.RoundTripper
	if httpClient != nil {
		inner = httpClient.Transport
	}

	client := github.NewClient(&http.Client{
		Transport: &authRT{transport: inner, timeout: clientTimeout},
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
