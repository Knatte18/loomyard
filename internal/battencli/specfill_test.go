// specfill_test.go pins the run-identity fields batten's arming fills onto its Spec: RunID, StepsDir
// and Routing. It calls specFor and loadRouting on a hand-populated receiver, so it spawns nothing.

package battencli

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestSpecFor_FillsRunIdentityForSlug verifies that a spec armed for a slug carries that slug as RunID, a StepsDir and ScratchDir under its .lyx tree, batten's own routing and its own missing-status way forward, an empty FrictionDir since batten carries no agent friction directory, and, for the step verb, a non-nil BuildShed.
func TestSpecFor_FillsRunIdentityForSlug(t *testing.T) {
	t.Parallel()

	const wayForward = `way forward: "lyx batten run task-a" creates the lifecycle's status file`
	want, err := battenrecipe.Routing()
	if err != nil {
		t.Fatal(err)
	}

	for _, verb := range []string{"status", "step", "goto"} {
		t.Run(verb, func(t *testing.T) {
			t.Parallel()

			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}
			c := &battenCLI{
				location:  loc,
				slug:      "task-a",
				shedPaths: shedbuild.ShedPaths{MaxBounces: 2, MissingStatusWayForward: wayForward},
			}
			if err := c.loadRouting(); err != nil {
				t.Fatalf("loadRouting: %v", err)
			}

			spec := c.specFor(verb)

			if spec.RunID != "task-a" {
				t.Errorf("RunID = %q, want %q", spec.RunID, "task-a")
			}
			wantSteps := filepath.Join(loc.AnchorPath(), ".lyx", "shed", "task-a", "steps")
			if spec.StepsDir != wantSteps {
				t.Errorf("StepsDir = %q, want %q", spec.StepsDir, wantSteps)
			}
			if wantScratch := shedrun.ScratchDir(loc, "task-a"); spec.ScratchDir != wantScratch {
				t.Errorf("ScratchDir = %q, want %q", spec.ScratchDir, wantScratch)
			}
			if spec.FrictionDir != "" {
				t.Errorf("FrictionDir = %q, want empty", spec.FrictionDir)
			}
			if spec.Routing.Entry != want.Entry || len(spec.Routing.Producers) != len(want.Producers) {
				t.Errorf("Routing = entry %q with %d producers, want entry %q with %d",
					spec.Routing.Entry, len(spec.Routing.Producers), want.Entry, len(want.Producers))
			}
			if spec.Routing.MaxBounces != 2 {
				t.Errorf("Routing.MaxBounces = %d, want 2", spec.Routing.MaxBounces)
			}
			if spec.MissingStatusWayForward != wayForward {
				t.Errorf("MissingStatusWayForward = %q, want %q", spec.MissingStatusWayForward, wayForward)
			}
			if verb == "step" && spec.BuildShed == nil {
				t.Error("specFor(step).BuildShed = nil; want a non-nil constructor")
			}
		})
	}
}
