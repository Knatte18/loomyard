// cli_test.go contains white-box unit tests for the selfreport CLI.
//
// Tests live in package selfreportcli so the local stdin seam can be replaced without exporting it.
// The GitHub transport is swapped via the exported selfreportengine.NewGitHubClient seam.
// Each test injects a real go-github client pointed at an httptest server.
// That server lets the tests assert on the request shape: method, path and JSON body.
// All tests drive the full cobra->flag->CreateIssue->go-github pipeline through clihelp.Execute.
// No test calls t.Parallel: each swaps the package-level NewGitHubClient seam, the stdin rows swap the stdin seam,
// and the body-file rows change the working directory.

package selfreportcli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/clihelp"
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

// runCLI drives clihelp.Execute into a buffer and returns the exit code and output text.
// No args are passed as an empty slice, not nil:
// cobra reads os.Args when handed nil and would then parse the test binary's own -test.* flags.
func runCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	if args == nil {
		args = []string{}
	}
	var buf bytes.Buffer
	code := clihelp.Execute(Command(), &buf, args)
	return code, buf.String()
}

// reportFileName is the file that chdirToReportFile writes, named by a relative path in the body-file rows.
const reportFileName = "report.md"

// reportFileContent is the content of the file that chdirToReportFile writes.
const reportFileContent = "## What happened\n\nThe report held in a file.\n"

// fencedFileName is the file that chdirToReportFile writes holding a code fence of its own.
const fencedFileName = "fenced.md"

// fencedFileContent is the content of fencedFileName.
const fencedFileContent = "```go\nx\n```\n"

// oversizedFileName is the file that chdirToReportFile writes, one character longer than GitHub's issue body limit.
const oversizedFileName = "huge.log"

// invalidUTF8FileName is the file that chdirToReportFile writes holding bytes that are not valid UTF-8.
const invalidUTF8FileName = "binary.bin"

// infixBacktickFileName and edgeBacktickFileName are files that chdirToReportFile writes whose names hold backticks.
const (
	infixBacktickFileName = "a`b.txt"
	edgeBacktickFileName  = "`c`"
)

// namedFileContent is the content of the backtick-named files.
const namedFileContent = "named\n"

// limitBody is a multibyte --body whose attachments, with it, fill the issue body limit in characters but not in bytes.
const limitBody = "ééé"

// fitFileName and overFileName are the files that chdirToReportFile writes to hit the issue body limit with limitBody.
// Filed after limitBody, the first makes the body exactly maxIssueBodyRunes characters and the second one more.
const (
	fitFileName  = "fit.log"
	overFileName = "over.log"
)

// limitFillerContent returns multibyte content for the attachment called name.
// Filed after limitBody, it makes the body maxIssueBodyRunes plus extraRunes characters.
func limitFillerContent(name string, extraRunes int) string {
	overhead := utf8.RuneCountInString(limitBody + "\n\n## Attached `" + name + "`\n\n```\n\n```")
	return strings.Repeat("é", maxIssueBodyRunes+extraRunes-overhead)
}

