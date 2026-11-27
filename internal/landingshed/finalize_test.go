// finalize_test.go covers Finalize against a faked resolver and a faked parent-pair opener:
// the opener returns a scripted merge outcome.
// It lives in the package itself, because the resolver seam and the parentMerger seam it substitutes are unexported.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordingParentMerger is the in-package fake standing in for the unexported parentMerger seam.
// Each `Merge` call records its source and options.
// It returns the scripted result and error at position `len(calls)`, clamped to the last entry,
// so a test can script a first-attempt failure followed by a second-attempt success.
type recordingParentMerger struct {
	calls   []mergeCall
	results []mergeCallResult
	// headSHA and headErr script HeadSHA's return.
	headSHA string
	headErr error
	// preMergeHeadSHA, when non-nil, is the parent head `HeadSHA` returns before any merge, which Finalize reads.
	preMergeHeadSHA *string
	// order, when non-nil, receives "merge" and "fast-forward" as those calls happen,
	// so a test can assert their order against other seams that append to the same slice.
	order *[]string
	// midMerge and midMergeErr script MidMerge's return after a parent-side merge, where the guard's refusal reads it;
	// probeMidMerge scripts the return before any merge, which Finalize's pre-merge probe reads.
	midMerge      fabriclayout.MidMergeState
	midMergeErr   error
	probeMidMerge fabriclayout.MidMergeState
	// dirtyCheckout, dirtyPaths and dirtyErr script CodeDirtyPaths's return.
	dirtyCheckout string
	dirtyPaths    []string
	dirtyErr      error
	// ffCalls records every FastForwardPair call's options; ffResult and ffErr script its return.
	ffCalls  []fabricengine.SyncOptions
	ffResult fabriclayout.FastForwardResult
	ffErr    error
	// onFastForward, when set, runs on each FastForwardPair call, so a test can move the head it fast-forwards.
	onFastForward func()
}

type mergeCall struct {
	source string
	opts   fabriclayout.MergeOptions
}

type mergeCallResult struct {
	result fabriclayout.MergeResult
	err    error
}

func (m *recordingParentMerger) Merge(source string, opts fabriclayout.MergeOptions) (fabriclayout.MergeResult, error) {
	idx := len(m.calls)
	m.calls = append(m.calls, mergeCall{source: source, opts: opts})
	if m.order != nil {
		*m.order = append(*m.order, "merge")
	}
	if idx >= len(m.results) {
		idx = len(m.results) - 1
	}
	if idx < 0 {
		return fabriclayout.MergeResult{}, nil
	}
	return m.results[idx].result, m.results[idx].err
}

func (m *recordingParentMerger) HeadSHA() (string, error) {
	if len(m.calls) == 0 && m.preMergeHeadSHA != nil {
		return *m.preMergeHeadSHA, nil
	}
	return m.headSHA, m.headErr
}

// stringPointer returns a pointer to s.
func stringPointer(s string) *string { return &s }

func (m *recordingParentMerger) MidMerge() (fabriclayout.MidMergeState, error) {
	if len(m.calls) == 0 {
		return m.probeMidMerge, nil
	}
	return m.midMerge, m.midMergeErr
}

func (m *recordingParentMerger) CodeDirtyPaths() (string, []string, error) {
	return m.dirtyCheckout, m.dirtyPaths, m.dirtyErr
}

func (m *recordingParentMerger) FastForwardPair(opts fabricengine.SyncOptions) (fabriclayout.FastForwardResult, error) {
	m.ffCalls = append(m.ffCalls, opts)
	if m.order != nil {
		*m.order = append(*m.order, "fast-forward")
	}
	if m.onFastForward != nil {
		m.onFastForward()
	}
	return m.ffResult, m.ffErr
}

// requireFinalizeReason returns the stuck reason carried on ptr, failing the test when it is empty.
func requireFinalizeReason(t *testing.T, ptr shedengine.OutputPointer) string {
	t.Helper()
	if ptr.Reason == "" {
		t.Fatal("OutputPointer.Reason is empty; want the producer's stuck reason")
	}
	return ptr.Reason
}

// --- NewFinalize construction ---

func TestNewFinalize_Refusals(t *testing.T) {
	tiergate.Require(t, 0)
	nilFabric := func() (*fabricengine.Fabric, error) { return nil, nil }
	cases := []struct {
		name     string
		mutate   func(*Deps)
		wantName string
	}{
		{"nil OpenFabric", func(d *Deps) { d.OpenParentFabric = nilFabric }, "Deps.OpenFabric"},
		{"nil OpenParentFabric", func(d *Deps) { d.OpenFabric = nilFabric }, "Deps.OpenParentFabric"},
		{"empty DescriptionPath", func(d *Deps) { d.OpenFabric, d.OpenParentFabric, d.DescriptionPath = nilFabric, nilFabric, "" }, "Deps.DescriptionPath"},
		{"nil Lock.Store", func(d *Deps) { d.OpenFabric, d.OpenParentFabric, d.Lock.Store = nilFabric, nilFabric, nil }, "Deps.Lock.Store"},
		{"nil Lock.Owner", func(d *Deps) { d.OpenFabric, d.OpenParentFabric, d.Lock.Owner = nilFabric, nilFabric, nil }, "Deps.Lock.Owner"},
		{"nil ReadParentTips", func(d *Deps) { d.OpenFabric, d.OpenParentFabric, d.ReadParentTips = nilFabric, nilFabric, nil }, "Deps.ReadParentTips"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestDeps(t)
			tc.mutate(&deps)
			if _, err := NewFinalize(deps); err == nil || !strings.Contains(err.Error(), tc.wantName) {
				t.Fatalf("NewFinalize() error = %v; want an error naming %s", err, tc.wantName)
			}
		})
	}
}

// --- Call behaviour ---

