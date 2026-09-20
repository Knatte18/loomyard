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

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestArmSeed_RunAndStepSeedBeforeWire asserts a first "lyx batten run <slug>" and a first
// "lyx batten step <slug>" each write a fresh seed when none exists -- the seeding arm performs
// ahead of wire building the Env, per the batch's own auto-seed-runs-inside-arm-ahead-of-wire
// decision. The seed carries recipe: "batten", never the Board task's own type.
func TestArmSeed_RunAndStepSeedBeforeWire(t *testing.T) {
	for _, verb := range []string{"run", "step"} {
		t.Run(verb, func(t *testing.T) {
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
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
			if seed.Params["slug"] != "some-slug" {
				t.Errorf("seed.Params[\"slug\"] = %q; want %q", seed.Params["slug"], "some-slug")
			}
		})
	}
}

// TestArmSeed_StatusAndPauseRefuseAndSeedNothing asserts "status" and "pause" against an unseeded
// run-id refuse with the listing rather than seeding -- the read-only-verbs-create-no-state rule,
// easy to lose in a refactor of arm.
func TestArmSeed_StatusAndPauseRefuseAndSeedNothing(t *testing.T) {
	for _, verb := range []string{"status", "pause"} {
		t.Run(verb, func(t *testing.T) {
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
			c := &battenCLI{}

			err := c.armSeed(loc, "some-slug", verb)
			if err == nil {
				t.Fatalf("armSeed(%q) = nil; want a refusal -- no seed exists", verb)
			}
			if !strings.Contains(err.Error(), `lyx batten run <slug>`) {
				t.Errorf("armSeed(%q) error = %q; want it to name \"lyx batten run <slug>\" as the remedy", verb, err.Error())
			}

			if _, found, readErr := shedrun.ReadSeed(loc, "some-slug"); readErr != nil || found {
				t.Errorf("ReadSeed after a %q refusal = (found=%v, err=%v); want (false, nil) -- nothing written", verb, found, readErr)
			}
			if entries, listErr := shedrun.List(loc); listErr != nil || len(entries) != 0 {
				t.Errorf("List after a %q refusal = (%v, %v); want (empty, nil) -- no run directory was created", verb, entries, listErr)
			}
		})
	}
}

// TestRefuseSelfAddress_ArgumentLessRefusesByNameBeforeTheGate asserts an omitted positional
// argument refuses by name, before the auto-seed gate ever runs, rather than silently taking the
// self default.
func TestRefuseSelfAddress_ArgumentLessRefusesByNameBeforeTheGate(t *testing.T) {
	runID, explicit := resolveBattenRunID(nil)
	if runID != shedrun.SelfRunID {
		t.Fatalf("resolveBattenRunID(nil) runID = %q; want %q", runID, shedrun.SelfRunID)
	}
	if explicit {
		t.Fatal("resolveBattenRunID(nil) explicit = true; want false")
	}

	err := refuseSelfAddress(runID, explicit)
	if err == nil {
		t.Fatal("refuseSelfAddress(self, explicit=false) = nil; want a refusal naming the missing slug")
	}
	if !strings.Contains(err.Error(), "no slug given") {
		t.Errorf("refuseSelfAddress error = %q; want it to name the missing slug", err.Error())
	}
}

// TestRefuseSelfAddress_IsReservedFiresForABoardSlugOfSelf asserts an explicitly-typed slug of
// "self" refuses via shedrun.IsReserved, worded as a reservation collision -- a different refusal
// than the argument-less case, even though both resolve to the identical run-id value.
func TestRefuseSelfAddress_IsReservedFiresForABoardSlugOfSelf(t *testing.T) {
	runID, explicit := resolveBattenRunID([]string{"self"})
	if runID != shedrun.SelfRunID {
		t.Fatalf("resolveBattenRunID([\"self\"]) runID = %q; want %q", runID, shedrun.SelfRunID)
	}
	if !explicit {
		t.Fatal("resolveBattenRunID([\"self\"]) explicit = false; want true")
	}

	err := refuseSelfAddress(runID, explicit)
	if err == nil {
		t.Fatal("refuseSelfAddress(self, explicit=true) = nil; want a refusal naming the reservation")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("refuseSelfAddress error = %q; want it to name the reservation", err.Error())
	}
}

// TestRefuseSelfAddress_OrdinarySlugNeverRefuses asserts an explicitly-typed ordinary slug never
// reaches either refusal.
func TestRefuseSelfAddress_OrdinarySlugNeverRefuses(t *testing.T) {
	runID, explicit := resolveBattenRunID([]string{"ordinary-slug"})
	if runID != "ordinary-slug" {
		t.Fatalf("resolveBattenRunID([\"ordinary-slug\"]) runID = %q; want %q", runID, "ordinary-slug")
	}
	if !explicit {
		t.Fatal("resolveBattenRunID([\"ordinary-slug\"]) explicit = false; want true")
	}
	if err := refuseSelfAddress(runID, explicit); err != nil {
		t.Errorf("refuseSelfAddress(%q, explicit=true) = %v; want nil", runID, err)
	}
}

// TestArmSeed_NoSeedGivesTheListingRefusal_SeedPresentWithNoStatusGivesFoundFalse asserts the two
// absences stay distinct: an addressed run-id with no seed at all gives armSeed's own listing
// refusal, while a run-id that IS seeded but has no status.json yet is a different, later question
// -- battenPreRun's own "found: false" disposition, which armSeed never touches.
func TestArmSeed_NoSeedGivesTheListingRefusal_SeedPresentWithNoStatusGivesFoundFalse(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
	c := &battenCLI{}

	// No seed at all: armSeed itself refuses with the listing.
	if err := c.armSeed(loc, "unseeded-slug", "status"); err == nil {
		t.Fatal("armSeed(\"status\") over an unseeded run-id = nil; want the listing refusal")
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
