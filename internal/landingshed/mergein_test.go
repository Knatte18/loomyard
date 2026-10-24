// mergein_test.go drives Merge-In's Call against a real parentlock.Store in a temporary directory and fake tip reads, resolver and verify seams.

package landingshed

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// tracedFailure is a failure trace with every field set, which the failure record tests tell the trace seam to return.
var tracedFailure = verifytree.FailureTrace{
	Cards: []verifytree.TracedCard{{
		Number: 3, Glyphs: []string{"internal/a#Run"}, Locations: []string{"internal/a/a.go:3"}, Batch: 2,
		Range: verifytree.CommitRange{From: "aaa", To: "bbb"},
	}},
	LaterRange: verifytree.CommitRange{From: "bbb", To: "ccc"},
	Untraced:   []string{"internal/z/z.go:1"},
}

// eventResolver is a resolver that records its call in the shared event log and the tip it was told to merge.
// lockHeld reads, when set, reports whether the parent lock was held at the call.
type eventResolver struct {
	events      *[]string
	result      mergeresolve.Result
	err         error
	tip         string
	lockHeld    func() bool
	heldAtMerge bool
	// onResolve, when set, runs at each call, standing in for the merge commit the resolver makes.
	onResolve func()
}

func (r *eventResolver) Resolve(_ context.Context, source, tip string) (mergeresolve.Result, error) {
	*r.events = append(*r.events, "merge "+source)
	r.tip = tip
	if r.lockHeld != nil {
		r.heldAtMerge = r.lockHeld()
	}
	if r.onResolve != nil {
		r.onResolve()
	}
	return r.result, r.err
}

// mergeInFixture is a MergeIn over a real lock store, with every seam but the lock a fake that logs into events.
type mergeInFixture struct {
	m      *MergeIn
	store  *parentlock.Store
	gate   *gateFixture
	events []string

	tips      fabricengine.ParentTips
	tipsErr   error
	tipsReads []tipsRead
	// head is the task head, which the merge moves to mergeHead and each fixer turn that commits moves on.
	head      string
	mergeHead string
	mergeBase string
	// holds answers HoldsWork by "base..source"; a pair it does not name holds the work, so the parent brought nothing.
	holds        map[string]bool
	holdsCalls   []string
	changedPaths map[string][]string
	changedCalls []string
	resolver     *eventResolver
	seats        *fakeFixerSeats
	// entry is the row entry RowEntry answers.
	entry int
	// reviewOff is what CodeReviewOff answers.
	reviewOff bool
	// lineChange is what LineChange answers, and lineChangeCalls records each "from..to".
	lineChange      fabricengine.LineChange
	lineChangeErr   error
	lineChangeCalls []string
}

// tipsRead is one ReadParentTips call: whether it fetched and whether the run held the parent lock during it.
type tipsRead struct {
	fetch    bool
	lockHeld bool
}

