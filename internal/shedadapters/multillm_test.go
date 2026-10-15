package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// Compile-time proof that *shedfake.SeatRunner satisfies SeatRunner.
var _ SeatRunner = (*shedfake.SeatRunner)(nil)

// liveChair is a non-nil Handle the fake seat runner hands back as a live chair; the producer never calls it.
var liveChair seatengine.Handle = (*shuttleengine.Run)(nil)

// multiLLMTable returns an unnamed lead with the given number of named advisors, whose files live under dir.
func multiLLMTable(dir string, advisors int) seatengine.Table {
	table := seatengine.Table{
		RolePrefix: "multi",
		Seats:      []seatengine.Seat{{Outputs: []string{filepath.Join(dir, "chair.md")}}},
	}
	for n := 1; n <= advisors; n++ {
		name := fmt.Sprintf("advisor-%d", n)
		output := filepath.Join(dir, name+".md")
		table.Seats[0].Inputs = append(table.Seats[0].Inputs, output)
		table.Seats = append(table.Seats, seatengine.Seat{Name: name, Outputs: []string{output}})
	}
	return table
}

// multiLLMFixedNow is the injected clock of the producer tests.
func multiLLMFixedNow() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

// writeTableFiles writes a placeholder at every output of table and returns the paths.
func writeTableFiles(t *testing.T, table seatengine.Table) []string {
	t.Helper()
	paths := table.Outputs()
	for _, path := range paths {
		writeRoundFile(t, path)
	}
	return paths
}

