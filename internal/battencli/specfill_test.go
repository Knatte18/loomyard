// specfill_test.go pins the run-identity fields batten's arming fills onto its Spec: RunID, StepsDir
// and Routing. It calls specFor and loadRouting on a hand-populated receiver, so it spawns nothing.

package battencli

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
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
				caller:    "orch",
				shedPaths: shedbuild.ShedPaths{MaxBounces: 2, MissingStatusWayForward: wayForward},
			}
			if err := c.loadRouting(); err != nil {
				t.Fatalf("loadRouting: %v", err)
			}

			spec := c.specFor(verb)

			if spec.RunID != "task-a" {
				t.Errorf("RunID = %q, want %q", spec.RunID, "task-a")
			}
			if spec.Caller != "orch" {
				t.Errorf("Caller = %q, want %q", spec.Caller, "orch")
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
			if want := shedrun.LoopLock(loc, "task-a"); spec.Loop.LockPath != want {
				t.Errorf("Loop.LockPath = %q, want %q", spec.Loop.LockPath, want)
			}
			if want := shedrun.LoopPIDFile(loc, "task-a"); spec.Loop.PIDPath != want {
				t.Errorf("Loop.PIDPath = %q, want %q", spec.Loop.PIDPath, want)
			}
			if want := shedrun.LoopLog(loc, "task-a"); spec.Loop.LogPath != want {
				t.Errorf("Loop.LogPath = %q, want %q", spec.Loop.LogPath, want)
			}
			if want := shedrun.LoopEnvelope(loc, "task-a"); spec.Loop.EnvelopePath != want {
				t.Errorf("Loop.EnvelopePath = %q, want %q", spec.Loop.EnvelopePath, want)
			}
			if spec.Loop.IdleTimeout != 0 || spec.Loop.Activity != nil || spec.Hooks.ConsumeAttempt != nil {
				t.Errorf("Loop.IdleTimeout = %v, Loop.Activity nil = %v, Hooks.ConsumeAttempt nil = %v; want a disarmed watchdog and no consume hook", spec.Loop.IdleTimeout, spec.Loop.Activity == nil, spec.Hooks.ConsumeAttempt == nil)
			}
			for row, want := range map[string]string{
				battenrecipe.NameRunShed:          shedverbs.InterruptPolicyReinvoke,
				battenrecipe.NameLand:             shedverbs.InterruptPolicyHandback,
				battenrecipe.NameWorktreeTeardown: shedverbs.InterruptPolicyHandback,
				battenrecipe.NameSeedChild:        shedverbs.InterruptPolicyHandback,
				"":                                shedverbs.InterruptPolicyHandback,
			} {
				if got := spec.Hooks.InterruptPolicyFor(row); got != want {
					t.Errorf("InterruptPolicyFor(%q) = %q, want %q", row, got, want)
				}
			}
		})
	}

	t.Run("a receiver without a location gets a zero Loop", func(t *testing.T) {
		t.Parallel()
		if spec := (&battenCLI{}).specFor("status"); !reflect.DeepEqual(spec.Loop, shedverbs.LoopSpec{}) {
			t.Errorf("Loop = %+v, want the zero value", spec.Loop)
		}
	})
}