func newMergeInFixture(t *testing.T) *mergeInFixture {
	t.Helper()
	dir := t.TempDir()
	fx := &mergeInFixture{
		store:        &parentlock.Store{Dir: filepath.Join(dir, "locks"), Alive: func(int, string) bool { return true }},
		gate:         newGateFixture(t, "go test ./...", nil),
		tips:         fabricengine.ParentTips{Local: "p1", Upstream: "p1", Relation: fabricengine.ParentEqual},
		head:         "t1",
		mergeHead:    "t2",
		mergeBase:    "b0",
		holds:        map[string]bool{},
		changedPaths: map[string][]string{},
		seats:        &fakeFixerSeats{},
		entry:        1,
	}
	fx.resolver = &eventResolver{events: &fx.events, result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	fx.resolver.onResolve = func() {
		if !fx.resolver.result.AlreadyUpToDate {
			fx.head = fx.mergeHead
		}
	}
	owner := parentlock.Owner{Slug: "me", Pair: "/p/me", RunID: "r1", PID: 222, StartTime: "t2"}
	lockHeld := func() bool {
		rec, found, _ := fx.store.Read("main")
		return found && rec.Owner.SameProcess(owner)
	}
	fx.resolver.lockHeld = lockHeld

	stencilsDir := t.TempDir()
	stencilkit.SeedInto(t, stencilsDir)
	deps := Deps{
		ParentBranch:       "main",
		TaskBranch:         "task-branch",
		Slug:               "my-slug",
		ScratchDir:         dir,
		StencilsDir:        stencilsDir,
		DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
		MergeRecordPath:    MergeRecordPath(dir),
		Config:             Config{Fixer: "claude:opus", FixerTimeoutMin: 5, MergeFixAttempts: 2, FinalizeFixMaxLines: 100},
		Registry:           modelspec.Registry{},
		FixerSeats:         fx.seats,
		TaskRemoteTip:      func() (string, error) { return "remote-1", nil },
		IsTestFile:         func(path string) bool { return strings.HasSuffix(path, "_test.go") },
		RowEntry:           func(string) (int, error) { return fx.entry, nil },
		CodeReviewOff:      func() (bool, error) { return fx.reviewOff, nil },
		HoldsWork: func(base, source string) (bool, error) {
			key := base + ".." + source
			fx.holdsCalls = append(fx.holdsCalls, key)
			holds, set := fx.holds[key]
			return holds || !set, nil
		},
		LineChange: func(from, to string) (fabricengine.LineChange, error) {
			fx.lineChangeCalls = append(fx.lineChangeCalls, from+".."+to)
			return fx.lineChange, fx.lineChangeErr
		},
		CommitStatus: func() error { fx.events = append(fx.events, "commit-status"); return nil },
		Lock: ParentLockDeps{
			Store:    fx.store,
			Owner:    func() (parentlock.Owner, error) { return owner, nil },
			WaitPath: filepath.Join(dir, "wait.json"),
		},
		ReadParentTips: func(fetch bool) (fabricengine.ParentTips, error) {
			fx.events = append(fx.events, "tips")
			fx.tipsReads = append(fx.tipsReads, tipsRead{fetch: fetch, lockHeld: lockHeld()})
			return fx.tips, fx.tipsErr
		},
		TaskHead:  func() (string, error) { return fx.head, nil },
		MergeBase: func(string) (string, error) { return fx.mergeBase, nil },
		ChangedPaths: func(from, to string) ([]string, error) {
			fx.changedCalls = append(fx.changedCalls, from+".."+to)
			return fx.changedPaths[from+".."+to], nil
		},
	}
	if err := os.MkdirAll(fx.gate.paths.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fx.gate.fake.onVerify = func() { fx.events = append(fx.events, "verify") }
	fx.gate.gate.baseCommand = func() (string, error) { return gateBaseCall, nil }
	fx.gate.gate.recorder = &failureRecorder{head: deps.TaskHead}
	fx.m = &MergeIn{
		deps:     deps,
		resolver: fx.resolver,
		gate:     fx.gate.gate,
		guard:    newParentGuard(deps),
	}
	return fx
}

func (fx *mergeInFixture) call(t *testing.T) (shedengine.Outcome, shedengine.OutputPointer) {
	t.Helper()
	outcome, ptr, err := fx.m.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	return outcome, ptr
}

func (fx *mergeInFixture) readRecord(t *testing.T) MergeRecord {
	t.Helper()
	rec, found, err := ReadMergeRecord(fx.m.deps.MergeRecordPath)
	if err != nil || !found {
		t.Fatalf("ReadMergeRecord() = %v, %v; want a record", found, err)
	}
	return rec
}

func TestMergeIn_RunsStatusCommitThenLockThenSyncThenMergeAndVerify(t *testing.T) {
	t.Parallel()

	const stoppedWayForward = `fetch "main" in the prime and reconcile it by hand`
	proceeds := "commit-status,reconcile,tips,merge main,verify"
	tests := []struct {
		name         string
		report       fabricengine.OriginReport
		reconcileErr error
		wantErr      bool
		wantOutcome  shedengine.Outcome
		wantEvents   string
		wantHalt     bool
		wantInReason []string
		wantNotIn    []string
	}{
		{
			name:        "an active run proceeds to the merge",
			report:      fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "me", State: fabricengine.EntryActive}}},
			wantOutcome: shedengine.Done, wantEvents: proceeds,
		},
		{
			name:        "an unreachable origin proceeds to the merge",
			report:      fabricengine.OriginReport{Unreachable: true},
			wantOutcome: shedengine.Done, wantEvents: proceeds,
		},
		{
			name:        "a run owned elsewhere proceeds to the merge",
			report:      fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "me", State: fabricengine.EntryOwnedElsewhere, Owner: "desk"}}},
			wantOutcome: shedengine.Done, wantEvents: proceeds,
		},
		{
			name:        "an ended run that landed halts naming the landing",
			report:      fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "me", State: fabricengine.EntryEnded, How: "done"}}},
			wantOutcome: shedengine.Stuck, wantEvents: "commit-status,reconcile", wantHalt: true,
			wantInReason: []string{"its landing is done", `"lyx cleanup --apply" from the prime`},
		},
		{
			name:        "an ended run removed from the board halts naming the removal",
			report:      fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "me", State: fabricengine.EntryEnded, How: "removed"}}},
			wantOutcome: shedengine.Stuck, wantEvents: "commit-status,reconcile", wantHalt: true,
			wantInReason: []string{"removed from the board", `"lyx cleanup --apply" from the prime`},
		},
		{
			name: "a stopped run names the branch and its way forward, then the resume",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{{Branch: "main", State: fabricengine.BranchDiverged, WayForward: stoppedWayForward}},
				Entries:  []fabricengine.EntryOutcome{{Slug: "me", State: fabricengine.EntryStopped, WayForward: stoppedWayForward}},
			},
			wantOutcome: shedengine.Stuck, wantEvents: "commit-status,reconcile",
			wantInReason: []string{"branch main is diverged against origin", stoppedWayForward + `; then run "lyx loom resume"`},
			wantNotIn:    []string{"lyx cleanup --apply"},
		},
		{
			name:         "a reconcile error is returned as an error",
			reconcileErr: errors.New("ls-remote failed"),
			wantErr:      true, wantEvents: "commit-status,reconcile",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newMergeInFixture(t)
			fx.m.deps.Reconcile = func() (fabricengine.OriginReport, error) {
				fx.events = append(fx.events, "reconcile")
				return tt.report, tt.reconcileErr
			}

			outcome, ptr, err := fx.m.Call(context.Background())

			if (err != nil) != tt.wantErr {
				t.Fatalf("Call() error = %v; want an error = %v", err, tt.wantErr)
			}
			if got := strings.Join(fx.events, ","); got != tt.wantEvents {
				t.Errorf("events = %q; want %q", got, tt.wantEvents)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), "ls-remote failed") {
					t.Errorf("Call() error = %v; want the reconcile's cause", err)
				}
			} else if outcome != tt.wantOutcome || ptr.Halt != tt.wantHalt {
				t.Fatalf("Call() = %q, %+v; want %q with halt %v", outcome, ptr, tt.wantOutcome, tt.wantHalt)
			}
			for _, want := range tt.wantInReason {
				if !strings.Contains(ptr.Reason, want) {
					t.Errorf("reason = %q; want it to contain %q", ptr.Reason, want)
				}
			}
			for _, unwanted := range tt.wantNotIn {
				if strings.Contains(ptr.Reason, unwanted) {
					t.Errorf("reason = %q; want it free of %q", ptr.Reason, unwanted)
				}
			}
			if rec, found, _ := fx.store.Read("main"); found {
				t.Errorf("lock record = %+v; want Merge-In to take no parent lock", rec)
			}
		})
	}
}

