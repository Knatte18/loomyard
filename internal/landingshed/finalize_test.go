//go:build tier0

// finalize_test.go covers Finalize against a faked resolver and a faked parent-pair opener closure returning a scripted merge outcome.
// This tier lives in the package itself too, for the same reason as its sibling: the resolver seam and the parentMerger seam it substitutes are both unexported.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedtransient"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// recordingParentMerger is the in-package fake standing in for the unexported parentMerger seam. It
// records every call's source/opts, and returns the scripted result/err at position len(calls)
// (clamped to the last entry), so a test can script a first-attempt failure followed by a
// second-attempt success.
type recordingParentMerger struct {
	calls   []mergeCall
	results []mergeCallResult
	// pushCalls records every PushBranch call's options; pushErr is returned by each of them.
	pushCalls []fabricengine.SyncOptions
	pushErr   error
	// headSHA and headErr script HeadSHA's return.
	headSHA string
	headErr error
	// order, when non-nil, receives "merge" and "push" as those calls happen, so a test can
	// assert their order against other seams that append to the same slice.
	order *[]string
	// midMerge and midMergeErr script MidMerge's return once a parent-side merge has been attempted, where the merge guard's refusal reads it;
	// probeMidMerge scripts the return before any merge, which Finalize's pre-merge probe reads.
	midMerge      fabricengine.MidMergeState
	midMergeErr   error
	probeMidMerge fabricengine.MidMergeState
	// dirtyCheckout, dirtyPaths and dirtyErr script CodeDirtyPaths's return.
	dirtyCheckout string
	dirtyPaths    []string
	dirtyErr      error
	// ffCalls records every FastForwardPair call's options; ffResult and ffErr script its return.
	ffCalls  []fabricengine.SyncOptions
	ffResult fabricengine.FastForwardResult
	ffErr    error
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
	if m.order != nil {
		*m.order = append(*m.order, "merge")
	}
	if idx >= len(m.results) {
		idx = len(m.results) - 1
	}
	if idx < 0 {
		return fabricengine.MergeResult{}, nil
	}
	return m.results[idx].result, m.results[idx].err
}

func (m *recordingParentMerger) PushBranch(opts fabricengine.SyncOptions) (fabricengine.PushResult, error) {
	m.pushCalls = append(m.pushCalls, opts)
	if m.order != nil {
		*m.order = append(*m.order, "push")
	}
	return fabricengine.PushResult{}, m.pushErr
}

func (m *recordingParentMerger) HeadSHA() (string, error) { return m.headSHA, m.headErr }

func (m *recordingParentMerger) MidMerge() (fabricengine.MidMergeState, error) {
	if len(m.calls) == 0 {
		return m.probeMidMerge, nil
	}
	return m.midMerge, m.midMergeErr
}

func (m *recordingParentMerger) CodeDirtyPaths() (string, []string, error) {
	return m.dirtyCheckout, m.dirtyPaths, m.dirtyErr
}

