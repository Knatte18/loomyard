//go:build tier0

// guardedrows_test.go drives Finalize, the row that holds the parent lock, against fake tip reads and a real temporary lock store:
// the lock's release at the edges that end the row's hold, and the halt of a paused lock wait.

package landingshed

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// guardedRowFixture is one lock-taking row built over deps, with a report of whether it did any work beyond taking the lock.
type guardedRowFixture struct {
	producer shedengine.ShedProducer
	// touched reports whether the row merged anything.
	touched func() bool
}

// guardedRow is one lock-taking row by its row name.
// build is handed deps after the test has set its tip reads and lock seams.
type guardedRow struct {
	name  string
	build func(t *testing.T, deps Deps) guardedRowFixture
}

// guardedRows lists the lock-taking rows, each built by the literal its own tests use.
var guardedRows = []guardedRow{
	{finalizeName, func(t *testing.T, deps Deps) guardedRowFixture {
		res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
		merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
		fz := &Finalize{deps: deps, resolver: res, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}
		return guardedRowFixture{producer: fz, touched: func() bool { return res.called || len(merger.calls) > 0 }}
	}},
}

// guardedOwnerOther is a run other than the one newTestDeps names, holding the lock in the waiting cases.
var guardedOwnerOther = parentlock.Owner{Slug: "other", Pair: "/p/other", RunID: "r9", PID: 111, StartTime: "t1"}

// TestGuardedRows_PausedLockWaitHalts pins that a pause during the wait for a lock another run holds is a halting Stuck at each row, naming the goto back to that row, with no work done.
func TestGuardedRows_PausedLockWaitHalts(t *testing.T) {
	for _, row := range guardedRows {
		t.Run(row.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Lock.PauseRequested = func() (bool, error) { return true, nil }
			if _, err := deps.Lock.Store.TryAcquire("main", guardedOwnerOther); err != nil {
				t.Fatal(err)
			}
			fx := row.build(t, deps)

			outcome, ptr, err := fx.producer.Call(context.Background())

			if err != nil || outcome != shedengine.Stuck || !ptr.Halt || ptr.Reroute {
				t.Fatalf("Call() = %q, %+v, %v; want a halting Stuck that does not reroute", outcome, ptr, err)
			}
			if want := `way forward: run "lym loom goto --to ` + row.name + `", then "lym loom resume"`; !strings.Contains(ptr.Reason, want) {
				t.Errorf("reason = %q; want it to contain %q", ptr.Reason, want)
			}
			if fx.touched() {
				t.Error("the row did work after a paused lock wait")
			}
		})
	}
}

// TestGuardedRows_LockLifecycle pins when the lock leaves the run.
// Finalize releases it on Done, whether it landed fresh or found the parent already holding the task's work, and on a Stuck or an error.
func TestGuardedRows_LockLifecycle(t *testing.T) {
	t.Run("Finalize releases the lock on every way out: Done, a Stuck and an error", func(t *testing.T) {
		tests := []struct {
			name        string
			setup       func(deps *Deps, res *recordingResolver, merger *recordingParentMerger)
			wantOutcome shedengine.Outcome
			wantReroute bool
			wantErr     bool
		}{
			{"a fresh landing is Done", func(*Deps, *recordingResolver, *recordingParentMerger) {}, shedengine.Done, false, false},
			{"a parent that already holds the task's work is Done", func(deps *Deps, _ *recordingResolver, _ *recordingParentMerger) {
				deps.ParentHoldsTaskWork = func() (bool, error) { return true, nil }
			}, shedengine.Done, false, false},
			{"a merge-in the resolver cannot finish is a Stuck", func(_ *Deps, res *recordingResolver, _ *recordingParentMerger) {
				res.result = mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "conflict could not be resolved"}
			}, shedengine.Stuck, false, false},
			{"a failed origin reconcile is an error", func(deps *Deps, _ *recordingResolver, _ *recordingParentMerger) {
				deps.Reconcile = func() (fabricengine.OriginReport, error) {
					return fabricengine.OriginReport{}, errors.New("ls-remote failed")
				}
			}, "", false, true},
		}
		for _, tt := range tests {
			deps := newTestDeps(t)
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
			tt.setup(&deps, res, merger)
			fz := &Finalize{deps: deps, resolver: res, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

			outcome, ptr, err := fz.Call(context.Background())

			if (err != nil) != tt.wantErr || outcome != tt.wantOutcome || ptr.Reroute != tt.wantReroute {
				t.Fatalf("%s: Call() = %q, %+v, %v; want %q with reroute %v and error %v", tt.name, outcome, ptr, err, tt.wantOutcome, tt.wantReroute, tt.wantErr)
			}
			if rec, found, _ := deps.Lock.Store.Read("main"); found {
				t.Errorf("%s: lock record = %+v after the Call; want none", tt.name, rec)
			}
		}
	})
}