// TestFinalize_MergeMarkPushOrderAndVerdict pins the landing order and its merge options.
// A landed merge pushes nothing and writes no board mark, and its message ends in the landing trailer.
// With `Deps.PushSkipped` set, the records are committed and the task is marked done, and a failing write is an error.
// No case makes a GitHub call.
func TestFinalize_MergeMarkPushOrderAndVerdict(t *testing.T) {
	tiergate.Require(t, 0)
	for _, tt := range []struct {
		name  string
		reaps bool
	}{{"a set Reap runs before the merge", true}, {"a nil Reap is skipped", false}} {
		t.Run(tt.name, func(t *testing.T) {
			var order []string
			deps := newTestDeps(t)
			if tt.reaps {
				deps.Reap = func() error { order = append(order, "reap"); return nil }
			}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}, headSHA: "squash111", order: &order}
			fz := &Finalize{deps: deps, resolver: &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}, parentOpener: func() (parentMerger, error) { return merger, nil }}

			shedfake.RequireOutcome(t, fz, shedengine.Done)
			reapAt, mergeAt := slices.Index(order, "reap"), slices.Index(order, "merge")
			if tt.reaps && (reapAt < 0 || reapAt > mergeAt) {
				t.Errorf("call order = %v; want the reap before the merge", order)
			}
			if !tt.reaps && reapAt >= 0 {
				t.Errorf("call order = %v; want no reap", order)
			}
		})
	}
	markFailure := errors.New("board unavailable")
	tests := []struct {
		name        string
		pushSkipped bool
		markErr     error
		wantOrder   string
	}{
		{"a landing hands the push on and writes no mark", false, nil, "commit,merge,commit"},
		{"pushes off commit the records and mark the task done", true, nil, "commit,merge,commit,mark"},
		{"a failing board write returns an error", true, markFailure, "commit,merge,commit,mark"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var order []string
			deps := newTestDeps(t)
			deps.Slug = "me"
			deps.PushSkipped = tt.pushSkipped
			if tt.pushSkipped {
				// A wired landing seam that pushes off must not reach, so no landing entry is recorded.
				deps.LandLocally = func(string) error { order = append(order, "land"); return nil }
			}
			deps.CommitStatus = func() error { order = append(order, "commit"); return nil }
			deps.MarkTaskDone = func() error {
				order = append(order, "mark")
				return tt.markErr
			}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}, headSHA: "squash111", order: &order}
			fz := &Finalize{deps: deps, resolver: &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}, parentOpener: func() (parentMerger, error) { return merger, nil }}

			if tt.markErr != nil {
				if _, _, err := fz.Call(context.Background()); !errors.Is(err, tt.markErr) {
					t.Fatalf("Call() error = %v; want it to wrap %v", err, tt.markErr)
				}
			} else {
				shedfake.RequireOutcome(t, fz, shedengine.Done)
			}
			if got := strings.Join(order, ","); got != tt.wantOrder {
				t.Errorf("call order = %q; want %q", got, tt.wantOrder)
			}
			if len(merger.calls) != 1 {
				t.Fatalf("parent-side merge calls = %d; want 1", len(merger.calls))
			}
			if got := merger.calls[0].opts; !got.NoFastForward || !strings.HasSuffix(got.Message, "\n\nLym-Landing: me") {
				t.Errorf("MergeOptions = %+v; want no fast-forward and a message ending in the landing trailer", got)
			}
		})
	}
}

// TestFinalize_MergeOptionsCarriesComposedMessage asserts that the merge-in runs first.
// A successful parent-side merge passes MergeOptions carrying the task branch as its source:
// the composed final-summary message and the configured Squash value, for both merge shapes.
// The message is set whether or not Squash is true.
//
//testtiming:keep pins the composed MergeOptions.Message, its engine credit line and the Squash value for both merge shapes, which its covering tests do not assert
func TestFinalize_MergeOptionsCarriesComposedMessage(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name   string
		squash bool
		// nilEnginesRun leaves the Deps.EnginesRun seam nil, so no engine ran and the message carries no engine line.
		nilEnginesRun bool
		// verdict is what Deps.DriftVerdict answers, which follows the body; empty leaves the seam nil.
		verdict string
	}{
		{"Squash", true, false, ""},
		{"NoSquash", false, false, ""},
		{"NilEnginesRun", true, true, ""},
		{"RegressionCheckFollowsTheBody", true, false, testVerdict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Slug = "me"
			deps.Config.Squash = tt.squash
			enginesRun := []string{"claude"}
			if tt.nilEnginesRun {
				deps.EnginesRun = nil
				enginesRun = nil
			}
			if tt.verdict != "" {
				deps.DriftVerdict = func() (string, error) { return tt.verdict, nil }
			}
			summary, err := summaryparser.Parse(deps.DescriptionPath)
			if err != nil {
				t.Fatalf("summaryparser.Parse() error = %v; want nil", err)
			}
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

			shedfake.RequireOutcome(t, fz, shedengine.Done)
			if !res.called {
				t.Error("resolver.Resolve was not called; want merge-in to run first")
			}
			if len(merger.calls) != 1 {
				t.Fatalf("parent-side merge calls = %d; want 1", len(merger.calls))
			}
			if merger.calls[0].source != deps.TaskBranch {
				t.Errorf("parent-side merge source = %q; want %q", merger.calls[0].source, deps.TaskBranch)
			}
			got := merger.calls[0].opts
			want := summary.LandingMessage(deps.Config.CreditBlock(enginesRun))
			if tt.verdict != "" {
				want = strings.TrimRight(summary.CommitMessage(), "\n") + "\n\n" + tt.verdict + "\n\n" + deps.Config.CreditBlock(enginesRun)
			}
			want = strings.TrimRight(want, "\n") + "\n\nLym-Landing: me"
			if got.Message != want {
				t.Errorf("MergeOptions.Message = %q; want %q", got.Message, want)
			}
			engineLine := " - " + deps.Config.CreditEngines["claude"]
			if hasLine := strings.Contains(got.Message, "\n"+engineLine+"\n"); hasLine == tt.nilEnginesRun {
				t.Errorf("MergeOptions.Message = %q; engine line %q present = %v; want %v", got.Message, engineLine, hasLine, !tt.nilEnginesRun)
			}
			for _, absent := range []string{"Lym-Task", "Co-Authored-By"} {
				if strings.Contains(got.Message, absent) {
					t.Errorf("MergeOptions.Message = %q; want no %s line", got.Message, absent)
				}
			}
			if got.Squash != tt.squash {
				t.Errorf("MergeOptions.Squash = %v; want %v (threaded from configuration)", got.Squash, tt.squash)
			}
		})
	}
}

