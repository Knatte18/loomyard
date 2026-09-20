// cli_test.go covers the battencli cobra seam: the built tree's Short completeness, the exact set
// of registered verbs, each verb's argument-count rejection, the bare-group invocation's git-free
// guard, and the unknown-subcommand JSON error envelope -- mirroring internal/loomcli/cli_test.go's
// shape.

package battencli

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/spf13/cobra"
)

// battenVerbCommand builds the single shedverbs command named verb ("run", "step", "status", or
// "pause"), armed against c.specFor(verb) assigned onto c.spec -- the tier-1 seam arm.go's
// specFor/arm split exists for, letting this untagged suite fill a Spec with no resolution and no
// git spawn. It sets Args: cobra.ExactArgs(1), a stricter arity than Command() itself sets on these
// four verbs (cobra.MaximumNArgs(1)) but a harmless one for these tests: every caller in this file
// supplies exactly one positional.
func battenVerbCommand(c *battenCLI, verb string) *cobra.Command {
	spec := c.specFor(verb)
	c.spec = &spec
	for _, cmd := range shedverbs.Verbs(battenVerbTexts, c.spec) {
		if cmd.Name() == verb {
			cmd.Args = cobra.ExactArgs(1)
			return cmd
		}
	}
	panic("battenVerbCommand: no shedverbs command named " + verb)
}

// battenCommandNamed returns the subcommand named verb from the built batten tree, or nil.
func battenCommandNamed(parent *cobra.Command, verb string) *cobra.Command {
	for _, sub := range parent.Commands() {
		if sub.Name() == verb {
			return sub
		}
	}
	return nil
}

// TestCommand_EveryCommandHasShort walks the full batten command tree and asserts that every
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
// exactly the four batten verbs, no more and no fewer -- "pause" and "step" both included, per this
// task's agreed additive surface change: batten now registers all four of shedverbs.Verbs' returned
// commands, "step" included, rather than the three-verb subtree an earlier card in this task shipped.
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

	want := []string{"pause", "run", "status", "step"}

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
			t.Errorf("verb %q is not registered under the batten parent command", name)
		}
	}
	for _, name := range got {
		if !wantSet[name] {
			t.Errorf("unexpected verb %q is registered under the batten parent command", name)
		}
	}
}

// TestCommand_EveryVerbRejectsTwoArgs asserts that all four verbs reject two positional arguments
// via cobra's own arity error, regardless of cwd -- unlike a zero-argument or absent-slug case,
// which now reaches arm's own refusal logic (see TestCommand_ZeroOrOneArgArePermittedByCobrasOwnArity
// below), a two-argument invocation is rejected by cobra itself before PersistentPreRunE ever runs.
func TestCommand_EveryVerbRejectsTwoArgs(t *testing.T) {
	for _, verb := range []string{"run", "step", "status", "pause"} {
		t.Run(verb, func(t *testing.T) {
			args := []string{verb, "a", "b"}
			var out bytes.Buffer
			exitCode := clihelp.Execute(Command(), &out, args)
			if exitCode != 1 {
				t.Errorf("Execute(%v) = %d; want 1", args, exitCode)
			}
		})
	}
}

// TestCommand_ZeroOrOneArgArePermittedByCobrasOwnArity asserts each of the four verbs' own Args
// validator, checked independently per verb rather than by reading one shared value, accepts both
// zero and one positional and rejects two -- cobra.MaximumNArgs(1), the one arity contract all
// three surfaces ("lyx shed", "lyx batten", "lyx loom") now share. Accepting zero here is what lets
// "lyx batten run" with no argument reach arm's own refuseSelfAddress (arm_seed_test.go) with a
// named message, rather than stopping at cobra's own generic count error.
func TestCommand_ZeroOrOneArgArePermittedByCobrasOwnArity(t *testing.T) {
	for _, verb := range []string{"run", "step", "status", "pause"} {
		t.Run(verb, func(t *testing.T) {
			cmd := battenCommandNamed(Command(), verb)
			if cmd == nil {
				t.Fatalf("verb %q not found under the batten parent command", verb)
			}
			if cmd.Args == nil {
				t.Fatalf("verb %q has a nil Args validator", verb)
			}

			if err := cmd.Args(cmd, nil); err != nil {
				t.Errorf("%s: Args(zero) = %v; want nil", verb, err)
			}
			if err := cmd.Args(cmd, []string{"one-run-id"}); err != nil {
				t.Errorf("%s: Args(one) = %v; want nil", verb, err)
			}
			if err := cmd.Args(cmd, []string{"one", "two"}); err == nil {
				t.Errorf("%s: Args(two) = nil; want a rejection -- MaximumNArgs(1)", verb)
			}
		})
	}
}

// TestArmSeed_DriverFlagMatrix pins the four-way matrix between --driver and --child-driver: batten's
// own driver flag still accepts "go" and still refuses both "llm" and an unknown value, while
// --child-driver accepts both "go" and "llm" -- the value it accepts reaches the auto-seeded seed's
// own "child_driver" param, which wire.go's own ChildDriver reader consults -- and still refuses an
// unknown value. The likeliest regression this batch names is lifting both refusals for symmetry;
// only a case asserting the batten driver flag still refuses catches it.
func TestArmSeed_DriverFlagMatrix(t *testing.T) {
	tests := []struct {
		name            string
		driverFlag      string
		childDriverFlag string
		wantErr         bool
	}{
		{"BothGo", shedrun.DriverGo, shedrun.DriverGo, false},
		{"ChildDriverLLMAccepted", shedrun.DriverGo, shedrun.DriverLLM, false},
		{"OwnDriverLLMRefused", shedrun.DriverLLM, shedrun.DriverGo, true},
		{"BothLLMRefusedByOwnDriver", shedrun.DriverLLM, shedrun.DriverLLM, true},
		{"OwnDriverUnknownRefused", "rust", shedrun.DriverGo, true},
		{"ChildDriverUnknownRefused", shedrun.DriverGo, "rust", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
			c := &battenCLI{driverFlag: tt.driverFlag, childDriverFlag: tt.childDriverFlag}

			err := c.armSeed(loc, "some-slug", "run")
			if (err != nil) != tt.wantErr {
				t.Fatalf("armSeed(driver=%q, childDriver=%q) error = %v; want error = %v", tt.driverFlag, tt.childDriverFlag, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			seed, found, readErr := shedrun.ReadSeed(loc, "some-slug")
			if readErr != nil || !found {
				t.Fatalf("ReadSeed after armSeed = (found=%v, err=%v); want (true, nil)", found, readErr)
			}
			if seed.Params["child_driver"] != tt.childDriverFlag {
				t.Errorf("seed.Params[\"child_driver\"] = %q; want %q", seed.Params["child_driver"], tt.childDriverFlag)
			}
		})
	}
}

// TestRunCLI_GroupGuard_NoGitRepoNeeded asserts that a bare "lyx batten" invocation succeeds
// without needing a git repository, proving the PersistentPreRunE guard for cmd.Name() ==
// "batten" fires before any cwd resolution.
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
