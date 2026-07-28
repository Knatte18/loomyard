// cli_test.go contains white-box unit tests for the selfreport CLI.
//
// Tests live in package selfreportcli (same package as the production code) so the
// local stdin seam can be replaced without exporting it. The GitHub transport is
// swapped via the exported selfreportengine.NewGitHubClient seam, injected with a
// real go-github client pointed at an httptest server rather than a fake RunGH --
// that server is what lets these tests assert on the actual request shape (method,
// path, JSON body) instead of an argv slice that no longer exists. All tests drive
// the full cobra->flag->CreateIssue->go-github pipeline through RunCLI.

package selfreportcli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
)

// requestCapture describes one HTTP request received by a test's issue
// server: the method and path the CLI/engine actually sent, and the decoded
// JSON body so a test can assert on title/body/labels directly instead of an
// argv slice.
type requestCapture struct {
	method string
	path   string
	body   map[string]any
}

// newIssueServer returns an httptest server that responds to every request
// with status and respBody, appending a requestCapture describing each
// request it receives to captured. Tests that expect no request at all
// (e.g. cobra arg-count rejection) assert len(*captured) == 0 instead of
// installing this server.
func newIssueServer(t *testing.T, status int, respBody string, captured *[]requestCapture) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		// A decode failure here would only ever indicate a genuine test bug --
		// every request this server receives in these tests carries a JSON body.
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

// installGitHubClient replaces selfreportengine.NewGitHubClient with a
// closure that returns a real go-github client pointed at baseURL,
// authenticated with a fixed placeholder token set directly on the client
// rather than through githubclient's resolution chain. Injecting the whole
// authenticated client, not just a base URL, is deliberate: it guarantees
// this suite never reaches token resolution, so it never reads GH_TOKEN or
// GITHUB_TOKEN, never shells out to `gh auth token`, and never touches the
// operator's real credential cache -- runnable on a machine with no gh
// installed and no GitHub credentials at all.
func installGitHubClient(t *testing.T, baseURL string) {
	t.Helper()
	client := github.NewClient(nil).WithAuthToken("test-token")
	parsed, err := url.Parse(baseURL + "/")
	if err != nil {
		t.Fatalf("url.Parse(%s): %v", baseURL, err)
	}
	client.BaseURL = parsed

	orig := selfreportengine.NewGitHubClient
	selfreportengine.NewGitHubClient = func() (*github.Client, error) { return client, nil }
	t.Cleanup(func() { selfreportengine.NewGitHubClient = orig })
}

// installGitHubClientPointedAtDeadAddress installs a client pointed at an
// address nothing listens on, producing a deterministic connection-refused
// network failure that must surface distinctly from an API rejection.
func installGitHubClientPointedAtDeadAddress(t *testing.T) {
	t.Helper()
	// Start and immediately close a server: its address is guaranteed to have
	// nothing listening on it afterward.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := server.URL
	server.Close()
	installGitHubClient(t, deadURL)
}

// installFailingGitHubClientFactory replaces selfreportengine.NewGitHubClient
// with a closure that itself returns err without ever handing back a client --
// the CLI-level analogue of the old "gh binary not found" case, now
// surfacing as a token-not-resolvable factory failure.
func installFailingGitHubClientFactory(t *testing.T, err error) {
	t.Helper()
	orig := selfreportengine.NewGitHubClient
	selfreportengine.NewGitHubClient = func() (*github.Client, error) { return nil, err }
	t.Cleanup(func() { selfreportengine.NewGitHubClient = orig })
}

// runCLI drives RunCLI into a buffer and returns the exit code and output text.
func runCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	code := RunCLI(&buf, args)
	return code, buf.String()
}

// parseEnvelope unmarshals the single-line JSON written by RunCLI.
// The test fails immediately when the output is not valid JSON.
func parseEnvelope(t *testing.T, stdout string) map[string]any {
	t.Helper()
	line := strings.TrimSpace(stdout)
	var env map[string]any
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		t.Fatalf("parseEnvelope: %v; raw=%q", err, stdout)
	}
	return env
}

