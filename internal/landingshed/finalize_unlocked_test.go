// finalize_unlocked_test.go drives Finalize's lock-free merge and gate and its race with a parent that moves.
// It runs over the gate fixture and a real parentlock.Store.

package landingshed

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// tipRecordingResolver records the tip of every merge of the parent and answers each as merged.
type tipRecordingResolver struct {
	tips []string
	// local, when set, is the local parent branch's head, which a merge of "" brings in.
	local *string
	// merged is the parent commit the last merge brought in.
	merged string
}

func (r *tipRecordingResolver) Resolve(_ context.Context, _, tip string) (mergeresolve.Result, error) {
	r.tips = append(r.tips, tip)
	r.merged = tip
	if tip == "" && r.local != nil {
		r.merged = *r.local
	}
	return resolved(false), nil
}

// TestFinalize_GatesWithNoLockAndLocksToLand pins which merges and gates run with the parent lock held.
// The merge and the gate run with no lock, and the locked pass lands while the parent's head is the tip they merged.
// The locked pass then verifies nothing, since the tree it merges has passed already.
// A parent that moved before the lock is merged and gated again with no lock until the lost-race bound is spent.
// It is then merged and gated under the lock.
// A bound of 0 and a parent already holding the task's work merge and gate under the lock alone.
// A local parent behind its upstream is fast-forwarded before its head is compared with the gated tip.
func TestFinalize_GatesWithNoLockAndLocksToLand(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		lostRaces int
		held      bool
		// behind makes the local parent p0 sit behind its upstream u1 until the parent pair is fast-forwarded.
		behind bool
		// movesAt lists the verify calls, counted from one, during which another landing moves the parent.
		movesAt []int
		// wantLockAtVerify is whether the run held the parent lock at each verify of a tree not verified before, in order.
		wantLockAtVerify []bool
		// wantTips is the tip of each merge of the parent, in order; "" merges the local parent branch.
		wantTips []string
		// wantParentCalls is what the parent pair was asked to do, in order.
		wantParentCalls []string
	}{
		{name: "an unmoved parent lands on the tree gated with no lock", lostRaces: 2, wantLockAtVerify: []bool{false}, wantTips: []string{"p1", ""}, wantParentCalls: []string{"merge", "push"}},
		{name: "a parent that moved before the lock is merged and gated again with no lock", lostRaces: 2, movesAt: []int{1}, wantLockAtVerify: []bool{false, false}, wantTips: []string{"p1", "p2", ""}, wantParentCalls: []string{"merge", "push"}},
		{name: "a spent lost-race bound merges and gates under the lock", lostRaces: 1, movesAt: []int{1}, wantLockAtVerify: []bool{false, true}, wantTips: []string{"p1", ""}, wantParentCalls: []string{"merge", "push"}},
		{name: "a bound of 0 merges and gates under the lock from the start", wantLockAtVerify: []bool{true}, wantTips: []string{""}, wantParentCalls: []string{"merge", "push"}},
		{name: "a parent already holding the work merges and gates under the lock", lostRaces: 2, held: true, wantLockAtVerify: []bool{true}, wantTips: []string{""}, wantParentCalls: []string{"merge", "push"}},
		{name: "a parent behind its upstream is fast-forwarded to the gated tip", lostRaces: 2, behind: true, wantLockAtVerify: []bool{false}, wantTips: []string{"u1", ""}, wantParentCalls: []string{"fast-forward", "merge", "push"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newFinalizeGateFixture(t)
			parent := "p1"
			if tt.behind {
				parent = "p0"
				fx.merger.onFastForward = func() { parent = "u1" }
			}
			fx.fz.deps.Config.FinalizeLostRaces = tt.lostRaces
			fx.fz.deps.ReadParentTips = func(bool) (fabricengine.ParentTips, error) {
				if tt.behind {
					return fabricengine.ParentTips{Local: parent, Upstream: "u1", Relation: fabricengine.ParentBehind}, nil
				}
				return fabricengine.ParentTips{Local: parent, Upstream: parent, Relation: fabricengine.ParentEqual}, nil
			}
			fx.fz.deps.ParentHoldsTaskWork = func() (bool, error) { return tt.held, nil }
			fx.merger.preMergeHeadSHA = &parent
			var parentCalls []string
			fx.merger.order = &parentCalls
			res := &tipRecordingResolver{local: &parent}
			fx.fz.resolver = res
			fx.fz.guard = newParentGuard(fx.deps)
			lockHeld := func() bool {
				record, found, _ := fx.deps.Lock.Store.Read("main")
				return found && record.Owner.Slug == "me"
			}
			var lockAtVerify []bool
			// gateAskedUnderLock records a gate call with the lock held, whether it verified or skipped the tree.
			gateAskedUnderLock := false
			verified := map[string]bool{}
			fx.gate.fake.onVerify = func() {
				gateAskedUnderLock = gateAskedUnderLock || lockHeld()
				// The verified tree is named by the parent commit merged into it, and a tree that passed is skipped.
				if verified[res.merged] {
					fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusSkipped}
					return
				}
				verified[res.merged] = true
				fx.gate.fake.result = verifytree.Result{Status: verifytree.StatusPassed}
				lockAtVerify = append(lockAtVerify, lockHeld())
				if slices.Contains(tt.movesAt, fx.gate.fake.verifyCalls) {
					parent = "p2"
				}
			}

			outcome, ptr := fx.call(t)

			fx.requireLanded(t, outcome, ptr)
			if !gateAskedUnderLock {
				t.Error("the locked pass never ran the gate over its merged tree")
			}
			if !slices.Equal(lockAtVerify, tt.wantLockAtVerify) {
				t.Errorf("lock held at each verify = %v; want %v", lockAtVerify, tt.wantLockAtVerify)
			}
			if !slices.Equal(res.tips, tt.wantTips) {
				t.Errorf("merged parent tips = %q; want %q", res.tips, tt.wantTips)
			}
			if !slices.Equal(parentCalls, tt.wantParentCalls) {
				t.Errorf("parent pair calls = %q; want %q", parentCalls, tt.wantParentCalls)
			}
			if lockHeld() {
				t.Error("the lock outlived the landed Finalize")
			}
		})
	}
}

