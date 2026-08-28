// finalize_test.go covers Finalize against a faked resolver and a faked parent-pair opener closure
// returning a scripted merge outcome. This tier lives in the package itself too, for the same
// reason as its sibling: the resolver seam and the parentMerger seam it substitutes are both
// unexported.

package landingshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// recordingParentMerger is the in-package fake standing in for the unexported parentMerger seam. It
// records every call's source/opts, and returns the scripted result/err at position len(calls)
// (clamped to the last entry), so a test can script a first-attempt failure followed by a
// second-attempt success.
type recordingParentMerger struct {
	calls   []mergeCall
	results []mergeCallResult
}

type mergeCall struct {
	source string
	opts   fabricengine.MergeOptions
}

type mergeCallResult struct {
	result fabricengine.MergeResult
	err    error
}

func (m *recordingParentMerger) Merge(source string, opts fabricengine.MergeOptions) (fabricengine.MergeResult, error) {
	idx := len(m.calls)
	m.calls = append(m.calls, mergeCall{source: source, opts: opts})
	if idx >= len(m.results) {
		idx = len(m.results) - 1
	}
	if idx < 0 {
		return fabricengine.MergeResult{}, nil
	}
	return m.results[idx].result, m.results[idx].err
}

// newFinalizeDeps returns a minimal Deps for a Finalize test, with a well-formed final-summary
// artifact already written at FinalSummaryPath -- Call's own top-of-Call parse (see finalize.go's
// step 1a) requires one to exist for every test that does not override this field itself.
func newFinalizeDeps(t *testing.T) Deps {
	t.Helper()
	summaryPath := summaryparser.Path(t.TempDir())
	writeSummary(t, summaryPath, "A landing title", "A landing body.")
	return Deps{
		WorktreeRoot:     t.TempDir(),
		TaskBranch:       "task-branch",
		ParentBranch:     "main",
		FinalSummaryPath: summaryPath,
		ScratchDir:       filepath.Join(t.TempDir(), "scratch"),
		Config: Config{
			RequirePRToBase:    []string{"main"},
			Squash:             true,
			Conflict:           "sonnet",
			ConflictTimeoutMin: 30,
		},
	}
}

func readFinalizeStuckFile(t *testing.T, scratchDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scratchDir, finalizeName+stuckFileSuffix))
	if err != nil {
		t.Fatalf("read stuck file: %v", err)
	}
	return string(data)
}

// --- NewFinalize construction ---

func TestNewFinalize_RejectsNilOpenFabric(t *testing.T) {
	deps := newFinalizeDeps(t)
	deps.OpenParentFabric = func() (*fabricengine.Fabric, error) { return nil, nil }
	if _, err := NewFinalize(deps); err == nil {
		t.Fatal("NewFinalize() error = nil; want an error naming Deps.OpenFabric")
	}
}

func TestNewFinalize_RejectsNilOpenParentFabric(t *testing.T) {
	deps := newFinalizeDeps(t)
	deps.OpenFabric = func() (*fabricengine.Fabric, error) { return nil, nil }
	if _, err := NewFinalize(deps); err == nil {
		t.Fatal("NewFinalize() error = nil; want an error naming Deps.OpenParentFabric")
	}
}

// --- Call behaviour ---

func TestFinalize_HappyPath_MergeInThenParentMerge(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if !res.called {
		t.Error("resolver.Resolve was not called; want merge-in to run first")
	}
	if len(merger.calls) != 1 {
		t.Fatalf("parent-side merge calls = %d; want 1", len(merger.calls))
	}
	if merger.calls[0].source != deps.TaskBranch {
		t.Errorf("parent-side merge source = %q; want %q", merger.calls[0].source, deps.TaskBranch)
	}
	if !merger.calls[0].opts.Squash {
		t.Error("parent-side merge Squash = false; want true (threaded from configuration)")
	}
}

func TestFinalize_MergeInRequired_RetriesExactlyOnce(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{
		{err: &fabricengine.ErrMergeInRequired{Source: deps.TaskBranch}},
		{err: &fabricengine.ErrMergeInRequired{Source: deps.TaskBranch}},
	}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if len(merger.calls) != 2 {
		t.Fatalf("parent-side merge calls = %d; want exactly 2 (one attempt, one retry)", len(merger.calls))
	}
	readFinalizeStuckFile(t, deps.ScratchDir)
}

func TestFinalize_ParentOpenerError_StuckNamesParentBranch(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return nil, errors.New("no such worktree") }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	got := readFinalizeStuckFile(t, deps.ScratchDir)
	if !strings.Contains(got, deps.ParentBranch) {
		t.Errorf("stuck reason %q does not name parent branch %q", got, deps.ParentBranch)
	}
}

func TestFinalize_GuardErrorDirtyWorktree_StuckSurfacesReasonVerbatim(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	guardErr := &fabricengine.MergeGuardError{Reasons: []string{"worktree dirty"}}
	merger := &recordingParentMerger{results: []mergeCallResult{{err: guardErr}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if len(merger.calls) != 1 {
		t.Errorf("parent-side merge calls = %d; want exactly 1 (no retry on a dirty-worktree guard error)", len(merger.calls))
	}
	got := readFinalizeStuckFile(t, deps.ScratchDir)
	if !strings.Contains(got, guardErr.Error()) {
		t.Errorf("stuck reason %q does not surface the guard error verbatim (%q)", got, guardErr.Error())
	}
}

func TestFinalize_UnrecognizedMergeError_StuckWithErrorSurfaced(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{{err: errors.New("some unrecognized failure")}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	got := readFinalizeStuckFile(t, deps.ScratchDir)
	if !strings.Contains(got, "some unrecognized failure") {
		t.Errorf("stuck reason %q does not surface the underlying error", got)
	}
}

func TestFinalize_MergeInStuck(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "conflict could not be resolved"}}
	merger := &recordingParentMerger{}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if len(merger.calls) != 0 {
		t.Errorf("parent-side merge calls = %d; want 0 (merge-in never resolved)", len(merger.calls))
	}
}

// TestFinalize_StuckCausesProduceDifferentReasonFiles pins that two different stuck causes leave
// different reason-file contents.
func TestFinalize_StuckCausesProduceDifferentReasonFiles(t *testing.T) {
	deps1 := newFinalizeDeps(t)
	res1 := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	fz1 := &Finalize{deps: deps1, resolver: res1, parentOpener: func() (parentMerger, error) { return nil, errors.New("no worktree") }}
	if _, _, err := fz1.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	reason1 := readFinalizeStuckFile(t, deps1.ScratchDir)

	deps2 := newFinalizeDeps(t)
	res2 := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger2 := &recordingParentMerger{results: []mergeCallResult{{err: errors.New("some other failure")}}}
	fz2 := &Finalize{deps: deps2, resolver: res2, parentOpener: func() (parentMerger, error) { return merger2, nil }}
	if _, _, err := fz2.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	reason2 := readFinalizeStuckFile(t, deps2.ScratchDir)

	if reason1 == reason2 {
		t.Errorf("two different stuck causes produced identical reason-file contents: %q", reason1)
	}
}

func TestFinalize_CancellationAtEntry_SurfacesAsError(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{}
	fz := &Finalize{deps: deps, resolver: res}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outcome, _, err := fz.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error on entry cancellation")
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty on a cancelled entry", outcome)
	}
}