func TestMultiLLMProducer_ProbesBeforeArchiving(t *testing.T) {
	t.Parallel()

	t.Run("a live chair is resumed and the outputs are left alone", func(t *testing.T) {
		t.Parallel()
		table := multiLLMTable(t.TempDir(), 1)
		paths := writeTableFiles(t, table)
		runner := &shedfake.SeatRunner{
			LiveTables:    []seatengine.LiveTable{{Chair: liveChair}},
			ResumeResults: []seatengine.Result{{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, ChairOutputs: table.Chair().Outputs}},
		}
		producer := NewMultiLLMProducer("multi", table, runner, multiLLMFixedNow)

		ptr := shedfake.RequireOutcome(t, producer, shedengine.Done)

		if ptr.Path != paths[0] {
			t.Errorf("pointer path = %q, want the chair's first output %q", ptr.Path, paths[0])
		}
		if runner.ProbeCalls != 1 || runner.ResumeCalls != 1 || runner.Calls != 0 {
			t.Errorf("probe/resume/run calls = %d/%d/%d, want 1/1/0", runner.ProbeCalls, runner.ResumeCalls, runner.Calls)
		}
		for _, path := range paths {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("output %s after resuming: %v, want it untouched", path, err)
			}
		}
	})

	t.Run("a fresh table archives every seat's stale outputs before it runs", func(t *testing.T) {
		t.Parallel()
		table := multiLLMTable(t.TempDir(), 2)
		paths := writeTableFiles(t, table)
		runner := &shedfake.SeatRunner{}
		runner.RunFn = func(seatengine.Table) (seatengine.Result, error) {
			for _, path := range paths {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("output %s when Run starts: %v, want it archived", path, err)
				}
			}
			return seatengine.Result{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, ChairOutputs: table.Chair().Outputs}, nil
		}
		producer := NewMultiLLMProducer("multi", table, runner, multiLLMFixedNow)

		shedfake.RequireOutcome(t, producer, shedengine.Done)

		if runner.ProbeCalls != 1 || runner.Calls != 1 || runner.ResumeCalls != 0 {
			t.Errorf("probe/resume/run calls = %d/%d/%d, want 1/0/1", runner.ProbeCalls, runner.ResumeCalls, runner.Calls)
		}
		archived, err := filepath.Glob(filepath.Join(filepath.Dir(paths[0]), "*20260102T030405Z*"))
		if err != nil || len(archived) != len(paths) {
			t.Errorf("archived files = %v (%v), want one per seat output", archived, err)
		}
	})

	t.Run("a table source is evaluated once per call and its table is probed and run", func(t *testing.T) {
		t.Parallel()
		table := multiLLMTable(t.TempDir(), 1)
		evaluations := 0
		source := func() (seatengine.Table, error) {
			evaluations++
			return table, nil
		}
		runner := &shedfake.SeatRunner{Results: []seatengine.Result{{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, ChairOutputs: table.Chair().Outputs}}}
		producer := NewMultiLLMProducerSourced("multi", source, runner, multiLLMFixedNow, nil)

		shedfake.RequireOutcome(t, producer, shedengine.Done)

		if evaluations != 1 {
			t.Errorf("table source evaluations = %d, want 1", evaluations)
		}
		if !reflect.DeepEqual(runner.GotProbeTables, []seatengine.Table{table}) || !reflect.DeepEqual(runner.GotTables, []seatengine.Table{table}) {
			t.Errorf("probed tables = %+v, run tables = %+v, want the source's table in both", runner.GotProbeTables, runner.GotTables)
		}
	})

	t.Run("the preparation runs after the probe and before the archive on the fresh path", func(t *testing.T) {
		t.Parallel()
		table := multiLLMTable(t.TempDir(), 1)
		paths := writeTableFiles(t, table)
		runner := &shedfake.SeatRunner{Results: []seatengine.Result{{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, ChairOutputs: table.Chair().Outputs}}}
		var probesWhenPrepared int
		var staleWhenPrepared []bool
		prepare := func() (string, error) {
			probesWhenPrepared = runner.ProbeCalls
			for _, path := range paths {
				_, err := os.Stat(path)
				staleWhenPrepared = append(staleWhenPrepared, err == nil)
			}
			return "", nil
		}
		producer := NewMultiLLMProducerSourced("multi", func() (seatengine.Table, error) { return table, nil }, runner, multiLLMFixedNow, prepare)

		shedfake.RequireOutcome(t, producer, shedengine.Done)

		if probesWhenPrepared != 1 {
			t.Errorf("probe calls when the preparation ran = %d, want 1", probesWhenPrepared)
		}
		if len(staleWhenPrepared) != len(paths) || slices.Contains(staleWhenPrepared, false) {
			t.Errorf("stale outputs present when the preparation ran = %v, want every output still in place", staleWhenPrepared)
		}
		for _, path := range paths {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("output %s after the call: %v, want it archived", path, err)
			}
		}
	})

	t.Run("the preparation never runs when the probe finds a live chair", func(t *testing.T) {
		t.Parallel()
		table := multiLLMTable(t.TempDir(), 1)
		runner := &shedfake.SeatRunner{
			LiveTables:    []seatengine.LiveTable{{Chair: liveChair}},
			ResumeResults: []seatengine.Result{{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, ChairOutputs: table.Chair().Outputs}},
		}
		prepared := false
		producer := NewMultiLLMProducerSourced("multi", func() (seatengine.Table, error) { return table, nil }, runner, multiLLMFixedNow, func() (string, error) {
			prepared = true
			return "amendment", nil
		})

		shedfake.RequireOutcome(t, producer, shedengine.Done)

		if prepared || runner.ResumeCalls != 1 {
			t.Errorf("preparation ran = %v, resume calls = %d, want it unrun on a resume", prepared, runner.ResumeCalls)
		}
		if len(runner.GotResumeTables) != 1 || runner.GotResumeTables[0].Amendment != "" {
			t.Errorf("resumed tables = %+v, want one carrying no amendment", runner.GotResumeTables)
		}
	})

	t.Run("the preparation's amendment rides the run's table on a copy and not the probe's or the source's", func(t *testing.T) {
		t.Parallel()
		table := multiLLMTable(t.TempDir(), 1)
		runner := &shedfake.SeatRunner{Results: []seatengine.Result{{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, ChairOutputs: table.Chair().Outputs}}}
		producer := NewMultiLLMProducerSourced("multi", func() (seatengine.Table, error) { return table, nil }, runner, multiLLMFixedNow, func() (string, error) {
			return "\nFresh start.\n", nil
		})

		shedfake.RequireOutcome(t, producer, shedengine.Done)

		if len(runner.GotTables) != 1 || runner.GotTables[0].Amendment != "\nFresh start.\n" {
			t.Errorf("run tables = %+v, want one carrying the preparation's amendment", runner.GotTables)
		}
		if len(runner.GotProbeTables) != 1 || runner.GotProbeTables[0].Amendment != "" || table.Amendment != "" {
			t.Errorf("probed table amendment = %+v and source table amendment = %q, want both empty", runner.GotProbeTables, table.Amendment)
		}
	})
}

