// cli_test.go contains white-box unit tests for the selfreport CLI.
//
// Tests live in package selfreportcli so the local stdin seam can be replaced without exporting it.
// The GitHub transport is swapped via the exported selfreportengine.NewGitHubClient seam.
// Each test injects a real go-github client pointed at an httptest server.
// That server lets the tests assert on the request shape: method, path and JSON body.
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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// requestCapture describes one HTTP request received by a test's issue server.
// It records the method and path the CLI or engine sent, and the decoded JSON body,
// so a test can assert on title, body and labels directly.
type requestCapture struct {
	method string
	path   string
	body   map[string]any
}

// newIssueServer returns an httptest server that responds to every request with status and respBody.
// It appends a requestCapture for each request it receives to captured.
// Tests that expect no request, such as a cobra arg-count rejection, assert len(*captured) == 0.
// They install no server.
func newIssueServer(t *testing.T, status int, respBody string, captured *[]requestCapture) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		// A decode failure here indicates a test bug:
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

// installGitHubClient replaces selfreportengine.NewGitHubClient with a closure that returns a real go-github client.
// The client points at baseURL, and carries a fixed placeholder token set directly on it, bypassing token resolution.
// Injecting the whole client, not just a base URL, keeps this suite off token resolution:
// it never reads GH_TOKEN or GITHUB_TOKEN, never shells out to `gh auth token`;
// it never touches the operator's real credential cache,
// so it runs on a machine with no gh installed and no GitHub credentials at all.
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

// installGitHubClientPointedAtDeadAddress points a client at an address nothing listens on.
// The resulting connection-refused failure is deterministic and must surface distinctly from an API rejection.
func installGitHubClientPointedAtDeadAddress(t *testing.T) {
	t.Helper()
	// Start and immediately close a server; its address then has nothing listening on it.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := server.URL
	server.Close()
	installGitHubClient(t, deadURL)
}

// installFailingGitHubClientFactory replaces selfreportengine.NewGitHubClient with a closure.
// The closure returns err and no client, simulating a token that cannot be resolved.
func installFailingGitHubClientFactory(t *testing.T, err error) {
	t.Helper()
	orig := selfreportengine.NewGitHubClient
	selfreportengine.NewGitHubClient = func() (*github.Client, error) { return nil, err }
	t.Cleanup(func() { selfreportengine.NewGitHubClient = orig })
}

// runCLI drives RunCLI into a buffer and returns the exit code and output text.
// No args are passed as an empty slice, not nil:
// cobra reads os.Args when handed nil and would then parse the test binary's own -test.* flags.
func runCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	if args == nil {
		args = []string{}
	}
	var buf bytes.Buffer
	code := RunCLI(&buf, args)
	return code, buf.String()
}

// labelsFromBody extracts the "labels" array from a decoded request body as a []string.
// It preserves order, so tests can assert multi-label ordering survives encoding.
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

// TestRunCreate_Success drives the successful create flow through each flag combination.
// It checks exit 0, ok:true, and the url and number from the server's response.
// It also checks the request: its method, path, title, labels and body match the flags.
func TestRunCreate_Success(t *testing.T) {
	tiergate.Require(t, 0)
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
		// A response that carries no number leaves the envelope's number field absent.
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

// TestRunCreate_Failure verifies each way the create call can fail.
// Each case expects ok:false, exit 1 and an error message naming the cause:
// a client factory that cannot resolve a token, a non-2xx response whose message text surfaces,
// and a connection refused, which must give a non-empty message distinct from an API rejection.
func TestRunCreate_Failure(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestRunCLI_RefusedBeforeTransport verifies the cobra-level surface of the selfreport group.
// A bare invocation lists create; an unknown subcommand gets the shared envelope;
// a wrong positional count gets cobra's "accepts 1 arg(s)" message.
// None of them reaches the transport.
func TestRunCLI_RefusedBeforeTransport(t *testing.T) {
	tiergate.Require(t, 0)
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
