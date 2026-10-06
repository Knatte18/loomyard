// cli_test.go covers the battencli cobra seam: each verb's argument-count rejection and the bare-group invocation's git-free guard -- mirroring internal/loomcli/cli_test.go's shape.

package battencli

import (
	"bytes"
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

// TestCommand_EveryVerbArity asserts each of the four verbs' own Args validator, checked independently per verb rather than by reading one shared value, accepts both zero and one positional and rejects two -- cobra.MaximumNArgs(1), the one arity contract all three surfaces ("lyx shed", "lyx batten", "lyx loom") share -- and that two positionals fail the whole command through cobra's own arity error, regardless of cwd, before PersistentPreRunE ever runs.
// Accepting zero is what lets "lyx batten run" with no argument reach arm's own refuseSelfAddress (arm_seed_test.go) with a named message, rather than stopping at cobra's generic count error.
func TestCommand_EveryVerbArity(t *testing.T) {
	t.Parallel()

	for _, verb := range []string{"run", "step", "status", "pause"} {
		t.Run(verb, func(t *testing.T) {
			t.Parallel()

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

			args := []string{verb, "a", "b"}
			var out bytes.Buffer
			if exitCode := clihelp.Execute(Command(), &out, args); exitCode != 1 {
				t.Errorf("Execute(%v) = %d; want 1", args, exitCode)
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
		wantSubstr      string
	}{
		{"BothGo", shedrun.DriverGo, shedrun.DriverGo, false, ""},
		{"ChildDriverLLMAccepted", shedrun.DriverGo, shedrun.DriverLLM, false, ""},
		// Batten has no bootstrap verb, so the driver it seeds itself with IS the process the operator typed and there is no spawn seam to branch on a seed's driver: the refusal names the missing bootstrap verb.
		{"OwnDriverLLMRefused", shedrun.DriverLLM, shedrun.DriverGo, true, "no bootstrap verb"},
		{"BothLLMRefusedByOwnDriver", shedrun.DriverLLM, shedrun.DriverLLM, true, "no bootstrap verb"},
		{"OwnDriverUnknownRefused", "rust", shedrun.DriverGo, true, ""},
		{"ChildDriverUnknownRefused", shedrun.DriverGo, "rust", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}
			c := &battenCLI{driverFlag: tt.driverFlag, childDriverFlag: tt.childDriverFlag}

			err := c.armSeed(loc, "some-slug", "run")
			if (err != nil) != tt.wantErr {
				t.Fatalf("armSeed(driver=%q, childDriver=%q) error = %v; want error = %v", tt.driverFlag, tt.childDriverFlag, err, tt.wantErr)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantSubstr) {
					t.Errorf("armSeed(driver=%q, childDriver=%q) error = %q; want it to contain %q", tt.driverFlag, tt.childDriverFlag, err.Error(), tt.wantSubstr)
				}
				if strings.Contains(err.Error(), "roadmap") {
					t.Errorf("armSeed(driver=%q, childDriver=%q) error = %q; want it to no longer name a roadmap item", tt.driverFlag, tt.childDriverFlag, err.Error())
				}
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
	// An empty, non-nil slice: a nil one makes cobra read os.Args, which carries the test binary's own flags.
	exitCode := RunCLIIn(dir, &out, []string{})

	if exitCode != 0 {
		t.Errorf("RunCLIIn(%q, no args) = %d; want 0", dir, exitCode)
	}
}

// TestCommand_HelpCarriesNoTeardownGotoExample asserts no help text shows the forward move to Worktree-Teardown as an example.
func TestCommand_HelpCarriesNoTeardownGotoExample(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if strings.Contains(cmd.Long, "--to Worktree-Teardown") {
			t.Errorf("%q help carries a --to Worktree-Teardown example", cmd.CommandPath())
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(Command())
}