func TestMergeIn_MergeTip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tips        fabricengine.ParentTips
		pushSkipped bool
		otherHolds  bool
		wantTip     string
		wantReads   []tipsRead
		wantHeld    bool
	}{
		{
			name:       "an upstream tip is merged while another run holds the lock, and no lock is taken",
			tips:       fabricengine.ParentTips{Local: "p1", Upstream: "u2", Relation: fabricengine.ParentBehind},
			otherHolds: true,
			wantTip:    "u2", wantReads: []tipsRead{{fetch: true}}, wantHeld: true,
		},
		{
			name:    "a local parent ahead of its upstream merges the upstream tip, never the local-only commit",
			tips:    fabricengine.ParentTips{Local: "p2", Upstream: "p1", Relation: fabricengine.ParentAhead},
			wantTip: "p1", wantReads: []tipsRead{{fetch: true}},
		},
		{
			name:    "a diverged local parent merges the upstream tip",
			tips:    fabricengine.ParentTips{Local: "p2", Upstream: "u2", Relation: fabricengine.ParentDiverged},
			wantTip: "u2", wantReads: []tipsRead{{fetch: true}},
		},
		{
			name:        "the push skip holds the lock for the local-tip read only",
			tips:        fabricengine.ParentTips{Local: "p1", Upstream: "u2", Relation: fabricengine.ParentBehind},
			pushSkipped: true,
			wantTip:     "p1", wantReads: []tipsRead{{fetch: false, lockHeld: true}},
		},
		{
			name:    "no upstream holds the lock for the local-tip read only",
			tips:    fabricengine.ParentTips{Local: "p1", Relation: fabricengine.ParentNoUpstream},
			wantTip: "p1", wantReads: []tipsRead{{fetch: true}, {fetch: false, lockHeld: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newMergeInFixture(t)
			fx.tips = tt.tips
			fx.m.deps.PushSkipped = tt.pushSkipped
			if tt.otherHolds {
				if _, err := fx.store.TryAcquire("main", parentlock.Owner{Slug: "other", Pair: "/p/other", RunID: "r9", PID: 111, StartTime: "t1"}); err != nil {
					t.Fatal(err)
				}
			}

			outcome, ptr := fx.call(t)

			if outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %+v; want Done", outcome, ptr)
			}
			if fx.resolver.tip != tt.wantTip {
				t.Errorf("resolver tip = %q; want %q", fx.resolver.tip, tt.wantTip)
			}
			if got := fx.readRecord(t).MergedTip; got != tt.wantTip {
				t.Errorf("merged_tip = %q; want %q", got, tt.wantTip)
			}
			if !slices.Equal(fx.tipsReads, tt.wantReads) {
				t.Errorf("ReadParentTips calls = %+v; want %+v", fx.tipsReads, tt.wantReads)
			}
			if fx.resolver.heldAtMerge {
				t.Error("the parent lock was held while the resolver ran")
			}
			if rec, found, _ := fx.store.Read("main"); found != tt.wantHeld {
				t.Errorf("lock record after Done = %+v, %v; want found = %v", rec, found, tt.wantHeld)
			}
		})
	}

	t.Run("a resolver answer that the upstream ref is missing is Stuck with its way forward", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		const reason = `the upstream ref of "main" is missing; way forward: fetch it, then run "lyx loom resume"`
		fx.resolver.result = mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: reason}

		outcome, ptr := fx.call(t)

		if outcome != shedengine.Stuck || ptr.Reason != reason || ptr.Halt {
			t.Errorf("Call() = %q, %+v; want a non-halting Stuck %q", outcome, ptr, reason)
		}
	})

	t.Run("a failed tips read is the returned error", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		fx.tipsErr = errors.New("fetch failed")

		_, _, err := fx.m.Call(context.Background())

		if err == nil || !strings.Contains(err.Error(), "fetch failed") {
			t.Errorf("Call() error = %v; want the tips read's cause", err)
		}
		if strings.Contains(strings.Join(fx.events, ","), "merge") {
			t.Errorf("a merge ran after a failed tips read: %v", fx.events)
		}
	})
}