// TestFinalize_UnusableMessageInput_ErrorBeforeMergeOrCommit asserts Call returns an error on unusable input:
// a missing or malformed final-summary artifact, or an unreadable set of engines the task ran under.
// No merge is attempted and no status commit performed, which proves the top-of-Call reads run before either.
func TestFinalize_UnusableMessageInput_ErrorBeforeMergeOrCommit(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name   string
		damage func(t *testing.T, deps *Deps)
	}{
		{"missing", func(t *testing.T, deps *Deps) {
			deps.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
		}},
		{"malformed", func(t *testing.T, deps *Deps) {
			deps.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
			if err := os.WriteFile(deps.DescriptionPath, []byte("not a heading\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(malformed summary): %v", err)
			}
		}},
		{"engines_unreadable", func(t *testing.T, deps *Deps) {
			deps.EnginesRun = func() ([]string, error) { return nil, errors.New("engine ledger unreadable") }
		}},
		{"regression_check_unreadable", func(t *testing.T, deps *Deps) {
			deps.DriftVerdict = func() (string, error) { return "", errors.New("scope record unreadable") }
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			tt.damage(t, &deps)
			committed := false
			deps.CommitStatus = func() error { committed = true; return nil }
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

			outcome, _, err := fz.Call(context.Background())
			if err == nil {
				t.Fatal("Call() error = nil; want an error for an unusable landing message input")
			}
			if outcome != "" {
				t.Errorf("Call() outcome = %q; want empty on a hard error", outcome)
			}
			if len(merger.calls) != 0 {
				t.Errorf("parent-side merge calls = %d; want 0 -- the parse runs before any merge", len(merger.calls))
			}
			if committed {
				t.Error("CommitStatus was called; want the parse to run before the status commit")
			}
		})
	}
}

// orderedResolver appends "merge-in" to order on each Resolve, then delegates to the embedded resolver.
type orderedResolver struct {
	recordingResolver
	order *[]string
}

func (r *orderedResolver) Resolve(ctx context.Context, source, tip string) (mergeresolve.Result, error) {
	*r.order = append(*r.order, "merge-in")
	return r.recordingResolver.Resolve(ctx, source, tip)
}

// TestFinalize_MergeInRequiredRetry pins that an ErrMergeInRequired merge is retried exactly once.
// The retry carries the same composed message.
// The parent run records are committed after each merge-in and before the parent-side merge.
// This holds on the first pass and on the retry alike.
// The parent pair's head is recorded as the landing commit after the retry that lands, never after a failed attempt.
func TestFinalize_MergeInRequiredRetry(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name        string
		secondTry   mergeCallResult
		wantOutcome shedengine.Outcome
		wantOrder   string
		wantCalls   int
		wantReroute bool
		// parentMoves moves the parent tip from p2 to p3 at the first parent-side merge call, as a code-side commit would.
		parentMoves bool
	}{
		{"retry lands", mergeCallResult{result: fabriclayout.MergeResult{Committed: true}}, shedengine.Done, "merge-in,records,merge,merge-in,records,merge,record landed1", 2, false, false},
		{"retry also required stays Stuck", mergeCallResult{err: &fabricengine.ErrMergeInRequired{}}, shedengine.Stuck, "merge-in,records,merge,merge-in,records,merge", 2, false, false},
		{"a code-side parent commit between the passes lands through the retry", mergeCallResult{result: fabriclayout.MergeResult{Committed: true}}, shedengine.Done, "merge-in,records,merge,merge-in,records,merge,record landed1", 2, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var order []string
			deps := newTestDeps(t)
			deps.Slug = "me"
			deps.RecordLanding = func(commit string, _ bool) error { order = append(order, "record "+commit); return nil }
			deps.CommitParentRecords = func() error { order = append(order, "records"); return nil }
			summary, err := summaryparser.Parse(deps.DescriptionPath)
			if err != nil {
				t.Fatalf("summaryparser.Parse() error = %v; want nil", err)
			}
			res := &orderedResolver{recordingResolver: recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}, order: &order}
			merger := &recordingParentMerger{
				results: []mergeCallResult{{err: &fabricengine.ErrMergeInRequired{Source: deps.TaskBranch}}, tt.secondTry},
				order:   &order,
				headSHA: "landed1",
			}
			var changed []string
			if tt.parentMoves {
				deps.MergeBase = func(string) (string, error) { return "b0", nil }
				deps.ReadParentTips = func(bool) (fabriclayout.ParentTips, error) {
					tip := "p2"
					if len(merger.calls) > 0 {
						tip = "p3"
					}
					return fabriclayout.ParentTips{Local: tip, Upstream: tip, Relation: fabriclayout.ParentEqual}, nil
				}
				deps.ChangedPaths = func(from, to string) ([]string, error) {
					changed = append(changed, from+".."+to)
					return []string{"x.go"}, nil
				}
			}
			fz := &Finalize{deps: deps, resolver: res, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

			ptr := shedfake.RequireOutcome(t, fz, tt.wantOutcome)
			if tt.parentMoves && !slices.Contains(changed, "p2..p3") {
				t.Errorf("ChangedPaths calls = %v; want the retry to read the parent's changes since p2 as a p2..p3 call", changed)
			}
			if len(merger.calls) != tt.wantCalls {
				t.Fatalf("parent-side merge calls = %d; want %d", len(merger.calls), tt.wantCalls)
			}
			if ptr.Reroute != tt.wantReroute {
				t.Errorf("OutputPointer.Reroute = %v; want %v", ptr.Reroute, tt.wantReroute)
			}
			want := strings.TrimRight(summary.LandingMessage(deps.Config.CreditBlock([]string{"claude"})), "\n") + "\n\nLym-Landing: me"
			for i, call := range merger.calls {
				if call.opts.Message != want {
					t.Errorf("attempt %d MergeOptions.Message = %q; want %q (the same composed message every attempt)", i+1, call.opts.Message, want)
				}
			}
			if got := strings.Join(order, ","); got != tt.wantOrder {
				t.Errorf("call order = %q; want %q", got, tt.wantOrder)
			}
			if tt.wantOutcome == shedengine.Stuck {
				requireFinalizeReason(t, ptr)
			}
		})
	}
}

// TestFinalize_RecordsTheLandingCommitOnTheFirstTry pins that a parent-side merge lands on the first call:
// the parent pair's head is recorded after the merge.
// It then pins that the landed size is recorded from the parent head read before the merge-in to the landing commit.
//
//testtiming:keep pins the landed-size range recorded from the pre-merge parent head and the record order on the first-try merge
func TestFinalize_RecordsTheLandingCommitOnTheFirstTry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	var order []string
	deps := newTestDeps(t)
	deps.RecordLanding = func(commit string, _ bool) error { order = append(order, "record "+commit); return nil }
	deps.RecordLandedSize = func(from, to string) error { order = append(order, "size "+from+".."+to); return nil }
	merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}, order: &order, headSHA: "landed1", preMergeHeadSHA: stringPointer("parent0")}
	fz := &Finalize{deps: deps, resolver: &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

	shedfake.RequireOutcome(t, fz, shedengine.Done)
	if got, want := strings.Join(order, ","), "merge,record landed1,size parent0..landed1"; got != want {
		t.Errorf("call order = %q; want %q", got, want)
	}
}