func (m *recordingParentMerger) FastForwardPair(opts fabricengine.SyncOptions) (fabricengine.FastForwardResult, error) {
	m.ffCalls = append(m.ffCalls, opts)
	if m.order != nil {
		*m.order = append(*m.order, "fast-forward")
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

// TestFinalize_MergeMarkPushOrderAndVerdict pins that a landed merge is marked done on the board and then pushed to the parent's upstream before Done, honouring Deps.PushSkipped, that a failed push is Stuck rather than Done, and that neither a failing push nor a failing board seam changes what the other is called for.
// A failed push also carries the adopter-first repair note only when origin's parent holds the work and a parent commit is listed: a reset for the squash alone, a rebase dropping it when another commit is listed.
// No case makes a GitHub call.
func TestFinalize_MergeMarkPushOrderAndVerdict(t *testing.T) {
	failedPush := func(record fabricengine.FailedPush, cause error) *fabricengine.PushFailedError {
		return &fabricengine.PushFailedError{Record: record, Err: cause}
	}
	rewound := failedPush(fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/parent", Branch: "parent", Class: fabricengine.PushFailureRewound, OriginHead: "abc123", Commits: []string{"def456"}}, errors.New("push held back"))
	primeDiverged := failedPush(fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/main", Branch: "main", Prime: true, Class: fabricengine.PushFailureNotFastForward}, gitrepo.ErrPushRejected)
	authRefused := failedPush(fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/parent", Branch: "parent", Class: fabricengine.PushFailureAuth}, gitrepo.ErrPushAuthRefused)
	// repairSeams scripts the adopter-first repair note's reads: whether origin's parent holds the work and the parent commits origin lacks.
	type repairSeams struct {
		holds    bool
		holdsErr error
		lacks    []string
		lacksErr error
	}
	squash := "squash111"
	tests := []struct {
		name        string
		pushSkipped bool
		pushErr     error
		markErr     error
		wantOutcome shedengine.Outcome
		// wantWayForward is the exact way-forward line the Stuck reason ends in, with no hand-push advice after it.
		wantWayForward string
		// repair scripts the repair note's reads; nil wires none.
		repair *repairSeams
		// wantRepair names what the reason must carry; empty means the reason must carry no repair note.
		wantRepair []string
	}{
		{"pushed", false, nil, nil, shedengine.Done, "", nil, nil},
		{"push skipped", true, nil, nil, shedengine.Done, "", nil, nil},
		{"failing push still marks", false, errors.New("remote rejected"), nil, shedengine.Stuck, "", nil, nil},
		{"erroring board seam still Done", false, nil, errors.New("board unavailable"), shedengine.Done, "", nil, nil},
		{"rewound hold-back", false, rewound, nil, shedengine.Stuck, fabricengine.FailedPushWayForward(rewound.Record), nil, nil},
		{"prime not fast-forward", false, primeDiverged, nil, shedengine.Stuck, fabricengine.FailedPushWayForward(primeDiverged.Record), nil, nil},
		{"auth refusal", false, authRefused, nil, shedengine.Stuck, fabricengine.FailedPushWayForward(authRefused.Record), nil, nil},
		{
			"the squash alone names the reset", false, errors.New("remote rejected"), nil, shedengine.Stuck, "",
			&repairSeams{holds: true, lacks: []string{squash}},
			[]string{squash, `git reset --keep origin/main`, `lyx fabric remove me`},
		},
		{
			"another commit listed names the rebase with the squash SHA", false, errors.New("remote rejected"), nil, shedengine.Stuck, "",
			&repairSeams{holds: true, lacks: []string{squash, "other222"}},
			[]string{"git rebase --onto " + squash + "~1 " + squash + " main", "git rebase --abort", `lyx fabric remove me`},
		},
		{"origin's parent lacking the work names no repair", false, errors.New("remote rejected"), nil, shedengine.Stuck, "", &repairSeams{lacks: []string{squash}}, nil},
		{"a failed read of origin's parent names no repair", false, errors.New("remote rejected"), nil, shedengine.Stuck, "", &repairSeams{holdsErr: errors.New("fetch failed")}, nil},
		{"a failed commit listing names no repair", false, errors.New("remote rejected"), nil, shedengine.Stuck, "", &repairSeams{holds: true, lacksErr: errors.New("no branch")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var order []string
			deps := newTestDeps(t)
			deps.Slug = "me"
			if tt.repair != nil {
				deps.OriginParentHoldsWork = func() (bool, error) { return tt.repair.holds, tt.repair.holdsErr }
				deps.ParentCommitsOriginLacks = func() ([]string, error) { return tt.repair.lacks, tt.repair.lacksErr }
			}
			deps.PushSkipped = tt.pushSkipped
			deps.MarkTaskDone = func() error {
				order = append(order, "mark")
				return tt.markErr
			}
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{
				results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}},
				pushErr: tt.pushErr,
				headSHA: squash,
				order:   &order,
			}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

			ptr := shedfake.RequireOutcome(t, fz, tt.wantOutcome)
			if got, want := strings.Join(order, ","), "merge,mark,push"; got != want {
				t.Errorf("call order = %q; want %q", got, want)
			}
			if len(merger.pushCalls) != 1 {
				t.Fatalf("PushBranch calls = %d; want 1", len(merger.pushCalls))
			}
			if got := merger.pushCalls[0].SkipPush; got != tt.pushSkipped {
				t.Errorf("PushBranch SkipPush = %v; want %v", got, tt.pushSkipped)
			}
			switch {
			case tt.wantWayForward != "":
				if got := requireFinalizeReason(t, ptr); !strings.HasSuffix(got, tt.wantWayForward) || strings.Contains(got, "push it by hand") {
					t.Errorf("reason = %q; want it to end in %q with no `push it by hand`", got, tt.wantWayForward)
				}
			case tt.pushErr != nil:
				if got := requireFinalizeReason(t, ptr); !strings.Contains(got, "push it by hand") {
					t.Errorf("reason = %q; want it to tell the operator to push by hand", got)
				}
			}
			if tt.pushErr == nil {
				return
			}
			reason := requireFinalizeReason(t, ptr)
			for _, want := range tt.wantRepair {
				if !strings.Contains(reason, want) {
					t.Errorf("reason = %q; want the repair note to carry %q", reason, want)
				}
			}
			if hasNote := strings.Contains(reason, "duplicates it"); hasNote != (len(tt.wantRepair) > 0) {
				t.Errorf("reason = %q; repair note present = %v; want %v", reason, hasNote, len(tt.wantRepair) > 0)
			}
		})
	}
}

// TestFinalize_MergeOptionsCarriesComposedMessage asserts the merge-in runs first and a successful parent-side merge passes MergeOptions carrying the task branch as its source, both the composed final-summary message and the configured Squash value, for both merge shapes -- the message is set whether or not Squash is true.
//
//testtiming:keep pins the composed MergeOptions.Message, its engine trailer and the Squash value for both merge shapes, which its covering tests do not assert
func TestFinalize_MergeOptionsCarriesComposedMessage(t *testing.T) {
	tests := []struct {
		name   string
		squash bool
		// nilEnginesRun leaves the Deps.EnginesRun seam nil, so no engine ran and the message carries no engine line.
		nilEnginesRun bool
	}{
		{"Squash", true, false},
		{"NoSquash", false, false},
		{"NilEnginesRun", true, true},
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
			summary, err := summaryparser.Parse(deps.DescriptionPath)
			if err != nil {
				t.Fatalf("summaryparser.Parse() error = %v; want nil", err)
			}
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
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
			want := summary.LandingMessage(taskTrailer(deps.Config.TrailerBlock(enginesRun), "me"))
			if got.Message != want {
				t.Errorf("MergeOptions.Message = %q; want %q", got.Message, want)
			}
			trailer := deps.Config.EngineTrailers["claude"]
			if hasLine := strings.HasSuffix(got.Message, "\n\n"+trailer+"\nLyx-Task: me"); hasLine == tt.nilEnginesRun {
				t.Errorf("MergeOptions.Message = %q; engine line %q present = %v; want %v", got.Message, trailer, hasLine, !tt.nilEnginesRun)
			}
			if !strings.HasSuffix(got.Message, "\nLyx-Task: me") {
				t.Errorf("MergeOptions.Message = %q; want it to end in the Lyx-Task trailer line", got.Message)
			}
			if got.Squash != tt.squash {
				t.Errorf("MergeOptions.Squash = %v; want %v (threaded from configuration)", got.Squash, tt.squash)
			}
		})
	}
}

// TestFinalize_UnusableMessageInput_ErrorBeforeMergeOrCommit asserts a missing or malformed final-summary artifact, or an unreadable set of engines the task ran under, makes Call return an error with no merge attempted and no status commit performed -- proving the top-of-Call reads run before either.
func TestFinalize_UnusableMessageInput_ErrorBeforeMergeOrCommit(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			tt.damage(t, &deps)
			committed := false
			deps.CommitStatus = func() error { committed = true; return nil }
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
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

// orderedResolver is a recordingResolver that also appends "merge-in" to order on each Resolve.
type orderedResolver struct {
	recordingResolver
	order *[]string
}

func (r *orderedResolver) Resolve(ctx context.Context, source, tip string) (mergeresolve.Result, error) {
	*r.order = append(*r.order, "merge-in")
	return r.recordingResolver.Resolve(ctx, source, tip)
}

// TestFinalize_MergeInRequiredRetry pins that an ErrMergeInRequired merge is retried exactly once with the same composed message, and that the board seam runs once, after the retry that lands, never after a failed attempt.
// It also pins that the parent run records are committed after each merge-in and before the parent-side merge, on the first pass and on the retry alike.
func TestFinalize_MergeInRequiredRetry(t *testing.T) {
	tests := []struct {
		name        string
		secondTry   mergeCallResult
		wantOutcome shedengine.Outcome
		wantOrder   string
		wantCalls   int
		wantReroute bool
	}{
		{"retry lands", mergeCallResult{result: fabricengine.MergeResult{Committed: true}}, shedengine.Done, "merge-in,records,merge,merge-in,records,merge,mark,push", 2, false},
		{"retry also required stays Stuck", mergeCallResult{err: &fabricengine.ErrMergeInRequired{}}, shedengine.Stuck, "merge-in,records,merge,merge-in,records,merge", 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var order []string
			deps := newTestDeps(t)
			deps.MarkTaskDone = func() error { order = append(order, "mark"); return nil }
			deps.CommitParentRecords = func() error { order = append(order, "records"); return nil }
			summary, err := summaryparser.Parse(deps.DescriptionPath)
			if err != nil {
				t.Fatalf("summaryparser.Parse() error = %v; want nil", err)
			}
			res := &orderedResolver{recordingResolver: recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}, order: &order}
			merger := &recordingParentMerger{
				results: []mergeCallResult{{err: &fabricengine.ErrMergeInRequired{Source: deps.TaskBranch}}, tt.secondTry},
				order:   &order,
			}
			fz := &Finalize{deps: deps, resolver: res, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

			ptr := shedfake.RequireOutcome(t, fz, tt.wantOutcome)
			if len(merger.calls) != tt.wantCalls {
				t.Fatalf("parent-side merge calls = %d; want %d", len(merger.calls), tt.wantCalls)
			}
			if ptr.Reroute != tt.wantReroute {
				t.Errorf("OutputPointer.Reroute = %v; want %v", ptr.Reroute, tt.wantReroute)
			}
			want := summary.LandingMessage(taskTrailer(deps.Config.TrailerBlock([]string{"claude"}), deps.Slug))
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

// TestFinalize_StuckReasons pins, for each way a landing stops Stuck, the reason it surfaces, that the board seam is never called, and that no stuck-reason file is written.
func TestFinalize_StuckReasons(t *testing.T) {
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
	const gotoResume = `lyx loom goto --to Finalize`
	tests := []struct {
		name      string
		resolved  mergeresolve.Result
		openerErr error
		mergeErr  error
		// midMerge and midMergeErr are what the parent pair's parked-merge read returns.
		midMerge    fabricengine.MidMergeState
		midMergeErr error
		// wantInOrder names substrings the reason must carry, each found after the previous one.
		wantInOrder []string
		// recordsErr is what the parent run-records seam returns.
		recordsErr error
		wantMerges int
		// wantInReason names what the stuck reason must carry, given the deps the case ran under.
		wantInReason func(d Deps) string
		// report is what the run-scope reconcile returns; nil wires none.
		report *fabricengine.OriginReport
		// wantHalt is whether the Stuck must carry Halt, and wantNotInReason names text the reason must not carry.
		wantHalt        bool
		wantNotInReason string
	}{
		{
			name:     "run origin ended after its landing halts naming the landing and the cleanup from the prime",
			report:   &fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "me", State: fabricengine.EntryEnded, How: "done"}}},
			wantHalt: true,
			wantInReason: func(Deps) string {
				return `its landing is done and its board entry is done; way forward: pause the run, then run "lyx cleanup --apply" from the prime`
			},
		},
		{
			name: "run stopped by a branch names it and its way forward verbatim, then the resume",
			report: &fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{{Branch: "parent", State: fabricengine.BranchDiverged, WayForward: "reconcile parent in the prime"}},
				Entries:  []fabricengine.EntryOutcome{{Slug: "me", State: fabricengine.EntryStopped, WayForward: "reconcile parent in the prime"}},
			},
			wantInReason: func(Deps) string {
				return `branch parent is diverged against origin; way forward: reconcile parent in the prime; then run "lyx loom resume"`
			},
			wantNotInReason: "lyx cleanup --apply",
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
				"commit or discard them in /hub/parent with plain git", gotoResume, "lyx loom resume"},
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
			midMerge:        fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Verb: fabricengine.MergeVerbMerge, Source: "task-x"},
			wantMerges:      1,
			wantInOrder:     []string{"merge already in progress; worktree dirty", "/hub/parent", "merge of task-x", "lyx fabric merge --continue", "lyx fabric merge --abort", gotoResume, "lyx loom resume", "hard-resets the parent's code checkout", "reports status: FAILED and the orch runs it"},
			wantNotInReason: `"a b.go"`,
		},
		{
			name:            "merge already in progress with an unreadable parked merge names no verb and keeps the way forward",
			resolved:        mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:        guardWith("merge already in progress", "worktree dirty"),
			midMergeErr:     errors.New("probe failed"),
			wantMerges:      1,
			wantInOrder:     []string{"/hub/parent", "lyx fabric merge --continue", "lyx fabric merge --abort", gotoResume, "lyx loom resume"},
			wantNotInReason: "merge of",
		},
		{
			name:            "merge already in progress with no parked merge left names only the goto and the resume",
			resolved:        mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:        guardWith("merge already in progress", "worktree dirty"),
			midMerge:        fabricengine.MidMergeState{Kind: fabricengine.MidMergeNone},
			wantMerges:      1,
			wantInOrder:     []string{"/hub/parent", gotoResume, "lyx loom resume"},
			wantNotInReason: "merge --continue",
		},
		{
			name:            "merge already in progress with a foreign merge names only the goto and the resume",
			resolved:        mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:        guardWith("merge already in progress", "worktree dirty"),
			midMerge:        fabricengine.MidMergeState{Kind: fabricengine.MidMergeForeign},
			wantMerges:      1,
			wantInOrder:     []string{"/hub/parent", gotoResume, "lyx loom resume"},
			wantNotInReason: "merge --continue",
		},
		{
			name:        "branch not synced beside dirt names the list then commit or discard, then reconciling the parent",
			resolved:    mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:    guardWith("branch not synced to upstream", "worktree dirty"),
			wantMerges:  1,
			wantInOrder: []string{"branch not synced to upstream; worktree dirty", `"a b.go"`, "commit them on the parent branch or discard them", "push or reconcile the parent branch", gotoResume, "lyx loom resume"},
		},
		{
			name:        "detached head beside dirt names checking out the parent branch first",
			resolved:    mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:    guardWith("checkout is not on a branch", "worktree dirty"),
			wantMerges:  1,
			wantInOrder: []string{`"a b.go"`, "check out the parent branch", "set them aside first", "commit them on the parent branch or discard them", gotoResume, "lyx loom resume"},
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
				return `the parent pair's run records could not be committed: git refused; way forward: run "lyx fabric commit" in the parent pair, then "lyx loom resume"`
			},
		},
		{
			name:         "merge-in stuck surfaces the resolver's reason and never reaches the parent-side merge",
			resolved:     mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "conflict could not be resolved"},
			wantInReason: func(Deps) string { return "conflict could not be resolved" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			markCalled := false
			deps.MarkTaskDone = func() error { markCalled = true; return nil }
			if tt.recordsErr != nil {
				deps.CommitParentRecords = func() error { return tt.recordsErr }
			}
			if tt.report != nil {
				deps.Reconcile = func() (fabricengine.OriginReport, error) { return *tt.report, nil }
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
				t.Error("MarkTaskDone was called on a stuck landing; want it never called")
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
	tests := []struct {
		name         string
		report       fabricengine.OriginReport
		reconcileErr error
		wantMerges   int
	}{
		{"active run proceeds", fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "me", State: fabricengine.EntryActive}}}, nil, 1},
		{"reconcile error is returned", fabricengine.OriginReport{}, errors.New("ls-remote failed"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Reconcile = func() (fabricengine.OriginReport, error) { return tt.report, tt.reconcileErr }
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
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

// TestFinalize_ConfigChangeNotice pins the one notice Finalize queues about the task's config changes: at Done only, one line carrying what the operator needs to re-apply them, a read error reported instead of stopping, and neither a failing nor an absent Notify changing the verdict.
func TestFinalize_ConfigChangeNotice(t *testing.T) {
	changed := fabricengine.ConfigChanges{
		Files: []string{"loom.yaml", "shuttle.yaml"}, Base: "base111", Tip: "tip222", ParentTip: "parent333",
		Contents: map[string]fabricengine.ConfigFileContents{
			"loom.yaml":    {Base: []byte("code_max_bounces: 2\n"), Tip: []byte("code_max_bounces: 1\n"), Parent: []byte("code_max_bounces: 2\n")},
			"shuttle.yaml": {Tip: []byte("model: x\n")},
		},
	}
	commentsOnly := fabricengine.ConfigChanges{
		Files: []string{"loom.yaml"}, Base: "base111", Tip: "tip222", ParentTip: "parent333",
		Contents: map[string]fabricengine.ConfigFileContents{
			"loom.yaml": {Base: []byte("a: 1\n"), Tip: []byte("# note\na: 1\n"), Parent: []byte("a: 1\n")},
		},
	}
	unparseable := fabricengine.ConfigChanges{
		Files: []string{"loom.yaml"}, Base: "base111", Tip: "tip222", ParentTip: "parent333",
		Contents: map[string]fabricengine.ConfigFileContents{
			"loom.yaml": {Base: []byte("a: 1\n"), Tip: []byte("a: [1\n"), Parent: []byte("a: 1\n")},
		},
	}
	tests := []struct {
		name        string
		changes     fabricengine.ConfigChanges
		readErr     error
		notifyErr   error
		noNotify    bool
		mergeErr    error
		wantOutcome shedengine.Outcome
		// wantInNotice is empty when no notice may be queued.
		wantInNotice []string
	}{
		{
			name:         "changed keys queue one notice naming the branch, each file and module, each key with its base, tip and parent values, absent included, and the re-apply instruction",
			changes:      changed,
			wantOutcome:  shedengine.Done,
			wantInNotice: []string{"loom:", `"task-branch"`, "loom.yaml (module loom)", "loom.code_max_bounces: base 2, tip 1, parent 2", "shuttle.yaml (module shuttle)", "shuttle.model: base absent, tip x, parent absent", "does not carry them", "re-apply"},
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
			deps.ConfigChanges = func() (fabricengine.ConfigChanges, error) { return tt.changes, tt.readErr }
			if !tt.noNotify {
				deps.Notify = func(line string) error {
					notices = append(notices, line)
					return tt.notifyErr
				}
			}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}, err: tt.mergeErr}}}
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
			if strings.ContainsAny(notices[0], "\r\n") {
				t.Errorf("notice %q spans more than one line", notices[0])
			}
		})
	}
}