func TestMergeIn_MergeRecord(t *testing.T) {
	t.Parallel()

	t.Run("a fresh merge records the ranges and where they overlap", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		fx.mergeHead = "m1"
		fx.changedPaths["b0..p1"] = []string{"PATTERN.md", "a/x.go", "shared.txt"}
		fx.changedPaths["b0..t1"] = []string{"a/y.go", "shared.txt"}

		fx.call(t)

		want := MergeRecord{
			ParentBranch: "main", MergedTip: "p1", AlignedTip: "p1", TaskBase: "b0", TaskHead: "t1", MergeCommit: "m1",
			Overlap: Overlap{Files: []string{"shared.txt"}, Packages: []string{"a"}, Pattern: []string{"PATTERN.md"}},
		}
		if got := fx.readRecord(t); !recordsEqual(got, want) {
			t.Errorf("record = %+v; want %+v", got, want)
		}
	})

	t.Run("an up-to-date merge records no merge commit and no overlap", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		fx.resolver.result.AlreadyUpToDate = true
		fx.changedPaths["b0..p1"] = []string{"shared.txt"}
		fx.changedPaths["b0..t1"] = []string{"shared.txt"}

		fx.call(t)

		want := MergeRecord{ParentBranch: "main", MergedTip: "p1", AlignedTip: "p1", TaskBase: "b0", TaskHead: "t1", UpToDate: true}
		if got := fx.readRecord(t); !recordsEqual(got, want) {
			t.Errorf("record = %+v; want %+v", got, want)
		}
	})

	t.Run("a second merge starts the task range at the first merge's tip and the parent range at the earlier aligned tip", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		fx.mergeHead = "m1"
		fx.changedPaths["b0..p1"] = []string{"old.txt"}
		fx.changedPaths["b0..t1"] = []string{"task.txt"}
		fx.call(t)

		fx.mergeBase = "p1"
		fx.mergeHead = "m2"
		fx.tips = fabricengine.ParentTips{Local: "p2", Upstream: "p2", Relation: fabricengine.ParentEqual}
		fx.changedPaths["p1..p2"] = []string{"new.txt"}
		fx.changedPaths["p1..m1"] = []string{"later.txt"}
		fx.changedCalls = nil
		fx.call(t)

		got := fx.readRecord(t)
		if got.TaskBase != "p1" || got.TaskHead != "m1" || got.AlignedTip != "p2" || got.MergedTip != "p2" {
			t.Errorf("record = %+v; want task_base p1, task_head m1, aligned_tip p2, merged_tip p2", got)
		}
		if want := "p1..p2,p1..m1"; strings.Join(fx.changedCalls, ",") != want {
			t.Errorf("ChangedPaths calls = %v; want %q, so the first merge's parent changes are not the task's", fx.changedCalls, want)
		}
		if len(got.Overlap.Files) != 0 {
			t.Errorf("overlap files = %v; want none", got.Overlap.Files)
		}
	})

	t.Run("an earlier aligned tip starts the parent range, and the record advances it to the merged tip", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		if err := WriteMergeRecord(fx.m.deps.MergeRecordPath, MergeRecord{AlignedTip: "a0"}); err != nil {
			t.Fatal(err)
		}
		fx.changedPaths["a0..p1"] = []string{"shared.txt"}
		fx.changedPaths["b0..t1"] = []string{"shared.txt"}

		fx.call(t)

		got := fx.readRecord(t)
		if got.AlignedTip != "p1" {
			t.Errorf("aligned_tip = %q; want the merged tip p1", got.AlignedTip)
		}
		if want := "shared.txt"; strings.Join(got.Overlap.Files, ",") != want {
			t.Errorf("overlap files = %v; want [%s], read from the earlier aligned tip", got.Overlap.Files, want)
		}
	})

	t.Run("an undecodable earlier record falls back to the fork point", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		if err := os.WriteFile(fx.m.deps.MergeRecordPath, []byte("parent_branch: [unclosed"), 0o644); err != nil {
			t.Fatal(err)
		}
		fx.changedPaths["b0..p1"] = []string{"shared.txt"}
		fx.changedPaths["b0..t1"] = []string{"shared.txt"}

		fx.call(t)

		got := fx.readRecord(t)
		if want := "shared.txt"; strings.Join(got.Overlap.Files, ",") != want {
			t.Errorf("overlap files = %v; want [%s], read from the fork point", got.Overlap.Files, want)
		}
		if want := "b0..p1"; len(fx.holdsCalls) != 1 || fx.holdsCalls[0] != want {
			t.Errorf("HoldsWork calls = %v; want one over the fork point, %q", fx.holdsCalls, want)
		}
	})
}

