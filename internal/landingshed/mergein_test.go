// mergein_test.go drives Merge-In's Call against a real parentlock.Store in a temporary directory and fake tip reads, parent puller, resolver and verify seams.

package landingshed

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// eventResolver is a resolver that records its call in the shared event log.
type eventResolver struct {
	events *[]string
	result mergeresolve.Result
	err    error
}

func (r *eventResolver) Resolve(_ context.Context, source string) (mergeresolve.Result, error) {
	*r.events = append(*r.events, "merge "+source)
	return r.result, r.err
}

// fakeParentPuller is a parentPuller that records its call in the shared event log.
type fakeParentPuller struct {
	events *[]string
	opts   fabricengine.SyncOptions
	result fabricengine.FastForwardResult
	err    error
}

func (p *fakeParentPuller) FastForwardPair(opts fabricengine.SyncOptions) (fabricengine.FastForwardResult, error) {
	*p.events = append(*p.events, "fast-forward")
	p.opts = opts
	return p.result, p.err
}

// mergeInFixture is a MergeIn over a real lock store, with every seam but the lock a fake that logs into events.
type mergeInFixture struct {
	m      *MergeIn
	store  *parentlock.Store
	gate   *gateFixture
	events []string

	tips         fabricengine.ParentTips
	tipsFetch    *bool
	lockHeld     bool
	taskHeads    []string
	mergeBase    string
	changedPaths map[string][]string
	changedCalls []string
	puller       *fakeParentPuller
	pullerOpen   error
	resolver     *eventResolver
}

