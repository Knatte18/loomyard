// cli_test.go covers armFromSeed's seed-driven arming: the run-id positional defaulting to "self", the pinned refusal precedence (run-id listing, unknown recipe, verb gate, the recipe's own refusal), and run's positional-argument arity.
// It stays untagged Tier 1 throughout: armFromSeed performs no lyxcwd.Resolve of its own (cli.go's own doc comment),
// so every case here drives it directly against a hand-built *lyxcwd.Location, with no real git repository behind it --
// matching the repo's own convention that a real lyxcwd.Resolve spawn belongs only in an integration-tagged file (internal/lyxcwd/lyxcwd_test.go is the precedent).
package shedcli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

// fixtureLocation builds a synthetic *lyxcwd.Location by hand, mirroring the field derivation
// Resolve performs, without spawning git.
func fixtureLocation(t *testing.T) *lyxcwd.Location {
	t.Helper()
	return &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
}

// TestArmFromSeed_RunIDDefaultsToSelf asserts an omitted run-id positional (nil args) addresses
// shedrun.SelfRunID: seeding only "self" and calling with nil args succeeds, while calling with an
// explicit different run-id -- which has no seed -- refuses.
func TestArmFromSeed_RunIDDefaultsToSelf(t *testing.T) {
	loc := fixtureLocation(t)
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("WriteSeed(self) = %v; want nil", err)
	}

	if _, err := armFromSeed(loc, "status", nil); err != nil {
		t.Errorf("armFromSeed(status, nil) = %v; want nil -- nil args must default to %q, which is seeded", err, shedrun.SelfRunID)
	}

	if _, err := armFromSeed(loc, "status", []string{"other-run"}); err == nil {
		t.Error("armFromSeed(status, [other-run]) = nil; want a refusal -- \"other-run\" has no seed")
	}
}

// TestArmFromSeed_MissingRunRefusalCarriesNoKindField pins that a missing-run refusal, rendered
// through the same output.Err envelope resolvePersistentPreRun itself uses, carries no "kind" field
// -- the regression the Shed Verb-Set Invariant's five-value step vocabulary most invites, since
// armFromSeed's own doc comment states every error it returns is a plain error, never a
// fields-carrying envelope.
func TestArmFromSeed_MissingRunRefusalCarriesNoKindField(t *testing.T) {
	loc := fixtureLocation(t)

	_, err := armFromSeed(loc, "status", nil)
	if err == nil {
		t.Fatal("armFromSeed over an unseeded worktree = nil; want a refusal")
	}

	var buf bytes.Buffer
	output.Err(&buf, err.Error())

	var envelope map[string]any
	if unmarshalErr := json.Unmarshal(buf.Bytes(), &envelope); unmarshalErr != nil {
		t.Fatalf("json.Unmarshal(%q) = %v; want nil", buf.String(), unmarshalErr)
	}
	if _, present := envelope["kind"]; present {
		t.Errorf("missing-run refusal envelope = %v; must carry no \"kind\" field", envelope)
	}
	if len(envelope) != 2 {
		t.Errorf("missing-run refusal envelope = %v; want exactly the two keys \"ok\" and \"error\"", envelope)
	}
}

// TestArmFromSeed_RefusalPrecedence pins armFromSeed's refusals as a table, in the order they fire.
// Stage 1 (lyxcwd.Resolve's own not-a-git-repository sentinel) sits above armFromSeed entirely, inside
// resolvePersistentPreRun, which calls lyxcwd.Resolve unconditionally before ever calling
// armFromSeed -- first by construction, not by a value armFromSeed itself could reorder -- and is
// exercised by lyxcwd's own integration-tagged suite (internal/lyxcwd/lyxcwd_test.go).
// The rows pin stages 2 through 4 against the identical location and verb shape, so a regression
// reordering any of them shows up as the wrong row's assertion failing rather than a passing test
// for the wrong reason.
// Rows shrink or replace entries of the package-global recipes table, so neither the rows nor this test run in parallel.
func TestArmFromSeed_RefusalPrecedence(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, loc *lyxcwd.Location)
		verb  string
		args  []string
		// wants lists substrings the refusal must contain.
		wants []string
	}{
		{
			name: "stage2_run-id-listing",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				if err := shedrun.WriteSeed(loc, "alpha", shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
					t.Fatalf("WriteSeed(alpha) = %v; want nil", err)
				}
				if err := shedrun.WriteSeed(loc, "bravo", shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
					t.Fatalf("WriteSeed(bravo) = %v; want nil", err)
				}
			},
			verb:  "status",
			args:  []string{"charlie"},
			wants: []string{"no seed found", "alpha", "bravo", "charlie", "lyx shed seed charlie --recipe"},
		},
		{
			// shedrun.ReadSeed never validates Recipe against shedrun's own vocabulary (only Driver),
			// so a hand-edited or stale seed.json naming an unrecognised recipe is a real, reachable case.
			name: "stage2_seed-naming-unknown-recipe",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				if err := os.MkdirAll(shedrun.RunDir(loc, "some-run"), 0o755); err != nil {
					t.Fatalf("MkdirAll(RunDir) = %v; want nil", err)
				}
				if err := os.WriteFile(shedrun.SeedFile(loc, "some-run"), []byte(`{"recipe":"bogus-recipe","driver":"go"}`), 0o644); err != nil {
					t.Fatalf("write raw seed.json = %v; want nil", err)
				}
			},
			verb:  "status",
			args:  []string{"some-run"},
			wants: names(),
		},
		{
			// Both shipped recipes support every generic verb, so the row shrinks loom's Verbs to have an excluded verb to drive against.
			// The shrink never reaches Arm: the verb gate refuses before armFromSeed calls it.
			name: "stage3_verb-gate",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				original := recipes["loom"]
				recipes["loom"] = entry{Arm: original.Arm, Verbs: []string{"run", "status", "pause"}}
				t.Cleanup(func() { recipes["loom"] = original })
				if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
					t.Fatalf("WriteSeed = %v; want nil", err)
				}
			},
			verb:  "step",
			args:  nil,
			wants: []string{"does not support verb", "step", "loom", "run", "status", "pause"},
		},
		{
			name: "stage4_arms-own-refusal",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
					t.Fatalf("WriteSeed = %v; want nil", err)
				}
				// loomcli's own wire() (called from ArmAt for the "run" verb) loads loom.yaml
				// itself; an unparseable model-spec value fails that load without touching YAML
				// syntax, giving armFromSeed a real Arm-level refusal to reach -- one that stages 2
				// and 3 above have already let through -- with no git spawn anywhere in loomcli's
				// own wire(), which is what keeps this case Tier 1.
				cfgDir := configengine.ConfigDir(loc.AnchorPath())
				if err := os.MkdirAll(cfgDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", cfgDir, err)
				}
				cfgPath := configengine.ConfigFile(loc.AnchorPath(), "loom")
				if err := os.WriteFile(cfgPath, []byte("discussion: \"::not-a-valid-modelspec::\"\n"), 0o644); err != nil {
					t.Fatalf("write broken loom.yaml: %v", err)
				}
			},
			verb: "run",
			args: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := fixtureLocation(t)
			tt.setup(t, loc)

			_, err := armFromSeed(loc, tt.verb, tt.args)
			if err == nil {
				t.Fatalf("armFromSeed(%q, %v) = nil; want a refusal", tt.verb, tt.args)
			}
			for _, want := range tt.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("armFromSeed(%q, %v) error = %q; want it to contain %q", tt.verb, tt.args, err.Error(), want)
				}
			}
		})
	}
}

