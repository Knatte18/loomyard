// specfill_test.go pins the run-identity fields batten's arming fills onto its Spec: RunID, StepsDir
// and Routing. It calls specFor and loadRouting on a hand-populated receiver, so it spawns nothing.

package battencli

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// TestSpecFor_FillsRunIdentityForSlug verifies that a spec armed for a slug carries that slug as
// RunID, a StepsDir under its .lyx scratch directory, and batten's own routing.
func TestSpecFor_FillsRunIdentityForSlug(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
	c := &battenCLI{
		location:  loc,
		slug:      "task-a",
		shedPaths: shedbuild.ShedPaths{MaxBounces: 2},
	}
	if err := c.loadRouting(); err != nil {
		t.Fatalf("loadRouting: %v", err)
	}

	spec := c.specFor("status")

	if spec.RunID != "task-a" {
		t.Errorf("RunID = %q, want %q", spec.RunID, "task-a")
	}
	wantSteps := filepath.Join(loc.AnchorPath(), ".lyx", "shed", "task-a", "steps")
	if spec.StepsDir != wantSteps {
		t.Errorf("StepsDir = %q, want %q", spec.StepsDir, wantSteps)
	}
	want, err := battenrecipe.Routing()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Routing.Entry != want.Entry || len(spec.Routing.Producers) != len(want.Producers) {
		t.Errorf("Routing = entry %q with %d producers, want entry %q with %d",
			spec.Routing.Entry, len(spec.Routing.Producers), want.Entry, len(want.Producers))
	}
	if spec.Routing.MaxBounces != 2 {
		t.Errorf("Routing.MaxBounces = %d, want 2", spec.Routing.MaxBounces)
	}
}

// TestSpecFor_CarriesBattenMissingStatusWayForward verifies batten's armed spec names the slug's own run verb for a missing status file.
func TestSpecFor_CarriesBattenMissingStatusWayForward(t *testing.T) {
	c := &battenCLI{
		slug: "task-a",
		shedPaths: shedbuild.ShedPaths{
			MissingStatusWayForward: `way forward: "lyx batten run task-a" creates the lifecycle's status file`,
		},
	}

	spec := c.specFor("goto")

	want := `way forward: "lyx batten run task-a" creates the lifecycle's status file`
	if spec.MissingStatusWayForward != want {
		t.Errorf("MissingStatusWayForward = %q, want %q", spec.MissingStatusWayForward, want)
	}
}
