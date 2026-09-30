// finalize_verify_test.go covers the post-merge verify gate as Finalize.mergeInStep wires it after
// every parent merge-in, against a fake runner, a scripted resolver and the package's fake merger.

package landingshed

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// scriptedResolver returns results[i] on the i-th Resolve call, clamped to the last entry.
type scriptedResolver struct {
	results []mergeresolve.Result
	calls   int
}

func (r *scriptedResolver) Resolve(ctx context.Context, source string) (mergeresolve.Result, error) {
	idx := r.calls
	r.calls++
	if idx >= len(r.results) {
		idx = len(r.results) - 1
	}
	return r.results[idx], nil
}

// finalizeVerifyFixture is a Finalize over a real gate with a fake runner.
type finalizeVerifyFixture struct {
	fz         *Finalize
	gate       *gateFixture
	merger     *recordingParentMerger
	markedDone int
}

func newFinalizeVerifyFixture(t *testing.T, res resolver, results ...mergeCallResult) *finalizeVerifyFixture {
	t.Helper()
	fx := &finalizeVerifyFixture{gate: newGateFixture(t, "go test ./...", nil)}
	deps := newFinalizeDeps(t)
	deps.MarkTaskDone = func() error { fx.markedDone++; return nil }
	if len(results) == 0 {
		results = []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}
	}
	fx.merger = &recordingParentMerger{results: results}
	fx.fz = &Finalize{
		deps:         deps,
		resolver:     res,
		gate:         fx.gate.gate,
		parentOpener: func() (parentMerger, error) { return fx.merger, nil },
	}
	return fx
}

func resolved(alreadyUpToDate bool) mergeresolve.Result {
	return mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved, AlreadyUpToDate: alreadyUpToDate}
}

// requireNotLanded fails the test when the parent-side merge, push or board update ran.
func (fx *finalizeVerifyFixture) requireNotLanded(t *testing.T) {
	t.Helper()
	if len(fx.merger.calls) != 0 || len(fx.merger.pushCalls) != 0 {
		t.Errorf("merge calls = %d, push calls = %d; want none after a failed verify", len(fx.merger.calls), len(fx.merger.pushCalls))
	}
	if fx.markedDone != 0 {
		t.Errorf("MarkTaskDone called %d time(s); want never", fx.markedDone)
	}
}

func TestFinalizeVerify_TreeChangedFail(t *testing.T) {
	fx := newFinalizeVerifyFixture(t, &recordingResolver{result: resolved(false)})
	fx.gate.fake.code = 1
	outcome, ptr, err := fx.fz.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = (%q, %v); want (Stuck, nil)", outcome, err)
	}
	requireFinalizeReason(t, ptr)
	fx.requireNotLanded(t)
	if !fx.gate.markerExists(t) {
		t.Error("marker missing after a failed verify")
	}
}

func TestFinalizeVerify_TreeChangedPass(t *testing.T) {
	fx := newFinalizeVerifyFixture(t, &recordingResolver{result: resolved(false)})
	outcome, _, err := fx.fz.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = (%q, %v); want (Done, nil)", outcome, err)
	}
	if len(fx.merger.calls) != 1 || len(fx.merger.pushCalls) != 1 {
		t.Errorf("merge calls = %d, push calls = %d; want 1 each", len(fx.merger.calls), len(fx.merger.pushCalls))
	}
	if fx.gate.markerExists(t) {
		t.Error("marker present after a passing verify")
	}
}

func TestFinalizeVerify_UpToDateWithMarkerFail(t *testing.T) {
	fx := newFinalizeVerifyFixture(t, &recordingResolver{result: resolved(true)})
	fx.gate.seedMarker(t)
	fx.gate.fake.code = 1
	outcome, _, err := fx.fz.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = (%q, %v); want (Stuck, nil)", outcome, err)
	}
	if fx.gate.fake.calls != 1 {
		t.Errorf("runner calls = %d; want 1", fx.gate.fake.calls)
	}
	fx.requireNotLanded(t)
	if !fx.gate.markerExists(t) {
		t.Error("marker missing after a failed verify")
	}
}

func TestFinalizeVerify_UpToDateWithMarkerPass(t *testing.T) {
	fx := newFinalizeVerifyFixture(t, &recordingResolver{result: resolved(true)})
	fx.gate.seedMarker(t)
	outcome, _, err := fx.fz.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = (%q, %v); want (Done, nil)", outcome, err)
	}
	if fx.gate.fake.calls != 1 {
		t.Errorf("runner calls = %d; want 1", fx.gate.fake.calls)
	}
	if fx.gate.markerExists(t) {
		t.Error("marker present after a passing verify")
	}
	if len(fx.merger.calls) != 1 {
		t.Errorf("merge calls = %d; want 1", len(fx.merger.calls))
	}
}

// TestFinalizeVerify_RetryMergeInRunsGateAgain pins that the merge-in-required retry's second
// merge-in is verified too: the first verify passes, the second fails, and nothing lands.
func TestFinalizeVerify_RetryMergeInRunsGateAgain(t *testing.T) {
	res := &scriptedResolver{results: []mergeresolve.Result{resolved(false), resolved(false)}}
	fx := newFinalizeVerifyFixture(t, res, mergeCallResult{err: &fabricengine.ErrMergeInRequired{}})
	fx.gate.fake.onRun = func() {
		if fx.gate.fake.calls == 2 {
			fx.gate.fake.code = 1
		}
	}
	outcome, _, err := fx.fz.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = (%q, %v); want (Stuck, nil)", outcome, err)
	}
	if fx.gate.fake.calls != 2 {
		t.Errorf("runner calls = %d; want 2", fx.gate.fake.calls)
	}
	if len(fx.merger.calls) != 1 || len(fx.merger.pushCalls) != 0 || fx.markedDone != 0 {
		t.Errorf("merge calls = %d, push calls = %d, marked done = %d; want 1, 0, 0", len(fx.merger.calls), len(fx.merger.pushCalls), fx.markedDone)
	}
	if !fx.gate.markerExists(t) {
		t.Error("marker missing after a failed verify")
	}
}
