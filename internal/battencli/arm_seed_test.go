// arm_seed_test.go covers arm's run-id resolution, its self-address refusal, and its gated
// auto-seed: resolveBattenRunID, refuseSelfAddress and armSeed each drive against a hand-built
// *lyxcwd.Location, with no real git repository behind it (shedrun.ReadSeed, shedrun.WriteSeed and
// shedrun.List are all plain filesystem reads/writes under AnchorPath), so this file stays Tier 1.

package battencli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestArmSeed_RunAndStepSeedBeforeWire asserts a first "lyx batten run <slug>" and a first "lyx batten step <slug>" each write a fresh seed when none exists, whether or not LYX_STRAND_NAME is set -- the seeding arm performs ahead of wire building the Env, per the batch's own auto-seed-runs-inside-arm-ahead-of-wire decision.
// The seed carries recipe: "batten", never the Board task's own type.
func TestArmSeed_RunAndStepSeedBeforeWire(t *testing.T) {
	// No t.Parallel: each subtest sets the process environment through t.Setenv.
	for _, verb := range []string{"run", "step"} {
		for _, strandName := range []string{"ab:hub", ""} {
			t.Run(verb+"/strand="+strandName, func(t *testing.T) {
				t.Setenv(agentname.StrandNameEnv, strandName)
				loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}
				c := &battenCLI{}

				if _, found, err := shedrun.ReadSeed(loc, "some-slug"); err != nil || found {
					t.Fatalf("precondition: ReadSeed = (found=%v, err=%v); want (false, nil)", found, err)
				}

				if err := c.armSeed(loc, "some-slug", verb); err != nil {
					t.Fatalf("armSeed(%q) = %v; want nil", verb, err)
				}

				seed, found, err := shedrun.ReadSeed(loc, "some-slug")
				if err != nil || !found {
					t.Fatalf("ReadSeed after armSeed(%q) = (found=%v, err=%v); want (true, nil)", verb, found, err)
				}
				if seed.Recipe != shedrun.RecipeBatten {
					t.Errorf("seed.Recipe = %q; want %q -- prime's own seed, never the Board task's type", seed.Recipe, shedrun.RecipeBatten)
				}
				if _, present := seed.Params["slug"]; present {
					t.Errorf("seed.Params carries %q; the run-id is the slug and nothing reads a copy", "slug")
				}
				// Unset flags: batten's own driver defaults to go, since batten has no bootstrap verb,
				// while the loom child's defaults to llm.
				if seed.Driver != shedrun.DriverGo {
					t.Errorf("seed.Driver = %q; want the default %q", seed.Driver, shedrun.DriverGo)
				}
				if got := seed.Params["child_driver"]; got != shedrun.DriverLLM {
					t.Errorf("seed.Params[child_driver] = %q; want the default %q", got, shedrun.DriverLLM)
				}
			})
		}
	}
}

// TestArmSeed_OwnDriverLLMRefusesTheSameWayOnASeededRun asserts a typed --driver llm reads as the
// impossibility it is on an already-seeded run too, not as a mere disagreement with a recorded
// value.
//
// Before this, refuseAdoptedSeed ran first and answered "--driver \"llm\" cannot change a seeded
// run's recorded driver" -- true, but it reads as "not now", inviting the operator to delete the
// seed and re-seed with a value batten can never honour at all.
func TestArmSeed_OwnDriverLLMRefusesTheSameWayOnASeededRun(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}
	if err := shedrun.WriteSeed(loc, "some-slug", shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("shedrun.WriteSeed = %v; want nil", err)
	}

	c := &battenCLI{driverFlag: shedrun.DriverLLM, driverFlagSet: true}
	err := c.armSeed(loc, "some-slug", "run")
	if err == nil {
		t.Fatal("armSeed() with driverFlag=llm against a seeded run = nil; want a refusal")
	}
	if !strings.Contains(err.Error(), "no bootstrap verb") {
		t.Errorf("armSeed() error = %q; want it to name the missing bootstrap verb", err.Error())
	}
	if strings.Contains(err.Error(), "cannot change a seeded run") {
		t.Errorf("armSeed() error = %q; want the impossibility, not the seeded-value disagreement", err.Error())
	}
}

