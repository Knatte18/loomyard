// cli_test.go covers the loomcli cobra seam: the built tree's Short completeness, the exact set of
// registered verbs, the bare-group invocation's git-free guard, and the run/pause/status verbs' own
// refusal paths driven directly against a hand-populated receiver -- bypassing wire entirely, since
// none of the three refusals needs a wired hub.

package loomcli

import (
	"bytes"
	"os"
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
// exactly loom's seven verbs, no more and no fewer -- a genuine exact-set guard rather than a subset
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

	want := []string{"pause", "run", "start", "status", "step", "validate-discussion", "validate-plan"}

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
}

// TestStartAliasCommand_StaysOneCommandWithSubtreeVerb guards StartAliasCommand and the subtree's own
// startCmd against drifting into two different commands: the alias must carry a non-empty Short, its
// Use must be the bare verb ("start"), it must expose the same --parent flag the subtree's own start
// verb does, and its Use must equal the subtree verb's own Use so the alias and the subtree verb can
// never drift apart.
func TestStartAliasCommand_StaysOneCommandWithSubtreeVerb(t *testing.T) {
	alias := StartAliasCommand()

	if alias.Short == "" {
		t.Error("StartAliasCommand() has empty Short")
	}
	if alias.Use != "start" {
		t.Errorf("StartAliasCommand().Use = %q; want %q", alias.Use, "start")
	}
	if alias.Flags().Lookup("parent") == nil {
		t.Error("StartAliasCommand() is missing the --parent flag the subtree's start verb exposes")
	}

	subtreeVerb := (&loomCLI{}).startCmd()
	if alias.Use != subtreeVerb.Use {
		t.Errorf("StartAliasCommand().Use = %q; want it to equal the subtree verb's own Use %q", alias.Use, subtreeVerb.Use)
	}
	if alias.Flags().Lookup("no-attach") == nil {
		t.Error("StartAliasCommand() is missing the --no-attach flag the subtree's start verb exposes")
	}
}

// TestCommand_StartVerb_RegistersNoAttachFlag asserts that the "start" verb registered under the
// "loom" parent command -- not just its bare-root alias -- exposes --no-attach, since it is the
// flag's primary home.
func TestCommand_StartVerb_RegistersNoAttachFlag(t *testing.T) {
	parent := Command()

	var start *cobra.Command
	for _, sub := range parent.Commands() {
		if sub.Name() == "start" {
			start = sub
		}
	}
	if start == nil {
		t.Fatal(`"start" verb is not registered under the loom parent command`)
	}
	if start.Flags().Lookup("no-attach") == nil {
		t.Error(`"loom start" is missing the --no-attach flag`)
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

// TestNewLoomCLI_SetsBothInjectedSeams pins newLoomCLI's own fields, since neither Command() nor
// StartAliasCommand() exposes the receiver each constructs and neither may grow an accessor purely
// for a test.
func TestNewLoomCLI_SetsBothInjectedSeams(t *testing.T) {
	c := newLoomCLI()

	if c.spawnWatchdog == nil {
		t.Error("newLoomCLI().spawnWatchdog = nil; want reedengine.SpawnWatchdog")
	}
	if c.suppressWatchdogSpawn != testing.Testing() {
		t.Errorf("newLoomCLI().suppressWatchdogSpawn = %v; want %v (testing.Testing())", c.suppressWatchdogSpawn, testing.Testing())
	}
}

// TestProductionFiles_LoomCLILiteralOnlyInFactory proves no production file in this package builds a
// *loomCLI composite literal outside newLoomCLI, by scanning this package's own non-_test.go *.go
// files for the "&loomCLI{" token and allowing it only in cli.go, newLoomCLI's own file -- the same
// source-scan shape internal/burlercli/wiring_test.go already uses
// (TestProductionFiles_NeverReferenceHubWatchdogMechanism) for a boundary property that has no other
// static form.
//
// Together with TestNewLoomCLI_SetsBothInjectedSeams above, this is what would have caught the alias
// gotcha newLoomCLI now designs out: that test proves the fields are set, this one proves both
// constructors go through the place that sets them.
func TestProductionFiles_LoomCLILiteralOnlyInFactory(t *testing.T) {
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}

	const token = "&loomCLI{"
	const factoryFile = "cli.go"

	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(data), token) && filepath.Base(path) != factoryFile {
			t.Errorf("%s references %q outside %s; every *loomCLI must be built through newLoomCLI", path, token, factoryFile)
		}
	}
}

// TestVerbRefusals covers the run verb's seed-missing pre-flight, the pause verb's absent-file
// refusal, and the status verb's !found refusal. All three are driven directly against the leaf
// command built by runCmd/pauseCmd/statusCmd on a hand-populated *loomCLI -- never through the full
// PersistentPreRunE/wire path, which needs a real git repository this untagged suite must not spawn.
// c.shedPaths.StatusPath/StatusLockPath point at a plain temporary directory that never receives a
// status.json, so each verb's own refusal fires on exactly the precondition it owns.
func TestVerbRefusals(t *testing.T) {
	tests := []struct {
		name       string
		buildCmd   func(c *loomCLI) *cobra.Command
		wantRemedy string
	}{
		{
			name:       "Run_SeedMissing",
			buildCmd:   (*loomCLI).runCmd,
			wantRemedy: `lyx loom start`,
		},
		{
			name:       "Pause_AbsentFile",
			buildCmd:   (*loomCLI).pauseCmd,
			wantRemedy: `lyx loom start`,
		},
		{
			name:       "Status_NotFound",
			buildCmd:   (*loomCLI).statusCmd,
			wantRemedy: `lyx loom start`,
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
