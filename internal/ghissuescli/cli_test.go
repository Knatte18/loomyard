// cli_test.go contains white-box unit tests for the ghissues CLI.
//
// Tests live in package ghissuescli (same package as the production code) so the
// local stdin seam can be replaced without exporting it; the gh executor is
// swapped via the exported ghissuesengine.RunGH seam. All tests drive the full
// cobra→flag→CreateIssue→RunGH pipeline through RunCLI.

package ghissuescli

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/ghissuesengine"
)

// installFakeGH replaces the exported ghissuesengine.RunGH seam with a fake that
// records every argv slice it receives and returns the caller-supplied values.
// The original seam is restored via t.Cleanup so each test case is independent.
func installFakeGH(t *testing.T, fakeStdout, fakeStderr string, fakeExitCode int, fakeErr error) *[][]string {
	t.Helper()
	var recorded [][]string
	orig := ghissuesengine.RunGH
	ghissuesengine.RunGH = func(args []string) (string, string, int, error) {
		recorded = append(recorded, args)
		return fakeStdout, fakeStderr, fakeExitCode, fakeErr
	}
	t.Cleanup(func() { ghissuesengine.RunGH = orig })
	return &recorded
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

// TestRunCreate_HappyPath drives the normal successful create flow: exit 0,
// ok:true, url matches the fake output, number is 123 (float64 in the decoded
// map), and the recorded gh argv matches the expected slice exactly including
// the default "bug" label.
func TestRunCreate_HappyPath(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/123"
	argv := installFakeGH(t, issueURL+"\n", "", 0, nil)

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

	if len(*argv) != 1 {
		t.Fatalf("RunGH call count = %d; want 1", len(*argv))
	}
	got := (*argv)[0]
	wantArgv := []string{"issue", "create", "--repo", "Knatte18/loomyard", "--title", "My bug title", "--label", "bug"}
	if len(got) != len(wantArgv) {
		t.Fatalf("argv len = %d; want %d\ngot:  %v\nwant: %v", len(got), len(wantArgv), got, wantArgv)
	}
	for i := range wantArgv {
		if got[i] != wantArgv[i] {
			t.Errorf("argv[%d] = %q; want %q", i, got[i], wantArgv[i])
		}
	}
}

// TestRunCreate_CustomLabels verifies that explicit --label flags replace the
// default "bug" label entirely: only the specified labels appear in the gh argv.
func TestRunCreate_CustomLabels(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/99"
	argv := installFakeGH(t, issueURL+"\n", "", 0, nil)

	code, stdout := runCLI(t, "create", "T", "--label", "enhancement", "--label", "p1")
	env := parseEnvelope(t, stdout)

	if code != 0 {
		t.Errorf("RunCLI() exit = %d; want 0\nstdout: %s", code, stdout)
	}
	if ok, _ := env["ok"].(bool); !ok {
		t.Errorf("envelope ok = %v; want true", env["ok"])
	}
	if len(*argv) != 1 {
		t.Fatalf("RunGH call count = %d; want 1", len(*argv))
	}
	got := (*argv)[0]
	labelArgs := extractLabelValues(got)

	// Both specified labels must appear; "bug" must NOT appear because the
	// default is replaced, not appended, when explicit labels are provided.
	if !contains(labelArgs, "enhancement") {
		t.Errorf("labels %v do not contain \"enhancement\"", labelArgs)
	}
	if !contains(labelArgs, "p1") {
		t.Errorf("labels %v do not contain \"p1\"", labelArgs)
	}
	if contains(labelArgs, "bug") {
		t.Errorf("labels %v contain \"bug\" but default should be replaced; argv: %v", labelArgs, got)
	}
}

// TestRunCreate_BodyViaFlag verifies that -b/--body passes the flag value
// directly to gh as the --body argument.
func TestRunCreate_BodyViaFlag(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/1"
	argv := installFakeGH(t, issueURL+"\n", "", 0, nil)

	code, _ := runCLI(t, "create", "T", "-b", "details")

	if code != 0 {
		t.Errorf("RunCLI() exit = %d; want 0", code)
	}
	if len(*argv) != 1 {
		t.Fatalf("RunGH call count = %d; want 1", len(*argv))
	}
	got := (*argv)[0]
	bodyVal := extractFlagValue(got, "--body")
	if bodyVal == nil {
		t.Fatalf("argv has no --body flag; got %v", got)
	}
	if *bodyVal != "details" {
		t.Errorf("--body = %q; want %q", *bodyVal, "details")
	}
}

// TestRunCreate_BodyViaStdin verifies that -b "-" reads the entire stdin seam
// and passes its content intact to gh as --body, preserving multi-line markdown.
func TestRunCreate_BodyViaStdin(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/2"
	argv := installFakeGH(t, issueURL+"\n", "", 0, nil)

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
	if len(*argv) != 1 {
		t.Fatalf("RunGH call count = %d; want 1", len(*argv))
	}
	got := (*argv)[0]
	bodyVal := extractFlagValue(got, "--body")
	if bodyVal == nil {
		t.Fatalf("argv has no --body flag; got %v", got)
	}
	if *bodyVal != markdownBody {
		t.Errorf("--body = %q; want %q", *bodyVal, markdownBody)
	}
}

// TestRunCreate_BodyOmitted verifies that when --body is not set no --body flag
// appears in the gh argv, and the command still succeeds with ok:true.
func TestRunCreate_BodyOmitted(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/3"
	argv := installFakeGH(t, issueURL+"\n", "", 0, nil)

	code, stdout := runCLI(t, "create", "T")
	env := parseEnvelope(t, stdout)

	if code != 0 {
		t.Errorf("RunCLI() exit = %d; want 0\nstdout: %s", code, stdout)
	}
	if ok, _ := env["ok"].(bool); !ok {
		t.Errorf("envelope ok = %v; want true", env["ok"])
	}
	if len(*argv) != 1 {
		t.Fatalf("RunGH call count = %d; want 1", len(*argv))
	}
	got := (*argv)[0]
	if bodyVal := extractFlagValue(got, "--body"); bodyVal != nil {
		t.Errorf("argv contains --body = %q but no body was provided; argv: %v", *bodyVal, got)
	}
}

// TestRunCreate_WrongArgCount verifies that cobra's ExactArgs(1) guard rejects
// both too-few and too-many positional arguments: non-zero exit, the cobra
// "accepts 1 arg(s)" message in output, and the fake RunGH never called.
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
			argv := installFakeGH(t, "", "", 0, nil)

			code, stdout := runCLI(t, tt.args...)

			if code == 0 {
				t.Errorf("RunCLI(%v) exit = 0; want non-zero", tt.args)
			}
			// Cobra writes "Error: accepts 1 arg(s), received N" to the error
			// writer (merged into out by clihelp.Execute).
			if !strings.Contains(stdout, "accepts 1 arg") {
				t.Errorf("output does not contain cobra arg-count message; stdout: %q", stdout)
			}
			// RunGH must never be reached when cobra's validation rejects the args.
			if len(*argv) != 0 {
				t.Errorf("RunGH called %d time(s); want 0", len(*argv))
			}
		})
	}
}