func TestFinalize_CancellationAtEntry_SurfacesAsError(t *testing.T) {
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

// TestFinalize_TransportPushFailure_ReturnsClassifiedError pins that a parent push failing on transport is an error the Shed classifier marks, while a plain push error keeps its Stuck verdict.
func TestFinalize_TransportPushFailure_ReturnsClassifiedError(t *testing.T) {
	deps := newTestDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{
		results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}},
		pushErr: transportPushErr(),
	}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	_, _, err := fz.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want a transient error")
	}
	if got := shedtransient.Class(err); got != shedengine.TransientGitTransport {
		t.Errorf("Class(err) = %q; want %q", got, shedengine.TransientGitTransport)
	}
}

// TestFinalize_LockFirstAndFastForward pins Finalize's entry.
// The parent lock is held before the run-scope reconcile and every read of the parent, and a parent behind its upstream is fast-forwarded under the lock.
// Each refusal of the parent pair is a Stuck with its way forward before any merge.
func TestFinalize_LockFirstAndFastForward(t *testing.T) {
	// lockHeld reports whether the run holds the parent lock in deps' store.
	lockHeld := func(deps Deps) bool {
		rec, found, _ := deps.Lock.Store.Read("main")
		return found && rec.Owner.Slug == "me"
	}
	tipsOf := func(relation fabricengine.ParentRelation) fabricengine.ParentTips {
		return fabricengine.ParentTips{Local: "p1", Upstream: "u2", Relation: relation}
	}

	t.Run("the lock is held at the reconcile and the tips read, and a parent behind its upstream is fast-forwarded", func(t *testing.T) {
		deps := newTestDeps(t)
		var held []string
		deps.Reconcile = func() (fabricengine.OriginReport, error) {
			held = append(held, fmt.Sprintf("reconcile:%v", lockHeld(deps)))
			return fabricengine.OriginReport{}, nil
		}
		deps.ReadParentTips = func(fetch bool) (fabricengine.ParentTips, error) {
			held = append(held, fmt.Sprintf("tips:%v:%v", fetch, lockHeld(deps)))
			return tipsOf(fabricengine.ParentBehind), nil
		}
		var order []string
		res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
		merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}, order: &order}
		fz := &Finalize{deps: deps, resolver: res, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

		shedfake.RequireOutcome(t, fz, shedengine.Done)

		if got, want := strings.Join(held, ","), "reconcile:true,tips:true:true,tips:false:true"; got != want {
			t.Errorf("lock holds at the seams = %q; want %q", got, want)
		}
		if got, want := strings.Join(order, ","), "fast-forward,merge,push"; got != want {
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
		midMerge   fabricengine.MidMergeState
		wantReason string
	}{
		{"a missing pair", errors.New("no such worktree"), nil, fabricengine.MidMergeState{}, `no live pair for parent branch "main": no such worktree; way forward: re-create the parent pair, then run "lyx loom resume"`},
		{"a dirty code checkout", nil, []string{"a.go"}, fabricengine.MidMergeState{}, DirtyParentRefusal("/hub/parent", []string{"a.go"})},
		{"a pair mid-merge", nil, nil, fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked}, `a merge is in progress in the parent pair of "main"; way forward: finish or abort that merge in the parent pair, then run "lyx loom resume"`},
	}
	states := []struct {
		name        string
		tips        fabricengine.ParentTips
		pushSkipped bool
	}{
		{"equal to its upstream", tipsOf(fabricengine.ParentEqual), false},
		{"without an upstream", fabricengine.ParentTips{Local: "p1", Relation: fabricengine.ParentNoUpstream}, false},
		{"under the push skip", tipsOf(fabricengine.ParentBehind), true},
	}
	for _, refusal := range refusals {
		for _, state := range states {
			t.Run(refusal.name+" with the parent "+state.name+" is Stuck before any merge", func(t *testing.T) {
				deps := newTestDeps(t)
				deps.PushSkipped = state.pushSkipped
				deps.ReadParentTips = func(bool) (fabricengine.ParentTips, error) { return state.tips, nil }
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
		tips       fabricengine.ParentTips
		ff         fabricengine.FastForwardResult
		wantReason string
	}{
		{"a behind parent that is no longer a fast-forward", tipsOf(fabricengine.ParentBehind), fabricengine.FastForwardResult{Refusal: fabricengine.FastForwardNotFastForward}, `the upstream of "main" is no longer a fast-forward of the parent pair; way forward: reconcile "main" with its upstream in its own pair, then run "lyx loom resume"`},
		{"a parent ahead of its upstream", tipsOf(fabricengine.ParentAhead), fabricengine.FastForwardResult{}, `the local parent branch "main" is ahead of its upstream; way forward: push or reconcile "main" in its own pair, then run "lyx loom resume"`},
		{"a parent diverged from its upstream", tipsOf(fabricengine.ParentDiverged), fabricengine.FastForwardResult{}, `the local parent branch "main" has diverged from its upstream; way forward: push or reconcile "main" in its own pair, then run "lyx loom resume"`},
	}
	for _, move := range moves {
		t.Run(move.name+" is Stuck before any merge", func(t *testing.T) {
			deps := newTestDeps(t)
			deps.ReadParentTips = func(bool) (fabricengine.ParentTips, error) { return move.tips, nil }
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
