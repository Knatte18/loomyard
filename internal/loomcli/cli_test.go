// cli_test.go covers the loomcli cobra seam: the built tree's Short completeness, the bare-group
// invocation's git-free guard, and the drive/pause verbs' own refusal paths driven directly against a
// hand-populated receiver -- bypassing wire entirely, since neither refusal needs a wired hub.

package loomcli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/spf13/cobra"
)

// TestCommand_EveryCommandHasShort walks the full loom command tree and asserts that every command --
// the parent group and every subcommand -- carries a non-empty Short, per the CLI/Cobra Invariant.
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

// TestCommand_AllFourVerbsRegistered asserts every one of loom's four verbs is registered under the
// parent command.
func TestCommand_AllFourVerbsRegistered(t *testing.T) {
	parent := Command()
	want := map[string]bool{"run": false, "drive": false, "status": false, "pause": false}
	for _, sub := range parent.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("verb %q is not registered under the loom parent command", name)
		}
	}
}

// TestRunCLI_GroupGuard_NoGitRepoNeeded asserts that a bare "lyx loom" invocation succeeds without
// needing a git repository, proving the PersistentPreRunE guard for cmd.Name() == "loom" fires before
// any cwd resolution.
func TestRunCLI_GroupGuard_NoGitRepoNeeded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var out bytes.Buffer
	exitCode := RunCLIIn(dir, &out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLIIn(%q, nil) = %d; want 0", dir, exitCode)
	}
}

// TestRunCLI_UnknownSubcommand_NoGitRepoNeeded asserts an unknown subcommand also skips cwd
// resolution: cmd.Name() for the group's own RunE error path is still "loom".
func TestRunCLI_UnknownSubcommand_NoGitRepoNeeded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var out bytes.Buffer
	exitCode := RunCLIIn(dir, &out, []string{"bogus"})

	if exitCode != 1 {
		t.Errorf("RunCLIIn(%q, [bogus]) = %d; want 1", dir, exitCode)
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Errorf("RunCLIIn(%q, [bogus]) output missing ok:false envelope; got: %q", dir, out.String())
	}
}

// TestVerbRefusals covers the drive verb's seed-missing pre-flight and the pause verb's absent-file
// refusal. Both are driven directly against the leaf command built by driveCmd/pauseCmd on a
// hand-populated *loomCLI -- never through the full PersistentPreRunE/wire path, which needs a real
// git repository this untagged suite must not spawn. c.deps.StatusPath/StatusLockPath point at a
// plain temporary directory that never receives a status.json, so each verb's own refusal fires on
// exactly the precondition it owns.
func TestVerbRefusals(t *testing.T) {
	tests := []struct {
		name       string
		buildCmd   func(c *loomCLI) *cobra.Command
		wantRemedy string
	}{
		{
			name:       "Drive_SeedMissing",
			buildCmd:   (*loomCLI).driveCmd,
			wantRemedy: `lyx loom run`,
		},
		{
			name:       "Pause_AbsentFile",
			buildCmd:   (*loomCLI).pauseCmd,
			wantRemedy: `lyx loom run`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			c := &loomCLI{
				deps: loomshed.Deps{
					StatusPath:     filepath.Join(dir, "status.json"),
					StatusLockPath: filepath.Join(dir, "status.json.lock"),
				},
			}

			var out bytes.Buffer
			exitCode := clihelp.Execute(tt.buildCmd(c), &out, nil)

			if exitCode != 1 {
				t.Errorf("%s: exit code = %d; want 1", tt.name, exitCode)
			}
			if !strings.Contains(out.String(), `"ok":false`) {
				t.Errorf("%s: output missing ok:false envelope; got: %q", tt.name, out.String())
			}
			if !strings.Contains(out.String(), tt.wantRemedy) {
				t.Errorf("%s: output missing remedy %q; got: %q", tt.name, tt.wantRemedy, out.String())
			}
		})
	}
}
