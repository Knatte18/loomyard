// selfreport_test.go holds selfreportengine's tests for CreateIssue, driving it through the NewGitHubClient seam.
// Each case injects a real go-github client, or a factory closure that itself fails.
// The clients point at an httptest server, or at a dead address for the network-failure case.
// So no test reaches real token resolution, a real git spawn, a real process, or a fixture tree.
// The CreateIssue tests do not run in parallel: each swaps the package-level NewGitHubClient seam.

package selfreportengine

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// requestCapture records one HTTP request sent by CreateIssue: its method, its path, and its decoded JSON body.
// Tests assert on the title, body and label encoding through it.
type requestCapture struct {
	method string
	path   string
	body   map[string]any
}

// newIssueServer returns an httptest server that answers every request with status and respBody.
// It appends a requestCapture for each request to captured.
func newIssueServer(t *testing.T, status int, respBody string, captured *[]requestCapture) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		_ = json.Unmarshal(raw, &decoded)
		*captured = append(*captured, requestCapture{method: r.Method, path: r.URL.Path, body: decoded})
		w.WriteHeader(status)
		if respBody != "" {
			_, _ = w.Write([]byte(respBody))
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// installGitHubClient replaces the package-level NewGitHubClient seam with a closure.
// The closure returns a go-github client pointed at baseURL.
// It sets a fixed placeholder token directly on the client, not through githubclient's resolution chain.
// That keeps the suite off real token resolution, the `gh auth token` shell-out, and the operator's credential cache.
func installGitHubClient(t *testing.T, baseURL string) {
	t.Helper()
	client := github.NewClient(nil).WithAuthToken("test-token")
	parsed, err := url.Parse(baseURL + "/")
	if err != nil {
		t.Fatalf("url.Parse(%s): %v", baseURL, err)
	}
	client.BaseURL = parsed

	orig := NewGitHubClient
	NewGitHubClient = func() (*github.Client, error) { return client, nil }
	t.Cleanup(func() { NewGitHubClient = orig })
}

// installGitHubClientPointedAtDeadAddress installs a client pointed at an address nothing listens on.
// The request fails with a deterministic connection refusal.
// That failure is distinct from an API rejection and from a token-resolution failure.
func installGitHubClientPointedAtDeadAddress(t *testing.T) {
	t.Helper()
	// Start and immediately close a server: its address is guaranteed to have
	// nothing listening on it afterward.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := server.URL
	server.Close()
	installGitHubClient(t, deadURL)
}

// installFailingGitHubClientFactory replaces NewGitHubClient with a closure that returns err and no client.
// It exercises the token-not-resolvable path.
// CreateIssue must surface a typed error there, not dereference a nil client.
func installFailingGitHubClientFactory(t *testing.T, err error) {
	t.Helper()
	orig := NewGitHubClient
	NewGitHubClient = func() (*github.Client, error) { return nil, err }
	t.Cleanup(func() { NewGitHubClient = orig })
}

// TestDefaultLabels checks that DefaultLabels returns exactly the single-element "bug" default.
// The automatic and manual filing paths share that default.
// It also checks that each call returns its own slice.
// Mutating one result must not corrupt the default that the next caller sees.
func TestDefaultLabels(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	first := DefaultLabels()
	second := DefaultLabels()

	if len(first) != 1 || first[0] != "bug" {
		t.Errorf("DefaultLabels() = %v; want [\"bug\"]", first)
	}

	first[0] = "mutated"

	if second[0] != "bug" {
		t.Errorf("second DefaultLabels() call = %v after mutating the first's result; want [\"bug\"] unaffected", second)
	}
}

// TestCreateIssue_Success covers the successful path with and without a body.
// It checks that the returned url and number match the server's typed response.
// It also checks that the request carries the expected method, path, title, and labels in order.
// It checks the "body" field only when a body was given.
func TestCreateIssue_Success(t *testing.T) {
	tiergate.Require(t, 0)
	const issueURL = "https://github.com/Knatte18/loomyard/issues/42"
	tests := []struct {
		name string
		// body is passed to CreateIssue; nil must leave the request without a "body" field.
		body *string
	}{
		{"with body", new("details")},
		{"nil body omits the field", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured []requestCapture
			server := newIssueServer(t, http.StatusCreated, `{"html_url":"`+issueURL+`","number":42}`, &captured)
			installGitHubClient(t, server.URL)

			url, number, err := CreateIssue("a title", tt.body, []string{"bug", "p1"})
			if err != nil {
				t.Fatalf("CreateIssue() error = %v; want nil", err)
			}
			if url != issueURL {
				t.Errorf("CreateIssue() url = %q; want %q", url, issueURL)
			}
			if number != 42 {
				t.Errorf("CreateIssue() number = %d; want 42", number)
			}

			if len(captured) != 1 {
				t.Fatalf("request count = %d; want 1", len(captured))
			}
			got := captured[0]
			if got.method != http.MethodPost {
				t.Errorf("request method = %q; want %q", got.method, http.MethodPost)
			}
			if got.path != "/repos/Knatte18/loomyard/issues" {
				t.Errorf("request path = %q; want %q", got.path, "/repos/Knatte18/loomyard/issues")
			}
			if title, _ := got.body["title"].(string); title != "a title" {
				t.Errorf("request body title = %q; want %q", title, "a title")
			}
			labelsRaw, _ := got.body["labels"].([]any)
			if len(labelsRaw) != 2 || labelsRaw[0] != "bug" || labelsRaw[1] != "p1" {
				t.Errorf("request body labels = %v; want [\"bug\" \"p1\"] in order", labelsRaw)
			}
			gotBody, hasBody := got.body["body"]
			switch {
			case tt.body == nil && hasBody:
				t.Errorf("request has \"body\" field %v but none was provided", gotBody)
			case tt.body != nil && gotBody != *tt.body:
				t.Errorf("request body \"body\" field = %q; want %q", gotBody, *tt.body)
			}
		})
	}
}