// TestShedVerbTexts_GotoAdmissionRuleOnlyOnGoto pins that the goto admission sentence sits on goto's help alone.
//
//testtiming:keep pins that the goto admission sentence sits on goto's help alone, which its covering test does not
func TestShedVerbTexts_GotoAdmissionRuleOnlyOnGoto(t *testing.T) {
	const rule = "goto moves a halted run only back"
	if !strings.Contains(shedVerbTexts.Goto.Long, rule) {
		t.Errorf("goto Long does not state the admission rule %q", rule)
	}
	for name, long := range map[string]string{"run": shedVerbTexts.Run.Long, "step": shedVerbTexts.Step.Long, "status": shedVerbTexts.Status.Long, "pause": shedVerbTexts.Pause.Long} {
		if strings.Contains(long, rule) {
			t.Errorf("%s Long carries goto's admission rule", name)
		}
	}
}

// findRunCommand returns cmd's own "run" subcommand, t.Fatal-ing if none is registered.
func findRunCommand(t *testing.T, parent *cobra.Command) *cobra.Command {
	t.Helper()
	for _, sub := range parent.Commands() {
		if sub.Name() == "run" {
			return sub
		}
	}
	t.Fatalf("%q has no \"run\" subcommand", parent.Name())
	return nil
}

// TestParity_PositionalArgs asserts positional-argument parity at the structural level: with the
// shared cobra.ExactArgs(1) value gone (batch 7's own MaximumNArgs(1) replaces it on both trees),
// each path's own "run" command independently carries cobra.MaximumNArgs(1) -- no positional
// argument is a legal parse (this batch's own no-slug default-to-"self"/omitted-slug behaviour lives
// past Args, in Arm/ArmAt, not at this layer), and two are refused as an arity error on both sides.
func TestParity_PositionalArgs(t *testing.T) {
	battenRun := findRunCommand(t, battencli.Command())
	shedRun := findRunCommand(t, Command())

	t.Run("ZeroArgs", func(t *testing.T) {
		if err := battenRun.Args(battenRun, nil); err != nil {
			t.Errorf("battencli run.Args(nil) = %v; want nil -- zero args is no longer a refusal", err)
		}
		if err := shedRun.Args(shedRun, nil); err != nil {
			t.Errorf("shed run.Args(nil) = %v; want nil -- zero args is no longer a refusal", err)
		}
	})

	t.Run("OneArg", func(t *testing.T) {
		args := []string{"some-slug"}
		if err := battenRun.Args(battenRun, args); err != nil {
			t.Errorf("battencli run.Args(%v) = %v; want nil", args, err)
		}
		if err := shedRun.Args(shedRun, args); err != nil {
			t.Errorf("shed run.Args(%v) = %v; want nil", args, err)
		}
	})

	t.Run("TwoSlugs", func(t *testing.T) {
		args := []string{"slug-one", "slug-two"}
		if err := battenRun.Args(battenRun, args); err == nil {
			t.Errorf("battencli run.Args(%v) = nil; want an arity refusal", args)
		}
		if err := shedRun.Args(shedRun, args); err == nil {
			t.Errorf("shed run.Args(%v) = nil; want an arity refusal", args)
		}
	})
}