// labelsFromBody extracts the "labels" array from a decoded request body as
// a []string, preserving order, so tests can assert multi-label ordering
// survives encoding.
func labelsFromBody(body map[string]any) []string {
	raw, ok := body["labels"].([]any)
	if !ok {
		return nil
	}
	labels := make([]string, len(raw))
	for i, v := range raw {
		labels[i], _ = v.(string)
	}
	return labels
}

// contains reports whether s appears in slice.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// TestRunCreate_HappyPath drives the normal successful create flow: exit 0,
// ok:true, url and number match the server's typed response, and the
// recorded request matches the expected method, path, title, and default
// "bug" label.
func TestRunCreate_HappyPath(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/123"
	var captured []requestCapture
	server := newIssueServer(t, http.StatusCreated, `{"html_url":"`+issueURL+`","number":123}`, &captured)
	installGitHubClient(t, server.URL)

	code, stdout := runCLI(t, "create", "My bug title")
	env := parseEnvelope(t, stdout)

	if code != 0 {
		t.Errorf("RunCLI() exit = %d; want 0\nstdout: %s", code, stdout)
	}
	if ok, _ := env["ok"].(bool); !ok {
		t.Errorf("envelope ok = %v; want true", env["ok"])
	}
	if url, _ := env["url"].(string); url != issueURL {
		t.Errorf("envelope url = %q; want %q", url, issueURL)
	}
	// JSON numbers decode to float64 in a map[string]any; compare accordingly.
	if num, _ := env["number"].(float64); num != float64(123) {
		t.Errorf("envelope number = %v; want float64(123)", env["number"])
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
	if title, _ := got.body["title"].(string); title != "My bug title" {
		t.Errorf("request body title = %q; want %q", title, "My bug title")
	}
	if labels := labelsFromBody(got.body); len(labels) != 1 || labels[0] != "bug" {
		t.Errorf("request body labels = %v; want [\"bug\"]", labels)
	}
	if _, hasBody := got.body["body"]; hasBody {
		t.Errorf("request body has \"body\" field but none was provided; got %v", got.body)
	}
}

// TestRunCreate_CustomLabels verifies that explicit --label flags replace the
// default "bug" label entirely, and that multiple labels survive in the
// order they were given.
func TestRunCreate_CustomLabels(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/99"
	var captured []requestCapture
	server := newIssueServer(t, http.StatusCreated, `{"html_url":"`+issueURL+`","number":99}`, &captured)
	installGitHubClient(t, server.URL)

	code, stdout := runCLI(t, "create", "T", "--label", "enhancement", "--label", "p1")
	env := parseEnvelope(t, stdout)

	if code != 0 {
		t.Errorf("RunCLI() exit = %d; want 0\nstdout: %s", code, stdout)
	}
	if ok, _ := env["ok"].(bool); !ok {
		t.Errorf("envelope ok = %v; want true", env["ok"])
	}
	if len(captured) != 1 {
		t.Fatalf("request count = %d; want 1", len(captured))
	}
	labels := labelsFromBody(captured[0].body)

	// Both specified labels must appear in the order given; "bug" must NOT
	// appear because the default is replaced, not appended, when explicit
	// labels are provided.
	if len(labels) != 2 || labels[0] != "enhancement" || labels[1] != "p1" {
		t.Errorf("request body labels = %v; want [\"enhancement\" \"p1\"] in order", labels)
	}
	if contains(labels, "bug") {
		t.Errorf("labels %v contain \"bug\" but default should be replaced", labels)
	}
}

// TestRunCreate_BodyViaFlag verifies that -b/--body passes the flag value
// directly through as the request body's "body" field.
func TestRunCreate_BodyViaFlag(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/1"
	var captured []requestCapture
	server := newIssueServer(t, http.StatusCreated, `{"html_url":"`+issueURL+`","number":1}`, &captured)
	installGitHubClient(t, server.URL)

	code, _ := runCLI(t, "create", "T", "-b", "details")

	if code != 0 {
		t.Errorf("RunCLI() exit = %d; want 0", code)
	}
	if len(captured) != 1 {
		t.Fatalf("request count = %d; want 1", len(captured))
	}
	bodyVal, _ := captured[0].body["body"].(string)
	if bodyVal != "details" {
		t.Errorf("request body \"body\" field = %q; want %q", bodyVal, "details")
	}
}

// TestRunCreate_BodyViaStdin verifies that -b "-" reads the entire stdin seam
// and passes its content intact as the request body's "body" field,
// preserving multi-line markdown.
func TestRunCreate_BodyViaStdin(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/2"
	var captured []requestCapture
	server := newIssueServer(t, http.StatusCreated, `{"html_url":"`+issueURL+`","number":2}`, &captured)
	installGitHubClient(t, server.URL)

	const markdownBody = "# Bug Report\n\nThis is a *markdown* body.\nSecond paragraph.\n"

	// Swap the stdin seam for this test; restore via t.Cleanup so the seam
	// is reset before the next test regardless of pass/fail.
	origStdin := stdin
	stdin = strings.NewReader(markdownBody)
	t.Cleanup(func() { stdin = origStdin })

	code, _ := runCLI(t, "create", "T", "-b", "-")

	if code != 0 {
		t.Errorf("RunCLI() exit = %d; want 0", code)
	}
	if len(captured) != 1 {
		t.Fatalf("request count = %d; want 1", len(captured))
	}
	bodyVal, _ := captured[0].body["body"].(string)
	if bodyVal != markdownBody {
		t.Errorf("request body \"body\" field = %q; want %q", bodyVal, markdownBody)
	}
}

// TestRunCreate_BodyOmitted verifies that when --body is not set the request
// body carries no "body" field at all, and the command still succeeds.
func TestRunCreate_BodyOmitted(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/3"
	var captured []requestCapture
	server := newIssueServer(t, http.StatusCreated, `{"html_url":"`+issueURL+`","number":3}`, &captured)
	installGitHubClient(t, server.URL)

	code, stdout := runCLI(t, "create", "T")
	env := parseEnvelope(t, stdout)

	if code != 0 {
		t.Errorf("RunCLI() exit = %d; want 0\nstdout: %s", code, stdout)
	}
	if ok, _ := env["ok"].(bool); !ok {
		t.Errorf("envelope ok = %v; want true", env["ok"])
	}
	if len(captured) != 1 {
		t.Fatalf("request count = %d; want 1", len(captured))
	}
	if _, hasBody := captured[0].body["body"]; hasBody {
		t.Errorf("request body has \"body\" field but none was provided; got %v", captured[0].body)
	}
}

// TestRunCreate_WrongArgCount verifies that cobra's ExactArgs(1) guard rejects
// both too-few and too-many positional arguments: non-zero exit, the cobra
// "accepts 1 arg(s)" message in output, and no request ever reaches the
// transport at all.
func TestRunCreate_WrongArgCount(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"too_few", []string{"create"}},
		{"too_many", []string{"create", "a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured []requestCapture
			server := newIssueServer(t, http.StatusCreated, `{"html_url":"https://example.invalid/1","number":1}`, &captured)
			installGitHubClient(t, server.URL)

			code, stdout := runCLI(t, tt.args...)

			if code == 0 {
				t.Errorf("RunCLI(%v) exit = 0; want non-zero", tt.args)
			}
			// Cobra writes "Error: accepts 1 arg(s), received N" to the error
			// writer (merged into out by clihelp.Execute).
			if !strings.Contains(stdout, "accepts 1 arg") {
				t.Errorf("output does not contain cobra arg-count message; stdout: %q", stdout)
			}
			// The transport must never be reached when cobra's validation
			// rejects the args.
			if len(captured) != 0 {
				t.Errorf("request count = %d; want 0", len(captured))
			}
		})
	}
}

