// cli_test.go covers the loomcli cobra seam: the built tree's Short completeness, the exact set of
// registered verbs, the bare-group invocation's git-free guard, and the drive/pause verbs' own
// refusal paths driven directly against a hand-populated receiver -- bypassing wire entirely, since
// neither refusal needs a wired hub.

package loomcli

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
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

// TestCommand_RegisteredVerbs_ExactSet asserts that the parent command's registered subcommands are
// exactly loom's six verbs, no more and no fewer -- a genuine exact-set guard rather than a subset
// check, so a stray extra verb fails this test as surely as a missing one.
//
// cobra auto-adds a "help" command lazily, on Execute/help generation, not at AddCommand time, so
// Command().Commands() on a freshly built tree returns only the six explicitly registered verbs; a
// "completion" or "help" entry is filtered out below rather than pinned, so this guard stays
// resilient to a cobra upgrade that changes when those auto-commands appear.
func TestCommand_RegisteredVerbs_ExactSet(t *testing.T) {
	parent := Command()

	var got []string
	for _, sub := range parent.Commands() {
		name := sub.Name()
		if name == "help" || name == "completion" {
			continue
		}
		got = append(got, name)
	}
	sort.Strings(got)

	want := []string{"drive", "pause", "run", "status", "validate-discussion", "validate-plan"}

	gotSet := make(map[string]bool, len(got))
	for _, name := range got {
		gotSet[name] = true
	}
	wantSet := make(map[string]bool, len(want))
	for _, name := range want {
		wantSet[name] = true
	}

	for _, name := range want {
		if !gotSet[name] {
			t.Errorf("verb %q is not registered under the loom parent command", name)
		}
	}
	for _, name := range got {
		if !wantSet[name] {
			t.Errorf("unexpected verb %q is registered under the loom parent command", name)
		}
	}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("registered verbs = %v; want exactly %v", got, want)
	}
}

// TestRunAliasCommand_StaysOneCommandWithSubtreeVerb guards RunAliasCommand and the subtree's own
// runCmd against drifting into two different commands: the alias must carry a non-empty Short, its
// Use must be the bare verb ("run"), and it must expose the same --parent flag the subtree's own run
// verb does.
func TestRunAliasCommand_StaysOneCommandWithSubtreeVerb(t *testing.T) {
	alias := RunAliasCommand()

	if alias.Short == "" {
		t.Error("RunAliasCommand() has empty Short")
	}
	if alias.Use != "run" {
		t.Errorf("RunAliasCommand().Use = %q; want %q", alias.Use, "run")
	}
	if alias.Flags().Lookup("parent") == nil {
		t.Error("RunAliasCommand() is missing the --parent flag the subtree's run verb exposes")
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
// git repository this untagged suite must not spawn. c.shedPaths.StatusPath/StatusLockPath point at
// a plain temporary directory that never receives a status.json, so each verb's own refusal fires on
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
				shedPaths: loomrecipe.ShedPaths{
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
