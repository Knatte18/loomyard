// cli_test.go covers the reedcli cobra seam through RunCLI: bare-group listing, the
// unknown-subcommand JSON envelope, and the not-a-git-repo error surface.
// No live tmux session is required by any test in this file;
// the real up/add/status/down round-trip lives in smoke_test.go behind //go:build smoke.
// Config resolution against a real fixture hub now lives in cli_integration_test.go per the Test
// Tier Purity Invariant.

package reedcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestRunCLI_NoArgs verifies that "lyx reed" with no subcommand lists all seven registered verbs
// and exits 0.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}

	got := out.String()
	wantSubs := []string{"up", "down", "add", "remove", "status", "resume", "attach"}
	for _, sub := range wantSubs {
		if !strings.Contains(got, sub) {
			t.Errorf("RunCLI(nil) no-arg listing missing subcommand %q; got:\n%s", sub, got)
		}
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1 and emits a JSON error
// envelope.
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"bogus"})

	if exitCode != 1 {
		t.Errorf("RunCLI(bogus) = %d; want 1", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(bogus) output is not valid JSON: %v; got: %q", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(bogus) ok = true; want false")
	}
	if errMsg, _ := env["error"].(string); !strings.Contains(errMsg, "unknown") {
		t.Errorf("RunCLI(bogus) error = %q; want \"unknown\" substring", errMsg)
	}
}

// TestRunCLI_NotAGitRepo verifies that a real verb invoked from a non-git directory surfaces the
// ErrNotAGitRepo error.
func TestRunCLI_NotAGitRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})

	if exitCode != 1 {
		t.Errorf("RunCLI(status) in non-git dir = %d; want 1", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(status) output is not valid JSON: %v; got: %q", err, out.String())
	}
	if errMsg, _ := env["error"].(string); errMsg != "not a git repository" {
		t.Errorf("RunCLI(status) error = %q; want exactly \"not a git repository\"", errMsg)
	}
}