// TestFinalize_RecordLandingFailureKeepsDone pins that a failed head read or record is only logged:
// the landing is still Done.
// The landed size is recorded whatever RecordLanding returned, and not at all when no merged parent head is held.
func TestFinalize_RecordLandingFailureKeepsDone(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name            string
		headErr         error
		recordErr       error
		sizeErr         error
		preMergeHeadSHA *string
		wantSizeCalls   int
	}{
		{"HeadReadFails", errors.New("no head"), nil, nil, stringPointer("parent0"), 0},
		{"RecordFails", nil, errors.New("disk full"), nil, stringPointer("parent0"), 1},
		{"LandedSizeFails", nil, nil, errors.New("no diff"), stringPointer("parent0"), 1},
		{"NoMergedHeadHeld", nil, nil, nil, stringPointer(""), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sizeCalls := 0
			deps := newTestDeps(t)
			deps.RecordLanding = func(string, bool) error { return tt.recordErr }
			deps.RecordLandedSize = func(string, string) error { sizeCalls++; return tt.sizeErr }
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}, headSHA: "landed1", headErr: tt.headErr, preMergeHeadSHA: tt.preMergeHeadSHA}
			fz := &Finalize{deps: deps, resolver: &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

			shedfake.RequireOutcome(t, fz, shedengine.Done)
			if sizeCalls != tt.wantSizeCalls {
				t.Errorf("RecordLandedSize calls = %d; want %d", sizeCalls, tt.wantSizeCalls)
			}
		})
	}
}

// TestFinalize_RecordsWhetherTheLandedDiffIsEmpty pins the empty-diff flag handed to RecordLanding.
// It is set only when the landing changed no code path against the parent head merged into the task worktree.
// A failed path read, or no merged head held, records the landing as not empty.
func TestFinalize_RecordsWhetherTheLandedDiffIsEmpty(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name            string
		landedPaths     []string
		pathsErr        error
		preMergeHeadSHA string
		want            bool
	}{
		{"NoPathChanged", nil, nil, "parent0", true},
		{"APathChanged", []string{"docs/research/report.md"}, nil, "parent0", false},
		{"ThePathReadFails", nil, errors.New("no diff"), "parent0", false},
		{"NoMergedHeadHeld", nil, nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var recorded []bool
			deps := newTestDeps(t)
			deps.ChangedPaths = func(from, to string) ([]string, error) {
				if from == "parent0" && to == "landed1" {
					return tt.landedPaths, tt.pathsErr
				}
				return []string{"another.go"}, nil
			}
			deps.RecordLanding = func(_ string, emptyDiff bool) error { recorded = append(recorded, emptyDiff); return nil }
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}, headSHA: "landed1", preMergeHeadSHA: stringPointer(tt.preMergeHeadSHA)}
			fz := &Finalize{deps: deps, resolver: &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

			shedfake.RequireOutcome(t, fz, shedengine.Done)
			if want := []bool{tt.want}; !slices.Equal(recorded, want) {
				t.Errorf("RecordLanding empty-diff flags = %v; want %v", recorded, want)
			}
		})
	}
}

