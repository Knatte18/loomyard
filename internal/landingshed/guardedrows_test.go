// guardedrows_test.go drives Publish, PR-Gate and Finalize, the rows that hold the parent lock, against fake tip reads and a real temporary lock store:
// the reroute on a moved parent, the lock's release at the edges that leave the Merge-In-to-Finalize window, and the halt of a paused lock wait.
// None of its tests runs in parallel: each builds its Deps through newTestDeps, which swaps the package-level NewGitHubClient.

package landingshed

import (
	"context"
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
	// touched reports whether the row pushed, reached GitHub or merged anything.
	touched func() bool
}

// guardedRow is one lock-taking row by its row name.
// build is handed deps after the test has set its tip reads and lock seams.
type guardedRow struct {
	name  string
	build func(t *testing.T, deps Deps) guardedRowFixture
}

// guardedRowNamed returns the guardedRows entry named name.
func guardedRowNamed(name string) guardedRow {
	for _, row := range guardedRows {
		if row.name == name {
			return row
		}
	}
	panic("no guarded row named " + name)
}

// guardedRows lists the three lock-taking rows, each built by the constructor or the literal its own tests use.
var guardedRows = []guardedRow{
	{publishName, func(t *testing.T, deps Deps) guardedRowFixture {
		pushed := false
		deps.PushBranch = func() error { pushed = true; return nil }
		p, err := NewPublish(deps)
		if err != nil {
			t.Fatalf("NewPublish() error = %v", err)
		}
		failOnGitHubClient(t)
		return guardedRowFixture{producer: p, touched: func() bool { return pushed }}
	}},
	{prGateName, func(t *testing.T, deps Deps) guardedRowFixture {
		deps.ApprovalPath = "approval.json"
		deps.RejectionPath = "rejection.json"
		g, err := NewPRGate(deps)
		if err != nil {
			t.Fatalf("NewPRGate() error = %v", err)
		}
		failOnGitHubClient(t)
		return guardedRowFixture{producer: g, touched: func() bool { return false }}
	}},
	{finalizeName, func(t *testing.T, deps Deps) guardedRowFixture {
		res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
		merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
		fz := &Finalize{deps: deps, resolver: res, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}
		return guardedRowFixture{producer: fz, touched: func() bool { return res.called || len(merger.calls) > 0 }}
	}},
}

// guardedOwnerOther is a run other than the one newTestDeps names, holding the lock in the waiting cases.
var guardedOwnerOther = parentlock.Owner{Slug: "other", Pair: "/p/other", RunID: "r9", PID: 111, StartTime: "t1"}

// TestGuardedRows_MovedParentReroutes pins that a moved parent at each lock-taking row is a reroute Stuck, which carries no halt and so spends no bounce budget.
// It also pins that the row has done no work beyond taking the lock, and that it keeps the lock it took.
func TestGuardedRows_MovedParentReroutes(t *testing.T) {
	for _, row := range guardedRows {
		t.Run(row.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.ReadParentTips = func(bool) (fabricengine.ParentTips, error) {
				return fabricengine.ParentTips{Local: "newer-tip", Upstream: "newer-tip", Relation: fabricengine.ParentEqual}, nil
			}
			fx := row.build(t, deps)

			outcome, ptr, err := fx.producer.Call(context.Background())

			if err != nil || outcome != shedengine.Stuck || !ptr.Reroute || ptr.Halt {
				t.Fatalf("Call() = %q, %+v, %v; want a reroute Stuck with no halt", outcome, ptr, err)
			}
			if !strings.Contains(ptr.Reason, "moved since Merge-In merged") {
				t.Errorf("reason = %q; want it to say the parent moved since Merge-In merged it", ptr.Reason)
			}
			if fx.touched() {
				t.Error("the row pushed, reached GitHub or merged after finding the parent moved")
			}
			if rec, found, _ := deps.Lock.Store.Read("main"); !found || rec.Owner.Slug != "me" {
				t.Errorf("lock record = %+v, %v; want it held by the run", rec, found)
			}
		})
	}
}

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
			if want := `way forward: run "lyx loom goto --to ` + row.name + `", then "lyx loom resume"`; !strings.Contains(ptr.Reason, want) {
				t.Errorf("reason = %q; want it to contain %q", ptr.Reason, want)
			}
			if fx.touched() {
				t.Error("the row did work after a paused lock wait")
			}
		})
	}
}

// TestGuardedRows_LockLifecycle pins when the lock leaves the run.
// PR-Gate keeps it through an approval and releases it on a matching rejection.
// Finalize releases it on Done, whether it landed fresh or found the parent already holding the task's work.
func TestGuardedRows_LockLifecycle(t *testing.T) {
	t.Run("PR-Gate keeps the lock on approval and releases it on rejection", func(t *testing.T) {
		for _, rejected := range []bool{false, true} {
			f := newPRGateFixture(t, openPR(7, prGateHead))
			if rejected {
				writeRejectionAt(t, f.deps.RejectionPath, 7, prGateHead)
			} else {
				writeApprovalAt(t, f.deps.ApprovalPath, 7, prGateHead)
			}

			outcome, _, err := f.call(t)

			wantOutcome := shedengine.Done
			if rejected {
				wantOutcome = shedengine.Stuck
			}
			if err != nil || outcome != wantOutcome {
				t.Fatalf("rejected=%v: Call() = %q, %v; want %q", rejected, outcome, err, wantOutcome)
			}
			if _, found, _ := f.deps.Lock.Store.Read("main"); found == rejected {
				t.Errorf("rejected=%v: lock record found = %v; want it kept on approval and freed on rejection", rejected, found)
			}
		}
	})

	t.Run("Finalize releases the lock on Done, fresh or already landed", func(t *testing.T) {
		tests := []struct {
			name      string
			holdsWork bool
			tip       string
		}{
			{"a fresh landing", false, testParentTip},
			{"a parent that already holds the task's work reads as not moved", true, "landed-tip"},
		}
		for _, tt := range tests {
			deps := newTestDeps(t)
			deps.HoldsWork = func(string, string) (bool, error) { return tt.holdsWork, nil }
			deps.ReadParentTips = func(bool) (fabricengine.ParentTips, error) {
				return fabricengine.ParentTips{Local: tt.tip, Upstream: tt.tip, Relation: fabricengine.ParentEqual}, nil
			}
			fx := guardedRowNamed(finalizeName).build(t, deps)

			outcome, ptr, err := fx.producer.Call(context.Background())

			if err != nil || outcome != shedengine.Done || ptr.Reroute {
				t.Fatalf("%s: Call() = %q, %+v, %v; want Done with no reroute", tt.name, outcome, ptr, err)
			}
			if rec, found, _ := deps.Lock.Store.Read("main"); found {
				t.Errorf("%s: lock record = %+v after Done; want none", tt.name, rec)
			}
		}
	})
}
