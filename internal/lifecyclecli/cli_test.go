// cli_test.go covers the lifecyclecli cobra seam: the built tree's Short completeness, the exact set
// of registered verbs, each verb's argument-count rejection, the bare-group invocation's git-free
// guard, and the unknown-subcommand JSON error envelope -- mirroring internal/loomcli/cli_test.go's
// shape.

package lifecyclecli

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/spf13/cobra"
)

// lifecycleVerbCommand builds the single shedverbs command named verb ("run", "status", or
// "pause"), armed against c.specFor(verb) assigned onto c.spec -- the tier-1 seam arm.go's
// specFor/arm split exists for, letting this untagged suite fill a Spec with no resolution and no
// git spawn. It sets Args: cobra.ExactArgs(1), mirroring what Command() itself sets on every one
// of these three verbs.
func lifecycleVerbCommand(c *lifecycleCLI, verb string) *cobra.Command {
	spec := c.specFor(verb)
	c.spec = &spec
	for _, cmd := range shedverbs.Verbs(lifecycleVerbTexts, c.spec) {
		if cmd.Name() == verb {
			cmd.Args = cobra.ExactArgs(1)
			return cmd
		}
	}
	panic("lifecycleVerbCommand: no shedverbs command named " + verb)
}

// TestCommand_EveryCommandHasShort walks the full lifecycle command tree and asserts that every
// command -- the parent group and every subcommand -- carries a non-empty Short, per the CLI/Cobra
// Invariant.
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
// exactly the three lifecycle verbs, no more and no fewer -- "pause" included, per this task's
// agreed additive surface change, and "step" deliberately excluded, since lifecycle has no
// analogue for it and shedverbs.Verbs' returned step command is never added to this subtree.
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

	want := []string{"pause", "run", "status"}

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
			t.Errorf("verb %q is not registered under the lifecycle parent command", name)
		}
	}
	for _, name := range got {
		if !wantSet[name] {
			t.Errorf("unexpected verb %q is registered under the lifecycle parent command", name)
		}
	}
}

// TestCommand_EveryVerbRejectsWrongArgCount asserts that all three verbs reject zero arguments and
// two arguments -- each takes exactly one, the slug.
func TestCommand_EveryVerbRejectsWrongArgCount(t *testing.T) {
	for _, verb := range []string{"run", "status", "pause"} {
		for _, args := range [][]string{{verb}, {verb, "a", "b"}} {
			t.Run(verb+"_"+strings.Join(args, "_"), func(t *testing.T) {
				var out bytes.Buffer
				exitCode := clihelp.Execute(Command(), &out, args)
				if exitCode != 1 {
					t.Errorf("Execute(%v) = %d; want 1", args, exitCode)
				}
			})
		}
	}
}

// TestRunCLI_GroupGuard_NoGitRepoNeeded asserts that a bare "lyx lifecycle" invocation succeeds
// without needing a git repository, proving the PersistentPreRunE guard for cmd.Name() ==
// "lifecycle" fires before any cwd resolution.
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
// resolution and emits a JSON error envelope.
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