// recordsEqual compares two merge records field by field, treating nil and empty overlap lists alike.
func recordsEqual(a, b MergeRecord) bool {
	same := func(x, y []string) bool { return strings.Join(x, "\x00") == strings.Join(y, "\x00") }
	return a.ParentBranch == b.ParentBranch && a.MergedTip == b.MergedTip && a.AlignedTip == b.AlignedTip &&
		a.TaskBase == b.TaskBase && a.TaskHead == b.TaskHead && a.MergeCommit == b.MergeCommit && a.UpToDate == b.UpToDate &&
		same(a.Overlap.Files, b.Overlap.Files) && same(a.Overlap.Packages, b.Overlap.Packages) && same(a.Overlap.Pattern, b.Overlap.Pattern)
}

func TestMergeIn_PlanVerifyFailureRecord(t *testing.T) {
	t.Parallel()

	t.Run("a failed verify writes the failure record with row Merge-In and no merge record", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		fx.mergeHead = "m1"
		fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, Log: "/verify/verify-1.log"}

		outcome, ptr := fx.call(t)

		if outcome != shedengine.Stuck || !ptr.Halt || !strings.HasPrefix(ptr.Reason, LandingStopPrefix) || len(fx.seats.tables) != 1 {
			t.Fatalf("Call() = %q, %+v with %d fixer session(s); want the fixer spawned and a halting Stuck opening with %q", outcome, ptr, len(fx.seats.tables), LandingStopPrefix)
		}
		failure, found, err := verifytree.ReadLandingFailure(fx.gate.paths)
		if err != nil || !found {
			t.Fatalf("ReadLandingFailure() = %v, %v; want a record", found, err)
		}
		// The log names no failing tier, so the record takes the highest tier of the landing list.
		wantTier := slices.Max(fixedGateTierLists["landing"])
		if failure.MergeCommit != "m1" || failure.Kind != verifytree.FailureKindLanding || failure.Tier != wantTier {
			t.Errorf("failure record = %+v; want merge commit m1, kind landing, tier %d", failure, wantTier)
		}
		if _, found, _ := ReadMergeRecord(fx.m.deps.MergeRecordPath); found {
			t.Error("a merge record was written after a failed verify")
		}
	})

	t.Run("a told trace lands in the record, and a failing, absent or head-less trace leaves today's record", func(t *testing.T) {
		t.Parallel()
		const verifyLog = "verify output"
		tests := []struct {
			name string
			// trace is the told seam; nil leaves it unwired.
			trace func(log, head string) (verifytree.FailureTrace, error)
			// head overrides the recorder's HEAD read when non-nil.
			head      func() (string, error)
			wantTrace verifytree.FailureTrace
			wantCalls int
		}{
			{name: "a trace is copied onto the record", trace: func(string, string) (verifytree.FailureTrace, error) { return tracedFailure, nil }, wantTrace: tracedFailure, wantCalls: 1},
			{name: "a trace error writes the record without the fields", trace: func(string, string) (verifytree.FailureTrace, error) {
				return verifytree.FailureTrace{}, errors.New("no webster state")
			}, wantCalls: 1},
			{name: "a nil seam writes today's record"},
			{name: "an empty HEAD calls no trace", trace: func(string, string) (verifytree.FailureTrace, error) { return tracedFailure, nil }, head: func() (string, error) { return "", nil }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fx := newMergeInFixture(t)
				fx.mergeHead = "m1"
				fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
				writeVerifyLog(t, fx.gate, verifyLog)
				calls := 0
				if tt.trace != nil {
					fx.m.gate.recorder.traceFailure = func(log, head string) (verifytree.FailureTrace, error) {
						calls++
						if log != verifyLog || head != "m1" {
							t.Errorf("traceFailure(%q, %q); want the verify log and the record's head %q", log, head, "m1")
						}
						return tt.trace(log, head)
					}
				}
				if tt.head != nil {
					fx.m.gate.recorder.head = tt.head
				}

				outcome, ptr := fx.call(t)

				if outcome != shedengine.Stuck || !strings.HasPrefix(ptr.Reason, LandingStopPrefix) || len(fx.seats.tables) != 1 {
					t.Fatalf("Call() = %q, %+v with %d fixer session(s); want the fixer spawned and a halting Stuck", outcome, ptr, len(fx.seats.tables))
				}
				failure, found, err := verifytree.ReadLandingFailure(fx.gate.paths)
				if err != nil || !found {
					t.Fatalf("ReadLandingFailure() = %v, %v; want a record", found, err)
				}
				got := verifytree.FailureTrace{Cards: failure.Cards, LaterRange: failure.LaterRange, Untraced: failure.Untraced}
				if !reflect.DeepEqual(got, tt.wantTrace) {
					t.Errorf("record trace fields = %+v; want %+v", got, tt.wantTrace)
				}
				if calls != tt.wantCalls {
					t.Errorf("traceFailure calls = %d; want %d", calls, tt.wantCalls)
				}
			})
		}
	})

	t.Run("a passing verify clears an earlier failure record", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		if err := verifytree.WriteLandingFailure(fx.gate.paths, verifytree.LandingFailure{Kind: verifytree.FailureKindLanding}); err != nil {
			t.Fatal(err)
		}

		fx.call(t)

		if _, found, _ := verifytree.ReadLandingFailure(fx.gate.paths); found {
			t.Error("the earlier failure record is still present after a passing verify")
		}
	})
}