// TestFinalize_StuckReasons pins, for each way a landing stops Stuck, the reason it surfaces.
// In each case the board seam is never called, and no stuck-reason file is written.
func TestFinalize_StuckReasons(t *testing.T) {
	tiergate.Require(t, 0)
	numberedPaths := func(n int) []string {
		paths := make([]string, n)
		for i := range paths {
			paths[i] = fmt.Sprintf("p%02d.go", i+1)
		}
		return paths
	}
	guardErr := &fabricengine.MergeGuardError{Reasons: []string{"worktree dirty"}, Checkout: "/hub/parent", DirtyPaths: []string{"a b.go", "c,d.go"}}
	guardWith := func(reasons ...string) *fabricengine.MergeGuardError {
		return &fabricengine.MergeGuardError{Reasons: reasons, Checkout: "/hub/parent", DirtyPaths: []string{"a b.go", "c,d.go"}}
	}
	const gotoResume = `lym loom goto --to Finalize`
	tests := []struct {
		name      string
		resolved  mergeresolve.Result
		openerErr error
		mergeErr  error
		// midMerge and midMergeErr are what the parent pair's parked-merge read returns.
		midMerge    fabriclayout.MidMergeState
		midMergeErr error
		// wantInOrder names substrings the reason must carry, each found after the previous one.
		wantInOrder []string
		// recordsErr is what the parent run-records seam returns.
		recordsErr error
		wantMerges int
		// wantInReason names what the stuck reason must carry, given the deps the case ran under.
		wantInReason func(d Deps) string
		// report is what the run-scope reconcile returns; nil wires none.
		report *fabriclayout.OriginReport
		// wantHalt is whether the Stuck must carry Halt, and wantNotInReason names text the reason must not carry.
		wantHalt        bool
		wantNotInReason string
		// followUp is what Deps.RemoveFollowUp returns, empty wiring none; parentHolds makes Deps.ParentHoldsTaskWork read true.
		followUp    string
		parentHolds bool
		// reapErr is what Deps.Reap returns, nil wiring none.
		reapErr error
	}{
		{
			name:     "run origin ended after its landing halts naming the landing and the cleanup from the prime",
			report:   &fabriclayout.OriginReport{Entries: []fabriclayout.EntryOutcome{{Slug: "me", State: fabriclayout.EntryEnded, How: "done"}}},
			wantHalt: true,
			wantInReason: func(Deps) string {
				return `its landing is done and its board entry is done; way forward: pause the run, then run "lym cleanup --apply" from the prime`
			},
		},
		{
			name: "run stopped by a branch names it and its way forward verbatim, then the resume",
			report: &fabriclayout.OriginReport{
				Branches: []fabriclayout.BranchOutcome{{Branch: "parent", State: fabriclayout.BranchDiverged, WayForward: "reconcile parent in the prime"}},
				Entries:  []fabriclayout.EntryOutcome{{Slug: "me", State: fabriclayout.EntryStopped, WayForward: "reconcile parent in the prime"}},
			},
			wantInReason: func(Deps) string {
				return `branch parent is diverged against origin; way forward: reconcile parent in the prime; then run "lym loom resume"`
			},
			wantNotInReason: "lym cleanup --apply",
		},
		{
			name: "stopped branch naming the removal ends in the follow-up in place of the resume",
			report: &fabriclayout.OriginReport{
				Branches: []fabriclayout.BranchOutcome{{Branch: "parent", State: fabriclayout.BranchDiverged, WayForward: "run `lym fabric remove me` to clear it"}},
				Entries:  []fabriclayout.EntryOutcome{{Slug: "me", State: fabriclayout.EntryStopped, WayForward: "run `lym fabric remove me` to clear it"}},
			},
			followUp:        "then run `lym batten cancel me --remove`",
			wantInOrder:     []string{"lym fabric remove me", "then run `lym batten cancel me --remove`"},
			wantNotInReason: "lym loom resume",
		},
		{
			name: "a parent already holding the task's work keeps the resume and carries no follow-up",
			report: &fabriclayout.OriginReport{
				Branches: []fabriclayout.BranchOutcome{{Branch: "parent", State: fabriclayout.BranchDiverged, WayForward: "run `lym fabric remove me` to clear it"}},
				Entries:  []fabriclayout.EntryOutcome{{Slug: "me", State: fabriclayout.EntryStopped, WayForward: "run `lym fabric remove me` to clear it"}},
			},
			followUp:        "then run `lym batten cancel me --remove`",
			parentHolds:     true,
			wantInOrder:     []string{"lym fabric remove me", `then run "lym loom resume"`},
			wantNotInReason: "lym batten cancel",
		},
		{
			name:         "parent opener error names the parent branch",
			resolved:     mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			openerErr:    errors.New("no such worktree"),
			wantInReason: func(d Deps) string { return d.ParentBranch },
		},
		{
			name:       "dirty-worktree guard error names the checkout and the quoted paths and is not retried",
			resolved:   mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:   guardErr,
			wantMerges: 1,
			wantInOrder: []string{guardErr.Error(), "/hub/parent", `"a b.go", "c,d.go"`,
				"commit or discard them in /hub/parent with plain git", gotoResume, "lym loom resume"},
		},
		{
			name:            "twenty-five dirty paths list the first twenty and count the rest",
			resolved:        mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:        &fabricengine.MergeGuardError{Reasons: []string{"worktree dirty"}, Checkout: "/hub/parent", DirtyPaths: numberedPaths(25)},
			wantMerges:      1,
			wantInOrder:     []string{`"p01.go"`, `"p20.go"`, "and 5 more"},
			wantNotInReason: `"p21.go"`,
		},
		{
			name:            "twenty dirty paths are all listed with no count",
			resolved:        mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:        &fabricengine.MergeGuardError{Reasons: []string{"worktree dirty"}, Checkout: "/hub/parent", DirtyPaths: numberedPaths(20)},
			wantMerges:      1,
			wantInOrder:     []string{`"p01.go"`, `"p20.go"`},
			wantNotInReason: "more",
		},
		{
			name:            "source-not-found beside dirt keeps the dirt-only way forward",
			resolved:        mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:        guardWith("source branch not found", "worktree dirty"),
			wantMerges:      1,
			wantInOrder:     []string{"source branch not found; worktree dirty", "/hub/parent", `"a b.go"`, "commit or discard them in /hub/parent with plain git", gotoResume},
			wantNotInReason: "source branch \"",
		},
		{
			name:            "merge already in progress names the parked merge and the continue or abort way forward",
			resolved:        mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:        guardWith("merge already in progress", "worktree dirty"),
			midMerge:        fabriclayout.MidMergeState{Kind: fabriclayout.MidMergeParked, Verb: fabriclayout.MergeVerbMerge, Source: "task-x"},
			wantMerges:      1,
			wantInOrder:     []string{"merge already in progress; worktree dirty", "/hub/parent", "merge of task-x", "lym fabric merge --continue", "lym fabric merge --abort", gotoResume, "lym loom resume", "hard-resets the parent's code checkout", "reports status: FAILED and the orch runs it"},
			wantNotInReason: `"a b.go"`,
		},
		{
			name:            "merge already in progress with an unreadable parked merge names no verb and keeps the way forward",
			resolved:        mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:        guardWith("merge already in progress", "worktree dirty"),
			midMergeErr:     errors.New("probe failed"),
			wantMerges:      1,
			wantInOrder:     []string{"/hub/parent", "lym fabric merge --continue", "lym fabric merge --abort", gotoResume, "lym loom resume"},
			wantNotInReason: "merge of",
		},
		{
			name:            "merge already in progress with no parked merge left names only the goto and the resume",
			resolved:        mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:        guardWith("merge already in progress", "worktree dirty"),
			midMerge:        fabriclayout.MidMergeState{Kind: fabriclayout.MidMergeNone},
			wantMerges:      1,
			wantInOrder:     []string{"/hub/parent", gotoResume, "lym loom resume"},
			wantNotInReason: "merge --continue",
		},
		{
			name:            "merge already in progress with a foreign merge names only the goto and the resume",
			resolved:        mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:        guardWith("merge already in progress", "worktree dirty"),
			midMerge:        fabriclayout.MidMergeState{Kind: fabriclayout.MidMergeForeign},
			wantMerges:      1,
			wantInOrder:     []string{"/hub/parent", gotoResume, "lym loom resume"},
			wantNotInReason: "merge --continue",
		},
		{
			name:        "branch not synced beside dirt names the list then commit or discard, then reconciling the parent",
			resolved:    mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:    guardWith("branch not synced to upstream", "worktree dirty"),
			wantMerges:  1,
			wantInOrder: []string{"branch not synced to upstream; worktree dirty", `"a b.go"`, "commit them on the parent branch or discard them", "push or reconcile the parent branch", gotoResume, "lym loom resume"},
		},
		{
			name:        "detached head beside dirt names checking out the parent branch first",
			resolved:    mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:    guardWith("checkout is not on a branch", "worktree dirty"),
			wantMerges:  1,
			wantInOrder: []string{`"a b.go"`, "check out the parent branch", "set them aside first", "commit them on the parent branch or discard them", gotoResume, "lym loom resume"},
		},
		{
			name:         "unrecognized merge error surfaces",
			resolved:     mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:     errors.New("some unrecognized failure"),
			wantMerges:   1,
			wantInReason: func(Deps) string { return "some unrecognized failure" },
		},
		{
			name:       "parent run records commit failure names the parent pair and its way forward and merges nothing",
			resolved:   mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			recordsErr: errors.New("git refused"),
			wantInReason: func(Deps) string {
				return `the parent pair's run records could not be committed: git refused; way forward: run "lym fabric commit" in the parent pair, then "lym loom resume"`
			},
		},
		{
			name:         "merge-in stuck surfaces the resolver's reason and never reaches the parent-side merge",
			resolved:     mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "conflict could not be resolved"},
			wantInReason: func(Deps) string { return "conflict could not be resolved" },
		},
		{
			name:     "a failing sandbox reap halts naming the reap's error and the clear verb, and merges nothing",
			resolved: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			reapErr:  errors.New("a reed server would not stop"),
			wantHalt: true,
			wantInReason: func(Deps) string {
				return "a reed server would not stop; way forward: run `lym harden sandbox clear` in the task worktree, then `lym loom resume`"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			markCalled := false
			deps.MarkTaskDone = func() error { markCalled = true; return nil }
			deps.RecordLanding = func(string, bool) error { markCalled = true; return nil }
			if tt.reapErr != nil {
				deps.Reap = func() error { return tt.reapErr }
			}
			if tt.recordsErr != nil {
				deps.CommitParentRecords = func() error { return tt.recordsErr }
			}
			if tt.followUp != "" {
				deps.RemoveFollowUp = func() string { return tt.followUp }
			}
			if tt.parentHolds {
				deps.ParentHoldsTaskWork = func() (bool, error) { return true, nil }
			}
			if tt.report != nil {
				deps.Reconcile = func() (fabriclayout.OriginReport, error) { return *tt.report, nil }
			}
			res := &recordingResolver{result: tt.resolved}
			merger := &recordingParentMerger{midMerge: tt.midMerge, midMergeErr: tt.midMergeErr}
			if tt.mergeErr != nil {
				merger.results = []mergeCallResult{{err: tt.mergeErr}}
			}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) {
				if tt.openerErr != nil {
					return nil, tt.openerErr
				}
				return merger, nil
			}}

			ptr := shedfake.RequireOutcome(t, fz, shedengine.Stuck)
			got := requireFinalizeReason(t, ptr)
			if tt.wantInReason != nil {
				if want := tt.wantInReason(deps); !strings.Contains(got, want) {
					t.Errorf("stuck reason %q does not carry %q", got, want)
				}
			}
			rest := got
			for _, want := range tt.wantInOrder {
				idx := strings.Index(rest, want)
				if idx < 0 {
					t.Errorf("stuck reason %q does not carry %q after the previous wanted text", got, want)
					break
				}
				rest = rest[idx+len(want):]
			}
			if tt.wantNotInReason != "" && strings.Contains(got, tt.wantNotInReason) {
				t.Errorf("stuck reason %q carries %q; want it free of it", got, tt.wantNotInReason)
			}
			if ptr.Halt != tt.wantHalt {
				t.Errorf("stuck Halt = %v; want %v", ptr.Halt, tt.wantHalt)
			}
			if len(merger.calls) != tt.wantMerges {
				t.Errorf("parent-side merge calls = %d; want %d", len(merger.calls), tt.wantMerges)
			}
			if markCalled {
				t.Error("MarkTaskDone or RecordLanding was called on a stuck landing; want neither called")
			}
			matches, err := filepath.Glob(filepath.Join(deps.ScratchDir, "*-stuck.md"))
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 0 {
				t.Errorf("stuck-reason files = %v; want none", matches)
			}
		})
	}
}

