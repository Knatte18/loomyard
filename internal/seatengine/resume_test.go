// resume_test.go covers Engine.Probe and Engine.Resume: probing before anything is archived, attaching to a live chair, and telling it of the advisors the probe lost.

package seatengine

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestEngine_ProbeAndResume_AttachToALiveChair(t *testing.T) {
	t.Parallel()
	shuttle := newFakeShuttle(t)
	engine, geom := engineFixture(t, shuttle)
	table := engineTable(geom.WorktreeRoot, 3)

	// advisor-1 is live, advisor-2 is a done advisor kept by its pane that the probe removes, and advisor-3 is gone.
	chair := shuttle.newHandle(role(RoleChair))
	live := shuttle.newHandle(role(AdvisorName(1)))
	kept := shuttle.newHandle(role(AdvisorName(2)))
	shuttle.live = map[string]*fakeHandle{role(RoleChair): chair, role(AdvisorName(1)): live}
	shuttle.removedOnProbe = map[string]*fakeHandle{role(AdvisorName(2)): kept}
	keptOutput := table.Seats[2].Outputs[0]
	if err := os.WriteFile(keptOutput, []byte("advice"), 0o644); err != nil {
		t.Fatalf("write the kept advisor's output: %v", err)
	}

	found, err := engine.Probe(table)
	if err != nil {
		t.Fatalf("Probe() = %v", err)
	}
	if found.Chair != Handle(chair) || len(found.Advisors) != 1 || found.Advisors[AdvisorName(1)] != Handle(live) {
		t.Fatalf("Probe() = %+v, want the live chair and advisor-1 only", found)
	}
	if kept.stopCount() != 1 || live.stopCount() != 0 || chair.stopCount() != 0 {
		t.Errorf("stops after the probe: kept %d, live %d, chair %d, want only the kept advisor removed", kept.stopCount(), live.stopCount(), chair.stopCount())
	}
	if _, err := os.Stat(keptOutput); err != nil {
		t.Errorf("the kept advisor's output after the probe: %v, want it left in place", err)
	}
	if len(shuttle.gates[role(RoleChair)]) != 1 || len(shuttle.gates[role(AdvisorName(1))]) != 0 {
		t.Errorf("probe gates: chair %d entries, advisor %d, want the table's gate on the chair only", len(shuttle.gates[role(RoleChair)]), len(shuttle.gates[role(AdvisorName(1))]))
	}

	ended := make(chan engineEnd, 1)
	go func() {
		result, err := engine.Resume(table, found)
		ended <- engineEnd{result, err}
	}()
	lines := []string{receive(t, chair), receive(t, chair)}
	if !strings.Contains(lines[0], "Advisor ly:task:multi-advisor-2 finished before this step was resumed") || !strings.Contains(lines[0], keptOutput) {
		t.Errorf("finished advisor notice = %q, want the finished wording and its output", lines[0])
	}
	if !strings.Contains(lines[1], "Advisor ly:task:multi-advisor-3 ended and no longer answers") {
		t.Errorf("ended advisor notice = %q, want the ended wording", lines[1])
	}
	chair.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
	end := await(t, ended)

	if end.err != nil || end.result.Chair.Outcome != shuttleengine.OutcomeDone {
		t.Fatalf("Resume() = %+v, %v, want the chair's done result", end.result, end.err)
	}
	if len(shuttle.handles) != 0 {
		t.Errorf("Resume() started %d seats, want none restarted", len(shuttle.handles))
	}
	if live.stopCount() != 1 {
		t.Errorf("live advisor stopped %d times, want once when the chair ended", live.stopCount())
	}
	if got := end.result.Advisors; !got[1].Notified || !got[2].Notified || got[0].Notified {
		t.Errorf("advisor results = %+v, want advisors 2 and 3 notified and advisor 1 not", got)
	}
}

func TestEngine_Probe_ChairNotLiveStopsTheLiveAdvisorsAndLeavesAFreshStart(t *testing.T) {
	t.Parallel()
	shuttle := newFakeShuttle(t)
	engine, geom := engineFixture(t, shuttle)
	table := engineTable(geom.WorktreeRoot, 2)
	advisor := shuttle.newHandle(role(AdvisorName(1)))
	shuttle.live = map[string]*fakeHandle{role(AdvisorName(1)): advisor}
	output := table.Seats[1].Outputs[0]
	if err := os.WriteFile(output, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write the advisor's output: %v", err)
	}

	found, err := engine.Probe(table)
	if err != nil {
		t.Fatalf("Probe() = %v", err)
	}
	if found.Chair != nil || len(found.Advisors) != 0 {
		t.Errorf("Probe() = %+v, want an empty LiveTable", found)
	}
	if advisor.stopCount() != 1 {
		t.Errorf("live advisor stopped %d times, want once", advisor.stopCount())
	}
	if _, err := os.Stat(output); err != nil {
		t.Errorf("the advisor's output after the probe: %v, want the engine to archive nothing", err)
	}
	if len(shuttle.handles) != 0 {
		t.Errorf("Probe() started %d seats, want none", len(shuttle.handles))
	}

	// A strand of the earlier step still holding the chair's name is a start error of the fresh run that follows.
	shuttle.strandNames = map[string]string{role(RoleChair): "ly:task:multi-chair-2"}
	_, err = engine.Run(table)
	if err == nil || !strings.Contains(err.Error(), `lyx reed remove --name ly:task:multi-chair"`) {
		t.Errorf("Run() after the probe = %v, want a start error naming the holder", err)
	}
}

func TestEngine_ProbeAndResume_Refusals(t *testing.T) {
	t.Parallel()
	probeFailure := errors.New("reed unreachable")
	tests := []struct {
		name    string
		act     func(*Engine, *fakeShuttle, Table) error
		script  func(*fakeShuttle)
		wantIs  error
		wantMsg string
	}{
		{
			name:    "resume without a live chair",
			act:     func(e *Engine, _ *fakeShuttle, table Table) error { _, err := e.Resume(table, LiveTable{}); return err },
			wantMsg: "resume needs a live chair; way forward: run the row again",
		},
		{
			name:   "a probe error",
			script: func(f *fakeShuttle) { f.probeErr = probeFailure },
			act:    func(e *Engine, _ *fakeShuttle, table Table) error { _, err := e.Probe(table); return err },
			wantIs: probeFailure,
		},
		{
			name: "an advisor that cannot be stopped",
			script: func(f *fakeShuttle) {
				f.live = map[string]*fakeHandle{role(AdvisorName(1)): f.newHandle(role(AdvisorName(1)))}
				f.live[role(AdvisorName(1))].stopErr = errors.New("pane gone")
			},
			act:     func(e *Engine, _ *fakeShuttle, table Table) error { _, err := e.Probe(table); return err },
			wantIs:  ErrSeatNotStopped,
			wantMsg: `lyx reed remove guid-multi-advisor-1"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shuttle := newFakeShuttle(t)
			if tt.script != nil {
				tt.script(shuttle)
			}
			engine, geom := engineFixture(t, shuttle)

			err := tt.act(engine, shuttle, engineTable(geom.WorktreeRoot, 1))

			if err == nil {
				t.Fatal("error = nil, want a refusal")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("error = %v, want it to wrap %v", err, tt.wantIs)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantMsg)
			}
			if len(shuttle.handles) != 0 {
				t.Errorf("%d seats started, want none", len(shuttle.handles))
			}
		})
	}
}