// TestRunCreate_TokenNotResolvable verifies that when the GitHub client
// factory itself fails -- the CLI-level analogue of the old "gh binary not
// found on PATH" case -- the envelope is ok:false with exit 1 and the error
// message surfaces the underlying token-resolution failure.
func TestRunCreate_TokenNotResolvable(t *testing.T) {
	installFailingGitHubClientFactory(t, githubclient.ErrTokenUnresolvable)

	code, stdout := runCLI(t, "create", "T")
	env := parseEnvelope(t, stdout)

	if code != 1 {
		t.Errorf("RunCLI() exit = %d; want 1\nstdout: %s", code, stdout)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("envelope ok = true; want false")
	}
	errMsg, _ := env["error"].(string)
	if !strings.Contains(errMsg, "token") {
		t.Errorf("error %q does not mention the token-resolution failure", errMsg)
	}
}

// TestRunCreate_NonSuccessResponse verifies that when GitHub responds with a
// non-2xx status the envelope is ok:false with exit 1, and the error message
// surfaces the response's message text -- the go-github equivalent of the
// old "gh issue create failed: <stderr>" case.
func TestRunCreate_NonSuccessResponse(t *testing.T) {
	const errMessage = "Validation Failed"
	var captured []requestCapture
	server := newIssueServer(t, http.StatusUnprocessableEntity, `{"message":"`+errMessage+`"}`, &captured)
	installGitHubClient(t, server.URL)

	code, stdout := runCLI(t, "create", "T")
	env := parseEnvelope(t, stdout)

	if code != 1 {
		t.Errorf("RunCLI() exit = %d; want 1\nstdout: %s", code, stdout)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("envelope ok = true; want false")
	}
	errMsg, _ := env["error"].(string)
	if !strings.Contains(errMsg, errMessage) {
		t.Errorf("error %q does not contain response message %q", errMsg, errMessage)
	}
}

