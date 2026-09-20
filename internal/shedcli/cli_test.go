// cli_test.go covers Command's bare-listing and unknown-subcommand behaviour, plus armFromSeed's
// seed-driven arming: the run-id positional defaulting to "self", an absent seed's run-id listing
// refusal, an unknown-recipe refusal, the verb gate, and the pinned refusal precedence. It stays
// untagged Tier 1 throughout: armFromSeed performs no lyxcwd.Resolve of its own (cli.go's own doc
// comment), so every case here drives it directly against a hand-built *lyxcwd.Location, with no
// real git repository behind it -- matching the repo's own convention that a real lyxcwd.Resolve
// spawn belongs only in an integration-tagged file (internal/lyxcwd/lyxcwd_test.go is the
// precedent).
//
// The two bare-listing/unknown-subcommand cases below do call RunCLIIn(t.TempDir(), …), because
// RunCLI delegates to RunCLIIn("", …) and therefore reads the process cwd -- the package source
// directory, which is inside this repo's worktree -- so an injected non-git cwd is reachable only
// through the cwd-carrying seam. What keeps those two cases Tier 1 is that each one short-circuits
// at cmd.Name() == "shed" inside resolvePersistentPreRun, before lyxcwd.Resolve is ever called.
package shedcli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestRunCLIIn_BareListingNeedsNoGitRepository asserts a bare "lyx shed" lists the four
// subcommands and succeeds against a cwd that is not a git worktree at all.
func TestRunCLIIn_BareListingNeedsNoGitRepository(t *testing.T) {
	var out bytes.Buffer
	exitCode := RunCLIIn(t.TempDir(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("RunCLIIn(nil) exit code = %d; want 0; output: %s", exitCode, out.String())
	}
	for _, sub := range []string{"run", "step", "status", "pause", "seed"} {
		if !strings.Contains(out.String(), sub) {
			t.Errorf("bare shed listing missing subcommand %q; got:\n%s", sub, out.String())
		}
	}
}

// TestRunCLIIn_UnknownSubcommandEmitsJSONEnvelope asserts "lyx shed bogus" emits a JSON error
// envelope rather than cobra's own plain-text help.
func TestRunCLIIn_UnknownSubcommandEmitsJSONEnvelope(t *testing.T) {
	var out bytes.Buffer
	exitCode := RunCLIIn(t.TempDir(), &out, []string{"bogus"})

	if exitCode != 1 {
		t.Fatalf("RunCLIIn([bogus]) exit code = %d; want 1; output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Errorf("RunCLIIn([bogus]) output missing ok:false envelope; got: %q", out.String())
	}
}

// fixtureLocation builds a synthetic *lyxcwd.Location by hand, mirroring the field derivation
// Resolve performs, without spawning git.
func fixtureLocation(t *testing.T) *lyxcwd.Location {
	t.Helper()
	return &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
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

// TestArmFromSeed_AbsentSeedRefusesWithListing asserts an addressed run-id with no seed refuses,
// naming every existing seeded run-id.
func TestArmFromSeed_AbsentSeedRefusesWithListing(t *testing.T) {
	loc := fixtureLocation(t)
	if err := shedrun.WriteSeed(loc, "alpha", shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("WriteSeed(alpha) = %v; want nil", err)
	}
	if err := shedrun.WriteSeed(loc, "bravo", shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("WriteSeed(bravo) = %v; want nil", err)
	}

	_, err := armFromSeed(loc, "status", []string{"charlie"})
	if err == nil {
		t.Fatal("armFromSeed(status, [charlie]) = nil; want a refusal -- \"charlie\" has no seed")
	}
	for _, want := range []string{"alpha", "bravo", "charlie"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("armFromSeed error = %q; want it to name %q", err.Error(), want)
		}
	}
	if !strings.Contains(err.Error(), `lyx shed seed charlie --recipe`) {
		t.Errorf("armFromSeed error = %q; want it to name the \"lyx shed seed\" remedy", err.Error())
	}
}

// TestArmFromSeed_SeedNamingUnknownRecipeRefusesWithAvailableNames asserts a seed whose recipe
// field names something outside this table refuses via lookup's own unknown-recipe error, naming
// the available recipes. shedrun.ReadSeed never validates Recipe against shedrun's own vocabulary
// (only Driver), so a hand-edited or stale seed.json naming an unrecognised recipe is a real,
// reachable case here, not merely hypothetical.
func TestArmFromSeed_SeedNamingUnknownRecipeRefusesWithAvailableNames(t *testing.T) {
	loc := fixtureLocation(t)
	if err := os.MkdirAll(shedrun.RunDir(loc, "some-run"), 0o755); err != nil {
		t.Fatalf("MkdirAll(RunDir) = %v; want nil", err)
	}
	if err := os.WriteFile(shedrun.SeedFile(loc, "some-run"), []byte(`{"recipe":"bogus-recipe","driver":"go"}`), 0o644); err != nil {
		t.Fatalf("write raw seed.json = %v; want nil", err)
	}

	_, err := armFromSeed(loc, "status", []string{"some-run"})
	if err == nil {
		t.Fatal("armFromSeed over a seed naming an unknown recipe = nil; want a refusal")
	}
	for _, name := range names() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("armFromSeed error = %q; want it to name available recipe %q", err.Error(), name)
		}
	}
}

