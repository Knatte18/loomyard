// cli_test.go contains white-box unit tests for the selfreport CLI.
//
// Tests live in package selfreportcli (same package as the production code) so the local stdin seam can be replaced without exporting it.
// The GitHub transport is swapped via the exported selfreportengine.NewGitHubClient seam, injected with a real go-github client pointed at an httptest server rather than a fake RunGH -- that server is what lets these tests assert on the actual request shape (method, path, JSON body) instead of an argv slice that no longer exists.
// All tests drive the full cobra->flag->CreateIssue->go-github pipeline through RunCLI.
// No test calls t.Parallel: each swaps the package-level NewGitHubClient seam, and the stdin rows swap the stdin seam.

package selfreportcli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
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
// No args are passed as an empty slice, because cobra reads os.Args when handed nil and would then parse a test binary's own -test.* flags.
func runCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	if args == nil {
		args = []string{}
	}
	var buf bytes.Buffer
	code := RunCLI(&buf, args)
	return code, buf.String()
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

// TestRunCreate_Success drives the successful create flow through each flag combination: exit 0, ok:true, the url and number of the server's typed response, and a request whose method, path, title, labels and body field match what the flags asked for.
func TestRunCreate_Success(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/123"
	const markdownBody = "# Bug Report\n\nThis is a *markdown* body.\nSecond paragraph.\n"
	withNumber := `{"html_url":"` + issueURL + `","number":123}`
	withoutNumber := `{"html_url":"` + issueURL + `"}`

	tests := []struct {
		name       string
		args       []string
		stdin      string
		response   string
		wantTitle  string
		wantLabels []string
		// wantBody is the request's "body" field; nil means the field must be absent.
		wantBody *string
		// wantNumber is the envelope's number; nil means the field must be absent.
		wantNumber *float64
	}{
		{"defaults", []string{"create", "My bug title"}, "", withNumber, "My bug title", []string{"bug"}, nil, new(123.0)},
		// Explicit labels replace the default "bug" entirely and keep the order given.
		{"custom labels replace default", []string{"create", "T", "--label", "enhancement", "--label", "p1"}, "", withNumber, "T", []string{"enhancement", "p1"}, nil, new(123.0)},
		{"body via flag", []string{"create", "T", "-b", "details"}, "", withNumber, "T", []string{"bug"}, new("details"), new(123.0)},
		{"body via stdin", []string{"create", "T", "-b", "-"}, markdownBody, withNumber, "T", []string{"bug"}, new(markdownBody), new(123.0)},
		// The surviving form of the old unparseable-URL convention: a response without a number.
		{"number omitted when response has none", []string{"create", "T"}, "", withoutNumber, "T", []string{"bug"}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured []requestCapture
			server := newIssueServer(t, http.StatusCreated, tt.response, &captured)
			installGitHubClient(t, server.URL)
			if tt.stdin != "" {
				origStdin := stdin
				stdin = strings.NewReader(tt.stdin)
				t.Cleanup(func() { stdin = origStdin })
			}

			code, stdout := runCLI(t, tt.args...)
			env := envelope.Decode(t, stdout)

			if code != 0 {
				t.Errorf("RunCLI() exit = %d; want 0\nstdout: %s", code, stdout)
			}
			if !env.OK {
				t.Errorf("envelope ok = %v; want true", env.OK)
			}
			if url, _ := env.Raw["url"].(string); url != issueURL {
				t.Errorf("envelope url = %q; want %q", url, issueURL)
			}
			// JSON numbers decode to float64 in a map[string]any.
			num, hasNumber := env.Raw["number"]
			switch {
			case tt.wantNumber == nil && hasNumber:
				t.Errorf("envelope has number = %v but the response carried none; want number absent", num)
			case tt.wantNumber != nil && num != *tt.wantNumber:
				t.Errorf("envelope number = %v (%T); want %v", num, num, *tt.wantNumber)
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
			if title, _ := got.body["title"].(string); title != tt.wantTitle {
				t.Errorf("request body title = %q; want %q", title, tt.wantTitle)
			}
			if labels := labelsFromBody(got.body); !slices.Equal(labels, tt.wantLabels) {
				t.Errorf("request body labels = %v; want %v", labels, tt.wantLabels)
			}
			gotBody, hasBody := got.body["body"]
			switch {
			case tt.wantBody == nil && hasBody:
				t.Errorf("request has \"body\" field %v but none was provided", gotBody)
			case tt.wantBody != nil && gotBody != *tt.wantBody:
				t.Errorf("request body \"body\" field = %q; want %q", gotBody, *tt.wantBody)
			}
		})
	}
}

// TestRunCreate_Failure verifies that each way the create call can fail yields ok:false with exit 1 and an error message naming the cause: a client factory that cannot resolve a token, a non-2xx response (the message text surfaces) and a connection refused (a non-empty message distinct from an API rejection).
func TestRunCreate_Failure(t *testing.T) {
	tests := []struct {
		name    string
		install func(t *testing.T)
		// wantError is a substring of the envelope error; empty means any non-empty error.
		wantError string
	}{
		{"token not resolvable", func(t *testing.T) { installFailingGitHubClientFactory(t, githubclient.ErrTokenUnresolvable) }, "token"},
		{"non-success response", func(t *testing.T) {
			var captured []requestCapture
			server := newIssueServer(t, http.StatusUnprocessableEntity, `{"message":"Validation Failed"}`, &captured)
			installGitHubClient(t, server.URL)
		}, "Validation Failed"},
		{"network failure", installGitHubClientPointedAtDeadAddress, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.install(t)

			code, stdout := runCLI(t, "create", "T")
			env := envelope.Decode(t, stdout)

			if code != 1 {
				t.Errorf("RunCLI() exit = %d; want 1\nstdout: %s", code, stdout)
			}
			if env.OK {
				t.Errorf("envelope ok = true; want false")
			}
			if env.Error == "" || !strings.Contains(env.Error, tt.wantError) {
				t.Errorf("error %q does not contain %q", env.Error, tt.wantError)
			}
		})
	}
}

// TestRunCLI_RefusedBeforeTransport verifies the cobra-level surface of the selfreport group: a bare invocation lists create, an unknown subcommand gets the shared envelope, and a wrong positional count gets cobra's "accepts 1 arg(s)" message; none of them reaches the transport.
func TestRunCLI_RefusedBeforeTransport(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantOutput string
	}{
		{"bare lists create", nil, 0, "create"},
		{"unknown subcommand", []string{"bogus"}, 1, "unknown subcommand"},
		{"too few args", []string{"create"}, 1, "accepts 1 arg"},
		{"too many args", []string{"create", "a", "b"}, 1, "accepts 1 arg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured []requestCapture
			server := newIssueServer(t, http.StatusCreated, `{"html_url":"https://example.invalid/1","number":1}`, &captured)
			installGitHubClient(t, server.URL)

			code, stdout := runCLI(t, tt.args...)

			if code != tt.wantExit {
				t.Errorf("RunCLI(%v) exit = %d; want %d\nstdout: %s", tt.args, code, tt.wantExit, stdout)
			}
			if !strings.Contains(stdout, tt.wantOutput) {
				t.Errorf("output does not contain %q; stdout: %q", tt.wantOutput, stdout)
			}
			if len(captured) != 0 {
				t.Errorf("request count = %d; want 0", len(captured))
			}
		})
	}
}
