// arm_seed_test.go covers arm's seed-presence refusal: resolveRunID drives the whole check against
// a hand-built *lyxcwd.Location, with no real git repository behind it (shedrun.ReadSeed and
// shedrun.List are both plain filesystem reads under AnchorPath), so this file stays Tier 1.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// seedRunID seeds a run-id under loc with shedrun.RecipeLoom/shedrun.DriverGo, t.Fatal-ing on
// failure.
func seedRunID(t *testing.T, loc *lyxcwd.Location, runID string) {
	t.Helper()
	if err := shedrun.WriteSeed(loc, runID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("shedrun.WriteSeed(%q) = %v; want nil", runID, err)
	}
}

// TestResolveRunID_RefusesEachGenericVerbWhenNoSeed asserts each of the four generic verbs --
// run, step, status, pause -- refuses with a non-nil error when no seed exists at the addressed
// run-id (shedrun.SelfRunID, since no args are given).
func TestResolveRunID_RefusesEachGenericVerbWhenNoSeed(t *testing.T) {
	for _, verb := range []string{"run", "step", "status", "pause"} {
		t.Run(verb, func(t *testing.T) {
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
			c := &loomCLI{}

			err := c.resolveRunID(loc, verb, nil)
			if err == nil {
				t.Fatalf("resolveRunID(%q) = nil; want a refusal (no seed exists)", verb)
			}
			if !strings.Contains(err.Error(), shedrun.SelfRunID) {
				t.Errorf("resolveRunID(%q) error = %q; want it to name the addressed run-id %q", verb, err.Error(), shedrun.SelfRunID)
			}
		})
	}
}

// TestResolveRunID_NonGenericVerbsNeverRefuse asserts "start", "validate-discussion", and
// "validate-plan" -- none of them a generic verb -- never reach the seed-presence refusal, even
// when no seed exists.
func TestResolveRunID_NonGenericVerbsNeverRefuse(t *testing.T) {
	for _, verb := range []string{"start", "validate-discussion", "validate-plan"} {
		t.Run(verb, func(t *testing.T) {
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
			c := &loomCLI{}

			if err := c.resolveRunID(loc, verb, nil); err != nil {
				t.Errorf("resolveRunID(%q) = %v; want nil -- %q never reaches the seed-presence refusal", verb, err, verb)
			}
		})
	}
}

// TestResolveRunID_RefusalNamesEveryExistingRunID asserts the refusal's text lists every run-id
// shedrun.List finds, sorted, when the addressed run-id itself has no seed.
func TestResolveRunID_RefusalNamesEveryExistingRunID(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
	seedRunID(t, loc, "alpha")
	seedRunID(t, loc, "bravo")
	c := &loomCLI{}

	err := c.resolveRunID(loc, "status", []string{"charlie"})
	if err == nil {
		t.Fatal("resolveRunID(\"status\", [\"charlie\"]) = nil; want a refusal -- \"charlie\" has no seed")
	}
	for _, want := range []string{"alpha", "bravo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("resolveRunID error = %q; want it to list the existing run-id %q", err.Error(), want)
		}
	}
	if !strings.Contains(err.Error(), "lyx loom start") {
		t.Errorf("resolveRunID error = %q; want it to name \"lyx loom start\" as the remedy", err.Error())
	}
}

// TestResolveRunID_EmptyListingReadsAsOrdinary asserts the refusal's text, over a worktree with no
// seeded run at all, reads as the ordinary "no run is seeded yet" case rather than as a fault --
// the pre-existing-worktree case shedrun.MissingSeedMessage documents.
func TestResolveRunID_EmptyListingReadsAsOrdinary(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
	c := &loomCLI{}

	err := c.resolveRunID(loc, "run", nil)
	if err == nil {
		t.Fatal("resolveRunID(\"run\", nil) = nil; want a refusal")
	}
	if !strings.Contains(err.Error(), "no run is seeded yet") {
		t.Errorf("resolveRunID error = %q; want it to read as the ordinary empty-listing case", err.Error())
	}
}

// TestResolveRunID_StatusFilePresentWithNoSeedTakesTheSameRefusal asserts a status file already
// present at the addressed run-id, with no seed beside it, still refuses -- the inconsistency the
// refusal exists to catch (e.g. a worktree from before this task).
func TestResolveRunID_StatusFilePresentWithNoSeedTakesTheSameRefusal(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
	statusPath := shedrun.StatusFile(loc, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(statusPath), err)
	}
	if err := os.WriteFile(statusPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("seed a bare status.json with no seed beside it: %v", err)
	}
	c := &loomCLI{}

	err := c.resolveRunID(loc, "pause", nil)
	if err == nil {
		t.Fatal("resolveRunID(\"pause\", nil) = nil; want a refusal -- a status file exists with no seed beside it")
	}
}

// TestResolveRunID_RunAndStepWriteNothingToDiskWhenTheyRefuse asserts the asymmetry that matters:
// when "run" or "step" refuses for want of a seed, the addressed run-id's whole run directory is
// never created -- resolveRunID performs reads alone (shedrun.ReadSeed, shedrun.List), and arm
// never reaches wireLightweight/wire on this path.
func TestResolveRunID_RunAndStepWriteNothingToDiskWhenTheyRefuse(t *testing.T) {
	for _, verb := range []string{"run", "step"} {
		t.Run(verb, func(t *testing.T) {
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
			c := &loomCLI{}

			if err := c.resolveRunID(loc, verb, nil); err == nil {
				t.Fatalf("resolveRunID(%q) = nil; want a refusal", verb)
			}

			if _, found, err := shedrun.ReadSeed(loc, shedrun.SelfRunID); err != nil || found {
				t.Errorf("ReadSeed after a %q refusal = (found=%v, err=%v); want (false, nil) -- nothing written", verb, found, err)
			}
			if entries, err := shedrun.List(loc); err != nil || len(entries) != 0 {
				t.Errorf("List after a %q refusal = (%v, %v); want (empty, nil) -- no run directory was created", verb, entries, err)
			}
		})
	}
}