// TestMergeIn_VerifyGate pins the clean-tree checks and the plan verify around the merge-in:
// the verify is asked once at site Merge-In between three checks, and every stop leaves no merge record.
func TestMergeIn_VerifyGate(t *testing.T) {
	t.Parallel()

	t.Run("a passed or skipped verify lets the record be written", func(t *testing.T) {
		t.Parallel()
		for _, status := range []verifytree.Status{verifytree.StatusPassed, verifytree.StatusSkipped} {
			fx := newMergeInFixture(t)
			fx.gate.fake.result = verifytree.Result{Status: status}

			outcome, ptr := fx.call(t)

			if outcome != shedengine.Done {
				t.Fatalf("status %q: Call() = %q, %+v; want Done", status, outcome, ptr)
			}
			if wantSite := (verifytree.Site{Label: "Merge-In", BaseCommand: gateBaseCall}); fx.gate.fake.verifyCalls != 1 || fx.gate.fake.site != wantSite {
				t.Errorf("status %q: verify calls=%d site=%+v; want 1 call at site %+v", status, fx.gate.fake.verifyCalls, fx.gate.fake.site, wantSite)
			}
			if fx.gate.fake.dirtyCalls != 3 {
				t.Errorf("status %q: clean-tree checks = %d; want 3", status, fx.gate.fake.dirtyCalls)
			}
			fx.readRecord(t)
		}
	})

	t.Run("a failed, dirty or unstartable verify spawns the fixer and halts without a record when it ends", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			result verifytree.Result
		}{
			{"failed", verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 3, Log: "/verify/verify-1.log"}},
			{"dirty result", verifytree.Result{Status: verifytree.StatusDirty, Dirty: []string{"late.txt"}}},
			{"could not start", verifytree.Result{Status: verifytree.StatusFailed, ExitCode: -1, Detail: "no shell"}},
		}
		for _, tt := range tests {
			fx := newMergeInFixture(t)
			fx.gate.fake.result = tt.result

			outcome, ptr := fx.call(t)

			if outcome != shedengine.Stuck || !ptr.Halt || !strings.HasPrefix(ptr.Reason, LandingStopPrefix) {
				t.Fatalf("%s: Call() = %q, %+v; want a halting Stuck opening with %q", tt.name, outcome, ptr, LandingStopPrefix)
			}
			if len(fx.seats.tables) != 1 || fx.seats.tables[0].Values["mode"] != string(FixerModeMergeIn) {
				t.Errorf("%s: fixer sessions = %d; want one in merge-in mode", tt.name, len(fx.seats.tables))
			}
			if _, found, _ := ReadMergeRecord(fx.m.deps.MergeRecordPath); found {
				t.Errorf("%s: a merge record was written after a halted verify", tt.name)
			}
		}
	})

	t.Run("a dirty tree at each of the three points is Stuck naming the paths", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			point      int
			wantPoint  string
			wantMerge  bool
			wantVerify int
		}{
			{0, "before the merge-in", false, 0},
			{1, "after the merge-in", true, 0},
			{2, "after the verify", true, 1},
		}
		for _, tt := range tests {
			fx := newMergeInFixture(t)
			fx.gate.fake.dirtyAt(tt.point, "stray.txt", "gen/out.go")

			outcome, ptr := fx.call(t)

			if outcome != shedengine.Stuck {
				t.Fatalf("%s: Call() = %q; want Stuck", tt.wantPoint, outcome)
			}
			for _, want := range []string{tt.wantPoint, "stray.txt", "gen/out.go"} {
				if !strings.Contains(ptr.Reason, want) {
					t.Errorf("%s: reason %q lacks %q", tt.wantPoint, ptr.Reason, want)
				}
			}
			if merged := strings.Contains(strings.Join(fx.events, ","), "merge main"); merged != tt.wantMerge {
				t.Errorf("%s: merge-in ran = %v; want %v", tt.wantPoint, merged, tt.wantMerge)
			}
			if fx.gate.fake.verifyCalls != tt.wantVerify {
				t.Errorf("%s: verify calls = %d; want %d", tt.wantPoint, fx.gate.fake.verifyCalls, tt.wantVerify)
			}
			if _, found, _ := ReadMergeRecord(fx.m.deps.MergeRecordPath); found {
				t.Errorf("%s: a merge record was written on a dirty tree", tt.wantPoint)
			}
		}
	})

	t.Run("a failing check, an unreadable command and a cancellation are returned as errors", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			setup func(fx *mergeInFixture) context.Context
			// wantErrHas is a substring of the returned error, when one is required.
			wantErrHas string
		}{
			{
				name: "clean-tree check error",
				setup: func(fx *mergeInFixture) context.Context {
					fx.gate.fake.dirtyErr = errors.New("git exploded")
					return context.Background()
				},
				wantErrHas: "git exploded",
			},
			{
				name: "verify command closure error",
				setup: func(fx *mergeInFixture) context.Context {
					fx.m.gate.command = func() (string, error) { return "", errors.New("plan unreadable") }
					return context.Background()
				},
				wantErrHas: "plan unreadable",
			},
			{
				name: "cancellation",
				setup: func(fx *mergeInFixture) context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					fx.gate.fake.onVerify = cancel
					fx.gate.fake.verifyErr = context.Canceled
					return ctx
				},
				wantErrHas: context.Canceled.Error(),
			},
		}
		for _, tt := range tests {
			fx := newMergeInFixture(t)
			ctx := tt.setup(fx)

			outcome, _, err := fx.m.Call(ctx)

			if err == nil || outcome == shedengine.Stuck || !strings.Contains(err.Error(), tt.wantErrHas) {
				t.Errorf("%s: Call() = %q, %v; want a returned error containing %q", tt.name, outcome, err, tt.wantErrHas)
			}
			if _, found, _ := ReadMergeRecord(fx.m.deps.MergeRecordPath); found {
				t.Errorf("%s: a merge record was written after a returned error", tt.name)
			}
		}
	})
}