// TestFinalize_QueuesAtTheFrontAfterALostRace pins the queue place of every lock wait of a run that lost a race.
// Another run holds the lock at each of the run's acquires, so each one waits with a ticket.
func TestFinalize_QueuesAtTheFrontAfterALostRace(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		pushSkipped bool
		// wantFronts is whether each lock wait queued at the front, in order.
		wantFronts []bool
	}{
		{"the locked pass after a lost race", false, []bool{false, true}},
		{"the local tip read and the locked pass after a lost race", true, []bool{false, false, true, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newFinalizeGateFixture(t)
			parent := "p1"
			fx.fz.deps.PushSkipped = tt.pushSkipped
			fx.fz.deps.Config.FinalizeLostRaces = 2
			fx.fz.deps.ReadParentTips = func(bool) (fabricengine.ParentTips, error) {
				return fabricengine.ParentTips{Local: parent, Upstream: parent, Relation: fabricengine.ParentEqual}, nil
			}
			fx.merger.preMergeHeadSHA = &parent
			fx.fz.resolver = &tipRecordingResolver{}

			store := fx.deps.Lock.Store
			other := parentlock.Owner{Slug: "other", Pair: "/p/other", RunID: "r2", PID: 111, StartTime: "t1"}
			contend := func() {
				if _, found, _ := store.Read("main"); found {
					return
				}
				if _, err := store.TryAcquire("main", other); err != nil {
					t.Errorf("take the lock for the other run: %v", err)
				}
			}
			var fronts []bool
			fx.fz.guard = newParentGuard(fx.deps)
			fx.fz.guard.sleep = func(context.Context, time.Duration) error {
				entries, err := store.List()
				if err != nil || len(entries) != 1 || len(entries[0].Waiters) != 1 {
					t.Errorf("List() = %+v, %v; want one record with one waiter", entries, err)
				} else {
					fronts = append(fronts, entries[0].Waiters[0].Front)
				}
				_, err = store.Release("main", other)
				return err
			}
			fx.fz.deps.ParentHoldsTaskWork = func() (bool, error) {
				contend()
				return false, nil
			}
			fx.gate.fake.onVerify = func() {
				contend()
				if fx.gate.fake.verifyCalls == 1 {
					parent = "p2"
				}
			}

			outcome, ptr := fx.call(t)

			fx.requireLanded(t, outcome, ptr)
			if !slices.Equal(fronts, tt.wantFronts) {
				t.Errorf("front of each lock wait = %v; want %v", fronts, tt.wantFronts)
			}
		})
	}
}

