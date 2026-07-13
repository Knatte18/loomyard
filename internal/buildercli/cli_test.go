// cli_test.go covers the buildercli cobra seam through RunCLI: bare-group
// listing, the unknown-subcommand JSON envelope, the PersistentPreRunE
// group-command guard, and the help-tree Short completeness check --
// mirroring perchcli's own cli_test.go (internal/perchcli/cli_test.go).
// Individual verb behavior is covered by each verb's own _test.go file as
// later cards land it.

package buildercli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRunCLI_NoArgs verifies that "lyx builder" with no subcommand exits 0
// and lists whatever subcommands are currently registered -- no git repo
// is needed, since the PersistentPreRunE guard skips layout/config/engine
// resolution for the group command itself.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1
// and emits a JSON error envelope with ok=false, without needing a git repo
// (the PersistentPreRunE guard for cmd.Name() == "builder" fires before
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
// bare "lyx builder" works outside a git repository, mirroring perchcli's
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

// TestCommand_EveryCommandHasShort walks the full builder command tree and
// asserts that every command -- the parent group and every subcommand --
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
