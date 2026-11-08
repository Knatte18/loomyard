// mergelock_integration_test.go pins the guard-under-lock ordering of the lifecycle verbs.
// MergeContinue and MergeAbort read the merge record only after they acquire the weft write lock.
// MergeIn and Merge re-verify the record under that lock.
// Each test holds the lock itself and launches the verb, which must block.
// It then mutates the state the pre-lock guards would have trusted and releases the lock.
// The verb must then answer what a strictly sequential run answers.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// setupResolvedConflictedMergeIn builds a pair mid-merge on "feature" with the warp-side conflict resolved and staged.
// The merge is then ready to be concluded or aborted.
// It returns the hub and the fabric handle.
func setupResolvedConflictedMergeIn(t *testing.T) (*hubforge.Hub, *fabricengine.Fabric) {
	t.Helper()

	h, f, _, _, _, _ := newMergeLayerFixture(t, &clashLayer)

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

// launchBlockedOnLock holds the pair's weft write lock while verb runs on a goroutine.
// It waits until verb signals that it is about to wait for that lock.
// It then asserts verb has not finished, which shows its guards ran before the lock and that it is parked on it.
// It returns the held lock and the channel that carries verb's result.
func launchBlockedOnLock(t *testing.T, f *fabricengine.Fabric, verb func() error) (*lock.FileLock, <-chan error) {
	t.Helper()

	lockPath := fabricengine.WeftWriteLockPathForTest(t, f)
	waiting := fabricengine.AwaitWeftLockWaitForTest(t, lockPath)
	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock(%q) error = %v", lockPath, err)
	}

	done := make(chan error, 1)
	go func() { done <- verb() }()

	// The deadline only bounds a failing test.
	select {
	case <-waiting:
	case verbErr := <-done:
		_ = held.Release()
		t.Fatalf("verb completed (error = %v) before waiting for the externally held weft write lock; want it to block", verbErr)
	case <-time.After(30 * time.Second):
		_ = held.Release()
		t.Fatal("verb never reached the weft write lock")
	}
	select {
	case verbErr := <-done:
		_ = held.Release()
		t.Fatalf("verb completed (error = %v) while the weft write lock was externally held; want it to block", verbErr)
	default:
	}
	return held, done
}

// awaitVerb releases held, then waits for the verb's result on done and returns it.
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

