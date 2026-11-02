// finalize_recheck_test.go drives Finalize's lock re-check before the landing over a real parentlock.Store, with the parent-records commit standing in for whatever releases the record between the acquire and the re-check.

package landingshed

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestFinalize_LockRecheckAndLanding pins what Finalize does when the parent lock record no longer names the run just before the landing:
// it lands nothing, halts with LandingStopPrefix and takes no lock, and its resume re-runs from step 1 under a lock taken afresh.
// The parent-records commit runs release, so the release happens between the guard's acquire and the re-check within one Call.
func TestFinalize_LockRecheckAndLanding(t *testing.T) {
	tiergate.Require(t, 0)

	// newFixture builds a Finalize over a passing gate whose parent-records commit runs release at its call numbered at, counted from one, and at no other.
	newFixture := func(t *testing.T, at int, release func(fx *finalizeGateFixture)) *finalizeGateFixture {
		t.Helper()
		fx := newFinalizeGateFixture(t)
		fx.fz.guard = newParentGuard(fx.deps)
		calls := 0
		fx.fz.deps.CommitParentRecords = func() error {
			calls++
			if calls == at {
				release(fx)
			}
			return nil
		}
		return fx
	}
	releaseOwn := func(fx *finalizeGateFixture) {
		owner, err := fx.deps.Lock.Owner()
		if err != nil {
			t.Error(err)
			return
		}
		if _, err := fx.deps.Lock.Store.Release("main", owner); err != nil {
			t.Error(err)
		}
	}
	lockHeld := func(fx *finalizeGateFixture) bool {
		_, found, _ := fx.deps.Lock.Store.Read("main")
		return found
	}
	haltFragments := []string{"no longer names this run", "an owner pause", "`lym batten lock release`", "`lym loom resume` once no owner pause stands"}

	releases := []struct {
		name    string
		release func(fx *finalizeGateFixture)
	}{
		{"released by an owner pause", releaseOwn},
		{"removed by an operator's release", func(fx *finalizeGateFixture) {
			if _, err := fx.deps.Lock.Store.ForceRelease("main"); err != nil {
				t.Error(err)
			}
		}},
	}
	for _, tt := range releases {
		t.Run("a record "+tt.name+" before the landing halts it, and the resume lands under a fresh lock", func(t *testing.T) {
			fx := newFixture(t, 1, tt.release)

			outcome, ptr := fx.call(t)

			requireHalt(t, outcome, ptr, haltFragments...)
			if len(fx.merger.calls) != 0 || len(fx.merger.pushCalls) != 0 {
				t.Errorf("parent-side merges = %d, pushes = %d; want none after the halt", len(fx.merger.calls), len(fx.merger.pushCalls))
			}
			if lockHeld(fx) {
				t.Error("the halted Finalize holds the parent lock; want it to have taken none")
			}

			outcome, ptr = fx.call(t)

			fx.requireLanded(t, outcome, ptr)
			if len(fx.merger.pushCalls) != 1 {
				t.Errorf("pushes = %d; want the resume's one", len(fx.merger.pushCalls))
			}
			if lockHeld(fx) {
				t.Error("the lock outlived the landed Finalize")
			}
		})
	}

	t.Run("a run owned by another machine on the resume stops at the owner check without landing", func(t *testing.T) {
		fx := newFixture(t, 1, releaseOwn)
		outcome, ptr := fx.call(t)
		requireHalt(t, outcome, ptr, haltFragments...)

		fx.fz.deps.Reconcile = func() (fabricengine.OriginReport, error) {
			return fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "me", State: fabricengine.EntryOwnedElsewhere, Owner: "other-box"}}}, nil
		}
		outcome, ptr = fx.call(t)

		if outcome != shedengine.Stuck || !ptr.Halt || !strings.HasPrefix(ptr.Reason, OwnerHaltPrefix) || !strings.Contains(ptr.Reason, `"other-box"`) {
			t.Errorf("Call() = %q, %+v; want a halting Stuck behind %q naming machine other-box", outcome, ptr, OwnerHaltPrefix)
		}
		if len(fx.merger.calls) != 0 {
			t.Errorf("parent-side merges = %d; want none", len(fx.merger.calls))
		}
	})

	t.Run("a release during the retry after a merge-in-required error halts before the retry's landing", func(t *testing.T) {
		fx := newFixture(t, 2, releaseOwn)
		fx.merger.results = []mergeCallResult{{err: &fabricengine.ErrMergeInRequired{Source: fx.deps.TaskBranch}}, {result: fabricengine.MergeResult{Committed: true}}}

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, haltFragments...)
		if len(fx.merger.calls) != 1 || len(fx.merger.pushCalls) != 0 {
			t.Errorf("parent-side merges = %d, pushes = %d; want the first attempt's one and no push", len(fx.merger.calls), len(fx.merger.pushCalls))
		}
	})

	t.Run("an unreadable record halts naming the release verb", func(t *testing.T) {
		fx := newFixture(t, 1, func(fx *finalizeGateFixture) {
			recordFile := filepath.Join(fx.deps.Lock.Store.Dir, hex.EncodeToString([]byte("main"))+".json")
			if err := os.WriteFile(recordFile, []byte("{nope"), 0o644); err != nil {
				t.Error(err)
			}
		})

		outcome, ptr := fx.call(t)

		requireHalt(t, outcome, ptr, "is unreadable before the landing", `"lym batten lock release main"`)
		if len(fx.merger.calls) != 0 {
			t.Errorf("parent-side merges = %d; want none after the halt", len(fx.merger.calls))
		}
	})

	t.Run("a record that still names the run lands", func(t *testing.T) {
		fx := newFinalizeGateFixture(t)
		fx.fz.guard = newParentGuard(fx.deps)

		outcome, ptr := fx.call(t)

		fx.requireLanded(t, outcome, ptr)
		if lockHeld(fx) {
			t.Error("the lock outlived the landed Finalize")
		}
	})
}