func newMergeInFixture(t *testing.T) *mergeInFixture {
	t.Helper()
	dir := t.TempDir()
	fx := &mergeInFixture{
		store:        &parentlock.Store{Dir: filepath.Join(dir, "locks"), Alive: func(int, string) bool { return true }},
		gate:         newGateFixture(t, "go test ./...", nil),
		tips:         fabricengine.ParentTips{Local: "p1", Upstream: "p1", Relation: fabricengine.ParentEqual},
		taskHeads:    []string{"t1", "t2"},
		mergeBase:    "b0",
		changedPaths: map[string][]string{},
	}
	fx.resolver = &eventResolver{events: &fx.events, result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	fx.puller = &fakeParentPuller{events: &fx.events}
	owner := parentlock.Owner{Slug: "me", Pair: "/p/me", RunID: "r1", PID: 222, StartTime: "t2"}

	headCalls := 0
	deps := Deps{
		ParentBranch:      "main",
		MergeRecordPath:   MergeRecordPath(dir),
		MergeReviewedPath: MergeReviewedPath(dir),
		CommitStatus:      func() error { fx.events = append(fx.events, "commit-status"); return nil },
		Lock: ParentLockDeps{
			Store:    fx.store,
			Owner:    func() (parentlock.Owner, error) { return owner, nil },
			WaitPath: filepath.Join(dir, "wait.json"),
		},
		ReadParentTips: func(fetch bool) (fabricengine.ParentTips, error) {
			fx.events = append(fx.events, "tips")
			fx.tipsFetch = &fetch
			rec, found, _ := fx.store.Read("main")
			fx.lockHeld = found && rec.Owner.SameProcess(owner)
			return fx.tips, nil
		},
		TaskHead: func() (string, error) {
			head := fx.taskHeads[min(headCalls, len(fx.taskHeads)-1)]
			headCalls++
			return head, nil
		},
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
	fx.gate.gate.failureTarget = "Told-Row"
	fx.gate.gate.recorder = &failureRecorder{head: deps.TaskHead}
	fx.m = &MergeIn{
		deps:         deps,
		resolver:     fx.resolver,
		gate:         fx.gate.gate,
		guard:        newParentGuard(deps),
		parentOpener: func() (parentPuller, error) { return fx.puller, fx.pullerOpen },
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

	fx := newMergeInFixture(t)

	outcome, _ := fx.call(t)
	if outcome != shedengine.Done {
		t.Fatalf("Call() = %q; want Done", outcome)
	}
	if got, want := strings.Join(fx.events, ","), "commit-status,tips,merge main,verify"; got != want {
		t.Errorf("events = %q; want %q", got, want)
	}
	if !fx.lockHeld {
		t.Error("the parent lock was not held when the tips were read")
	}
	if rec, found, _ := fx.store.Read("main"); !found || rec.Owner.Slug != "me" {
		t.Errorf("lock record after Done = %+v, %v; want it kept by the run", rec, found)
	}
}

func TestMergeIn_SyncParent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tips        fabricengine.ParentTips
		pushSkipped bool
		openErr     error
		fastForward fabricengine.FastForwardResult
		wantFetch   bool
		wantPulled  bool
		wantStuck   string
		wantTip     string
	}{
		{name: "equal", tips: fabricengine.ParentTips{Local: "p1", Upstream: "p1", Relation: fabricengine.ParentEqual}, wantFetch: true, wantTip: "p1"},
		{name: "no upstream", tips: fabricengine.ParentTips{Local: "p1", Relation: fabricengine.ParentNoUpstream}, wantFetch: true, wantTip: "p1"},
		{name: "the push skip ignores a moved upstream", tips: fabricengine.ParentTips{Local: "p1", Upstream: "u2", Relation: fabricengine.ParentBehind}, pushSkipped: true, wantTip: "p1"},
		{
			name:        "behind fast-forwards to the tip the pair moved to",
			tips:        fabricengine.ParentTips{Local: "p1", Upstream: "u2", Relation: fabricengine.ParentBehind},
			fastForward: fabricengine.FastForwardResult{NewHead: "u3"},
			wantFetch:   true, wantPulled: true, wantTip: "u3",
		},
		{
			name:      "ahead",
			tips:      fabricengine.ParentTips{Local: "p2", Upstream: "p1", Relation: fabricengine.ParentAhead},
			wantFetch: true,
			wantStuck: `the local parent branch "main" is ahead of its upstream; way forward: push or reconcile "main" in its own pair, then run "lyx loom resume"`,
		},
		{
			name:      "diverged",
			tips:      fabricengine.ParentTips{Local: "p2", Upstream: "u2", Relation: fabricengine.ParentDiverged},
			wantFetch: true,
			wantStuck: `the local parent branch "main" has diverged from its upstream; way forward: push or reconcile "main" in its own pair, then run "lyx loom resume"`,
		},
		{
			name:      "no parent pair",
			tips:      fabricengine.ParentTips{Local: "p1", Upstream: "u2", Relation: fabricengine.ParentBehind},
			openErr:   errors.New("not wired"),
			wantFetch: true,
			wantStuck: `no live pair for parent branch "main": not wired; way forward: re-create the parent pair, then run "lyx loom resume"`,
		},
		{
			name:        "dirty parent pair",
			tips:        fabricengine.ParentTips{Local: "p1", Upstream: "u2", Relation: fabricengine.ParentBehind},
			fastForward: fabricengine.FastForwardResult{Refusal: fabricengine.FastForwardDirty},
			wantFetch:   true, wantPulled: true,
			wantStuck: `the parent pair of "main" has uncommitted changes; way forward: commit or stash them in the parent pair, then run "lyx loom resume"`,
		},
		{
			name:        "merge in progress in the parent pair",
			tips:        fabricengine.ParentTips{Local: "p1", Upstream: "u2", Relation: fabricengine.ParentBehind},
			fastForward: fabricengine.FastForwardResult{Refusal: fabricengine.FastForwardMergeInProgress},
			wantFetch:   true, wantPulled: true,
			wantStuck: `a merge is in progress in the parent pair of "main"; way forward: finish or abort that merge in the parent pair, then run "lyx loom resume"`,
		},
		{
			name:        "upstream no longer a fast-forward",
			tips:        fabricengine.ParentTips{Local: "p1", Upstream: "u2", Relation: fabricengine.ParentBehind},
			fastForward: fabricengine.FastForwardResult{Refusal: fabricengine.FastForwardNotFastForward},
			wantFetch:   true, wantPulled: true,
			wantStuck: `the upstream of "main" is no longer a fast-forward of the parent pair; way forward: reconcile "main" with its upstream in its own pair, then run "lyx loom resume"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newMergeInFixture(t)
			fx.tips = tt.tips
			fx.m.deps.PushSkipped = tt.pushSkipped
			fx.pullerOpen = tt.openErr
			fx.puller.result = tt.fastForward

			outcome, ptr := fx.call(t)

			if fx.tipsFetch == nil || *fx.tipsFetch != tt.wantFetch {
				t.Errorf("ReadParentTips fetch = %v; want %v", fx.tipsFetch, tt.wantFetch)
			}
			pulled := strings.Contains(strings.Join(fx.events, ","), "fast-forward")
			if pulled != tt.wantPulled {
				t.Errorf("fast-forward ran = %v; want %v (events %v)", pulled, tt.wantPulled, fx.events)
			}
			if tt.wantStuck != "" {
				if outcome != shedengine.Stuck || ptr.Reason != tt.wantStuck || ptr.Halt {
					t.Errorf("Call() = %q, %+v; want a non-halting Stuck %q", outcome, ptr, tt.wantStuck)
				}
				if strings.Contains(strings.Join(fx.events, ","), "merge") {
					t.Errorf("a merge ran after a Stuck sync: %v", fx.events)
				}
				return
			}
			if outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %+v; want Done", outcome, ptr)
			}
			if got := fx.readRecord(t).MergedTip; got != tt.wantTip {
				t.Errorf("merged_tip = %q; want %q", got, tt.wantTip)
			}
			if tt.wantPulled && fx.puller.opts != (fabricengine.SyncOptions{}) {
				t.Errorf("FastForwardPair options = %+v; want the told push skip, false", fx.puller.opts)
			}
		})
	}
}

func TestMergeIn_MergeRecord(t *testing.T) {
	t.Parallel()

	t.Run("a fresh merge records the ranges and where they overlap", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		fx.taskHeads = []string{"t1", "m1"}
		fx.changedPaths["b0..p1"] = []string{"PATTERN.md", "a/x.go", "shared.txt"}
		fx.changedPaths["b0..t1"] = []string{"a/y.go", "shared.txt"}

		fx.call(t)

		want := MergeRecord{
			ParentBranch: "main", MergedTip: "p1", ReviewedTip: "b0", TaskBase: "b0", TaskHead: "t1", MergeCommit: "m1",
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

		want := MergeRecord{ParentBranch: "main", MergedTip: "p1", ReviewedTip: "b0", TaskBase: "b0", TaskHead: "t1", UpToDate: true}
		if got := fx.readRecord(t); !recordsEqual(got, want) {
			t.Errorf("record = %+v; want %+v", got, want)
		}
	})

	t.Run("a second merge starts the task range at the first merge's tip and the parent range at the earlier reviewed tip", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		fx.taskHeads = []string{"t1", "m1", "m1", "m2"}
		fx.changedPaths["b0..p1"] = []string{"old.txt"}
		fx.changedPaths["b0..t1"] = []string{"task.txt"}
		fx.call(t)

		fx.mergeBase = "p1"
		fx.tips = fabricengine.ParentTips{Local: "p2", Upstream: "p2", Relation: fabricengine.ParentEqual}
		fx.changedPaths["b0..p2"] = []string{"old.txt", "new.txt"}
		fx.changedPaths["p1..m1"] = []string{"later.txt"}
		fx.changedCalls = nil
		fx.call(t)

		got := fx.readRecord(t)
		if got.TaskBase != "p1" || got.TaskHead != "m1" || got.ReviewedTip != "b0" || got.MergedTip != "p2" {
			t.Errorf("record = %+v; want task_base p1, task_head m1, reviewed_tip b0, merged_tip p2", got)
		}
		if want := "b0..p2,p1..m1"; strings.Join(fx.changedCalls, ",") != want {
			t.Errorf("ChangedPaths calls = %v; want %q, so the first merge's parent changes are not the task's", fx.changedCalls, want)
		}
		if len(got.Overlap.Files) != 0 {
			t.Errorf("overlap files = %v; want none", got.Overlap.Files)
		}
	})

	t.Run("the reviewed-tip file outranks the earlier record", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		if err := WriteMergeRecord(fx.m.deps.MergeRecordPath, MergeRecord{ReviewedTip: "from-record"}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fx.m.deps.MergeReviewedPath, []byte("from-file\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		fx.call(t)

		if got := fx.readRecord(t).ReviewedTip; got != "from-file" {
			t.Errorf("reviewed_tip = %q; want from-file", got)
		}
	})
}

// recordsEqual compares two merge records field by field, treating nil and empty overlap lists alike.
func recordsEqual(a, b MergeRecord) bool {
	same := func(x, y []string) bool { return strings.Join(x, "\x00") == strings.Join(y, "\x00") }
	return a.ParentBranch == b.ParentBranch && a.MergedTip == b.MergedTip && a.ReviewedTip == b.ReviewedTip &&
		a.TaskBase == b.TaskBase && a.TaskHead == b.TaskHead && a.MergeCommit == b.MergeCommit && a.UpToDate == b.UpToDate &&
		same(a.Overlap.Files, b.Overlap.Files) && same(a.Overlap.Packages, b.Overlap.Packages) && same(a.Overlap.Pattern, b.Overlap.Pattern)
}

func TestMergeIn_PlanVerifyFailureRecord(t *testing.T) {
	t.Parallel()

	t.Run("a failed verify writes the failure record with row Merge-In and no merge record", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		fx.taskHeads = []string{"t1", "m1"}
		fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, Log: "/verify/verify-1.log"}

		outcome, ptr := fx.call(t)

		if outcome != shedengine.Stuck || !strings.Contains(ptr.Reason, "verify failed after merging parent branch") {
			t.Fatalf("Call() = %q, %+v; want Stuck for the failed verify", outcome, ptr)
		}
		failure, found, err := verifytree.ReadPublishFailure(fx.gate.paths)
		if err != nil || !found {
			t.Fatalf("ReadPublishFailure() = %v, %v; want a record", found, err)
		}
		if failure.Row != "Merge-In" || failure.Target != "Told-Row" || failure.MergeCommit != "m1" || failure.Kind != verifytree.FailureKindPlanVerify {
			t.Errorf("failure record = %+v; want row Merge-In, target Told-Row, merge commit m1, kind plan verify", failure)
		}
		if _, found, _ := ReadMergeRecord(fx.m.deps.MergeRecordPath); found {
			t.Error("a merge record was written after a failed verify")
		}
	})

	t.Run("a passing verify clears an earlier failure record", func(t *testing.T) {
		t.Parallel()
		fx := newMergeInFixture(t)
		if err := verifytree.WritePublishFailure(fx.gate.paths, verifytree.PublishFailure{Row: "Merge-In"}); err != nil {
			t.Fatal(err)
		}

		fx.call(t)

		if _, found, _ := verifytree.ReadPublishFailure(fx.gate.paths); found {
			t.Error("the earlier failure record is still present after a passing verify")
		}
	})
}

func TestMergeIn_UnreadableLockRecordHaltsNamingTheReleaseVerb(t *testing.T) {
	t.Parallel()

	fx := newMergeInFixture(t)
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
		t.Errorf("events = %q; want only the status commit before the lock", got)
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
