//go:build tier0

// resume_test.go covers Engine.Probe and Engine.Resume: probing before anything is archived, attaching to a live chair, and telling it of the advisors the probe lost.

package seatengine

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestEngine_ProbeAndResume_AttachToALiveChair(t *testing.T) {
	t.Parallel()
	shuttle := newFakeShuttle(t)
	engine, geom := engineFixture(t, shuttle)
	table := engineTable(geom.WorktreeRoot, 3)
	advisorGate := shuttleengine.GateSpec{{Name: "advisor-gate", Attempts: 2}}
	table.Seats[1].Gate = advisorGate

	// advisor-1 is live, advisor-2 is a done advisor kept by its pane that the probe removes, and advisor-3 is gone.
	chair := shuttle.newHandle(role(""))
	live := shuttle.newHandle(role(advisorName(1)))
	kept := shuttle.newHandle(role(advisorName(2)))
	shuttle.live = map[string]*fakeHandle{role(""): chair, role(advisorName(1)): live}
	shuttle.removedOnProbe = map[string]*fakeHandle{role(advisorName(2)): kept}
	keptOutput := table.Seats[2].Outputs[0]
	if err := os.WriteFile(keptOutput, []byte("advice"), 0o644); err != nil {
		t.Fatalf("write the kept advisor's output: %v", err)
	}

	found, err := engine.Probe(table)
	if err != nil {
		t.Fatalf("Probe() = %v", err)
	}
	if found.Chair != Handle(chair) || len(found.Advisors) != 1 || found.Advisors[advisorName(1)] != Handle(live) {
		t.Fatalf("Probe() = %+v, want the live chair and advisor-1 only", found)
	}
	if kept.stopCount() != 1 || live.stopCount() != 0 || chair.stopCount() != 0 {
		t.Errorf("stops after the probe: kept %d, live %d, chair %d, want only the kept advisor removed", kept.stopCount(), live.stopCount(), chair.stopCount())
	}
	if _, err := os.Stat(keptOutput); err != nil {
		t.Errorf("the kept advisor's output after the probe: %v, want it left in place", err)
	}
	if len(shuttle.gates[role("")]) != 1 || !reflect.DeepEqual(shuttle.gates[role(advisorName(1))], advisorGate) || len(shuttle.gates[role(advisorName(3))]) != 0 {
		t.Errorf("probe gates: chair %d entries, advisor-1 %+v, advisor-3 %d, want the table's gate on the chair, the seat's own on advisor-1 and none on advisor-3", len(shuttle.gates[role("")]), shuttle.gates[role(advisorName(1))], len(shuttle.gates[role(advisorName(3))]))
	}

	ended := make(chan engineEnd, 1)
	go func() {
		result, err := engine.Resume(table, found)
		ended <- engineEnd{result, err}
	}()
	lines := []string{receive(t, chair), receive(t, chair), receive(t, chair)}
	if want := advisorUpNotice("ly:task:multi-advisor-1"); lines[0] != want {
		t.Errorf("live advisor notice = %q, want %q", lines[0], want)
	}
	if !strings.Contains(lines[1], "Advisor ly:task:multi-advisor-2 finished before this step was resumed") || !strings.Contains(lines[1], keptOutput) {
		t.Errorf("finished advisor notice = %q, want the finished wording and its output", lines[1])
	}
	if !strings.Contains(lines[2], "Advisor ly:task:multi-advisor-3 ended and no longer answers") {
		t.Errorf("ended advisor notice = %q, want the ended wording", lines[2])
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

func TestEngine_Resume_Writers(t *testing.T) {
	t.Parallel()
	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}

	t.Run("a found writer is kept and the chair is sent no line", func(t *testing.T) {
		t.Parallel()
		shuttle := newFakeShuttle(t)
		engine, geom := engineFixture(t, shuttle)
		table := engineTable(geom.WorktreeRoot, 1)
		asWriters(&table)
		chair, writer := shuttle.newHandle(role("")), shuttle.newHandle(role(advisorName(1)))
		ended := make(chan engineEnd, 1)
		go func() {
			result, err := engine.Resume(table, LiveTable{Chair: chair, Advisors: map[string]Handle{advisorName(1): writer}})
			ended <- engineEnd{result, err}
		}()
		chair.release(done, nil)
		end := await(t, ended)
		if end.err != nil || end.result.Advisors[0].EndedStep || end.result.Advisors[0].Notified {
			t.Errorf("Resume() = %+v, %v; want the chair's end to decide and the writer unnotified", end.result.Advisors, end.err)
		}
		if writer.stopCount() != 1 || len(chair.sent) != 0 {
			t.Errorf("writer stops = %d, lines typed into the chair = %d; want the writer stopped once and no line", writer.stopCount(), len(chair.sent))
		}
	})

	t.Run("a writer not found whose outputs pass Accept is finished and the chair is told once", func(t *testing.T) {
		t.Parallel()
		shuttle := newFakeShuttle(t)
		engine, geom := engineFixture(t, shuttle)
		table := engineTable(geom.WorktreeRoot, 1)
		asWriters(&table)
		table.Seats[1].Accept = func(shuttleengine.Result) error { return nil }
		output := table.Seats[1].Outputs[0]
		if err := os.WriteFile(output, []byte("part"), 0o644); err != nil {
			t.Fatalf("write the writer's output: %v", err)
		}
		chair := shuttle.newHandle(role(""))
		ended := make(chan engineEnd, 1)
		go func() {
			result, err := engine.Resume(table, LiveTable{Chair: chair})
			ended <- engineEnd{result, err}
		}()
		if got, want := receive(t, chair), writerResumeNotice("ly:task:"+role(advisorName(1)), []string{output}); got != want {
			t.Errorf("line typed into the chair = %q, want %q", got, want)
		}
		chair.release(done, nil)
		end := await(t, ended)
		if got := end.result.Advisors[0]; end.err != nil || got.EndedStep || !got.Notified {
			t.Errorf("Resume() = %+v, %v; want the step to go on until the chair ends, with the writer notified", got, end.err)
		}
	})

	t.Run("a writer not found whose outputs are missing ends the step without calling Accept", func(t *testing.T) {
		t.Parallel()
		shuttle := newFakeShuttle(t)
		engine, geom := engineFixture(t, shuttle)
		table := engineTable(geom.WorktreeRoot, 1)
		asWriters(&table)
		acceptCalls := 0
		table.Seats[1].Accept = func(shuttleengine.Result) error { acceptCalls++; return nil }
		chair := shuttle.newHandle(role(""))
		ended := make(chan engineEnd, 1)
		go func() {
			result, err := engine.Resume(table, LiveTable{Chair: chair})
			ended <- engineEnd{result, err}
		}()

		end := await(t, ended)

		if end.err != nil {
			t.Fatalf("Resume() error = %v; want nil", end.err)
		}
		if got := end.result.Advisors[0]; !got.EndedStep || got.Notified || got.StrandGUID != "" || got.Outcome != "" || got.AcceptErr != nil {
			t.Errorf("writer result = %+v; want EndedStep, unnotified, with no strand, no outcome and no Accept error", got)
		}
		if acceptCalls != 0 || chair.stopCount() != 1 || len(chair.sent) != 0 {
			t.Errorf("Accept calls = %d, chair stops = %d, lines typed into the chair = %d; want none, one and none", acceptCalls, chair.stopCount(), len(chair.sent))
		}
	})

	t.Run("a writer not found whose outputs fail Accept ends the step and keeps the error", func(t *testing.T) {
		t.Parallel()
		shuttle := newFakeShuttle(t)
		engine, geom := engineFixture(t, shuttle)
		table := engineTable(geom.WorktreeRoot, 1)
		asWriters(&table)
		rejected := errors.New("part is malformed")
		table.Seats[1].Accept = func(shuttleengine.Result) error { return rejected }
		if err := os.WriteFile(table.Seats[1].Outputs[0], []byte("part"), 0o644); err != nil {
			t.Fatalf("write the writer's output: %v", err)
		}
		chair := shuttle.newHandle(role(""))
		ended := make(chan engineEnd, 1)
		go func() {
			result, err := engine.Resume(table, LiveTable{Chair: chair})
			ended <- engineEnd{result, err}
		}()

		end := await(t, ended)

		if end.err != nil {
			t.Fatalf("Resume() error = %v; want nil", end.err)
		}
		if got := end.result.Advisors[0]; !got.EndedStep || got.Notified || !errors.Is(got.AcceptErr, rejected) || got.StrandGUID != "" || got.Outcome != "" {
			t.Errorf("writer result = %+v; want EndedStep, unnotified, with the Accept error, no strand and no outcome", got)
		}
		if chair.stopCount() != 1 || len(chair.sent) != 0 {
			t.Errorf("chair stops = %d, lines typed into the chair = %d; want one and none", chair.stopCount(), len(chair.sent))
		}
	})
}

func TestEngine_Probe_ChairNotLiveStopsTheLiveAdvisorsAndLeavesAFreshStart(t *testing.T) {
	t.Parallel()
	shuttle := newFakeShuttle(t)
	engine, geom := engineFixture(t, shuttle)
	table := engineTable(geom.WorktreeRoot, 2)
	advisor := shuttle.newHandle(role(advisorName(1)))
	shuttle.live = map[string]*fakeHandle{role(advisorName(1)): advisor}
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
	shuttle.strandNames = map[string]string{role(""): "ly:task:multi-2"}
	_, err = engine.Run(table)
	if err == nil || !strings.Contains(err.Error(), `lym reed remove --name ly:task:multi"`) {
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
				f.live = map[string]*fakeHandle{role(advisorName(1)): f.newHandle(role(advisorName(1)))}
				f.live[role(advisorName(1))].stopErr = errors.New("pane gone")
			},
			act:     func(e *Engine, _ *fakeShuttle, table Table) error { _, err := e.Probe(table); return err },
			wantIs:  ErrSeatNotStopped,
			wantMsg: `lym reed remove guid-multi-advisor-1"`,
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
