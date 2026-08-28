// selfreport_test.go is selfreportengine's own test suite, covering CreateIssue directly through
// the NewGitHubClient seam.
// The package had no tests of its own before this batch -- every case previously lived one layer up
// in selfreportcli, driving the now-deleted RunGH seam.
// Each case here injects a real go-github client pointed at an httptest server (or a dead address,
// for the network-failure case) or a factory closure that itself fails, so no test ever reaches
// real token resolution, a real git spawn, a real process, or a fixture tree -- untagged Tier 1.

package selfreportengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/logger"
)

// captureLogOutput redirects logger output into a buffer for the duration of
// one test, restoring os.Stderr via t.Cleanup -- the test-log-capture-pattern
// shared decision's inline shape, modeled on
// internal/loomshed/gatefindings_test.go.
func captureLogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
	return &buf
}

// requestCapture describes one HTTP request CreateIssue actually sent: the
// method and path, plus the decoded JSON body, so a test can assert on
// title/body/label encoding directly.
type requestCapture struct {
	method string
	path   string
	body   map[string]any
}

// newIssueServer returns an httptest server that responds to every request
// with status and respBody, appending a requestCapture for each request it
// receives to captured.
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

// installGitHubClient replaces the package-level NewGitHubClient seam with a
// closure returning a real go-github client pointed at baseURL, authenticated
// with a fixed placeholder token set directly on the client rather than
// through githubclient's resolution chain. This is what keeps this suite from
// ever reaching real token resolution, a real `gh auth token` shell-out, or
// the operator's real credential cache.
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

// installGitHubClientPointedAtDeadAddress installs a client pointed at an
// address nothing listens on, producing a deterministic connection-refused
// network failure distinct from both an API rejection and a token-resolution
// failure.
func installGitHubClientPointedAtDeadAddress(t *testing.T) {
	t.Helper()
	// Start and immediately close a server: its address is guaranteed to have
	// nothing listening on it afterward.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := server.URL
	server.Close()
	installGitHubClient(t, deadURL)
}

// installFailingGitHubClientFactory replaces NewGitHubClient with a closure
// that itself returns err without ever handing back a client -- the new
// token-not-resolvable path, which CreateIssue must surface as a typed error
// rather than dereferencing a nil client.
func installFailingGitHubClientFactory(t *testing.T, err error) {
	t.Helper()
	orig := NewGitHubClient
	NewGitHubClient = func() (*github.Client, error) { return nil, err }
	t.Cleanup(func() { NewGitHubClient = orig })
}

// TestCreateIssue_Success drives the normal successful path: the returned url and number match the
// server's typed response,
// and the request sent carries the expected method, path, title, body, and labels in order.
func TestCreateIssue_Success(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/42"
	var captured []requestCapture
	server := newIssueServer(t, http.StatusCreated, `{"html_url":"`+issueURL+`","number":42}`, &captured)
	installGitHubClient(t, server.URL)

	body := "details"
	url, number, err := CreateIssue("a title", &body, []string{"bug", "p1"})
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
	if bodyVal, _ := got.body["body"].(string); bodyVal != "details" {
		t.Errorf("request body \"body\" field = %q; want %q", bodyVal, "details")
	}
	labelsRaw, _ := got.body["labels"].([]any)
	if len(labelsRaw) != 2 {
		t.Fatalf("request body labels = %v; want 2 entries", labelsRaw)
	}
	if labelsRaw[0] != "bug" || labelsRaw[1] != "p1" {
		t.Errorf("request body labels = %v; want [\"bug\" \"p1\"] in order", labelsRaw)
	}
}

// TestCreateIssue_NoBodyOmitsField verifies that a nil body pointer produces a request with no
// "body" field at all, rather than an empty string.
func TestCreateIssue_NoBodyOmitsField(t *testing.T) {
	var captured []requestCapture
	server := newIssueServer(t, http.StatusCreated, `{"html_url":"https://github.com/Knatte18/loomyard/issues/1","number":1}`, &captured)
	installGitHubClient(t, server.URL)

	if _, _, err := CreateIssue("t", nil, []string{"bug"}); err != nil {
		t.Fatalf("CreateIssue() error = %v; want nil", err)
	}

	if len(captured) != 1 {
		t.Fatalf("request count = %d; want 1", len(captured))
	}
	if _, hasBody := captured[0].body["body"]; hasBody {
		t.Errorf("request body has \"body\" field but none was provided; got %v", captured[0].body)
	}
}

// TestCreateIssue_TokenNotResolvable covers the new token-not-resolvable path: when the
// NewGitHubClient factory itself fails, CreateIssue must surface a typed error rather than
// proceeding with a nil client.
func TestCreateIssue_TokenNotResolvable(t *testing.T) {
	installFailingGitHubClientFactory(t, githubclient.ErrTokenUnresolvable)
	buf := captureLogOutput(t)

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

// TestCreateIssue_NetworkFailure covers a transport/network failure (the server address refuses the
// connection), asserting it is neither a token-resolution failure nor an API rejection -- a network
// problem calls for different operator action than either.
func TestCreateIssue_NetworkFailure(t *testing.T) {
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

// TestCreateIssue_NonSuccessResponse covers a non-2xx GitHub response, asserting the returned error
// surfaces the response's message and is distinct from both other error cases.
func TestCreateIssue_NonSuccessResponse(t *testing.T) {
	const errMessage = "Validation Failed"
	var captured []requestCapture
	server := newIssueServer(t, http.StatusUnprocessableEntity, `{"message":"`+errMessage+`"}`, &captured)
	installGitHubClient(t, server.URL)
	buf := captureLogOutput(t)

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