func TestMultiLLMProducer_Cancellation(t *testing.T) {
	t.Parallel()
	done := seatengine.Result{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, ChairOutputs: []string{"/w/chair.md"}}
	died := seatengine.Result{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}, ChairOutputs: []string{"/w/chair.md"}}

	t.Run("a context cancelled at entry starts nothing", func(t *testing.T) {
		t.Parallel()
		runner := &shedfake.SeatRunner{}
		producer := NewMultiLLMProducer("multi", multiLLMTable(t.TempDir(), 1), runner, multiLLMFixedNow)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := producer.Call(ctx)

		if !errors.Is(err, context.Canceled) || runner.ProbeCalls != 0 || runner.Calls != 0 {
			t.Errorf("Call() = %v with probe/run calls %d/%d, want the context error and nothing started", err, runner.ProbeCalls, runner.Calls)
		}
	})

	tests := []struct {
		name        string
		result      seatengine.Result
		runErr      error
		wantOutcome shedengine.Outcome
		wantIs      error
	}{
		{name: "a cancelled run that died is the context error", result: died, wantIs: context.Canceled},
		{name: "a finished chair survives cancellation", result: done, wantOutcome: shedengine.Done},
		{name: "a cancelled run error is the context error", runErr: errors.New("boom"), wantIs: context.Canceled},
		{name: "a cancelled run that cannot stop a seat keeps its way forward", runErr: seatengine.ErrSeatNotStopped, wantIs: seatengine.ErrSeatNotStopped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			runner := &shedfake.SeatRunner{RunFn: func(seatengine.Table) (seatengine.Result, error) {
				cancel()
				return tt.result, tt.runErr
			}}
			producer := NewMultiLLMProducer("multi", multiLLMTable(t.TempDir(), 1), runner, multiLLMFixedNow)

			outcome, _, err := producer.Call(ctx)

			if !errors.Is(err, tt.wantIs) || outcome != tt.wantOutcome {
				t.Errorf("Call() = %q, %v, want outcome %q and an error wrapping %v", outcome, err, tt.wantOutcome, tt.wantIs)
			}
		})
	}
}

