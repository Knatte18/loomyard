// beamup_test.go covers the Beam-Up and Beam-Down rows and the constructors' refusal of an unset seam.

package pawlengine_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// beamUpDeps returns the world's deps with two beam steps that log their calls.
func beamUpDeps(w *world, stepErr error) pawlengine.Deps {
	deps := w.deps()
	deps.Steps = []pawlengine.Step{
		{Name: "start the dobby", Run: func() error { w.record("step:dobby"); return stepErr }},
		{Name: "start the orch", Run: func() error { w.record("step:orch"); return nil }},
	}
	return deps
}

// TestBeamUp asserts a fresh pair runs every step in order and clears what an earlier run left, that a stored proposal is mailed to the orch after the orch step before the hand-off marker is written, and that a failing step or hand-off halts the row naming its cause and `lym loom resume` with no marker written.
func TestBeamUp(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const handOffLine = "this beam was filed by a raddle run"
	tests := []struct {
		name        string
		tweak       func(w *world)
		stepErr     error
		wantStuck   bool
		wantEvents  []string
		wantNotices []string
		wantReason  []string
	}{
		{
			name:       "FreshPairWithNoProposalRunsEveryStepAndSendsNoHandOff",
			wantEvents: []string{"mark", "push", "workspace", "step:dobby", "step:orch", "handoff"},
		},
		{
			name: "StoredProposalMailsTheHandOffAfterTheOrchStep",
			tweak: func(w *world) {
				w.handOffFound = true
				w.handOffLine = handOffLine
			},
			wantEvents:  []string{"mark", "push", "workspace", "step:dobby", "step:orch", "handoff"},
			wantNotices: []string{handOffLine},
		},
		{
			name:       "HandOffErrorHalts",
			tweak:      func(w *world) { w.handOffErr = errors.New("store unreadable") },
			wantStuck:  true,
			wantEvents: []string{"mark", "push", "workspace", "step:dobby", "step:orch", "handoff"},
			wantReason: []string{"store unreadable", "lym loom resume"},
		},
		{
			name: "NotifyFailureAfterAFoundLineHalts",
			tweak: func(w *world) {
				w.handOffFound = true
				w.handOffLine = handOffLine
				w.notifyErr = errors.New("creel refused")
			},
			wantStuck:   true,
			wantEvents:  []string{"mark", "push", "workspace", "step:dobby", "step:orch", "handoff"},
			wantNotices: []string{handOffLine},
			wantReason:  []string{"creel refused", "lym loom resume"},
		},
		{
			name: "NotifyThatPostsNothingAfterAFoundLineHalts",
			tweak: func(w *world) {
				w.handOffFound = true
				w.handOffLine = handOffLine
				w.notifyUnposted = true
			},
			wantStuck:   true,
			wantEvents:  []string{"mark", "push", "workspace", "step:dobby", "step:orch", "handoff"},
			wantNotices: []string{handOffLine},
			wantReason:  []string{"reached no orch", "lym loom resume"},
		},
		{
			name:       "FailingMarkHaltsBeforeAnyLaterStep",
			tweak:      func(w *world) { w.markErr = errors.New("origin refused") },
			wantStuck:  true,
			wantEvents: []string{"mark"},
			wantReason: []string{"origin refused", "lym loom resume"},
		},
		{
			name:       "FailingPushSpawnHaltsBeforeTheWorkspace",
			tweak:      func(w *world) { w.pushErr = errors.New("no spawn") },
			wantStuck:  true,
			wantEvents: []string{"mark", "push"},
			wantReason: []string{"no spawn", "lym loom resume"},
		},
		{
			name:       "FailingWorkspaceHalts",
			tweak:      func(w *world) { w.workspaceErr = errors.New("workspace unwritable") },
			wantStuck:  true,
			wantEvents: []string{"mark", "push", "workspace"},
			wantReason: []string{"workspace unwritable", "lym loom resume"},
		},
		{
			name:       "FailingStepHaltsBeforeTheNextStep",
			stepErr:    errors.New("dobby will not start"),
			wantStuck:  true,
			wantEvents: []string{"mark", "push", "workspace", "step:dobby"},
			wantReason: []string{"start the dobby", "dobby will not start", "lym loom resume"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			w.request(t, "an-earlier-run")
			touch(t, w.paths.AwaitMarker)
			if tt.tweak != nil {
				tt.tweak(w)
			}
			producer, err := pawlengine.NewBeamUp(pawlengine.NameBeamUp, beamUpDeps(w, tt.stepErr))
			if err != nil {
				t.Fatal(err)
			}

			outcome, output, err := producer.Call(context.Background())
			if err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if diff := cmp.Diff(tt.wantEvents, w.snapshotEvents()); diff != "" {
				t.Errorf("events mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantNotices, w.snapshotNotices()); diff != "" {
				t.Errorf("notices mismatch (-want +got):\n%s", diff)
			}
			if marker := exists(w.paths.HandOffMarker); marker == tt.wantStuck {
				t.Errorf("hand-off marker present = %t; want %t", marker, !tt.wantStuck)
			}
			if !tt.wantStuck {
				if outcome != shedengine.Done {
					t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
				}
				if exists(w.paths.LandRequest) || exists(w.paths.AwaitMarker) {
					t.Errorf("a land request or await marker of an earlier run still stands")
				}
				return
			}
			if outcome != shedengine.Stuck || !output.Halt {
				t.Errorf("Call() = %q halt %t; want a halting %q", outcome, output.Halt, shedengine.Stuck)
			}
			for _, part := range tt.wantReason {
				if !strings.Contains(output.Reason, part) {
					t.Errorf("reason = %q; want it to contain %q", output.Reason, part)
				}
			}
		})
	}
}

// TestBeamUp_RerunIsIdempotent asserts a second call over the same beam, as a resume makes, runs every step again and is Done, and that the hand-off is mailed again only when the marker is gone.
func TestBeamUp_RerunIsIdempotent(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const handOffLine = "this beam was filed by a raddle run"
	w := newWorld(t)
	w.handOffFound = true
	w.handOffLine = handOffLine
	producer, err := pawlengine.NewBeamUp(pawlengine.NameBeamUp, beamUpDeps(w, nil))
	if err != nil {
		t.Fatal(err)
	}
	call := func(n int) {
		t.Helper()
		if outcome, _, err := producer.Call(context.Background()); err != nil || outcome != shedengine.Done {
			t.Fatalf("call %d = %q, error %v; want Done", n, outcome, err)
		}
	}

	call(1)
	call(2)
	// The first call runs six steps and the hand-off; the rerun runs the same steps and, the marker standing, no hand-off.
	if got := len(w.snapshotEvents()); got != 11 {
		t.Errorf("recorded %d events over two calls; want every step twice and the hand-off once", got)
	}
	if diff := cmp.Diff([]string{handOffLine}, w.snapshotNotices()); diff != "" {
		t.Errorf("notices after a rerun with the marker present mismatch (-want +got):\n%s", diff)
	}

	// Removing the marker stands for an interruption between the mail and the marker.
	if err := os.Remove(w.paths.HandOffMarker); err != nil {
		t.Fatal(err)
	}
	call(3)
	if diff := cmp.Diff([]string{handOffLine, handOffLine}, w.snapshotNotices()); diff != "" {
		t.Errorf("notices after a rerun without the marker mismatch (-want +got):\n%s", diff)
	}
}

// TestBeamDown asserts a stop is called once and the row is Done, also when the stop fails.
func TestBeamDown(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		downErr error
	}{
		{"StopSucceeds", nil},
		{"FailingStopIsStillDone", errors.New("strand already gone")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			deps := w.deps()
			deps.Down = func() error { w.record("down"); return tt.downErr }
			producer, err := pawlengine.NewBeamDown(pawlengine.NameBeamDown, deps)
			if err != nil {
				t.Fatal(err)
			}

			outcome, _, err := producer.Call(context.Background())
			if err != nil || outcome != shedengine.Done {
				t.Errorf("Call() = %q, error %v; want Done", outcome, err)
			}
			if diff := cmp.Diff([]string{"down"}, w.snapshotEvents()); diff != "" {
				t.Errorf("events mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestNewProducers_RefuseAnUnsetSeam asserts each constructor refuses a Deps with a nil seam it uses, naming the field.
func TestNewProducers_RefuseAnUnsetSeam(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	build := map[string]func(pawlengine.Deps) error{
		"BeamUp":    func(d pawlengine.Deps) error { _, err := pawlengine.NewBeamUp("Beam-Up", d); return err },
		"BeamAwait": func(d pawlengine.Deps) error { _, err := pawlengine.NewBeamAwait("Beam-Await", d); return err },
		"BeamDown":  func(d pawlengine.Deps) error { _, err := pawlengine.NewBeamDown("Beam-Down", d); return err },
	}
	tests := []struct {
		producer string
		field    string
		unset    func(d *pawlengine.Deps)
	}{
		{"BeamUp", "MarkBeam", func(d *pawlengine.Deps) { d.MarkBeam = nil }},
		{"BeamUp", "SpawnPush", func(d *pawlengine.Deps) { d.SpawnPush = nil }},
		{"BeamUp", "RegenerateWorkspace", func(d *pawlengine.Deps) { d.RegenerateWorkspace = nil }},
		{"BeamUp", "HandOff", func(d *pawlengine.Deps) { d.HandOff = nil }},
		{"BeamUp", "Steps[0]", func(d *pawlengine.Deps) { d.Steps = []pawlengine.Step{{Name: "a step with no function"}} }},
		{"BeamUp", "Paths.LandRequest", func(d *pawlengine.Deps) { d.Paths.LandRequest = "" }},
		{"BeamAwait", "SetStatus", func(d *pawlengine.Deps) { d.SetStatus = nil }},
		{"BeamAwait", "EntryProducer", func(d *pawlengine.Deps) { d.EntryProducer = nil }},
		{"BeamAwait", "OwnedElsewhere", func(d *pawlengine.Deps) { d.OwnedElsewhere = nil }},
		{"BeamAwait", "StopDobby", func(d *pawlengine.Deps) { d.StopDobby = nil }},
		{"BeamAwait", "StartDobby", func(d *pawlengine.Deps) { d.StartDobby = nil }},
		{"BeamAwait", "DobbyLive", func(d *pawlengine.Deps) { d.DobbyLive = nil }},
		{"BeamAwait", "Recheck", func(d *pawlengine.Deps) { d.Recheck = nil }},
		{"BeamAwait", "ReadyStands", func(d *pawlengine.Deps) { d.ReadyStands = nil }},
		{"BeamAwait", "StampWake", func(d *pawlengine.Deps) { d.StampWake = nil }},
		{"BeamAwait", "Notify", func(d *pawlengine.Deps) { d.Notify = nil }},
		{"BeamAwait", "Now", func(d *pawlengine.Deps) { d.Now = nil }},
		{"BeamAwait", "Sleep", func(d *pawlengine.Deps) { d.Sleep = nil }},
		{"BeamAwait", "Slug", func(d *pawlengine.Deps) { d.Slug = "" }},
		{"BeamAwait", "RunID", func(d *pawlengine.Deps) { d.RunID = "" }},
		{"BeamDown", "Down", func(d *pawlengine.Deps) { d.Down = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.producer+"_"+tt.field, func(t *testing.T) {
			t.Parallel()
			deps := newWorld(t).deps()
			if err := build[tt.producer](deps); err != nil {
				t.Fatalf("constructor over complete deps = %v; want nil", err)
			}
			tt.unset(&deps)
			err := build[tt.producer](deps)
			if err == nil || !strings.Contains(err.Error(), "Deps."+tt.field) {
				t.Errorf("constructor with %s unset = %v; want an error naming Deps.%s", tt.field, err, tt.field)
			}
		})
	}
}
