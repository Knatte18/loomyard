// cli_test.go covers the perchcli cobra seam through RunCLI: bare-group
// listing, the unknown-subcommand JSON envelope, the PersistentPreRunE
// group-command guard, and the help-tree Short completeness check. run and
// pause verb behavior (missing --profile, missing --run-id, strict profile
// decode, pause-flag mechanics) is covered by run_test.go (card 15) and the
// pause-verb tests appended to this file (card 16). Engine.Run itself is NOT
// exercised here — it needs a live mux/claude session; that coverage lives
// in the smoke test and the sandbox suite. The fixture-backed pause tests
// (lyxtest's CopyPaired) live in cli_integration_test.go per the Test Tier
// Purity Invariant.

package perchcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRunCLI_NoArgs verifies that "lyx perch" with no subcommand lists the
// run and pause verbs and exits 0 — no git repo is needed, since the
// PersistentPreRunE guard skips layout/config/engine resolution for the
// group command itself.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}

	got := out.String()
	for _, want := range []string{"run", "pause"} {
		if !strings.Contains(got, want) {
			t.Errorf("RunCLI(nil) no-arg listing missing subcommand %q; got:\n%s", want, got)
		}
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1
// and emits a JSON error envelope with ok=false, without needing a git repo
// (the PersistentPreRunE guard for cmd.Name() == "perch" fires before
// layout resolution).
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"bogus"})

	if exitCode != 1 {
		t.Errorf("RunCLI(bogus) = %d; want 1", exitCode)
	}

	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("RunCLI(bogus) output missing ok:false envelope; got: %q", got)
	}
	if !strings.Contains(got, "unknown") {
		t.Errorf("RunCLI(bogus) output missing \"unknown\"; got: %q", got)
	}
}

// TestRunCLI_GroupGuard_OutsideGitRepo asserts the PersistentPreRunE guard:
// bare "lyx perch" works outside a git repository, mirroring burlercli's
// guard rationale (neither the bare listing nor the unknown-subcommand path
// should require layout/config resolution).
func TestRunCLI_GroupGuard_OutsideGitRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) outside a git repo = %d; want 0", exitCode)
	}
}

// TestCommand_EveryCommandHasShort walks the full perch command tree and
// asserts that every command — the parent group and every subcommand —
// carries a non-empty Short, per the CLI/Cobra Invariant.
func TestCommand_EveryCommandHasShort(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Short == "" {
			t.Errorf("command %q has empty Short", cmd.CommandPath())
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(Command())
}

// TestRunCLI_Pause_MissingRunID verifies that "lyx perch pause" without
// --run-id fails with pause's own manual flag-shape error (not cobra's
// MarkFlagRequired) before ever touching c.runDirBase. This case runs
// against an uninitialized (non-git) directory, so PersistentPreRunE's own
// abort error is also present in the captured output alongside the
// flag-specific error line — the same documented double-failure shape as
// run_test.go's TestRunCLI_Run_MissingProfile.
func TestRunCLI_Pause_MissingRunID(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"pause"})

	if exitCode != 1 {
		t.Errorf(`RunCLI([pause]) = %d; want 1`, exitCode)
	}
	if !strings.Contains(out.String(), "--run-id is required") {
		t.Errorf(`RunCLI([pause]) output missing "--run-id is required"; got: %q`, out.String())
	}
}
