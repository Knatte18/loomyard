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
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
)

// setupResolvedConflictedMergeIn builds a pair mid-merge on "feature" with the warp-side conflict
// already resolved and staged — the state where MergeContinue would conclude and MergeAbort would
// reset — and returns the hub and handle.
func setupResolvedConflictedMergeIn(t *testing.T) (*hubforge.Hub, *fabricengine.Fabric) {
	t.Helper()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want the warp-side conflict", res.Conflicts)
	}

	writeFileForTest(t, h.PrimeWorktree(), "clash.txt", "resolved\n")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "clash.txt")
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

	// While MergeAbort waits: the concurrent winner concludes the warp side and retires the record —
	// what a raced MergeContinue holding this same lock does.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "--no-edit")
	warpConcluded := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
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
// BOTH sides are driven and BOTH recorded starts are asserted, independently. Asserting the warp
// side alone left the weft re-read entirely unguarded — deleting it kept the whole integration
// suite green — and the weft side is not the cheaper half of the pair: a stale WeftStart is what
// makes MergeAbort hard-reset the weft checkout through a commit the previous lock holder (a
// concurrent Fabric.Commit, the most plausible one) had already landed there.
func TestMergeIn_StartsAreReReadUnderLock(t *testing.T) {
	h, f, _, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	commitOnWeftBranch("feature-weft", "_lyx/clean.txt", "clean\n", "weft: clean branch")

	warpBeforeWait := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBeforeWait := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	held, done := launchBlockedOnLock(t, f, func() error {
		res, mergeErr := f.MergeIn("feature")
		if mergeErr == nil && len(res.Conflicts) == 0 {
			t.Error("MergeIn(feature) reported no conflicts; the fixture must conflict so the record survives for inspection")
		}
		return mergeErr
	})

	// The lock's holder lands a commit on each side's current branch — what a concurrent Commit does —
	// before this MergeIn gets its turn.
	commitOnCurrentBranch(t, h.PrimeWorktree(), "landed-mid-wait.txt", "landed while merge-in waited\n", "concurrent commit")
	commitOnCurrentBranch(t, h.PrimeWeft(), "_lyx/landed-mid-wait.txt", "landed while merge-in waited\n", "concurrent weft commit")
	landedWarpSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	landedWeftSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	// Precondition, asserted rather than assumed: each side really did move while MergeIn waited, or
	// the "re-read under the lock" and "read before the lock" answers would be indistinguishable and
	// the assertions below would pass without proving anything.
	if landedWarpSHA == warpBeforeWait {
		t.Fatalf("warp HEAD did not move during the wait (still %q); the race this test needs is not present", warpBeforeWait)
	}
	if landedWeftSHA == weftBeforeWait {
		t.Fatalf("weft HEAD did not move during the wait (still %q); the race this test needs is not present", weftBeforeWait)
	}

	if mergeErr := awaitVerb(t, held, done); mergeErr != nil {
		t.Fatalf("MergeIn(feature) error = %v", mergeErr)
	}

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	if st.WarpStart != landedWarpSHA {
		t.Errorf("recorded WarpStart = %q; want %q — the start must be read under the lock, or MergeAbort resets through the concurrent commit", st.WarpStart, landedWarpSHA)
	}
	if st.WeftStart != landedWeftSHA {
		t.Errorf("recorded WeftStart = %q; want %q — the start must be read under the lock, or MergeAbort resets through the concurrent commit", st.WeftStart, landedWeftSHA)
	}
}

// newCleanMergeablePair builds a pair with clean, mergeable "feature"/"feature-weft" branches —
// the fixture whose MergeIn/Merge would succeed if nothing raced it — and returns the hub and
// handle.
func newCleanMergeablePair(t *testing.T) (*hubforge.Hub, *fabricengine.Fabric) {
	t.Helper()

	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch("feature", "clean-warp.txt", "clean\n", "warp: clean branch")
	commitOnWeftBranch("feature-weft", "_lyx/clean-weft.txt", "clean\n", "weft: clean branch")
	return h, f
}