func TestMergeIn_UnreadableLockRecordHaltsNamingTheReleaseVerb(t *testing.T) {
	t.Parallel()

	fx := newMergeInFixture(t)
	fx.m.deps.PushSkipped = true
	if err := os.MkdirAll(fx.store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	recordFile := filepath.Join(fx.store.Dir, hex.EncodeToString([]byte("main"))+".json")
	if err := os.WriteFile(recordFile, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome, ptr := fx.call(t)

	if outcome != shedengine.Stuck || !ptr.Halt || !strings.Contains(ptr.Reason, `"lyx batten lock release main"`) {
		t.Errorf("Call() = %q, %+v; want a halting Stuck naming lyx batten lock release", outcome, ptr)
	}
	if got := strings.Join(fx.events, ","); got != "commit-status" {
		t.Errorf("events = %q; want only the status commit before the local-tip read's lock", got)
	}
}

func TestNewMergeIn_RefusesAMissingSeam(t *testing.T) {
	t.Parallel()

	whole := func() Deps {
		return Deps{
			OpenFabric:       func() (*fabricengine.Fabric, error) { return nil, nil },
			OpenParentFabric: func() (*fabricengine.Fabric, error) { return nil, nil },
			ReadParentTips:   func(bool) (fabricengine.ParentTips, error) { return fabricengine.ParentTips{}, nil },
			ChangedPaths:     func(string, string) ([]string, error) { return nil, nil },
			MergeBase:        func(string) (string, error) { return "", nil },
			TaskHead:         func() (string, error) { return "", nil },
			Lock:             ParentLockDeps{Store: &parentlock.Store{}},
			MergeRecordPath:  "/scratch/merge-record.yaml",
		}
	}
	tests := []struct {
		name   string
		remove func(*Deps)
		want   string
	}{
		{"OpenFabric", func(d *Deps) { d.OpenFabric = nil }, "Deps.OpenFabric must not be nil"},
		{"OpenParentFabric", func(d *Deps) { d.OpenParentFabric = nil }, "Deps.OpenParentFabric must not be nil"},
		{"ReadParentTips", func(d *Deps) { d.ReadParentTips = nil }, "Deps.ReadParentTips must not be nil"},
		{"ChangedPaths", func(d *Deps) { d.ChangedPaths = nil }, "Deps.ChangedPaths must not be nil"},
		{"MergeBase", func(d *Deps) { d.MergeBase = nil }, "Deps.MergeBase must not be nil"},
		{"TaskHead", func(d *Deps) { d.TaskHead = nil }, "Deps.TaskHead must not be nil"},
		{"Lock.Store", func(d *Deps) { d.Lock.Store = nil }, "Deps.Lock.Store must not be nil"},
		{"MergeRecordPath", func(d *Deps) { d.MergeRecordPath = "" }, "Deps.MergeRecordPath must not be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := whole()
			tt.remove(&deps)

			m, err := NewMergeIn(deps)

			if m != nil || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("NewMergeIn() = %v, %v; want an error containing %q", m, err, tt.want)
			}
		})
	}
}
