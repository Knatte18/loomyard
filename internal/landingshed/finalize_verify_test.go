// finalize_verify_test.go covers the clean-tree checks and the post-merge verify gate as Finalize.mergeInStep wires them around every parent merge-in,
// against fake verifytree seams, a scripted resolver and the package's fake merger.

package landingshed

import (
	"context"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// The clean-tree checks one mergeInStep makes, by their zero-based position within it.
const (
	finalizeCleanBeforeMergeIn = iota
	finalizeCleanAfterMergeIn
	finalizeCleanAfterVerify
	finalizeChecksPerMergeIn
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

// finalizeVerifyFixture is a Finalize over a real gate with fake verifytree seams.
type finalizeVerifyFixture struct {
	fz           *Finalize
	gate         *gateFixture
	merger       *recordingParentMerger
	markedDone   int
	openerCalled int
}

func newFinalizeVerifyFixture(t *testing.T, res resolver, results ...mergeCallResult) *finalizeVerifyFixture {
	t.Helper()
	fx := &finalizeVerifyFixture{gate: newGateFixture(t, "go test ./...", nil)}
	deps := newTestDeps(t)
	deps.MarkTaskDone = func() error { fx.markedDone++; return nil }
	if len(results) == 0 {
		results = []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}
	}
	fx.merger = &recordingParentMerger{results: results}
	fx.fz = &Finalize{
		deps:     deps,
		resolver: res,
		gate:     fx.gate.gate,
		parentOpener: func() (parentMerger, error) {
			fx.openerCalled++
			return fx.merger, nil
		},
	}
	return fx
}

func resolved(alreadyUpToDate bool) mergeresolve.Result {
	return mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved, AlreadyUpToDate: alreadyUpToDate}
}

// requireNotLanded fails the test when the parent opener, the parent-side merge, the push or the board update ran.
func (fx *finalizeVerifyFixture) requireNotLanded(t *testing.T) {
	t.Helper()
	if fx.openerCalled != 0 || len(fx.merger.calls) != 0 || len(fx.merger.pushCalls) != 0 {
		t.Errorf("opener calls = %d, merge calls = %d, push calls = %d; want none after a halted gate", fx.openerCalled, len(fx.merger.calls), len(fx.merger.pushCalls))
	}
	if fx.markedDone != 0 {
		t.Errorf("MarkTaskDone called %d time(s); want never", fx.markedDone)
	}
}

func TestFinalizeVerify_Pass(t *testing.T) {
	fx := newFinalizeVerifyFixture(t, &recordingResolver{result: resolved(false)})
	shedfake.RequireOutcome(t, fx.fz, shedengine.Done)
	if len(fx.merger.calls) != 1 || len(fx.merger.pushCalls) != 1 {
		t.Errorf("merge calls = %d, push calls = %d; want 1 each", len(fx.merger.calls), len(fx.merger.pushCalls))
	}
	if fx.gate.fake.verifyCalls != 1 || fx.gate.fake.site.Label != "Finalize" {
		t.Errorf("verify calls = %d, site = %+v; want 1 call at site Finalize", fx.gate.fake.verifyCalls, fx.gate.fake.site)
	}
	if fx.gate.fake.dirtyCalls != finalizeChecksPerMergeIn {
		t.Errorf("clean-tree checks = %d; want %d", fx.gate.fake.dirtyCalls, finalizeChecksPerMergeIn)
	}
}

func TestFinalizeVerify_Fail(t *testing.T) {
	fx := newFinalizeVerifyFixture(t, &recordingResolver{result: resolved(false)})
	fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
	reason := requireFinalizeReason(t, shedfake.RequireOutcome(t, fx.fz, shedengine.Stuck))
	if !strings.Contains(reason, "exit code 1") || !strings.Contains(reason, fx.gate.paths.Log) {
		t.Errorf("reason %q lacks the exit code or the log path", reason)
	}
	fx.requireNotLanded(t)
}

// TestFinalizeVerify_VerifiedTreeSkipsAndLands pins that a verify the record skips still lets the landing proceed.
func TestFinalizeVerify_VerifiedTreeSkipsAndLands(t *testing.T) {
	fx := newFinalizeVerifyFixture(t, &recordingResolver{result: resolved(true)})
	fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusSkipped}
	shedfake.RequireOutcome(t, fx.fz, shedengine.Done)
	if len(fx.merger.calls) != 1 {
		t.Errorf("merge calls = %d; want 1", len(fx.merger.calls))
	}
}