// TestRunCreate_GhNotFound verifies that when the fake RunGH returns an error
// satisfying errors.Is(err, exec.ErrNotFound), the envelope is ok:false with
// exit 1 and the error message mentions both "gh" and "PATH".
func TestRunCreate_GhNotFound(t *testing.T) {
	// exec.ErrNotFound is the sentinel checked by errors.Is in CreateIssue to
	// distinguish a missing binary from a generic exec failure.
	installFakeGH(t, "", "", -1, exec.ErrNotFound)

	code, stdout := runCLI(t, "create", "T")
	env := parseEnvelope(t, stdout)

	if code != 1 {
		t.Errorf("RunCLI() exit = %d; want 1\nstdout: %s", code, stdout)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("envelope ok = true; want false")
	}
	errMsg, _ := env["error"].(string)
	lower := strings.ToLower(errMsg)
	if !strings.Contains(lower, "gh") || !strings.Contains(lower, "path") {
		t.Errorf("error %q does not mention gh/PATH", errMsg)
	}
}

// TestRunCreate_GhNonZeroExit verifies that when gh exits with a non-zero code
// the envelope is ok:false with exit 1, and the error message surfaces the
// stderr text returned by the fake.
func TestRunCreate_GhNonZeroExit(t *testing.T) {
	const stderrMsg = "repository not found"
	installFakeGH(t, "", stderrMsg, 1, nil)

	code, stdout := runCLI(t, "create", "T")
	env := parseEnvelope(t, stdout)

	if code != 1 {
		t.Errorf("RunCLI() exit = %d; want 1\nstdout: %s", code, stdout)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("envelope ok = true; want false")
	}
	errMsg, _ := env["error"].(string)
	if !strings.Contains(errMsg, stderrMsg) {
		t.Errorf("error %q does not contain stderr text %q", errMsg, stderrMsg)
	}
}

// TestRunCreate_UnparseableURL verifies that when gh returns a URL whose
// trailing path segment is not a valid integer the envelope is ok:true with
// the url field present and the number field absent.
func TestRunCreate_UnparseableURL(t *testing.T) {
	const weirdURL = "https://github.com/Knatte18/loomyard/issues/abc"
	installFakeGH(t, weirdURL+"\n", "", 0, nil)

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
	// number must be absent when the URL segment cannot be parsed as an integer.
	if _, hasNum := env["number"]; hasNum {
		t.Errorf("envelope has number = %v but URL segment is not parseable; want number absent", env["number"])
	}
}

// TestRunCreate_NumberParsing verifies that a URL ending in /issues/123 produces
// number == 123 in the success envelope. JSON unmarshal into map[string]any
// decodes all numbers as float64, so the comparison is against float64(123).
func TestRunCreate_NumberParsing(t *testing.T) {
	const issueURL = "https://github.com/Knatte18/loomyard/issues/123"
	installFakeGH(t, issueURL+"\n", "", 0, nil)

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

// --- test helpers ---

// extractLabelValues scans args for "--label" flag pairs and returns all values.
func extractLabelValues(args []string) []string {
	var labels []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--label" {
			labels = append(labels, args[i+1])
		}
	}
	return labels
}

// extractFlagValue scans args for the named flag and returns a pointer to the
// following element (its value). Returns nil when the flag is not present.
func extractFlagValue(args []string, flag string) *string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			v := args[i+1]
			return &v
		}
	}
	return nil
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
