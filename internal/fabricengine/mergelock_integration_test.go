//go:build integration

// mergelock_integration_test.go pins the lifecycle verbs' guard-under-lock ordering: every merge
// verb must read the merge record and evaluate its preconditions AFTER acquiring the weft write
// lock (MergeContinue/MergeAbort), or re-verify the record under it (MergeIn/Merge), so nothing a
// concurrent lock holder does between a verb's guard reads and its lock acquisition can be acted
// on stale.
// Each test uses the external-lock-hold pattern TestMerge_PreMergeSyncRunsInsideTheWriteLock
// established: hold the lock, launch the verb (it must block), mutate the state its pre-lock self
// would have trusted, release, and assert the verb answers what a strictly sequential run answers.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
)

// setupResolvedConflictedMergeIn builds a pair mid-merge on "feature" with conflicts on both sides
// already resolved and staged — the state where MergeContinue would conclude and MergeAbort would
// reset — and returns the hub and handle.
func setupResolvedConflictedMergeIn(t *testing.T) (*hubforge.Hub, *fabricengine.Fabric) {
	t.Helper()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	setupConflictingDivergence(t, h.PrimeWeft(), "feature-weft", "_lyx/weft-clash.txt")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) != 2 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want both sides conflicted", res.Conflicts)
	}

	commitOnCurrentBranchStage := func(dir, rel string) {
		t.Helper()
		writeFileForTest(t, dir, rel, "resolved\n")
		gitkit.MustRun(t, dir, "git", "add", rel)
	}
	commitOnCurrentBranchStage(h.PrimeWorktree(), "clash.txt")
	commitOnCurrentBranchStage(h.PrimeWeft(), "_lyx/weft-clash.txt")
	return h, f
}

// launchBlockedOnLock acquires the pair's weft write lock, launches verb on a goroutine, and
// asserts the verb is still blocked shortly after — proving its guards did not run to completion
// ahead of the lock. It returns the held lock and the verb's result channel.
func launchBlockedOnLock(t *testing.T, f *fabricengine.Fabric, verb func() error) (*lock.FileLock, <-chan error) {
	t.Helper()

	lockPath := fabricengine.WeftWriteLockPathForTest(t, f)
	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock(%q) error = %v", lockPath, err)
	}

	done := make(chan error, 1)
	go func() { done <- verb() }()

	select {
	case verbErr := <-done:
		_ = held.Release()
		t.Fatalf("verb completed (error = %v) while the weft write lock was externally held; want it to block", verbErr)
	case <-time.After(150 * time.Millisecond):
		// Still blocked, as required.
	}
	return held, done
}

// awaitVerb releases held and waits for the verb launched by launchBlockedOnLock to return.
func awaitVerb(t *testing.T, held *lock.FileLock, done <-chan error) error {
	t.Helper()

	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	select {
	case verbErr := <-done:
		return verbErr
	case <-time.After(30 * time.Second):
		t.Fatal("verb did not complete after the external lock was released")
		return nil
	}
}

// TestMergeAbort_ConcludeLandingWhileWaitingForLock_RefusesInsteadOfResetting pins the destructive
// direction of the guard-under-lock ordering: a MergeAbort that passed its conclude-landed guard
// and then waited for the lock must NOT act on that stale answer once a concurrent lock holder has
// concluded the merge and retired the record — the pre-fix behaviour reset both sides (force: true)
// through the freshly landed conclude commits, destroying the operator's resolutions with nothing
// left to notice (deleteMergeState tolerates absence).
func TestMergeAbort_ConcludeLandingWhileWaitingForLock_RefusesInsteadOfResetting(t *testing.T) {
	h, f := setupResolvedConflictedMergeIn(t)

	fresh := openFreshFabric(t, h.PrimeWorktree())
	held, done := launchBlockedOnLock(t, f, func() error {
		_, abortErr := fresh.MergeAbort()
		return abortErr
	})

	// While MergeAbort waits: the concurrent winner concludes both sides and retires the record —
	// what a raced MergeContinue holding this same lock does.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "--no-edit")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "--no-edit")
	warpConcluded := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftConcluded := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())
	if err := fabricengine.DeleteMergeStateForTest(f); err != nil {
		t.Fatalf("DeleteMergeStateForTest() error = %v", err)
	}

	abortErr := awaitVerb(t, held, done)
	var noMerge *fabricengine.ErrNoMergeInProgress
	if !errors.As(abortErr, &noMerge) {
		t.Fatalf("MergeAbort() after the record was retired mid-wait: error = %v (%T); want *fabricengine.ErrNoMergeInProgress", abortErr, abortErr)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpConcluded {
		t.Errorf("warp HEAD after refused abort = %q; want the landed conclude %q untouched", got, warpConcluded)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftConcluded {
		t.Errorf("weft HEAD after refused abort = %q; want the landed conclude %q untouched", got, weftConcluded)
	}
}

// TestMergeContinue_RecordRetiredWhileWaitingForLock_ReportsNoMergeInProgress pins the mirror
// direction: a MergeContinue that loses the lock race to the concluding winner must answer what a
// strictly sequential second call answers — *ErrNoMergeInProgress — instead of concluding from its
// stale record, which pre-fix adopted (and thereby resurrected) a record the winner had retired
// and answered committed:true.
func TestMergeContinue_RecordRetiredWhileWaitingForLock_ReportsNoMergeInProgress(t *testing.T) {
	h, f := setupResolvedConflictedMergeIn(t)

	fresh := openFreshFabric(t, h.PrimeWorktree())
	var contRes fabricengine.MergeResult
	held, done := launchBlockedOnLock(t, f, func() error {
		res, contErr := fresh.MergeContinue("")
		contRes = res
		return contErr
	})

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "--no-edit")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "--no-edit")
	if err := fabricengine.DeleteMergeStateForTest(f); err != nil {
		t.Fatalf("DeleteMergeStateForTest() error = %v", err)
	}

	contErr := awaitVerb(t, held, done)
	var noMerge *fabricengine.ErrNoMergeInProgress
	if !errors.As(contErr, &noMerge) {
		t.Fatalf("MergeContinue() after the record was retired mid-wait: (committed %v, error %v (%T)); want *fabricengine.ErrNoMergeInProgress", contRes.Committed, contErr, contErr)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() = (%v, %v); want (false, nil) — the retired record must not be resurrected", exists, err)
	}
}

