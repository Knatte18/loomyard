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

// TestResolveRunID asserts each generic verb -- run, step, status, pause -- refuses over a worktree with no seed at the addressed run-id,
// naming it and reading as the ordinary "no run is seeded yet" case, and writes nothing to disk;
// the non-generic verbs never reach the seed-presence refusal, and reject's positional review file leaves the run-id at self.
// resolveRunID performs reads alone (shedrun.ReadSeed, shedrun.List), and arm never reaches wireLightweight/wire on this path.
func TestResolveRunID(t *testing.T) {
	tests := []struct {
		name        string
		verb        string
		args        []string
		wantRefusal bool
		// wantRunID, when set, is the run-id the receiver is left addressing.
		wantRunID string
	}{
		{name: "run refuses", verb: "run", wantRefusal: true},
		{name: "step refuses", verb: "step", wantRefusal: true},
		{name: "status refuses", verb: "status", wantRefusal: true},
		{name: "pause refuses", verb: "pause", wantRefusal: true},
		{name: "start never refuses", verb: "start"},
		{name: "validate-discussion never refuses", verb: "validate-discussion"},
		{name: "validate-plan never refuses", verb: "validate-plan"},
		{name: "lint-comments never refuses", verb: "lint-comments"},
		{name: "reject's review file is not a run-id", verb: "reject", args: []string{"review.md"}, wantRunID: shedrun.SelfRunID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
			c := &loomCLI{}

			err := c.resolveRunID(loc, tt.verb, tt.args)

			if !tt.wantRefusal {
				if err != nil {
					t.Fatalf("resolveRunID(%q) = %v; want nil -- it never reaches the seed-presence refusal", tt.verb, err)
				}
				if tt.wantRunID != "" && c.runID != tt.wantRunID {
					t.Errorf("runID = %q; want %q", c.runID, tt.wantRunID)
				}
				return
			}
			if err == nil {
				t.Fatalf("resolveRunID(%q) = nil; want a refusal (no seed exists)", tt.verb)
			}
			if !strings.Contains(err.Error(), shedrun.SelfRunID) {
				t.Errorf("resolveRunID(%q) error = %q; want it to name the addressed run-id %q", tt.verb, err.Error(), shedrun.SelfRunID)
			}
			// The pre-existing-worktree case shedrun.MissingSeedMessage documents.
			if !strings.Contains(err.Error(), "no run is seeded yet") {
				t.Errorf("resolveRunID(%q) error = %q; want it to read as the ordinary empty-listing case", tt.verb, err.Error())
			}
			if _, found, err := shedrun.ReadSeed(loc, shedrun.SelfRunID); err != nil || found {
				t.Errorf("ReadSeed after a %q refusal = (found=%v, err=%v); want (false, nil) -- nothing written", tt.verb, found, err)
			}
			if entries, err := shedrun.List(loc); err != nil || len(entries) != 0 {
				t.Errorf("List after a %q refusal = (%v, %v); want (empty, nil) -- no run directory was created", tt.verb, entries, err)
			}
		})
	}
}

// TestResolveRunID_RefusalNamesEveryExistingRunID asserts the refusal's text lists every run-id
// shedrun.List finds, sorted, when the addressed run-id itself has no seed.
func TestResolveRunID_RefusalNamesEveryExistingRunID(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
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

// TestResolveRunID_StatusFilePresentWithNoSeedTakesTheSameRefusal asserts a status file already
// present at the addressed run-id, with no seed beside it, still refuses -- the inconsistency the
// refusal exists to catch (e.g. a worktree from before this task).
func TestResolveRunID_StatusFilePresentWithNoSeedTakesTheSameRefusal(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
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

// TestResolveRecipe asserts resolveRecipe records the recipe the addressed run's seed names, loom when the run has no seed, and leaves the seed unwritten.
func TestResolveRecipe(t *testing.T) {
	tests := []struct {
		name   string
		seed   *shedrun.Seed
		want   string
		runID  string
		seeded string
	}{
		{name: "no seed takes loom", want: shedrun.RecipeLoom, runID: shedrun.SelfRunID},
		{name: "loom seed", seed: &shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}, want: shedrun.RecipeLoom, runID: shedrun.SelfRunID, seeded: shedrun.SelfRunID},
		{name: "darn seed", seed: &shedrun.Seed{Recipe: shedrun.RecipeDarn, Driver: shedrun.DriverGo}, want: shedrun.RecipeDarn, runID: shedrun.SelfRunID, seeded: shedrun.SelfRunID},
		{name: "the addressed run-id's seed is read", seed: &shedrun.Seed{Recipe: shedrun.RecipeDarn, Driver: shedrun.DriverGo}, want: shedrun.RecipeDarn, runID: "other", seeded: "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
			if tt.seed != nil {
				if err := shedrun.WriteSeed(loc, tt.seeded, *tt.seed); err != nil {
					t.Fatalf("shedrun.WriteSeed() = %v; want nil", err)
				}
			}
			c := &loomCLI{runID: tt.runID}

			if err := c.resolveRecipe(loc); err != nil {
				t.Fatalf("resolveRecipe() = %v; want nil", err)
			}
			if c.recipe != tt.want {
				t.Errorf("recipe = %q; want %q", c.recipe, tt.want)
			}
			if tt.seed == nil {
				if _, found, err := shedrun.ReadSeed(loc, tt.runID); err != nil || found {
					t.Errorf("ReadSeed after resolveRecipe = (found=%v, err=%v); want (false, nil) -- nothing written", found, err)
				}
			}
		})
	}
}
