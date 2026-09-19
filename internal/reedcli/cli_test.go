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

// TestRunCLI_NoArgs verifies that "lyx reed" with no subcommand lists all nine registered verbs
// and exits 0.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}

	got := out.String()
	wantSubs := []string{"up", "down", "add", "remove", "status", "resume", "attach", "statusline", "watchdog"}
	for _, sub := range wantSubs {
		if !strings.Contains(got, sub) {
			t.Errorf("RunCLI(nil) no-arg listing missing subcommand %q; got:\n%s", sub, got)
		}
	}
}

// TestCommand_EveryVerbHasNonEmptyShort asserts every registered reed subcommand — statusline and
// watchdog included — carries a non-empty Short, per the CLI/Cobra Invariant.
func TestCommand_EveryVerbHasNonEmptyShort(t *testing.T) {
	t.Parallel()

	parent := Command()
	for _, sub := range parent.Commands() {
		if sub.Short == "" {
			t.Errorf("subcommand %q has an empty Short; want a non-empty short description", sub.Name())
		}
	}
}

// TestRunCLI_Watchdog_SkipsLocationResolution verifies that "watchdog" takes the
// PersistentPreRunE's early return: invoked from a directory that is not a git repository, it must
// not fail with lyxcwd.Resolve's not-a-git-repository error — the verb must run with no git
// repository present at all. Its own RunE then reports the missing --hub-path/--tmux flags on the
// envelope instead, which is the observable proof that PersistentPreRunE returned nil (skipping
// c.eng population) rather than aborting into the git-repo error.
func TestRunCLI_Watchdog_SkipsLocationResolution(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"watchdog"})

	if exitCode == 0 {
		t.Fatalf("RunCLI(watchdog) with no --hub-path = 0; want non-zero (missing required flag)")
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(watchdog) output is not valid JSON: %v; got: %q", err, out.String())
	}
	errMsg, _ := env["error"].(string)
	if errMsg == "not a git repository" {
		t.Errorf("RunCLI(watchdog) error = %q; want the --hub-path validation error, not lyxcwd.Resolve's not-a-git-repository error", errMsg)
	}
	if !strings.Contains(errMsg, "--hub-path") {
		t.Errorf("RunCLI(watchdog) error = %q; want it to name --hub-path", errMsg)
	}
}

// TestRunCLI_Watchdog_RefusesAbsentAndRelativeHubPath verifies that watchdog refuses both an
// absent and a relative --hub-path on the envelope, before it ever blocks.
func TestRunCLI_Watchdog_RefusesAbsentAndRelativeHubPath(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "absent", args: []string{"watchdog", "--tmux", "/usr/bin/tmux"}},
		{name: "relative", args: []string{"watchdog", "--hub-path", "relative/path", "--tmux", "/usr/bin/tmux"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := RunCLI(&out, tt.args)

			if exitCode == 0 {
				t.Fatalf("RunCLI(%v) = 0; want non-zero", tt.args)
			}
			var env map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
				t.Fatalf("RunCLI(%v) output is not valid JSON: %v; got: %q", tt.args, err, out.String())
			}
			errMsg, _ := env["error"].(string)
			if !strings.Contains(errMsg, "--hub-path") {
				t.Errorf("RunCLI(%v) error = %q; want it to name --hub-path", tt.args, errMsg)
			}
		})
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