// assertForeignRefusalUnderLock releases held, awaits the verb, and asserts the foreign-state
// refusal shape: *ErrForeignMergeState, no fabric record written, and the operator's own merge
// state byte-identically untouched.
func assertForeignRefusalUnderLock(t *testing.T, f *fabricengine.Fabric, warpDir, statusBefore string, held *lock.FileLock, done <-chan error) {
	t.Helper()

	mergeErr := awaitVerb(t, held, done)
	var foreignErr *fabricengine.ErrForeignMergeState
	if !errors.As(mergeErr, &foreignErr) {
		t.Fatalf("verb over foreign state that appeared mid-wait: error = %v (%T); want *fabricengine.ErrForeignMergeState", mergeErr, mergeErr)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (false, nil)", exists, err)
	}
	if got := gitkit.GitStatusPorcelain(t, warpDir); got != statusBefore {
		t.Errorf("git status changed: before = %q, after = %q; want the operator's own merge state untouched", statusBefore, got)
	}
}

// TestMergeIn_ForeignStateAppearingWhileWaitingForLock_Refuses pins the foreign-state half of
// recheckMergePreconditionsUnderLock on MergeIn: a human's plain-git merge that conflicts in the
// warp checkout while MergeIn waits for the lock must refuse as foreign, exactly as a strictly
// sequential MergeIn over that state does. Pre-fix, only the record was re-checked, so the failing
// MergeStart read the operator's conflicted entries as THIS merge's own conflicts — fabric wrote a
// record over, and reported conflicts from, a merge it never started.
func TestMergeIn_ForeignStateAppearingWhileWaitingForLock_Refuses(t *testing.T) {
	h, f := newCleanMergeablePair(t)

	held, done := launchBlockedOnLock(t, f, func() error {
		_, mergeErr := f.MergeIn("feature")
		return mergeErr
	})

	// While MergeIn waits: the operator's own plain-git merge conflicts in the warp checkout.
	setupConflictingDivergence(t, h.PrimeWorktree(), "operator-branch", "operator-clash.txt")
	gitMergeAllowConflict(t, h.PrimeWorktree(), "operator-branch")
	statusBefore := gitkit.GitStatusPorcelain(t, h.PrimeWorktree())
	if !strings.Contains(statusBefore, "operator-clash.txt") {
		t.Fatalf("git status = %q; the mid-wait fixture must have produced a real conflict on operator-clash.txt", statusBefore)
	}

	assertForeignRefusalUnderLock(t, f, h.PrimeWorktree(), statusBefore, held, done)
}

// TestMerge_ForeignStateAppearingWhileWaitingForLock_Refuses is the Merge-verb twin: Merge's
// pre-merge sync step and MergeStart calls run immediately after the acquisition, so a stale
// foreign answer there ends in Merge's conflict path force-resetting the operator's merge away.
func TestMerge_ForeignStateAppearingWhileWaitingForLock_Refuses(t *testing.T) {
	h, f := newCleanMergeablePair(t)

	held, done := launchBlockedOnLock(t, f, func() error {
		_, mergeErr := f.Merge("feature", fabricengine.MergeOptions{})
		return mergeErr
	})

	setupConflictingDivergence(t, h.PrimeWorktree(), "operator-branch", "operator-clash.txt")
	gitMergeAllowConflict(t, h.PrimeWorktree(), "operator-branch")
	statusBefore := gitkit.GitStatusPorcelain(t, h.PrimeWorktree())
	if !strings.Contains(statusBefore, "operator-clash.txt") {
		t.Fatalf("git status = %q; the mid-wait fixture must have produced a real conflict on operator-clash.txt", statusBefore)
	}

	assertForeignRefusalUnderLock(t, f, h.PrimeWorktree(), statusBefore, held, done)
}

