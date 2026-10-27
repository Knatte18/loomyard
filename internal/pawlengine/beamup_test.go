//go:build tier0

// beamup_test.go covers the Beam-Up and Beam-Down rows and the constructors' refusal of an unset seam.

package pawlengine_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
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

// TestBeamUp asserts a fresh pair runs every step in order and clears what an earlier run left, and that a failing step halts the row naming its cause and `lym loom resume`.
func TestBeamUp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		tweak      func(w *world)
		stepErr    error
		wantStuck  bool
		wantEvents []string
		wantReason []string
	}{
		{
			name:       "FreshPairRunsEveryStepInOrder",
			wantEvents: []string{"mark", "push", "workspace", "step:dobby", "step:orch"},
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

// TestBeamUp_RerunIsIdempotent asserts a second call over the same beam, as a resume makes, runs every step again and is Done.
func TestBeamUp_RerunIsIdempotent(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	producer, err := pawlengine.NewBeamUp(pawlengine.NameBeamUp, beamUpDeps(w, nil))
	if err != nil {
		t.Fatal(err)
	}
	for call := 1; call <= 2; call++ {
		if outcome, _, err := producer.Call(context.Background()); err != nil || outcome != shedengine.Done {
			t.Fatalf("call %d = %q, error %v; want Done", call, outcome, err)
		}
	}
	if got := len(w.snapshotEvents()); got != 10 {
		t.Errorf("recorded %d events over two calls; want every step twice", got)
	}
}

// TestBeamDown asserts a stop is called once and the row is Done, also when the stop fails.
func TestBeamDown(t *testing.T) {
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
		{"BeamUp", "Steps[0]", func(d *pawlengine.Deps) { d.Steps = []pawlengine.Step{{Name: "a step with no function"}} }},
		{"BeamUp", "Paths.LandRequest", func(d *pawlengine.Deps) { d.Paths.LandRequest = "" }},
		{"BeamAwait", "SetStatus", func(d *pawlengine.Deps) { d.SetStatus = nil }},
		{"BeamAwait", "EntryProducer", func(d *pawlengine.Deps) { d.EntryProducer = nil }},
		{"BeamAwait", "OwnedElsewhere", func(d *pawlengine.Deps) { d.OwnedElsewhere = nil }},
		{"BeamAwait", "StopDobby", func(d *pawlengine.Deps) { d.StopDobby = nil }},
		{"BeamAwait", "StartDobby", func(d *pawlengine.Deps) { d.StartDobby = nil }},
		{"BeamAwait", "DobbyLive", func(d *pawlengine.Deps) { d.DobbyLive = nil }},
		{"BeamAwait", "Recheck", func(d *pawlengine.Deps) { d.Recheck = nil }},
		{"BeamAwait", "WriteSnapshot", func(d *pawlengine.Deps) { d.WriteSnapshot = nil }},
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