// TestRunCreate_NetworkFailure verifies that a network-level failure (the
// server address refuses the connection) surfaces distinctly from an API
// rejection: still ok:false with exit 1, but without the response-message
// text a non-2xx case would carry.
func TestRunCreate_NetworkFailure(t *testing.T) {
	installGitHubClientPointedAtDeadAddress(t)

	code, stdout := runCLI(t, "create", "T")
	env := parseEnvelope(t, stdout)

	if code != 1 {
		t.Errorf("RunCLI() exit = %d; want 1\nstdout: %s", code, stdout)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("envelope ok = true; want false")
	}
	errMsg, _ := env["error"].(string)
	if errMsg == "" {
		t.Errorf("envelope error is empty; want a network-failure message")
	}
}

// TestRunCreate_NumberOmittedWhenZero verifies that when the server's typed
// response carries no issue number at all, the envelope's "number" field is
// absent -- the surviving form of the old unparseable-URL convention, now
// exercised through go-github's own nil-number case rather than a URL
// suffix parse.
func TestRunCreate_NumberOmittedWhenZero(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/999"
	var captured []requestCapture
	server := newIssueServer(t, http.StatusCreated, `{"html_url":"`+issueURL+`"}`, &captured)
	installGitHubClient(t, server.URL)

	code, stdout := runCLI(t, "create", "T")
	env := parseEnvelope(t, stdout)

	if code != 0 {
		t.Errorf("RunCLI() exit = %d; want 0\nstdout: %s", code, stdout)
	}
	if ok, _ := env["ok"].(bool); !ok {
		t.Errorf("envelope ok = %v; want true", env["ok"])
	}
	if _, hasURL := env["url"]; !hasURL {
		t.Errorf("envelope missing url field; got %v", env)
	}
	if _, hasNum := env["number"]; hasNum {
		t.Errorf("envelope has number = %v but the response carried none; want number absent", env["number"])
	}
}

// TestRunCreate_NumberParsing verifies that a 201 response carrying number
// 123 produces number == 123 in the success envelope. JSON unmarshal into
// map[string]any decodes all numbers as float64, so the comparison is
// against float64(123).
func TestRunCreate_NumberParsing(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/123"
	var captured []requestCapture
	server := newIssueServer(t, http.StatusCreated, `{"html_url":"`+issueURL+`","number":123}`, &captured)
	installGitHubClient(t, server.URL)

	code, stdout := runCLI(t, "create", "T")
	env := parseEnvelope(t, stdout)

	if code != 0 {
		t.Errorf("RunCLI() exit = %d; want 0\nstdout: %s", code, stdout)
	}
	num, ok := env["number"].(float64)
	if !ok {
		t.Fatalf("envelope number type = %T; want float64", env["number"])
	}
	if num != float64(123) {
		t.Errorf("envelope number = %v; want float64(123)", num)
	}
}
