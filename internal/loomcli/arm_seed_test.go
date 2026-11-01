//go:build tier0

// arm_seed_test.go covers arm's seed-presence refusal: resolveRunID drives the whole check against
// a hand-built *lymcwd.Location, with no real git repository behind it (shedrun.ReadSeed and
// shedrun.List are both plain filesystem reads under AnchorPath), so this file stays Tier 1.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// seedRunID seeds a run-id under loc with shedrun.RecipeTapestry/shedrun.DriverGo, t.Fatal-ing on
// failure.
func seedRunID(t *testing.T, loc *lymcwd.Location, runID string) {
	t.Helper()
	if err := shedrun.WriteSeed(loc, runID, shedrun.Seed{Recipe: shedrun.RecipeTapestry, Driver: shedrun.DriverGo}); err != nil {
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
		// wantDriverless is whether the refusal's remedy also names the driverless seed route, which only run and step on self offer.
		wantDriverless bool
		// wantRunID, when set, is the run-id the receiver is left addressing.
		wantRunID string
	}{
		{name: "run refuses", verb: "run", wantRefusal: true, wantDriverless: true},
		{name: "step refuses", verb: "step", wantRefusal: true, wantDriverless: true},
		{name: "step on another run-id refuses with the start remedy alone", verb: "step", args: []string{"other"}, wantRefusal: true},
		{name: "status refuses", verb: "status", wantRefusal: true},
		{name: "pause refuses", verb: "pause", wantRefusal: true},
		{name: "goto refuses", verb: "goto", wantRefusal: true},
		{name: "start never refuses", verb: "start"},
		{name: "validate-discussion never refuses", verb: "validate-discussion"},
		{name: "validate-plan never refuses", verb: "validate-plan"},
		{name: "lint-comments never refuses", verb: "lint-comments"},
		{name: "lint-pattern never refuses", verb: "lint-pattern"},
		{name: "insert-card's position is not a run-id", verb: "insert-card", args: []string{"3", "slug"}, wantRunID: shedrun.SelfRunID},
		{name: "remove-card's position is not a run-id", verb: "remove-card", args: []string{"3"}, wantRunID: shedrun.SelfRunID},
		{name: "fmt's file arguments are not a run-id", verb: "fmt", args: []string{"internal/x/y.go"}, wantRunID: shedrun.SelfRunID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
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
			if !strings.Contains(err.Error(), c.runID) {
				t.Errorf("resolveRunID(%q) error = %q; want it to name the addressed run-id %q", tt.verb, err.Error(), c.runID)
			}
			if !strings.Contains(err.Error(), "lym loom start") {
				t.Errorf("resolveRunID(%q) error = %q; want it to name \"lym loom start\"", tt.verb, err.Error())
			}
			const driverlessSeed = "lym shed seed self --recipe tapestry --driver go --param parent=<parent-branch>"
			if got := strings.Contains(err.Error(), driverlessSeed) && strings.Contains(err.Error(), "lym loom step"); got != tt.wantDriverless {
				t.Errorf("resolveRunID(%q) error = %q; names the driverless route = %v, want %v", tt.verb, err.Error(), got, tt.wantDriverless)
			}
			if !tt.wantDriverless && strings.Contains(err.Error(), "lym shed seed") {
				t.Errorf("resolveRunID(%q) error = %q; want no \"lym shed seed\" route", tt.verb, err.Error())
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
	loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
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
	if !strings.Contains(err.Error(), "lym loom start") {
		t.Errorf("resolveRunID error = %q; want it to name \"lym loom start\" as the remedy", err.Error())
	}
}

// TestResolveRunID_StatusFilePresentWithNoSeedTakesTheSameRefusal asserts a status file already
// present at the addressed run-id, with no seed beside it, still refuses -- the inconsistency the
// refusal exists to catch (e.g. a worktree from before this task).
func TestResolveRunID_StatusFilePresentWithNoSeedTakesTheSameRefusal(t *testing.T) {
	loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
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
		{name: "no seed takes loom", want: shedrun.RecipeTapestry, runID: shedrun.SelfRunID},
		{name: "loom seed", seed: &shedrun.Seed{Recipe: shedrun.RecipeTapestry, Driver: shedrun.DriverGo}, want: shedrun.RecipeTapestry, runID: shedrun.SelfRunID, seeded: shedrun.SelfRunID},
		{name: "darn seed", seed: &shedrun.Seed{Recipe: shedrun.RecipeDarn, Driver: shedrun.DriverGo}, want: shedrun.RecipeDarn, runID: shedrun.SelfRunID, seeded: shedrun.SelfRunID},
		{name: "the addressed run-id's seed is read", seed: &shedrun.Seed{Recipe: shedrun.RecipeDarn, Driver: shedrun.DriverGo}, want: shedrun.RecipeDarn, runID: "other", seeded: "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
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

// TestArmAt_RefusesOffLoomRecipes asserts the `lym loom` arming path refuses a run whose recipe lym loom does not serve, and a plan verb on a recipe with no Plan-Write row, before it writes anything;
// a pawl run and a tapestry run pass both checks, and the arming `lym shed` shares never refuses a non-loom recipe.
// The checks read a seed and a recipe's rows and write nothing, so a hand-built location with no git repository behind it drives them.
func TestArmAt_RefusesOffLoomRecipes(t *testing.T) {
	t.Parallel()

	const (
		notLoom = "is not a loom recipe"
		unknown = "names no recipe this lym knows"
		noPlan  = "does not apply to this run"
	)
	seeded := func(t *testing.T, recipe string) *lymcwd.Location {
		t.Helper()
		loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
		if recipe == "beam" {
			// WriteSeed refuses the name no recipe carries any more, so the seed an older lym wrote is written by hand.
			seedPath := shedrun.SeedFile(loc, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(seedPath), 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(seedPath), err)
			}
			if err := os.WriteFile(seedPath, []byte(`{"recipe":"beam","driver":"go"}`), 0o644); err != nil {
				t.Fatalf("write the beam seed by hand: %v", err)
			}
			return loc
		}
		if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: recipe, Driver: shedrun.DriverGo}); err != nil {
			t.Fatalf("shedrun.WriteSeed(%q) = %v; want nil", recipe, err)
		}
		return loc
	}

	tests := []struct {
		name   string
		recipe string
		verb   string
		// want is the refusal's marker and wayForward a phrase its way forward names; empty want means the verb passes both checks.
		want       string
		wayForward string
	}{
		{"BattenIsNotALoomRecipe", shedrun.RecipeBatten, "status", notLoom, "lym shed status"},
		{"BeamNamesNoRecipe", "beam", "status", unknown, "seed the run again"},
		{"DarnValidatePlan", shedrun.RecipeDarn, "validate-plan", noPlan, "lym loom validate-discussion"},
		{"DarnInsertCard", shedrun.RecipeDarn, "insert-card", noPlan, "lym loom validate-discussion"},
		{"DarnRemoveCard", shedrun.RecipeDarn, "remove-card", noPlan, "lym loom validate-discussion"},
		{"DarnRestoreRenumber", shedrun.RecipeDarn, "restore-renumber", noPlan, "lym loom validate-discussion"},
		{"DarnValidateParts", shedrun.RecipeDarn, "validate-parts", noPlan, "lym loom validate-plan"},
		{"DarnJoinParts", shedrun.RecipeDarn, "join-parts", noPlan, "lym loom insert-card"},
		{"PawlStatusPasses", shedrun.RecipePawl, "status", "", ""},
		{"TapestryJoinPartsPasses", shedrun.RecipeTapestry, "join-parts", "", ""},
		{"TapestryValidatePlanPasses", shedrun.RecipeTapestry, "validate-plan", "", ""},
		{"TapestryValidatePartsPasses", shedrun.RecipeTapestry, "validate-parts", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := seeded(t, tt.recipe)
			c := newLoomCLI()
			c.loomPath = true

			_, err := c.armAt(loc, tt.verb, nil)

			if tt.want == "" {
				for _, refusal := range []string{notLoom, unknown, noPlan} {
					if err != nil && strings.Contains(err.Error(), refusal) {
						t.Errorf("armAt(%q) on a %q seed = %v; want neither lym loom refusal", tt.verb, tt.recipe, err)
					}
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), tt.wayForward) {
				t.Fatalf("armAt(%q) on a %q seed = %v; want a refusal containing %q and a way forward naming %q", tt.verb, tt.recipe, err, tt.want, tt.wayForward)
			}
			if _, statErr := os.Stat(shedrun.StatusFile(loc, shedrun.SelfRunID)); !os.IsNotExist(statErr) {
				t.Errorf("status file after a refused %q = %v; want it absent", tt.verb, statErr)
			}
		})
	}

	t.Run("ArmAtNeverRefusesAnUnservedRecipe", func(t *testing.T) {
		t.Parallel()
		loc := seeded(t, shedrun.RecipeBatten)

		_, err := ArmAt(loc, "status", shedrun.SelfRunID)

		if err != nil && strings.Contains(err.Error(), notLoom) {
			t.Errorf("ArmAt(status) on a batten seed = %v; want lym shed's arming never to refuse it", err)
		}
	})
}