// TestMergeIn_RecordAppearingWhileWaitingForLock_RefusesInsteadOfOverwriting pins MergeIn's
// under-lock record re-check: a record written by another process while MergeIn waited for the
// lock must refuse the call with the aggregated in-progress guard reason — pre-fix, saveMergeState
// silently replaced the live record with this call's own source, so its conclude would have landed
// the other merge's content under this record's name.
func TestMergeIn_RecordAppearingWhileWaitingForLock_RefusesInsteadOfOverwriting(t *testing.T) {
	hub, f, _, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, hub.PrimeWorktree(), "feature", "clash.txt")
	commitOnWeftBranch("feature-weft", "_lyx/clean.txt", "clean\n", "weft: clean branch")

	held, done := launchBlockedOnLock(t, f, func() error {
		_, mergeErr := f.MergeIn("feature")
		return mergeErr
	})

	planted := fabricengine.MergeStateForTest{Verb: "merge-in", Source: "some-other-branch", WarpStart: "0000", WeftStart: "0000"}
	if err := fabricengine.SaveMergeStateForTest(f, planted); err != nil {
		t.Fatalf("SaveMergeStateForTest() error = %v", err)
	}

	mergeErr := awaitVerb(t, held, done)
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(mergeErr, &guardErr) {
		t.Fatalf("MergeIn() over a record that appeared mid-wait: error = %v (%T); want *fabricengine.MergeGuardError", mergeErr, mergeErr)
	}

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want the planted record still present", found, err)
	}
	if st.Source != "some-other-branch" {
		t.Errorf("record Source after refusal = %q; want the planted %q — MergeIn must not overwrite a live record", st.Source, "some-other-branch")
	}
}

// TestMerge_RecordAppearingWhileWaitingForLock_RefusesInsteadOfOverwriting is the Merge-verb twin
// of the MergeIn re-check test — Merge's window is wider still, because its pre-merge sync step
// mutates both checkouts immediately after the acquisition.
func TestMerge_RecordAppearingWhileWaitingForLock_RefusesInsteadOfOverwriting(t *testing.T) {
	_, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch("feature", "clean-warp.txt", "clean\n", "warp: clean branch")
	commitOnWeftBranch("feature-weft", "_lyx/clean-weft.txt", "clean\n", "weft: clean branch")

	held, done := launchBlockedOnLock(t, f, func() error {
		_, mergeErr := f.Merge("feature", fabricengine.MergeOptions{})
		return mergeErr
	})

	planted := fabricengine.MergeStateForTest{Verb: "merge", Source: "some-other-branch", WarpStart: "0000", WeftStart: "0000"}
	if err := fabricengine.SaveMergeStateForTest(f, planted); err != nil {
		t.Fatalf("SaveMergeStateForTest() error = %v", err)
	}

	mergeErr := awaitVerb(t, held, done)
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(mergeErr, &guardErr) {
		t.Fatalf("Merge() over a record that appeared mid-wait: error = %v (%T); want *fabricengine.MergeGuardError", mergeErr, mergeErr)
	}
	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found || st.Source != "some-other-branch" {
		t.Errorf("planted record after refusal = (source %q, found %v, err %v); want it left untouched", st.Source, found, err)
	}
}

// TestMergeIn_StartsAreReReadUnderLock pins that the record's pre-merge SHAs are captured under
// the lock, not before it: a commit landed by the lock's previous holder while MergeIn waited must
// appear as the recorded start, or MergeAbort would reset straight through it.
func TestMergeIn_StartsAreReReadUnderLock(t *testing.T) {
	h, f, _, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	commitOnWeftBranch("feature-weft", "_lyx/clean.txt", "clean\n", "weft: clean branch")

	held, done := launchBlockedOnLock(t, f, func() error {
		res, mergeErr := f.MergeIn("feature")
		if mergeErr == nil && len(res.Conflicts) == 0 {
			t.Error("MergeIn(feature) reported no conflicts; the fixture must conflict so the record survives for inspection")
		}
		return mergeErr
	})

	// The lock's holder lands a commit on the warp current branch — what a concurrent Commit does —
	// before this MergeIn gets its turn.
	commitOnCurrentBranch(t, h.PrimeWorktree(), "landed-mid-wait.txt", "landed while merge-in waited\n", "concurrent commit")
	landedSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	if mergeErr := awaitVerb(t, held, done); mergeErr != nil {
		t.Fatalf("MergeIn(feature) error = %v", mergeErr)
	}

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	if st.WarpStart != landedSHA {
		t.Errorf("recorded WarpStart = %q; want %q — the start must be read under the lock, or MergeAbort resets through the concurrent commit", st.WarpStart, landedSHA)
	}
}

// writeFileForTest writes rel under dir with content, failing the test on error.
func writeFileForTest(t *testing.T, dir, rel, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