// TestFinalize_ReconcileProceedsOrErrors pins the two ways the run-scope reconcile leaves Finalize going:
// an active run lands as usual, and a reconcile error is returned from Call with nothing merged.
func TestFinalize_ReconcileProceedsOrErrors(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name         string
		report       fabriclayout.OriginReport
		reconcileErr error
		wantMerges   int
	}{
		{"active run proceeds", fabriclayout.OriginReport{Entries: []fabriclayout.EntryOutcome{{Slug: "me", State: fabriclayout.EntryActive}}}, nil, 1},
		{"reconcile error is returned", fabriclayout.OriginReport{}, errors.New("ls-remote failed"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Reconcile = func() (fabriclayout.OriginReport, error) { return tt.report, tt.reconcileErr }
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}}
			fz := &Finalize{deps: deps, resolver: &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}, parentOpener: func() (parentMerger, error) { return merger, nil }}

			if tt.reconcileErr != nil {
				_, _, err := fz.Call(context.Background())
				if err == nil || !strings.Contains(err.Error(), "ls-remote failed") {
					t.Errorf("Call() error = %v; want the reconcile's cause", err)
				}
			} else {
				shedfake.RequireOutcome(t, fz, shedengine.Done)
			}
			if len(merger.calls) != tt.wantMerges {
				t.Errorf("parent-side merge calls = %d; want %d", len(merger.calls), tt.wantMerges)
			}
		})
	}
}