// TestFinalizeVerify_DirtyTreeHalts pins that a dirty tree at each of the three points ends Stuck naming the paths,
// before the parent opener is called.
func TestFinalizeVerify_DirtyTreeHalts(t *testing.T) {
	cases := []struct {
		name       string
		point      int
		wantPoint  string
		wantResolv int
		wantVerify int
	}{
		{"before the merge-in", finalizeCleanBeforeMergeIn, "before the merge-in", 0, 0},
		{"after the merge-in", finalizeCleanAfterMergeIn, "after the merge-in", 1, 0},
		{"after the verify", finalizeCleanAfterVerify, "after the verify", 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := &scriptedResolver{results: []mergeresolve.Result{resolved(false)}}
			fx := newFinalizeVerifyFixture(t, res)
			fx.gate.fake.dirtyAt(tc.point, "stray.txt", "gen/out.go")

			reason := requireFinalizeReason(t, shedfake.RequireOutcome(t, fx.fz, shedengine.Stuck))
			for _, want := range []string{tc.wantPoint, "stray.txt", "gen/out.go"} {
				if !strings.Contains(reason, want) {
					t.Errorf("reason %q lacks %q", reason, want)
				}
			}
			fx.requireNotLanded(t)
			if res.calls != tc.wantResolv {
				t.Errorf("resolver calls = %d; want %d", res.calls, tc.wantResolv)
			}
			if fx.gate.fake.verifyCalls != tc.wantVerify {
				t.Errorf("verify calls = %d; want %d", fx.gate.fake.verifyCalls, tc.wantVerify)
			}
		})
	}
}

// TestFinalizeVerify_RetryMergeInRunsGateAgain pins that the merge-in-required retry's second merge-in is verified too:
// the first verify passes, the second fails, and nothing lands.
func TestFinalizeVerify_RetryMergeInRunsGateAgain(t *testing.T) {
	res := &scriptedResolver{results: []mergeresolve.Result{resolved(false), resolved(false)}}
	fx := newFinalizeVerifyFixture(t, res, mergeCallResult{err: &fabricengine.ErrMergeInRequired{}})
	fx.gate.fake.onVerify = func() {
		if fx.gate.fake.verifyCalls == 2 {
			fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
		}
	}
	shedfake.RequireOutcome(t, fx.fz, shedengine.Stuck)
	if fx.gate.fake.verifyCalls != 2 {
		t.Errorf("verify calls = %d; want 2", fx.gate.fake.verifyCalls)
	}
	if len(fx.merger.calls) != 1 || len(fx.merger.pushCalls) != 0 || fx.markedDone != 0 {
		t.Errorf("merge calls = %d, push calls = %d, marked done = %d; want 1, 0, 0", len(fx.merger.calls), len(fx.merger.pushCalls), fx.markedDone)
	}
}

// TestFinalizeVerify_RetryMergeInRunsCleanChecksAgain pins that the retry merge-in is bracketed by the same three clean-tree checks.
func TestFinalizeVerify_RetryMergeInRunsCleanChecksAgain(t *testing.T) {
	res := &scriptedResolver{results: []mergeresolve.Result{resolved(false), resolved(false)}}
	fx := newFinalizeVerifyFixture(t, res, mergeCallResult{err: &fabricengine.ErrMergeInRequired{}})
	fx.gate.fake.dirtyAt(finalizeChecksPerMergeIn+finalizeCleanAfterMergeIn, "retry-stray.txt")

	reason := requireFinalizeReason(t, shedfake.RequireOutcome(t, fx.fz, shedengine.Stuck))
	if !strings.Contains(reason, "retry-stray.txt") || !strings.Contains(reason, "after the merge-in") {
		t.Errorf("reason %q lacks the retry's dirty path or point", reason)
	}
	if res.calls != 2 || len(fx.merger.calls) != 1 || len(fx.merger.pushCalls) != 0 || fx.markedDone != 0 {
		t.Errorf("resolver calls = %d, merge calls = %d, push calls = %d, marked done = %d; want 2, 1, 0, 0", res.calls, len(fx.merger.calls), len(fx.merger.pushCalls), fx.markedDone)
	}
}