// TestArmSeed_TypedChildDriverIsValidatedAheadOfTheSeedRead asserts an unknown --child-driver value
// is named for what it is on a seeded run, rather than being reported as disagreeing with the
// recorded one.
func TestArmSeed_TypedChildDriverIsValidatedAheadOfTheSeedRead(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}
	if err := shedrun.WriteSeed(loc, "some-slug", shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("shedrun.WriteSeed = %v; want nil", err)
	}

	c := &battenCLI{childDriverFlag: "bogus", childDriverFlagSet: true}
	err := c.armSeed(loc, "some-slug", "run")
	if err == nil {
		t.Fatal("armSeed() with childDriverFlag=bogus against a seeded run = nil; want a refusal")
	}
	if !strings.Contains(err.Error(), `unknown driver "bogus"`) {
		t.Errorf("armSeed() error = %q; want it to name the unknown driver value", err.Error())
	}
}

// TestRefuseSelfAddress asserts an omitted positional argument refuses by name before the auto-seed gate ever runs rather than silently taking the self default, an explicitly typed slug of "self" refuses as a reservation collision -- a different refusal, though both resolve to the identical run-id value -- and an ordinary slug reaches neither.
func TestRefuseSelfAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		args         []string
		wantRunID    string
		wantExplicit bool
		wantSubstr   string
	}{
		{name: "ArgumentLessRefusesByName", args: nil, wantRunID: shedrun.SelfRunID, wantSubstr: "no slug given"},
		{name: "ExplicitSelfRefusesAsReserved", args: []string{"self"}, wantRunID: shedrun.SelfRunID, wantExplicit: true, wantSubstr: "reserved"},
		{name: "OrdinarySlugNeverRefuses", args: []string{"ordinary-slug"}, wantRunID: "ordinary-slug", wantExplicit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runID, explicit := resolveBattenRunID(tt.args)
			if runID != tt.wantRunID || explicit != tt.wantExplicit {
				t.Fatalf("resolveBattenRunID(%v) = (%q, %v); want (%q, %v)", tt.args, runID, explicit, tt.wantRunID, tt.wantExplicit)
			}

			err := refuseSelfAddress(runID, explicit)
			if tt.wantSubstr == "" {
				if err != nil {
					t.Errorf("refuseSelfAddress(%q, explicit=%v) = %v; want nil", runID, explicit, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("refuseSelfAddress(%q, explicit=%v) = nil; want a refusal naming %q", runID, explicit, tt.wantSubstr)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("refuseSelfAddress error = %q; want it to contain %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

// TestArmSeed_NoSeedGivesTheListingRefusal_SeedPresentWithNoStatusGivesFoundFalse asserts the two
// absences stay distinct: an addressed run-id with no seed at all gives armSeed's own listing
// refusal, while a run-id that IS seeded but has no status.json yet is a different, later question
// -- battenPreRun's own "found: false" disposition, which armSeed never touches.
func TestArmSeed_NoSeedGivesTheListingRefusal_SeedPresentWithNoStatusGivesFoundFalse(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}
	c := &battenCLI{}

	// No seed at all: armSeed itself refuses with the listing and seeds nothing -- the read-only-verbs-create-no-state rule, easy to lose in a refactor of arm.
	for _, verb := range []string{"status", "pause"} {
		err := c.armSeed(loc, "unseeded-slug", verb)
		if err == nil {
			t.Fatalf("armSeed(%q) over an unseeded run-id = nil; want the listing refusal", verb)
		}
		if !strings.Contains(err.Error(), `lyx batten run <slug>`) {
			t.Errorf("armSeed(%q) error = %q; want it to name \"lyx batten run <slug>\" as the remedy", verb, err.Error())
		}
		if _, found, readErr := shedrun.ReadSeed(loc, "unseeded-slug"); readErr != nil || found {
			t.Errorf("ReadSeed after a %q refusal = (found=%v, err=%v); want (false, nil) -- nothing written", verb, found, readErr)
		}
		if entries, listErr := shedrun.List(loc); listErr != nil || len(entries) != 0 {
			t.Errorf("List after a %q refusal = (%v, %v); want (empty, nil) -- no run directory was created", verb, entries, listErr)
		}
	}

	// Seed the run-id directly (bypassing armSeed, as a prior "lyx batten run" would have), leaving
	// no status.json beside it -- StatusFile lives under a different path than SeedFile, so writing
	// one never creates the other.
	if err := shedrun.WriteSeed(loc, "seeded-slug", shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("shedrun.WriteSeed = %v; want nil", err)
	}
	if err := c.armSeed(loc, "seeded-slug", "status"); err != nil {
		t.Fatalf("armSeed(\"status\") over a seeded run-id = %v; want nil -- the seed exists, so armSeed itself takes no action", err)
	}
	if _, err := os.Stat(shedrun.StatusFile(loc, "seeded-slug")); !os.IsNotExist(err) {
		t.Fatalf("status.json exists at %q after armSeed alone; want it absent -- that is battenPreRun's own seed-when-absent job, not armSeed's", filepath.Dir(shedrun.StatusFile(loc, "seeded-slug")))
	}
}

// TestRefuseAdoptedSeed covers the two ways an already-existing seed can disagree with the
// invocation that found it -- a foreign recipe, and a typed driver flag that cannot take effect --
// and asserts an agreeing seed is left alone.
func TestRefuseAdoptedSeed(t *testing.T) {
	battenSeedGoChild := shedrun.Seed{
		Recipe: shedrun.RecipeBatten,
		Driver: shedrun.DriverGo,
		Params: map[string]string{"child_driver": shedrun.DriverGo},
	}
	battenSeedLLMChild := shedrun.Seed{
		Recipe: shedrun.RecipeBatten,
		Driver: shedrun.DriverGo,
		Params: map[string]string{"child_driver": shedrun.DriverLLM},
	}
	battenSeedNoChildParam := shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}
	loomSeed := shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}

	tests := []struct {
		name            string
		seed            shedrun.Seed
		driverFlag      string
		driverSet       bool
		childDriverFlag string
		childDriverSet  bool
		wantSubstr      string
	}{
		{
			name: "agreeing_batten_seed_with_no_typed_flags_is_left_alone",
			seed: battenSeedGoChild,
		},
		{
			name:            "defaulted_flags_never_contradict_an_llm_child_run",
			seed:            battenSeedLLMChild,
			driverFlag:      shedrun.DriverGo,
			childDriverFlag: shedrun.DriverGo,
		},
		{
			name:       "a_loom_seed_is_refused_naming_the_verb_that_would_honour_it",
			seed:       loomSeed,
			wantSubstr: `already seeded with recipe "loom"`,
		},
		{
			name:       "a_typed_driver_that_cannot_take_effect_is_refused",
			seed:       battenSeedGoChild,
			driverFlag: shedrun.DriverLLM,
			driverSet:  true,
			wantSubstr: `--driver "llm" cannot change a seeded run's recorded driver`,
		},
		{
			name:            "a_typed_child_driver_that_cannot_take_effect_is_refused",
			seed:            battenSeedLLMChild,
			childDriverFlag: shedrun.DriverGo,
			childDriverSet:  true,
			wantSubstr:      `already seeded with child driver "llm"`,
		},
		{
			name:            "an_absent_child_driver_param_compares_as_go",
			seed:            battenSeedNoChildParam,
			childDriverFlag: shedrun.DriverLLM,
			childDriverSet:  true,
			wantSubstr:      `already seeded with child driver "go"`,
		},
		{
			name:            "a_typed_child_driver_that_agrees_is_accepted",
			seed:            battenSeedLLMChild,
			childDriverFlag: shedrun.DriverLLM,
			childDriverSet:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := refuseAdoptedSeed(tt.seed, "some-slug", tt.driverFlag, tt.driverSet, tt.childDriverFlag, tt.childDriverSet)
			if tt.wantSubstr == "" {
				if err != nil {
					t.Fatalf("refuseAdoptedSeed(...) = %v; want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("refuseAdoptedSeed(...) = nil; want a refusal containing %q", tt.wantSubstr)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("refuseAdoptedSeed(...) = %q; want it to contain %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}