// TestFinalize_ConfigChangeNotice pins the one notice Finalize queues about the task's config changes.
// The notice is queued at Done only, as one line carrying what the operator needs to re-apply them.
// A read error is reported instead of stopping, and neither a failing nor an absent Notify changes the verdict.
func TestFinalize_ConfigChangeNotice(t *testing.T) {
	tiergate.Require(t, 0)
	changed := fabriclayout.ConfigChanges{
		Files: []string{"loom.yaml", "shuttle.yaml"}, Base: "base111", Tip: "tip222", ParentTip: "parent333",
		Contents: map[string]fabriclayout.ConfigFileContents{
			"loom.yaml":    {Base: []byte("code_max_bounces: 2\n"), Tip: []byte("code_max_bounces: 1\n"), Parent: []byte("code_max_bounces: 2\n")},
			"shuttle.yaml": {Tip: []byte("model: x\n")},
		},
	}
	commentsOnly := fabriclayout.ConfigChanges{
		Files: []string{"loom.yaml"}, Base: "base111", Tip: "tip222", ParentTip: "parent333",
		Contents: map[string]fabriclayout.ConfigFileContents{
			"loom.yaml": {Base: []byte("a: 1\n"), Tip: []byte("# note\na: 1\n"), Parent: []byte("a: 1\n")},
		},
	}
	unparseable := fabriclayout.ConfigChanges{
		Files: []string{"loom.yaml"}, Base: "base111", Tip: "tip222", ParentTip: "parent333",
		Contents: map[string]fabriclayout.ConfigFileContents{
			"loom.yaml": {Base: []byte("a: 1\n"), Tip: []byte("a: [1\n"), Parent: []byte("a: 1\n")},
		},
	}
	autonomyOnly := fabriclayout.ConfigChanges{
		Files: []string{"beam.yaml"}, Base: "base111", Tip: "tip222", ParentTip: "parent333",
		Contents: map[string]fabriclayout.ConfigFileContents{
			"beam.yaml": {Base: []byte("autonomy: false\n"), Tip: []byte("autonomy: true\n"), Parent: []byte("autonomy: false\n")},
		},
	}
	autonomyAndMaxRuns := fabriclayout.ConfigChanges{
		Files: []string{"beam.yaml"}, Base: "base111", Tip: "tip222", ParentTip: "parent333",
		Contents: map[string]fabriclayout.ConfigFileContents{
			"beam.yaml": {Base: []byte("autonomy: false\nmax_runs: 3\n"), Tip: []byte("autonomy: true\nmax_runs: 5\n"), Parent: []byte("autonomy: false\nmax_runs: 3\n")},
		},
	}
	parentMoved := fabriclayout.ConfigChanges{
		Files: []string{"loom.yaml"}, Base: "base111", Tip: "tip222", ParentTip: "parent444",
		Contents: map[string]fabriclayout.ConfigFileContents{
			"loom.yaml": {Base: []byte("code_max_bounces: 3\n"), Tip: []byte("code_max_bounces: 1\n"), Parent: []byte("code_max_bounces: 3\n")},
		},
	}
	tests := []struct {
		name    string
		changes fabriclayout.ConfigChanges
		// later, when set, is what every read after the first answers.
		later       *fabriclayout.ConfigChanges
		readErr     error
		notifyErr   error
		noNotify    bool
		mergeErr    error
		wantOutcome shedengine.Outcome
		// wantInNotice is empty when no notice may be queued.
		wantInNotice []string
		// wantNotInNotice names text the queued notice must not carry.
		wantNotInNotice []string
	}{
		{
			name:         "changed keys queue one notice naming the branch, each file and module, each key with its base, tip and parent values, absent included, and the re-apply instruction",
			changes:      changed,
			wantOutcome:  shedengine.Done,
			wantInNotice: []string{"loom:", `"task-branch"`, "loom.yaml (module loom)", "loom.code_max_bounces: base 2, tip 1, parent 2", "shuttle.yaml (module shuttle)", "shuttle.model: base absent, tip x, parent absent", "does not carry them", "re-apply"},
		},
		{
			name:            "the base is the first read's, the parent values are the later read's, and a file the parent came to hold is left out",
			changes:         changed,
			later:           &parentMoved,
			wantOutcome:     shedengine.Done,
			wantInNotice:    []string{"loom.code_max_bounces: base 2, tip 1, parent 3"},
			wantNotInNotice: []string{"shuttle"},
		},
		{
			name:        "an empty diff queues none",
			wantOutcome: shedengine.Done,
		},
		{
			name:        "a comments-only change queues none",
			changes:     commentsOnly,
			wantOutcome: shedengine.Done,
		},
		{
			name:        "a beam.yaml whose only changed key is autonomy queues none",
			changes:     autonomyOnly,
			wantOutcome: shedengine.Done,
		},
		{
			name:            "a beam.yaml changing autonomy and max_runs names only max_runs",
			changes:         autonomyAndMaxRuns,
			wantOutcome:     shedengine.Done,
			wantInNotice:    []string{"beam.max_runs: base 3, tip 5, parent 3"},
			wantNotInNotice: []string{"autonomy"},
		},
		{
			name:         "a file that does not parse is reported as not compared and still reaches Done",
			changes:      unparseable,
			wantOutcome:  shedengine.Done,
			wantInNotice: []string{"loom:", "could not be compared key by key", "loom.yaml (module loom)", "parse the tip config"},
		},
		{
			name:         "a read error still reaches Done and queues a notice carrying the error text on one line",
			readErr:      errors.New("compare branches:\nno merge-base"),
			wantOutcome:  shedengine.Done,
			wantInNotice: []string{"loom:", "could not be read", "compare branches: no merge-base", "by hand"},
		},
		{
			name:         "a Notify error leaves the verdict Done",
			changes:      changed,
			notifyErr:    errors.New("queue unavailable"),
			wantOutcome:  shedengine.Done,
			wantInNotice: []string{"loom.code_max_bounces"},
		},
		{
			name:        "a failed parent-side merge is Stuck and queues none",
			changes:     changed,
			mergeErr:    errors.New("some unrecognized failure"),
			wantOutcome: shedengine.Stuck,
		},
		{
			name:        "a nil Notify still reaches Done",
			changes:     changed,
			noNotify:    true,
			wantOutcome: shedengine.Done,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var notices []string
			deps := newTestDeps(t)
			reads := 0
			deps.ConfigChanges = func() (fabriclayout.ConfigChanges, error) {
				reads++
				if reads > 1 && tt.later != nil {
					return *tt.later, nil
				}
				return tt.changes, tt.readErr
			}
			if !tt.noNotify {
				deps.Notify = func(line string) error {
					notices = append(notices, line)
					return tt.notifyErr
				}
			}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}, err: tt.mergeErr}}}
			fz := &Finalize{
				deps:         deps,
				resolver:     &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}},
				parentOpener: func() (parentMerger, error) { return merger, nil },
			}

			shedfake.RequireOutcome(t, fz, tt.wantOutcome)
			if len(tt.wantInNotice) == 0 || tt.noNotify {
				if len(notices) != 0 {
					t.Fatalf("notices = %q; want none", notices)
				}
				return
			}
			if len(notices) != 1 {
				t.Fatalf("notices = %q; want exactly one", notices)
			}
			for _, want := range tt.wantInNotice {
				if !strings.Contains(notices[0], want) {
					t.Errorf("notice %q does not carry %q", notices[0], want)
				}
			}
			for _, unwanted := range tt.wantNotInNotice {
				if strings.Contains(notices[0], unwanted) {
					t.Errorf("notice %q carries %q; want it left out", notices[0], unwanted)
				}
			}
			if strings.ContainsAny(notices[0], "\r\n") {
				t.Errorf("notice %q spans more than one line", notices[0])
			}
		})
	}
}