// TestMergeAbort_ConcludeLandingWhileWaitingForLock_RefusesInsteadOfResetting covers a waiting MergeAbort.
// Its conclude-landed guard ran before the wait, so once it holds the lock it must refuse rather than reset.
// A concurrent lock holder may conclude the merge and retire the record while MergeAbort waits.
// A reset would run through the freshly landed conclude commits and destroy the operator's resolutions.
// Nothing else would catch it, because deleteMergeState tolerates an absent record.
//
//testtiming:keep a conclude landed while MergeAbort waited for the lock surviving, with no reset through the operator's resolutions; coverage of its blocks by other tests does not show an assertion of this
func TestMergeAbort_ConcludeLandingWhileWaitingForLock_RefusesInsteadOfResetting(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f := setupResolvedConflictedMergeIn(t)

	fresh := openFreshFabric(t, h.PrimeWorktree())
	held, done := launchBlockedOnLock(t, f, func() error {
		_, abortErr := fresh.MergeAbort()
		return abortErr
	})

	// While MergeAbort waits, the lock holder concludes the warp side and retires the record:
	// the conclude a MergeContinue holding this lock performs.
	gitkit.Git(t, h.PrimeWorktree(), "commit", "--no-edit")
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

// TestMergeIn_RecordAppearingWhileWaitingForLock_RefusesInsteadOfOverwriting pins MergeIn's record re-check.
// A record another process wrote while MergeIn waited for the lock must refuse the call.
// The refusal carries the aggregated in-progress guard reason.
// Without this check, saveMergeState would overwrite the live record with this call's own source,
// and its conclude would land the other merge's content under this record's name.
func TestMergeIn_RecordAppearingWhileWaitingForLock_RefusesInsteadOfOverwriting(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub, f, _, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, hub.PrimeWorktree(), "feature", "clash.txt")
	commitOnWeftBranch("feature-weft", "_lym/clean.txt", "clean\n", "weft: clean branch")

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

// TestMergeIn_StartsAreReReadUnderLock pins the record's pre-merge SHAs, read under the lock.
// A commit the lock's previous holder landed while MergeIn waited must be the recorded start.
// Otherwise MergeAbort would reset straight through it.
// The test drives both sides and asserts both recorded starts independently.
// A stale WeftStart makes MergeAbort hard-reset the weft checkout, undoing a concurrent Fabric.Commit.
//
//testtiming:keep the recorded start SHAs on both sides naming a commit landed while MergeIn waited, so an abort cannot reset through it; coverage of its blocks by other tests does not show an assertion of this
func TestMergeIn_StartsAreReReadUnderLock(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	commitOnWeftBranch("feature-weft", "_lym/clean.txt", "clean\n", "weft: clean branch")

	warpBeforeWait := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBeforeWait := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

	held, done := launchBlockedOnLock(t, f, func() error {
		res, mergeErr := f.MergeIn("feature")
		if mergeErr == nil && len(res.Conflicts) == 0 {
			t.Error("MergeIn(feature) reported no conflicts; the fixture must conflict so the record survives for inspection")
		}
		return mergeErr
	})

	// The lock's holder lands a commit on each side's current branch before this MergeIn gets its turn.
	// That is what a concurrent Commit does.
	gitkit.CommitFile(t, h.PrimeWorktree(), "landed-mid-wait.txt", "landed while merge-in waited\n", "concurrent commit")
	gitkit.CommitFile(t, h.PrimeRecords(), "_lym/landed-mid-wait.txt", "landed while merge-in waited\n", "concurrent weft commit")
	landedWarpSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	landedWeftSHA := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

	// Precondition, asserted rather than assumed: each side must move while MergeIn waits.
	// Otherwise the re-read under the lock and the read before it give the same answer,
	// and the assertions below pass without proving anything.
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

// newCleanMergeablePair returns a hub and handle whose "feature" and "feature-weft" branches are clean and mergeable.
// With nothing racing it, MergeIn and Merge over this pair succeed.
func newCleanMergeablePair(t *testing.T) (*hubforge.Hub, *fabricengine.Fabric) {
	t.Helper()

	h, f, _, _, _, _ := newMergeLayerFixture(t, &cleanMergeableLayer)
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

// TestMerge_ForeignStateAppearingWhileWaitingForLock_Refuses pins that Merge refuses foreign merge state.
// Its pre-merge sync step and MergeStart run right after the acquisition.
// A stale foreign answer there would reach Merge's conflict path, which force-resets the operator's merge away.
func TestMerge_ForeignStateAppearingWhileWaitingForLock_Refuses(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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

// assertGuardDirt asserts guardErr carries wantCheckout and exactly wantPaths as its dirt detail.
// The checkout is compared after EvalSymlinks on both sides, since a temp root may be a symlink.
func assertGuardDirt(t *testing.T, guardErr *fabricengine.MergeGuardError, wantCheckout string, wantPaths []string) {
	t.Helper()

	gotCheckout, err := filepath.EvalSymlinks(guardErr.Checkout)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", guardErr.Checkout, err)
	}
	want, err := filepath.EvalSymlinks(wantCheckout)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", wantCheckout, err)
	}
	if gotCheckout != want {
		t.Errorf("guard error Checkout = %q; want %q", gotCheckout, want)
	}
	if !slices.Equal(guardErr.DirtyPaths, wantPaths) {
		t.Errorf("guard error DirtyPaths = %q; want %q", guardErr.DirtyPaths, wantPaths)
	}
}

// TestMergeIn_PairTurningDirtyWhileWaitingForLock_RefusesPreservingDirt pins the dirty half of
// recheckMergePreconditionsUnderLock on MergeIn, on the overlapping shape: the mid-wait tracked
// edit lands on a file the merge source also touches.
// A MergeStart refusal on that overlap must not count as a genuine error.
// Otherwise selfAbortMergeAttempt resets the operator's edit away under force: true.
// The re-check refuses with the same aggregated reason as a strictly sequential MergeIn over a dirty pair.
// The edit survives.
//
//testtiming:keep the operator's uncommitted edit surviving a refused MergeIn instead of being reset away; coverage of its blocks by other tests does not show an assertion of this
func TestMergeIn_PairTurningDirtyWhileWaitingForLock_RefusesPreservingDirt(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	gitkit.CommitFile(t, h.PrimeWorktree(), "overlap.txt", "seed content\n", "seed overlap.txt")
	commitOnWarpBranch("feature", "overlap.txt", "feature content\n", "feature: modify overlap.txt")
	commitOnWeftBranch("feature-weft", "_lym/clean-weft.txt", "clean\n", "weft: clean branch")

	held, done := launchBlockedOnLock(t, f, func() error {
		_, mergeErr := f.MergeIn("feature")
		return mergeErr
	})

	// While MergeIn waits, the operator edits the file the merge source modifies, uncommitted.
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
	assertGuardDirt(t, guardErr, h.PrimeWorktree(), []string{"overlap.txt"})
	got, err := os.ReadFile(filepath.Join(h.PrimeWorktree(), "overlap.txt"))
	if err != nil || string(got) != dirtyContent {
		t.Errorf("overlap.txt after the refusal = (%q, %v); want the operator's edit preserved", got, err)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMerge_PairTurningDirtyWhileWaitingForLock_RefusesPreservingDirt pins the dirty re-check on Merge.
// It also asserts that the refusal fires before the pre-merge sync step records anything.
// The re-check sits between the acquisition and the sync.
func TestMerge_PairTurningDirtyWhileWaitingForLock_RefusesPreservingDirt(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	gitkit.CommitFile(t, h.PrimeWorktree(), "overlap.txt", "seed content\n", "seed overlap.txt")
	commitOnWarpBranch("feature", "overlap.txt", "feature content\n", "feature: modify overlap.txt")
	commitOnWeftBranch("feature-weft", "_lym/clean-weft.txt", "clean\n", "weft: clean branch")

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
	assertGuardDirt(t, guardErr, h.PrimeWorktree(), []string{"overlap.txt"})
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