// chdirToReportFile changes the working directory to a fresh temp directory.
// The directory holds every file the constants above name.
func chdirToReportFile(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		reportFileName:        reportFileContent,
		fencedFileName:        fencedFileContent,
		oversizedFileName:     strings.Repeat("x", maxIssueBodyRunes+1),
		invalidUTF8FileName:   "ok\xff\xfe",
		infixBacktickFileName: namedFileContent,
		edgeBacktickFileName:  namedFileContent,
		fitFileName:           limitFillerContent(fitFileName, 0),
		overFileName:          limitFillerContent(overFileName, 1),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%s): %v", name, err)
		}
	}
	t.Chdir(dir)
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
		{"body via file", []string{"create", "T", "--body-file", reportFileName}, "", withNumber, "T", []string{"bug"}, new(reportFileContent), new(123.0)},
		{"body file via stdin", []string{"create", "T", "--body-file", "-"}, markdownBody, withNumber, "T", []string{"bug"}, new(markdownBody), new(123.0)},
		// Body text that names a file only as part of a longer line is filed as text.
		{"body text mentioning a file", []string{"create", "T", "-b", "see " + reportFileName}, "", withNumber, "T", []string{"bug"}, new("see " + reportFileName), new(123.0)},
		// A directory is not a file to refuse, and an empty body is filed as empty rather than omitted.
		{"body text naming a directory", []string{"create", "T", "-b", "."}, "", withNumber, "T", []string{"bug"}, new("."), new(123.0)},
		{"empty body via flag", []string{"create", "T", "-b", ""}, "", withNumber, "T", []string{"bug"}, new(""), new(123.0)},
		// Attachments follow the body in order, each under its base name in a fence longer than any backtick run it holds.
		{"attachments follow the body", []string{"create", "T", "-b", "details\n", "--attach", "./" + reportFileName, "--attach", fencedFileName}, "", withNumber, "T", []string{"bug"},
			new("details\n\n## Attached `" + reportFileName + "`\n\n```\n## What happened\n\nThe report held in a file.\n```" +
				"\n\n## Attached `" + fencedFileName + "`\n\n````\n```go\nx\n```\n````"), new(123.0)},
		{"attachment alone is the body", []string{"create", "T", "--attach", reportFileName}, "", withNumber, "T", []string{"bug"},
			new("## Attached `" + reportFileName + "`\n\n```\n## What happened\n\nThe report held in a file.\n```"), new(123.0)},
		// An empty body adds no blank lines before the first section.
		{"empty body with an attachment", []string{"create", "T", "-b", "", "--attach", reportFileName}, "", withNumber, "T", []string{"bug"},
			new("## Attached `" + reportFileName + "`\n\n```\n## What happened\n\nThe report held in a file.\n```"), new(123.0)},
		// A file name with a backtick is a code span delimited by a longer backtick run, padded at an edge backtick.
		{"attachment names with backticks", []string{"create", "T", "--attach", infixBacktickFileName, "--attach", edgeBacktickFileName}, "", withNumber, "T", []string{"bug"},
			new("## Attached ``a`b.txt``\n\n```\nnamed\n```\n\n## Attached `` `c` ``\n\n```\nnamed\n```"), new(123.0)},
		// A body whose characters total the limit is filed, though its bytes exceed it.
		{"body at the limit in characters", []string{"create", "T", "-b", limitBody, "--attach", fitFileName}, "", withNumber, "T", []string{"bug"},
			new(limitBody + "\n\n## Attached `" + fitFileName + "`\n\n```\n" + limitFillerContent(fitFileName, 0) + "\n```"), new(123.0)},
		// A response that carries no number leaves the envelope's number field absent.
		{"number omitted when response has none", []string{"create", "T"}, "", withoutNumber, "T", []string{"bug"}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chdirToReportFile(t)
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
				t.Errorf("Execute() exit = %d; want 0\nstdout: %s", code, stdout)
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
				t.Errorf("Execute() exit = %d; want 1\nstdout: %s", code, stdout)
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

// TestCommand_RefusedBeforeTransport verifies the cobra-level surface of the selfreport group.
// A bare invocation lists create; an unknown subcommand gets the shared envelope;
// a wrong positional count gets cobra's "accepts 1 arg(s)" message.
// A body the flags cannot give is refused:
// --body text that is one line naming an existing file;
// both body flags together;
// a missing body file;
// a missing attachment;
// an attachment that pushes the body past GitHub's limit, by one character at the boundary or by a whole file;
// an attachment that is not valid UTF-8.
// None of them reaches the transport.
func TestCommand_RefusedBeforeTransport(t *testing.T) {
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
		{"body text naming an existing file", []string{"create", "T", "-b", reportFileName}, 1, `pass --body-file \"` + reportFileName + `\"`},
		{"body text naming an existing file with padding", []string{"create", "T", "-b", " " + reportFileName + "\n"}, 1, `pass --body-file \"` + reportFileName + `\"`},
		{"body and body file together", []string{"create", "T", "-b", "x", "--body-file", reportFileName}, 1, "[body body-file]"},
		{"missing body file", []string{"create", "T", "--body-file", "missing.md"}, 1, "failed to read --body-file"},
		{"missing attachment", []string{"create", "T", "--attach", "missing.txt"}, 1, "failed to read --attach"},
		{"attachment over the body limit", []string{"create", "T", "--attach", oversizedFileName}, 1, "over GitHub's limit of " + strconv.Itoa(maxIssueBodyRunes)},
		{"body and attachment one character over the limit", []string{"create", "T", "-b", limitBody, "--attach", overFileName}, 1,
			"is " + strconv.Itoa(maxIssueBodyRunes+1) + " characters, over GitHub's limit"},
		{"attachment not valid UTF-8", []string{"create", "T", "--attach", invalidUTF8FileName}, 1, "is not valid UTF-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chdirToReportFile(t)
			var captured []requestCapture
			server := newIssueServer(t, http.StatusCreated, `{"html_url":"https://example.invalid/1","number":1}`, &captured)
			installGitHubClient(t, server.URL)

			code, stdout := runCLI(t, tt.args...)

			if code != tt.wantExit {
				t.Errorf("Execute(%v) exit = %d; want %d\nstdout: %s", tt.args, code, tt.wantExit, stdout)
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