// TestArmFromSeed_VerbGateFiresForAnExcludedVerb asserts the verb gate refuses a verb a recipe's
// table entry excludes, naming the verb, the recipe, and the verbs that recipe does support. It
// temporarily shrinks recipes["batten"]'s own Verbs set for the duration of the test, restoring the
// original afterward, since both shipped recipes now support all four generic verbs and there is no
// standing example of an excluded verb/recipe pair to drive against. This shrink never reaches
// battencli.ArmAt: the verb gate refuses before armFromSeed ever calls Arm, which is what lets this
// case stay Tier 1 despite battencli.ArmAt's own fabricengine.PrimeName call spawning git.
func TestArmFromSeed_VerbGateFiresForAnExcludedVerb(t *testing.T) {
	original := recipes["batten"]
	recipes["batten"] = entry{Arm: original.Arm, Verbs: []string{"run", "status", "pause"}}
	t.Cleanup(func() { recipes["batten"] = original })

	loc := fixtureLocation(t)
	if err := shedrun.WriteSeed(loc, "some-slug", shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("WriteSeed = %v; want nil", err)
	}

	_, err := armFromSeed(loc, "step", []string{"some-slug"})
	if err == nil {
		t.Fatal("armFromSeed(step) over a shrunk batten entry = nil; want a refusal")
	}
	for _, want := range []string{"step", "batten", "run", "status", "pause"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("verb-gate refusal = %q; want it to name %q", err.Error(), want)
		}
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

// TestArmFromSeed_RefusalPrecedence pins this batch's own refusal precedence as a table. Stage 1
// (lyxcwd.Resolve's own not-a-git-repository sentinel) sits above armFromSeed entirely, inside
// resolvePersistentPreRun, which calls lyxcwd.Resolve unconditionally before ever calling
// armFromSeed -- first by construction, not by a value armFromSeed itself could reorder -- and is
// exercised by lyxcwd's own integration-tagged suite (internal/lyxcwd/lyxcwd_test.go). The three
// rows below pin stages 2 through 4 against the identical location and verb shape, so a regression
// reordering any of them shows up as the wrong row's assertion failing rather than a passing test
// for the wrong reason.
func TestArmFromSeed_RefusalPrecedence(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, loc *lyxcwd.Location)
		verb  string
		args  []string
		want  string
	}{
		{
			name: "stage2_run-id-listing",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				if err := shedrun.WriteSeed(loc, "alpha", shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
					t.Fatalf("WriteSeed = %v; want nil", err)
				}
			},
			verb: "status",
			args: []string{"unaddressed-run"},
			want: "no seed found",
		},
		{
			name: "stage3_verb-gate",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				original := recipes["loom"]
				recipes["loom"] = entry{Arm: original.Arm, Verbs: []string{"run", "status", "pause"}}
				t.Cleanup(func() { recipes["loom"] = original })
				if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
					t.Fatalf("WriteSeed = %v; want nil", err)
				}
			},
			verb: "step",
			args: nil,
			want: "does not support verb",
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
			want: "",
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
			if tt.want != "" && !strings.Contains(err.Error(), tt.want) {
				t.Errorf("armFromSeed(%q, %v) error = %q; want it to contain %q", tt.verb, tt.args, err.Error(), tt.want)
			}
		})
	}
}
