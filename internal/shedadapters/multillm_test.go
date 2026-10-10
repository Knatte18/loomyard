package shedadapters

import (
	"context"
	"errors"
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

// multiLLMTable returns a chair with the given number of advisors, whose files live under dir.
func multiLLMTable(dir string, advisors int) seatengine.Table {
	table := seatengine.Table{
		RolePrefix: "multi",
		Seats:      []seatengine.Seat{{Name: seatengine.RoleChair, Outputs: []string{filepath.Join(dir, "chair.md")}}},
	}
	for n := 1; n <= advisors; n++ {
		output := filepath.Join(dir, seatengine.AdvisorName(n)+".md")
		table.Seats[0].Inputs = append(table.Seats[0].Inputs, output)
		table.Seats = append(table.Seats, seatengine.Seat{Name: seatengine.AdvisorName(n), Outputs: []string{output}})
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
		prepare := func() error {
			probesWhenPrepared = runner.ProbeCalls
			for _, path := range paths {
				_, err := os.Stat(path)
				staleWhenPrepared = append(staleWhenPrepared, err == nil)
			}
			return nil
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
		producer := NewMultiLLMProducerSourced("multi", func() (seatengine.Table, error) { return table, nil }, runner, multiLLMFixedNow, func() error {
			prepared = true
			return nil
		})

		shedfake.RequireOutcome(t, producer, shedengine.Done)

		if prepared || runner.ResumeCalls != 1 {
			t.Errorf("preparation ran = %v, resume calls = %d, want it unrun on a resume", prepared, runner.ResumeCalls)
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

func TestMultiLLMProducer_MapsTheChairResultAsSingleLLMDoes(t *testing.T) {
	t.Parallel()
	gateFailed := &shuttleengine.GateOutcome{Passed: false, Attempts: 2, Reason: "the gate said no"}
	tests := []struct {
		name   string
		result shuttleengine.Result
	}{
		{name: "done", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}},
		{name: "done with a passing gate", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: true, Attempts: 1}}},
		{name: "done with a failing gate", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: gateFailed}},
		{name: "died", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}},
		{name: "timeout", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}},
		{name: "a provider that never came up", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied, NotStarted: true}},
		{name: "an unrecognized outcome", result: shuttleengine.Result{Outcome: "vanished"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			table := multiLLMTable(dir, 0)
			output := table.Chair().Outputs[0]

			single := NewSingleLLMProducer("single", func() (shuttleengine.Spec, error) {
				return shuttleengine.Spec{Prompt: "p", OutputFiles: []string{output}}, nil
			}, &shedfake.Shuttle{Result: tt.result}, multiLLMFixedNow, nil)
			multi := NewMultiLLMProducer("multi", table, &shedfake.SeatRunner{
				Results: []seatengine.Result{{Chair: tt.result, ChairOutputs: []string{output}}},
			}, multiLLMFixedNow)

			wantOutcome, wantPtr, wantErr := single.Call(context.Background())
			gotOutcome, gotPtr, gotErr := multi.Call(context.Background())

			if gotOutcome != wantOutcome || !reflect.DeepEqual(gotPtr, wantPtr) {
				t.Errorf("multi Call() = %q, %+v, want SingleLLMProducer's %q, %+v", gotOutcome, gotPtr, wantOutcome, wantPtr)
			}
			if (gotErr == nil) != (wantErr == nil) || errors.Is(gotErr, shuttleengine.ErrNotStarted) != errors.Is(wantErr, shuttleengine.ErrNotStarted) {
				t.Errorf("multi Call() error = %v, want the shape of SingleLLMProducer's %v", gotErr, wantErr)
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
			return NewMultiLLMProducerSourced("multi", func() (seatengine.Table, error) { return table, nil }, runner, multiLLMFixedNow, func() error { return boom })
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