// TestMergeIn_PairTurningDirtyWhileWaitingForLock_RefusesPreservingDirt pins the dirty half of
// recheckMergePreconditionsUnderLock on MergeIn, on the destructive shape: the mid-wait tracked
// edit lands on a file the merge source also touches, so pre-fix the MergeStart refusal ("local
// changes would be overwritten") classified as a genuine error and selfAbortMergeAttempt reset the
// operator's edit away under force: true. The re-check refuses with the same aggregated reason a
// strictly sequential MergeIn over a dirty pair reports, and the edit survives.
func TestMergeIn_PairTurningDirtyWhileWaitingForLock_RefusesPreservingDirt(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	commitOnCurrentBranch(t, h.PrimeWorktree(), "overlap.txt", "seed content\n", "seed overlap.txt")
	commitOnWarpBranch("feature", "overlap.txt", "feature content\n", "feature: modify overlap.txt")
	commitOnWeftBranch("feature-weft", "_lyx/clean-weft.txt", "clean\n", "weft: clean branch")

	held, done := launchBlockedOnLock(t, f, func() error {
		_, mergeErr := f.MergeIn("feature")
		return mergeErr
	})

	// While MergeIn waits: the operator edits the very file the merge source modifies, uncommitted.
	const dirtyContent = "operator's uncommitted edit\n"
	writeFileForTest(t, h.PrimeWorktree(), "overlap.txt", dirtyContent)

	mergeErr := awaitVerb(t, held, done)
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(mergeErr, &guardErr) {
		t.Fatalf("MergeIn() over a pair that turned dirty mid-wait: error = %v (%T); want *fabricengine.MergeGuardError", mergeErr, mergeErr)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "worktree dirty" {
		t.Errorf("guard reasons = %v; want exactly [\"worktree dirty\"]", guardErr.Reasons)
	}
	got, err := os.ReadFile(filepath.Join(h.PrimeWorktree(), "overlap.txt"))
	if err != nil || string(got) != dirtyContent {
		t.Errorf("overlap.txt after the refusal = (%q, %v); want the operator's edit preserved", got, err)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMerge_PairTurningDirtyWhileWaitingForLock_RefusesPreservingDirt is the Merge-verb twin of the
// dirty re-check test, additionally asserting the refusal fired before the pre-merge sync step
// recorded anything — the re-check sits between the acquisition and the sync.
func TestMerge_PairTurningDirtyWhileWaitingForLock_RefusesPreservingDirt(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	commitOnCurrentBranch(t, h.PrimeWorktree(), "overlap.txt", "seed content\n", "seed overlap.txt")
	commitOnWarpBranch("feature", "overlap.txt", "feature content\n", "feature: modify overlap.txt")
	commitOnWeftBranch("feature-weft", "_lyx/clean-weft.txt", "clean\n", "weft: clean branch")

	var mergeRes fabricengine.MergeResult
	held, done := launchBlockedOnLock(t, f, func() error {
		res, mergeErr := f.Merge("feature", fabricengine.MergeOptions{})
		mergeRes = res
		return mergeErr
	})

	const dirtyContent = "operator's uncommitted edit\n"
	writeFileForTest(t, h.PrimeWorktree(), "overlap.txt", dirtyContent)

	mergeErr := awaitVerb(t, held, done)
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(mergeErr, &guardErr) {
		t.Fatalf("Merge() over a pair that turned dirty mid-wait: error = %v (%T); want *fabricengine.MergeGuardError", mergeErr, mergeErr)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "worktree dirty" {
		t.Errorf("guard reasons = %v; want exactly [\"worktree dirty\"]", guardErr.Reasons)
	}
	if entries := mergeRes.Mutated().Entries(); len(entries) != 0 {
		t.Errorf("Merge().Mutated() = %v; want empty — the re-check must refuse before the pre-merge sync step records anything", entries)
	}
	got, err := os.ReadFile(filepath.Join(h.PrimeWorktree(), "overlap.txt"))
	if err != nil || string(got) != dirtyContent {
		t.Errorf("overlap.txt after the refusal = (%q, %v); want the operator's edit preserved", got, err)
	}
}

// writeFileForTest writes rel under dir with content, failing the test on error.
func writeFileForTest(t *testing.T, dir, rel, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