func TestMultiLLMProducer_MapsTheLeadResult(t *testing.T) {
	t.Parallel()
	one, two := 1, 2
	// The span times sit in a zone two hours east of UTC, so the wanted strings also pin the conversion to UTC.
	zone := time.FixedZone("east", 2*60*60)
	at := func(second, millisecond int) time.Time {
		return time.Date(2026, 8, 15, 11, 0, second, millisecond*int(time.Millisecond), zone)
	}
	tests := []struct {
		name string
		// result is the lead's result the fake seat runner reports.
		result       shuttleengine.Result
		wantOutcome  shedengine.Outcome
		wantAttempts *int
		wantSpans    []shedengine.GateSpan
		wantReason   string
		// wantErr is a substring of the error, and "" means no error.
		wantErr        string
		wantNotStarted bool
	}{
		{name: "done", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, wantOutcome: shedengine.Done},
		{
			name: "done with a passing gate",
			result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: true, Attempts: 1, Entries: []shuttleengine.GateEntryOutcome{
				{Name: "plan-parse", Spans: []shuttleengine.GateSpan{
					{Kind: shuttleengine.GateSpanGate, Start: at(0, 500), End: at(1, 0)},
					{Kind: shuttleengine.GateSpanHeld, Start: at(1, 0), End: at(5, 250)},
				}},
				{Name: "lint", Spans: []shuttleengine.GateSpan{{Kind: shuttleengine.GateSpanGate, Start: at(5, 250), End: at(6, 0)}}},
			}}},
			wantOutcome:  shedengine.Done,
			wantAttempts: &one,
			wantSpans: []shedengine.GateSpan{
				{Gate: "plan-parse", Kind: shedengine.GateSpanKindGate, Start: "2026-08-15T09:00:00.5Z", End: "2026-08-15T09:00:01Z"},
				{Gate: "plan-parse", Kind: shedengine.GateSpanKindHeld, Start: "2026-08-15T09:00:01Z", End: "2026-08-15T09:00:05.25Z"},
				{Gate: "lint", Kind: shedengine.GateSpanKindGate, Start: "2026-08-15T09:00:05.25Z", End: "2026-08-15T09:00:06Z"},
			},
		},
		{
			name:         "done with a gate whose entries recorded no spans",
			result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: true, Attempts: 1, Entries: []shuttleengine.GateEntryOutcome{{Name: "plan-parse"}}}},
			wantOutcome:  shedengine.Done,
			wantAttempts: &one,
		},
		{
			name: "done with a failing gate",
			result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: false, Attempts: 2, Reason: "the gate said no", Entries: []shuttleengine.GateEntryOutcome{
				{Name: "plan-parse", Spans: []shuttleengine.GateSpan{{Kind: shuttleengine.GateSpanGate, Start: at(0, 0), End: at(1, 0)}, {Kind: shuttleengine.GateSpanRework, Start: at(1, 0), End: at(9, 0)}}},
			}}},
			wantOutcome:  shedengine.Stuck,
			wantAttempts: &two,
			wantSpans: []shedengine.GateSpan{
				{Gate: "plan-parse", Kind: shedengine.GateSpanKindGate, Start: "2026-08-15T09:00:00Z", End: "2026-08-15T09:00:01Z"},
				{Gate: "plan-parse", Kind: shedengine.GateSpanKindRework, Start: "2026-08-15T09:00:01Z", End: "2026-08-15T09:00:09Z"},
			},
			wantReason: "the gate said no",
		},
		{name: "died", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}, wantErr: "shuttle run outcome died"},
		{name: "timeout", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}, wantErr: "shuttle run outcome timeout"},
		{
			name:           "a provider that never came up",
			result:         shuttleengine.Result{Outcome: shuttleengine.OutcomeDied, NotStarted: true},
			wantErr:        "shuttle run outcome died",
			wantNotStarted: true,
		},
		{
			name:           "a provider that never came up and reported why",
			result:         shuttleengine.Result{Outcome: shuttleengine.OutcomeDied, NotStarted: true, StartError: "no pane: tmux is gone"},
			wantErr:        "no pane: tmux is gone",
			wantNotStarted: true,
		},
		{name: "an unrecognized outcome", result: shuttleengine.Result{Outcome: "vanished"}, wantErr: `unrecognized shuttle outcome "vanished"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			table := multiLLMTable(t.TempDir(), 0)
			output := table.Chair().Outputs[0]
			producer := NewMultiLLMProducer("multi", table, &shedfake.SeatRunner{
				Results: []seatengine.Result{{Chair: tt.result, ChairOutputs: []string{output}}},
			}, multiLLMFixedNow)

			outcome, pointer, err := producer.Call(context.Background())

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Call() error = %v, want one containing %q", err, tt.wantErr)
				}
				if errors.Is(err, shuttleengine.ErrNotStarted) != tt.wantNotStarted {
					t.Errorf("Call() error = %v, want wrapping ErrNotStarted = %v", err, tt.wantNotStarted)
				}
				if outcome != "" || !reflect.DeepEqual(pointer, shedengine.OutputPointer{}) {
					t.Errorf("Call() = %q, %+v on error, want the zero outcome and pointer", outcome, pointer)
				}
				return
			}
			if err != nil {
				t.Fatalf("Call() error = %v, want nil", err)
			}
			want := shedengine.OutputPointer{Path: output, GateAttempts: tt.wantAttempts, GateSpans: tt.wantSpans, Reason: tt.wantReason}
			if outcome != tt.wantOutcome || !reflect.DeepEqual(pointer, want) {
				t.Errorf("Call() = %q, %+v, want %q, %+v", outcome, pointer, tt.wantOutcome, want)
			}
		})
	}
}

func TestMultiLLMProducer_ErrorExits(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	stuck := seatengine.ErrSeatNotStopped
	tests := []struct {
		name         string
		runner       func() *shedfake.SeatRunner
		build        func(table seatengine.Table, runner *shedfake.SeatRunner) *MultiLLMProducer
		wantIs       error
		wantArchived bool
		wantRun      int
		wantProbe    int
	}{
		{name: "a table source error", runner: func() *shedfake.SeatRunner { return &shedfake.SeatRunner{} }, build: func(_ seatengine.Table, runner *shedfake.SeatRunner) *MultiLLMProducer {
			return NewMultiLLMProducerSourced("multi", func() (seatengine.Table, error) { return seatengine.Table{}, boom }, runner, multiLLMFixedNow, nil)
		}, wantIs: boom},
		{name: "a preparation error", runner: func() *shedfake.SeatRunner { return &shedfake.SeatRunner{} }, build: func(table seatengine.Table, runner *shedfake.SeatRunner) *MultiLLMProducer {
			return NewMultiLLMProducerSourced("multi", func() (seatengine.Table, error) { return table, nil }, runner, multiLLMFixedNow, func() (string, error) { return "", boom })
		}, wantIs: boom, wantProbe: 1},
		{name: "a probe error", runner: func() *shedfake.SeatRunner { return &shedfake.SeatRunner{ProbeErrs: []error{boom}} }, wantIs: boom, wantProbe: 1},
		{name: "a probe that cannot stop a seat", runner: func() *shedfake.SeatRunner { return &shedfake.SeatRunner{ProbeErrs: []error{stuck}} }, wantIs: stuck, wantProbe: 1},
		{name: "a resume error", runner: func() *shedfake.SeatRunner {
			return &shedfake.SeatRunner{LiveTables: []seatengine.LiveTable{{Chair: liveChair}}, ResumeErrs: []error{boom}}
		}, wantIs: boom, wantProbe: 1},
		{name: "a run error", runner: func() *shedfake.SeatRunner { return &shedfake.SeatRunner{Errs: []error{boom}} }, wantIs: boom, wantArchived: true, wantRun: 1, wantProbe: 1},
		{name: "a run that cannot stop a seat", runner: func() *shedfake.SeatRunner { return &shedfake.SeatRunner{Errs: []error{stuck}} }, wantIs: stuck, wantArchived: true, wantRun: 1, wantProbe: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			table := multiLLMTable(t.TempDir(), 1)
			paths := writeTableFiles(t, table)
			runner := tt.runner()
			producer := NewMultiLLMProducer("multi", table, runner, multiLLMFixedNow)
			if tt.build != nil {
				producer = tt.build(table, runner)
			}

			_, _, err := producer.Call(context.Background())

			if !errors.Is(err, tt.wantIs) || !strings.Contains(err.Error(), "multi (seats)") {
				t.Errorf("Call() error = %v, want it to wrap %v and name the producer and engine", err, tt.wantIs)
			}
			if runner.Calls != tt.wantRun || runner.ProbeCalls != tt.wantProbe {
				t.Errorf("Run/Probe calls = %d/%d, want %d/%d", runner.Calls, runner.ProbeCalls, tt.wantRun, tt.wantProbe)
			}
			for _, path := range paths {
				_, statErr := os.Stat(path)
				if archived := errors.Is(statErr, os.ErrNotExist); archived != tt.wantArchived {
					t.Errorf("output %s archived = %v, want %v", path, archived, tt.wantArchived)
				}
			}
		})
	}
}