// TestFinalize_LockFreePassStops pins how a failing lock-free pass, or a failing compare after it, ends the call.
// A run this machine may not land halts before anything is merged into its worktree.
func TestFinalize_LockFreePassStops(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	injected := errors.New("injected failure")
	reconciles := func(entry fabricengine.EntryOutcome) func(*testing.T, *finalizeGateFixture) {
		return func(_ *testing.T, fx *finalizeGateFixture) {
			fx.fz.deps.Reconcile = func() (fabricengine.OriginReport, error) {
				return fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{entry}}, nil
			}
		}
	}
	tests := []struct {
		name      string
		configure func(*testing.T, *finalizeGateFixture)
		// wantHalt names what the halting reason must carry; empty wants the call to return an error.
		wantHalt []string
		// wantError is what the returned error's text must carry.
		wantError string
		// wantTips is the tip of each merge of the parent before the call stopped.
		wantTips []string
	}{
		{
			name:      "a run owned elsewhere",
			configure: reconciles(fabricengine.EntryOutcome{Slug: "me", State: fabricengine.EntryOwnedElsewhere, Owner: "desk"}),
			wantHalt:  []string{OwnerHaltPrefix, `machine "desk"`},
		},
		{
			name:      "a run origin ended",
			configure: reconciles(fabricengine.EntryOutcome{Slug: "me", State: fabricengine.EntryEnded, How: "removed"}),
			wantHalt:  []string{"origin ended this run"},
		},
		{
			name: "a failed read of whether the parent holds the work",
			configure: func(_ *testing.T, fx *finalizeGateFixture) {
				fx.fz.deps.ParentHoldsTaskWork = func() (bool, error) { return false, injected }
			},
			wantHalt: []string{OwnerHaltPrefix, injected.Error()},
		},
		{
			name: "a paused lock wait for the local tip read",
			configure: func(t *testing.T, fx *finalizeGateFixture) {
				other := parentlock.Owner{Slug: "other", Pair: "/p/other", RunID: "r2", PID: 111, StartTime: "t1"}
				if _, err := fx.deps.Lock.Store.TryAcquire("main", other); err != nil {
					t.Fatalf("take the lock for the other run: %v", err)
				}
				fx.fz.deps.PushSkipped = true
				fx.fz.deps.Lock.PauseRequested = func() (bool, error) { return true, nil }
			},
			wantHalt: []string{"halted while waiting for the parent lock"},
		},
		{
			name: "a failed read of the parent's tips",
			configure: func(_ *testing.T, fx *finalizeGateFixture) {
				fx.fz.deps.ReadParentTips = func(bool) (fabricengine.ParentTips, error) { return fabricengine.ParentTips{}, injected }
			},
			wantError: "Finalize: read the tips of parent branch",
		},
		{
			name:      "a failed read of the parent's head under the lock",
			configure: func(_ *testing.T, fx *finalizeGateFixture) { fx.merger.headErr = injected },
			wantError: `read the head of parent branch "main"`,
			wantTips:  []string{testParentTip},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newFinalizeGateFixture(t)
			fx.fz.deps.Config.FinalizeLostRaces = 2
			res := &tipRecordingResolver{}
			fx.fz.resolver = res
			tt.configure(t, fx)
			fx.fz.guard = newParentGuard(fx.fz.deps)

			outcome, ptr, err := fx.fz.Call(context.Background())

			if len(tt.wantHalt) > 0 {
				if err != nil || outcome != shedengine.Stuck || !ptr.Halt {
					t.Fatalf("Call() = %q, %+v, %v; want a halting Stuck", outcome, ptr, err)
				}
				for _, want := range tt.wantHalt {
					if !strings.Contains(ptr.Reason, want) {
						t.Errorf("reason = %q; want it to carry %q", ptr.Reason, want)
					}
				}
			} else if !errors.Is(err, injected) || !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("Call() error = %v; want %q wrapping %v", err, tt.wantError, injected)
			}
			if !slices.Equal(res.tips, tt.wantTips) {
				t.Errorf("merged parent tips = %q; want %q", res.tips, tt.wantTips)
			}
			if len(fx.merger.calls) != 0 {
				t.Errorf("parent-side merges = %d; want none", len(fx.merger.calls))
			}
		})
	}
}

// TestFinalize_FixerInTheLockFreePass pins a gate that fails with no lock held.
// The fixer runs with no lock and reads the merged parent tip from the merge record, and its fixed tree lands.
func TestFinalize_FixerInTheLockFreePass(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	fx := newFinalizeGateFixture(t)
	fx.fz.deps.Config.FinalizeLostRaces = 2
	fx.fz.deps.ReadParentTips = func(bool) (fabricengine.ParentTips, error) {
		return fabricengine.ParentTips{Local: "p1", Upstream: "p1", Relation: fabricengine.ParentEqual}, nil
	}
	fx.merger.preMergeHeadSHA = stringPointer("p1")
	fx.fz.guard = newParentGuard(fx.deps)
	fx.lineChange = fabricengine.LineChange{Lines: 3, Paths: []string{"a/a.go"}}
	fx.scriptGate(verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}, verifytree.Result{Status: verifytree.StatusPassed})
	sessions := 0
	fx.seats.turns = []func(){func() {
		sessions++
		if _, found, err := fx.deps.Lock.Store.Read("main"); found || err != nil {
			t.Errorf("lock record at the fixer's spawn: found %v, error %v; want none", found, err)
		}
		record, _, err := ReadMergeRecord(fx.deps.MergeRecordPath)
		if err != nil || record.MergedTip != "p1" {
			t.Errorf("merge record at the fixer's spawn = %+v, %v; want merged tip p1", record, err)
		}
		fx.turn(t, fixedReport, "f1")()
	}}

	outcome, ptr := fx.call(t)

	fx.requireLanded(t, outcome, ptr)
	if sessions != 1 {
		t.Errorf("fixer sessions = %d; want 1", sessions)
	}
	if _, found, _ := fx.deps.Lock.Store.Read("main"); found {
		t.Error("the lock outlived the landed Finalize")
	}
}