// TestCreateIssue_TokenNotResolvable covers the path where the NewGitHubClient factory itself fails.
// CreateIssue must surface a typed error there rather than proceed with a nil client.
func TestCreateIssue_TokenNotResolvable(t *testing.T) {
	tiergate.Require(t, 0)
	installFailingGitHubClientFactory(t, githubclient.ErrTokenUnresolvable)
	buf := logcapture.Capture(t)

	url, number, err := CreateIssue("t", nil, []string{"bug"})

	if !errors.Is(err, githubclient.ErrTokenUnresolvable) {
		t.Fatalf("CreateIssue() error = %v; want errors.Is(err, githubclient.ErrTokenUnresolvable)", err)
	}
	if url != "" || number != 0 {
		t.Errorf("CreateIssue() = (%q, %d); want (\"\", 0) alongside the error", url, number)
	}

	logged := buf.String()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log output = %q; want a WARN line", logged)
	}
	if !strings.Contains(logged, "action=") {
		t.Errorf("log output = %q; want an action field", logged)
	}
	if !strings.Contains(logged, "cause=") {
		t.Errorf("log output = %q; want a cause field", logged)
	}
}

// TestCreateIssue_NetworkFailure covers a transport failure, where the server address refuses the connection.
// It asserts the error is neither a token-resolution failure nor an API rejection;
// a network problem calls for different operator action than either.
func TestCreateIssue_NetworkFailure(t *testing.T) {
	tiergate.Require(t, 0)
	installGitHubClientPointedAtDeadAddress(t)

	url, number, err := CreateIssue("t", nil, []string{"bug"})

	if err == nil {
		t.Fatal("CreateIssue() error = nil; want a network-failure error")
	}
	if errors.Is(err, githubclient.ErrTokenUnresolvable) {
		t.Errorf("CreateIssue() error = %v; want it distinct from ErrTokenUnresolvable", err)
	}
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) {
		t.Errorf("CreateIssue() error = %v; want it distinct from *github.ErrorResponse (a network failure never reaches an HTTP response)", err)
	}
	if url != "" || number != 0 {
		t.Errorf("CreateIssue() = (%q, %d); want (\"\", 0) alongside the error", url, number)
	}
}

// TestCreateIssue_NonSuccessResponse covers a non-2xx GitHub response.
// It asserts the error surfaces the response's message and is distinct from both other error cases.
func TestCreateIssue_NonSuccessResponse(t *testing.T) {
	tiergate.Require(t, 0)
	const errMessage = "Validation Failed"
	var captured []requestCapture
	server := newIssueServer(t, http.StatusUnprocessableEntity, `{"message":"`+errMessage+`"}`, &captured)
	installGitHubClient(t, server.URL)
	buf := logcapture.Capture(t)

	url, number, err := CreateIssue("t", nil, []string{"bug"})

	if err == nil {
		t.Fatal("CreateIssue() error = nil; want a non-2xx response error")
	}
	if errors.Is(err, githubclient.ErrTokenUnresolvable) {
		t.Errorf("CreateIssue() error = %v; want it distinct from ErrTokenUnresolvable", err)
	}
	if !strings.Contains(err.Error(), errMessage) {
		t.Errorf("CreateIssue() error = %q; want it to contain response message %q", err.Error(), errMessage)
	}
	if url != "" || number != 0 {
		t.Errorf("CreateIssue() = (%q, %d); want (\"\", 0) alongside the error", url, number)
	}

	logged := buf.String()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log output = %q; want a WARN line", logged)
	}
	for _, field := range []string{"action=", "owner=", "repo=", "cause="} {
		if !strings.Contains(logged, field) {
			t.Errorf("log output = %q; want a %s field", logged, field)
		}
	}
}