func TestFinalize_CancellationAtEntry_SurfacesAsError(t *testing.T) {
	tiergate.Require(t, 0)
	deps := newTestDeps(t)
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

// TestFinalize_LockFirstAndFastForward pins Finalize's entry.
// The parent lock is held before the run-scope reconcile and every read of the parent,
// and a parent behind its upstream is fast-forwarded under the lock.
// Each refusal of the parent pair is a Stuck with its way forward before any merge.
func TestFinalize_LockFirstAndFastForward(t *testing.T) {
	tiergate.Require(t, 0)
	// lockHeld reports whether the run holds the parent lock in deps' store.
	lockHeld := func(deps Deps) bool {
		rec, found, _ := deps.Lock.Store.Read("main")
		return found && rec.Owner.Slug == "me"
	}
	tipsOf := func(relation fabriclayout.ParentRelation) fabriclayout.ParentTips {
		return fabriclayout.ParentTips{Local: "p1", Upstream: "u2", Relation: relation}
	}

	t.Run("the lock is held at the reconcile and the tips read, and a parent behind its upstream is fast-forwarded", func(t *testing.T) {
		deps := newTestDeps(t)
		var held []string
		deps.Reconcile = func() (fabriclayout.OriginReport, error) {
			held = append(held, fmt.Sprintf("reconcile:%v", lockHeld(deps)))
			return fabriclayout.OriginReport{}, nil
		}
		deps.ReadParentTips = func(fetch bool) (fabriclayout.ParentTips, error) {
			held = append(held, fmt.Sprintf("tips:%v:%v", fetch, lockHeld(deps)))
			return tipsOf(fabriclayout.ParentBehind), nil
		}
		var order []string
		res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
		merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}, order: &order}
		fz := &Finalize{deps: deps, resolver: res, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

		shedfake.RequireOutcome(t, fz, shedengine.Done)

		if got, want := strings.Join(held, ","), "reconcile:true,tips:true:true,tips:false:true"; got != want {
			t.Errorf("lock holds at the seams = %q; want %q", got, want)
		}
		if got, want := strings.Join(order, ","), "fast-forward,merge"; got != want {
			t.Errorf("call order = %q; want %q", got, want)
		}
		if len(merger.ffCalls) != 1 || merger.ffCalls[0].SkipPush {
			t.Errorf("FastForwardPair calls = %+v; want one without the push skip", merger.ffCalls)
		}
	})

	refusals := []struct {
		name       string
		openerErr  error
		dirty      []string
		midMerge   fabriclayout.MidMergeState
		wantReason string
	}{
		{"a missing pair", errors.New("no such worktree"), nil, fabriclayout.MidMergeState{}, `no live pair for parent branch "main": no such worktree; way forward: re-create the parent pair, then run "lym loom resume"`},
		{"a dirty code checkout", nil, []string{"a.go"}, fabriclayout.MidMergeState{}, DirtyParentRefusal("/hub/parent", []string{"a.go"})},
		{"a pair mid-merge", nil, nil, fabriclayout.MidMergeState{Kind: fabriclayout.MidMergeParked}, `a merge is in progress in the parent pair of "main"; way forward: finish or abort that merge in the parent pair, then run "lym loom resume"`},
	}
	states := []struct {
		name        string
		tips        fabriclayout.ParentTips
		pushSkipped bool
	}{
		{"equal to its upstream", tipsOf(fabriclayout.ParentEqual), false},
		{"without an upstream", fabriclayout.ParentTips{Local: "p1", Relation: fabriclayout.ParentNoUpstream}, false},
		{"under the push skip", tipsOf(fabriclayout.ParentBehind), true},
	}
	for _, refusal := range refusals {
		for _, state := range states {
			t.Run(refusal.name+" with the parent "+state.name+" is Stuck before any merge", func(t *testing.T) {
				deps := newTestDeps(t)
				deps.PushSkipped = state.pushSkipped
				deps.ReadParentTips = func(bool) (fabriclayout.ParentTips, error) { return state.tips, nil }
				res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
				merger := &recordingParentMerger{dirtyCheckout: "/hub/parent", dirtyPaths: refusal.dirty, probeMidMerge: refusal.midMerge}
				fz := &Finalize{deps: deps, resolver: res, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) {
					if refusal.openerErr != nil {
						return nil, refusal.openerErr
					}
					return merger, nil
				}}

				ptr := shedfake.RequireOutcome(t, fz, shedengine.Stuck)

				if got := requireFinalizeReason(t, ptr); got != refusal.wantReason {
					t.Errorf("reason = %q; want %q", got, refusal.wantReason)
				}
				if res.called || len(merger.calls) != 0 || len(merger.ffCalls) != 0 {
					t.Errorf("a merge or fast-forward ran after the refusal: resolver %v, merges %d, fast-forwards %d", res.called, len(merger.calls), len(merger.ffCalls))
				}
			})
		}
	}

	moves := []struct {
		name       string
		tips       fabriclayout.ParentTips
		ff         fabriclayout.FastForwardResult
		wantReason string
	}{
		{"a behind parent that is no longer a fast-forward", tipsOf(fabriclayout.ParentBehind), fabriclayout.FastForwardResult{Refusal: fabriclayout.FastForwardNotFastForward}, `the upstream of "main" is no longer a fast-forward of the parent pair; way forward: reconcile "main" with its upstream in its own pair, then run "lym loom resume"`},
		{"a parent ahead of its upstream", tipsOf(fabriclayout.ParentAhead), fabriclayout.FastForwardResult{}, `the local parent branch "main" is ahead of its upstream; way forward: push or reconcile "main" in its own pair, then run "lym loom resume"`},
		{"a parent diverged from its upstream", tipsOf(fabriclayout.ParentDiverged), fabriclayout.FastForwardResult{}, `the local parent branch "main" has diverged from its upstream; way forward: push or reconcile "main" in its own pair, then run "lym loom resume"`},
	}
	for _, move := range moves {
		t.Run(move.name+" is Stuck before any merge", func(t *testing.T) {
			deps := newTestDeps(t)
			deps.ReadParentTips = func(bool) (fabriclayout.ParentTips, error) { return move.tips, nil }
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{ffResult: move.ff}
			fz := &Finalize{deps: deps, resolver: res, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

			ptr := shedfake.RequireOutcome(t, fz, shedengine.Stuck)

			if got := requireFinalizeReason(t, ptr); got != move.wantReason {
				t.Errorf("reason = %q; want %q", got, move.wantReason)
			}
			if res.called || len(merger.calls) != 0 {
				t.Errorf("a merge ran after the refusal: resolver %v, merges %d", res.called, len(merger.calls))
			}
			if lockHeld(deps) {
				t.Error("the parent lock is still held after the Stuck")
			}
		})
	}
}
