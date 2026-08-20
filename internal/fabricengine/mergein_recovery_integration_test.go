//go:build integration

// mergein_recovery_integration_test.go covers MergeIn's crash-recovery, freshness, foreign-state,
// and illusion-integrity matrix: a fresh Fabric handle recovering a mid-merge record (both the
// conflicted and the crashed-after-clean-staging shapes), a conclude-phase partial failure and its
// idempotent retry, foreign git merge state's refusal-without-touching disposition, the freshness
// rule's three source-resolution outcomes, the fabric-managed guard, the dirty-pair guard's
// byte-identical error values, an unmappable weft-root conflict's self-abort, conflict-marker
// content that never leaks a "-weft" name, and path mapping on a subpath-anchored hub.
// Reuses newMergePairFixture and its sibling helpers from mergein_integration_test.go, since both
// files share package fabricengine_test.

package fabricengine_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// openFreshFabric opens a brand-new *fabricengine.Fabric handle over warpPath, independent of any
// handle a fixture already returned — the crash-recovery scenarios' "a different process resumes
// the merge" shape.
func openFreshFabric(t *testing.T, warpPath string) *fabricengine.Fabric {
	t.Helper()

	l, err := lyxcwd.ResolveWorktree(warpPath)
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", warpPath, err)
	}
	f, err := fabricengine.Open(l)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}
	return f
}

// installFailingPreCommitHook installs a pre-commit hook in repoDir's .git/hooks that always exits
// 1, so any commit attempted there fails.
func installFailingPreCommitHook(t *testing.T, repoDir string) {
	t.Helper()

	hookPath := filepath.Join(repoDir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write pre-commit hook at %s: %v", hookPath, err)
	}
}

// removePreCommitHook removes a hook installFailingPreCommitHook installed, tolerating its absence.
func removePreCommitHook(t *testing.T, repoDir string) {
	t.Helper()

	hookPath := filepath.Join(repoDir, ".git", "hooks", "pre-commit")
	if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove pre-commit hook at %s: %v", hookPath, err)
	}
}

// gitMergeAllowConflict runs `git merge ref` in dir directly (bypassing fabric entirely), tolerating
// a non-zero exit — the plain-git conflict this file's foreign-merge-state scenarios manufacture.
func gitMergeAllowConflict(t *testing.T, dir, ref string) {
	t.Helper()

	cmd := exec.Command("git", "merge", ref)
	cmd.Dir = dir
	_ = cmd.Run()
}

// pushDetachedContentAsRemoteBranch commits filename/content on a detached HEAD in dir (never
// naming a local branch) and pushes it to origin under branch — manufacturing a branch that exists
// only on the remote, with no local ref of that name at all — then returns dir to whatever branch
// was checked out before.
func pushDetachedContentAsRemoteBranch(t *testing.T, dir, branch, filename, content, msg string) {
	t.Helper()

	original := currentBranchName(t, dir)
	gitkit.MustRun(t, dir, "git", "checkout", "-q", "--detach", "HEAD")
	commitOnCurrentBranch(t, dir, filename, content, msg)
	gitkit.MustRun(t, dir, "git", "push", "origin", "HEAD:refs/heads/"+branch)
	gitkit.MustRun(t, dir, "git", "checkout", "-q", original)
}

// TestMergeAbort_FreshHandle_RecoversConflictedRecord covers the crash-recovery case: a real
// conflicted MergeIn leaves its record on disk, and a brand-new Fabric handle over the same pair
// drives MergeAbort to an exact two-sided restore.
func TestMergeAbort_FreshHandle_RecoversConflictedRecord(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	warpStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	if _, err := f.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	if _, err := fresh.MergeAbort(); err != nil {
		t.Fatalf("fresh handle MergeAbort() error = %v", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStartSHA {
		t.Errorf("warp HEAD after fresh-handle MergeAbort = %q; want %q", got, warpStartSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftStartSHA {
		t.Errorf("weft HEAD after fresh-handle MergeAbort = %q; want %q", got, weftStartSHA)
	}
}

// TestMergeContinue_FreshHandle_RecoversCrashedAfterCleanStaging covers the crash-after-clean-
// staging shape: both sides staged (via the gitrepo.Repo.MergeStart seam, never a conflict) with a
// hand-saved record and nothing concluded, then a fresh Fabric handle's MergeContinue concludes
// both.
func TestMergeContinue_FreshHandle_RecoversCrashedAfterCleanStaging(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "warp-branch.txt", "warp-current.txt")
	setupCleanNonFastForward(t, h.PrimeWeft(), "feature-weft", "weft-branch.txt", "weft-current.txt")

	warpRepo := fabricengine.WarpForTest(f)
	weftRepo := fabricengine.WeftForTest(f)

	warpSourceSHA, err := warpRepo.ResolveSHA("feature")
	if err != nil {
		t.Fatalf("ResolveSHA(feature) error = %v", err)
	}
	weftSourceSHA, err := weftRepo.ResolveSHA("feature-weft")
	if err != nil {
		t.Fatalf("ResolveSHA(feature-weft) error = %v", err)
	}

	if _, err := warpRepo.MergeStart(warpSourceSHA, false); err != nil {
		t.Fatalf("warp MergeStart(%s) error = %v", warpSourceSHA, err)
	}
	if _, err := weftRepo.MergeStart(weftSourceSHA, false); err != nil {
		t.Fatalf("weft MergeStart(%s) error = %v", weftSourceSHA, err)
	}

	if err := fabricengine.SaveMergeStateForTest(f, fabricengine.MergeStateForTest{
		Verb:        "merge-in",
		Source:      "feature",
		WarpStart:   warpStart,
		WeftStart:   weftStart,
		WarpOutcome: "staged",
		WeftOutcome: "staged",
		StartedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("SaveMergeStateForTest() error = %v", err)
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	res, err := fresh.MergeContinue("")
	if err != nil {
		t.Fatalf("fresh handle MergeContinue() error = %v", err)
	}
	if !res.Committed {
		t.Errorf("fresh handle MergeContinue().Committed = false; want true")
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after recovery = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMergeContinue_ConcludePartialFailure_RetryConcludesRemainingSideOnly covers a conclude-phase
// partial failure: the weft side's conclude-commit is sabotaged to fail, so MergeIn returns
// *ErrMergeIncomplete with the record retained and warp's landed SHA recorded; removing the
// sabotage and re-running MergeContinue then concludes only the remaining (weft) side, pinned by SHA
// comparison on the already-landed warp side.
func TestMergeContinue_ConcludePartialFailure_RetryConcludesRemainingSideOnly(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	// setupCleanNonFastForward forks the branch off the pre-divergence HEAD, then advances current
	// separately — a genuine (non-fast-forward) merge target on each side, so both sides land a real
	// conclude commit rather than a fast-forward (which concludeMergeSides skips outright).
	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "warp-branch.txt", "warp-current.txt")
	setupCleanNonFastForward(t, h.PrimeWeft(), "feature-weft", "weft-branch.txt", "weft-current.txt")

	installFailingPreCommitHook(t, h.PrimeWeft())

	_, err := f.MergeIn("feature")
	var incompleteErr *fabricengine.ErrMergeIncomplete
	if !errors.As(err, &incompleteErr) {
		t.Fatalf("MergeIn(feature) with weft conclude sabotaged: error = %v (%T); want *fabricengine.ErrMergeIncomplete", err, err)
	}

	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || !exists {
		t.Errorf("MergeRecordExistsForTest() after partial failure = (%v, %v); want (true, nil)", exists, err)
	}
	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	if st.WarpCommitted == "" {
		t.Errorf("merge state WarpCommitted = \"\"; want the landed warp conclude SHA")
	}
	warpCommittedBeforeRetry := st.WarpCommitted
	warpHEADBeforeRetry := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	removePreCommitHook(t, h.PrimeWeft())

	res, err := f.MergeContinue("")
	if err != nil {
		t.Fatalf("MergeContinue() after removing sabotage: error = %v", err)
	}
	if !res.Committed {
		t.Errorf("MergeContinue().Committed = false; want true")
	}

	warpHEADAfterRetry := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	if warpHEADAfterRetry != warpHEADBeforeRetry {
		t.Errorf("warp HEAD changed on retry: before = %q, after = %q; want unchanged (idempotent, already landed)", warpHEADBeforeRetry, warpHEADAfterRetry)
	}
	if warpHEADAfterRetry != warpCommittedBeforeRetry {
		t.Errorf("warp HEAD after retry = %q; want the SHA already recorded as committed %q", warpHEADAfterRetry, warpCommittedBeforeRetry)
	}
}

// TestMergeVerbs_ForeignMergeState_RefuseWithoutTouching covers foreign git merge state: a plain-git
// conflicted merge staged directly in the warp checkout leaves MergeInProgress() false with no
// error, while MergeIn, MergeContinue, and MergeAbort all refuse with *ErrForeignMergeState and
// leave the foreign state untouched (same MERGE_HEAD, same conflicted files).
func TestMergeVerbs_ForeignMergeState_RefuseWithoutTouching(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "other", "plain-conflict.txt")
	gitMergeAllowConflict(t, h.PrimeWorktree(), "other")

	mergeHeadBefore, err := os.ReadFile(filepath.Join(h.PrimeWorktree(), ".git", "MERGE_HEAD"))
	if err != nil {
		t.Fatalf("read MERGE_HEAD (test setup must have produced a real conflict): %v", err)
	}
	conflictedBefore := gitkit.GitStatusPorcelain(t, h.PrimeWorktree())

	if inProgress, err := f.MergeInProgress(); err != nil || inProgress {
		t.Errorf("MergeInProgress() with foreign state = (%v, %v); want (false, nil)", inProgress, err)
	}

	assertForeign := func(name string, err error) {
		t.Helper()
		var foreignErr *fabricengine.ErrForeignMergeState
		if !errors.As(err, &foreignErr) {
			t.Errorf("%s error = %v (%T); want *fabricengine.ErrForeignMergeState", name, err, err)
		}
	}

	_, err = f.MergeIn("feature")
	assertForeign("MergeIn", err)
	_, err = f.MergeContinue("")
	assertForeign("MergeContinue", err)
	_, err = f.MergeAbort()
	assertForeign("MergeAbort", err)

	mergeHeadAfter, err := os.ReadFile(filepath.Join(h.PrimeWorktree(), ".git", "MERGE_HEAD"))
	if err != nil {
		t.Fatalf("read MERGE_HEAD after refusals: %v", err)
	}
	if string(mergeHeadAfter) != string(mergeHeadBefore) {
		t.Errorf("MERGE_HEAD changed: before = %q, after = %q; want untouched", mergeHeadBefore, mergeHeadAfter)
	}
	if got := gitkit.GitStatusPorcelain(t, h.PrimeWorktree()); got != conflictedBefore {
		t.Errorf("git status changed: before = %q, after = %q; want untouched", conflictedBefore, got)
	}
}

// TestMergeVerbs_NoRecordNoForeignState_ReturnNoMergeInProgress covers the remaining disposition:
// with neither a fabric record nor foreign git merge state present, MergeContinue and MergeAbort
// both return *ErrNoMergeInProgress — pinning that the two errors are not interchangeable.
func TestMergeVerbs_NoRecordNoForeignState_ReturnNoMergeInProgress(t *testing.T) {
	_, f, _, _, _, _ := newMergePairFixture(t, ".")

	var noneErr *fabricengine.ErrNoMergeInProgress

	_, err := f.MergeContinue("")
	if !errors.As(err, &noneErr) {
		t.Errorf("MergeContinue() with nothing in progress: error = %v (%T); want *fabricengine.ErrNoMergeInProgress", err, err)
	}
	_, err = f.MergeAbort()
	if !errors.As(err, &noneErr) {
		t.Errorf("MergeAbort() with nothing in progress: error = %v (%T); want *fabricengine.ErrNoMergeInProgress", err, err)
	}
}

// TestMergeIn_Freshness_LocalBehindRemote covers the freshness rule's first branch: a local source
// branch behind its remote-tracking ref merges the remote-tracking SHA, not the stale local one.
func TestMergeIn_Freshness_LocalBehindRemote(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "feature")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "push", "origin", "feature")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "branch", "feature-weft")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "push", "origin", "feature-weft")

	// Advance origin's copy of "feature" past the local branch, via a separate clone so the local
	// "feature" ref itself never moves.
	remoteClone := t.TempDir()
	gitkit.MustRun(t, t.TempDir(), "git", "clone", h.WarpBare, remoteClone)
	gitkit.MustRun(t, remoteClone, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, remoteClone, "git", "config", "user.name", "Test")
	gitkit.MustRun(t, remoteClone, "git", "checkout", "feature")
	commitOnCurrentBranch(t, remoteClone, "remote-tip.txt", "remote tip content\n", "advance origin/feature")
	gitkit.MustRun(t, remoteClone, "git", "push", "origin", "feature")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	// Both sides fast-forward onto the remote-tracking tip, so no conclude-commit is fabricated and
	// Committed is false; the merge landing is asserted by the merged content below, not by the flag.
	if res.Committed {
		t.Fatalf("MergeIn(feature).Committed = true; want false — a fast-forward fabricates no commit")
	}

	if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), "remote-tip.txt")); err != nil {
		t.Errorf("remote-tip.txt not present in warp worktree after merge: %v; want the remote-tracking tip's content merged", err)
	}
}

// TestMergeIn_Freshness_SourceOnlyRemote covers the freshness rule's second branch: a source branch
// existing only on the remote (no local ref at all) merges cleanly once fetched.
func TestMergeIn_Freshness_SourceOnlyRemote(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	pushDetachedContentAsRemoteBranch(t, h.PrimeWorktree(), "feature", "remote-only.txt", "remote only content\n", "warp: remote-only feature")
	pushDetachedContentAsRemoteBranch(t, h.PrimeWeft(), "feature-weft", "remote-only-weft.txt", "remote only weft content\n", "weft: remote-only feature")

	if branchExistsLocally(t, h.PrimeWorktree(), "feature") {
		t.Fatal("test setup produced a local \"feature\" ref; want remote-only")
	}

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	// Both sides fast-forward onto the remote-only tip, so no conclude-commit is fabricated.
	if res.Committed {
		t.Fatalf("MergeIn(feature).Committed = true; want false — a fast-forward fabricates no commit")
	}
	if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), "remote-only.txt")); err != nil {
		t.Errorf("remote-only.txt not present in warp worktree after merge: %v", err)
	}
}

// TestMergeIn_Freshness_SourceResolvableNowhere covers the freshness rule's refusal branch: a
// source resolvable on neither local nor remote refuses with the fixed "source branch not found"
// reason — isolated by giving the weft counterpart a legitimate local branch, so the only guard
// reason that can fire is the warp-side one.
func TestMergeIn_Freshness_SourceResolvableNowhere(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	gitkit.MustRun(t, h.PrimeWeft(), "git", "branch", "feature-weft")

	_, err := f.MergeIn("feature")
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("MergeIn(feature) error = %v (%T); want *fabricengine.MergeGuardError", err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "source branch not found" {
		t.Errorf("MergeIn(feature) guard reasons = %v; want exactly [\"source branch not found\"]", guardErr.Reasons)
	}
}

// TestMergeIn_NotFabricManaged_NothingMutated covers the fabric-managed guard: a source branch with
// no "-weft" counterpart, locally or remotely, refuses with the fixed "source branch is not
// fabric-managed" reason, mutating neither side's HEAD.
func TestMergeIn_NotFabricManaged_NothingMutated(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "feature")

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	_, err := f.MergeIn("feature")
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("MergeIn(feature) error = %v (%T); want *fabricengine.MergeGuardError", err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "source branch is not fabric-managed" {
		t.Errorf("MergeIn(feature) guard reasons = %v; want exactly [\"source branch is not fabric-managed\"]", guardErr.Reasons)
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpBefore {
		t.Errorf("warp HEAD changed to %q; want unchanged %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftBefore {
		t.Errorf("weft HEAD changed to %q; want unchanged %q", got, weftBefore)
	}
}

// TestMergeIn_DirtyPair_ByteIdenticalErrorEitherSide covers the dirty-pair guard: a dirty warp-only
// pair and a dirty weft-only pair both refuse with the fixed "worktree dirty" reason, and the two
// error values are byte-identical (never revealing which side was dirty).
func TestMergeIn_DirtyPair_ByteIdenticalErrorEitherSide(t *testing.T) {
	hWarpDirty, fWarpDirty, _, _, _, _ := newMergePairFixture(t, ".")
	gitkit.MustRun(t, hWarpDirty.PrimeWorktree(), "git", "branch", "feature")
	gitkit.MustRun(t, hWarpDirty.PrimeWeft(), "git", "branch", "feature-weft")
	if err := os.WriteFile(filepath.Join(hWarpDirty.PrimeWorktree(), "dirty.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}
	gitkit.MustRun(t, hWarpDirty.PrimeWorktree(), "git", "add", "dirty.txt")
	_, errWarpDirty := fWarpDirty.MergeIn("feature")

	hWeftDirty, fWeftDirty, _, _, _, _ := newMergePairFixture(t, ".")
	gitkit.MustRun(t, hWeftDirty.PrimeWorktree(), "git", "branch", "feature")
	gitkit.MustRun(t, hWeftDirty.PrimeWeft(), "git", "branch", "feature-weft")
	if err := os.WriteFile(filepath.Join(hWeftDirty.PrimeWeft(), "dirty.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}
	gitkit.MustRun(t, hWeftDirty.PrimeWeft(), "git", "add", "dirty.txt")
	_, errWeftDirty := fWeftDirty.MergeIn("feature")

	var guardErrWarp, guardErrWeft *fabricengine.MergeGuardError
	if !errors.As(errWarpDirty, &guardErrWarp) {
		t.Fatalf("warp-dirty MergeIn() error = %v (%T); want *fabricengine.MergeGuardError", errWarpDirty, errWarpDirty)
	}
	if !errors.As(errWeftDirty, &guardErrWeft) {
		t.Fatalf("weft-dirty MergeIn() error = %v (%T); want *fabricengine.MergeGuardError", errWeftDirty, errWeftDirty)
	}
	if len(guardErrWarp.Reasons) != 1 || guardErrWarp.Reasons[0] != "worktree dirty" {
		t.Errorf("warp-dirty guard reasons = %v; want exactly [\"worktree dirty\"]", guardErrWarp.Reasons)
	}
	if errWarpDirty.Error() != errWeftDirty.Error() {
		t.Errorf("error values diverge by side: warp-dirty = %q, weft-dirty = %q; want byte-identical", errWarpDirty.Error(), errWeftDirty.Error())
	}
}

// TestMergeIn_UnmappablePathConflict_SelfAbortsBothSides covers a weft-side conflict on a repo-root
// path outside the wired name-set: the merge self-aborts on both sides, returns
// *ErrUnmergeableState, restores the pair to its pre-merge SHAs, and leaves no record.
func TestMergeIn_UnmappablePathConflict_SelfAbortsBothSides(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	branchAtCurrentHEAD(t, h.PrimeWorktree(), "feature")
	setupConflictingDivergence(t, h.PrimeWeft(), "feature-weft", "unmappable-root-conflict.txt")

	warpStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	_, err := f.MergeIn("feature")
	var unmergeableErr *fabricengine.ErrUnmergeableState
	if !errors.As(err, &unmergeableErr) {
		t.Fatalf("MergeIn(feature) with an unmappable weft-root conflict: error = %v (%T); want *fabricengine.ErrUnmergeableState", err, err)
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStartSHA {
		t.Errorf("warp HEAD after self-abort = %q; want restored pre-merge SHA %q", got, warpStartSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftStartSHA {
		t.Errorf("weft HEAD after self-abort = %q; want restored pre-merge SHA %q", got, weftStartSHA)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after self-abort = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMergeIn_ConflictMarkers_NeverLeakWeftName covers conflict-marker content: a weft-only
// conflict's markers carry the merged source SHA as their trailing label and never a "-weft"-suffixed
// name, and a warp-only conflict's markers are styled identically, so the two are indistinguishable.
func TestMergeIn_ConflictMarkers_NeverLeakWeftName(t *testing.T) {
	hWeft, fWeft, _, _, _, _ := newMergePairFixture(t, ".")
	branchAtCurrentHEAD(t, hWeft.PrimeWorktree(), "feature")
	setupConflictingDivergence(t, hWeft.PrimeWeft(), "feature-weft", "_lyx/marker-conflict.txt")
	weftSourceSHA, err := fabricengine.WeftForTest(fWeft).ResolveSHA("feature-weft")
	if err != nil {
		t.Fatalf("ResolveSHA(feature-weft) error = %v", err)
	}
	if _, err := fWeft.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) [weft-only conflict] error = %v", err)
	}
	weftConflictContent, err := os.ReadFile(filepath.Join(hWeft.PrimeWeft(), "_lyx", "marker-conflict.txt"))
	if err != nil {
		t.Fatalf("read conflicted file: %v", err)
	}
	if strings.Contains(string(weftConflictContent), "-weft") {
		t.Errorf("weft-only conflict markers leak a \"-weft\" name: %s", weftConflictContent)
	}
	if !strings.Contains(string(weftConflictContent), ">>>>>>> "+weftSourceSHA) {
		t.Errorf("weft-only conflict markers = %s; want a trailing \">>>>>>> %s\" label", weftConflictContent, weftSourceSHA)
	}

	hWarp, fWarp, _, _, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, hWarp.PrimeWorktree(), "feature", "marker-conflict.txt")
	branchAtCurrentHEAD(t, hWarp.PrimeWeft(), "feature-weft")
	warpSourceSHA, err := fabricengine.WarpForTest(fWarp).ResolveSHA("feature")
	if err != nil {
		t.Fatalf("ResolveSHA(feature) error = %v", err)
	}
	if _, err := fWarp.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) [warp-only conflict] error = %v", err)
	}
	warpConflictContent, err := os.ReadFile(filepath.Join(hWarp.PrimeWorktree(), "marker-conflict.txt"))
	if err != nil {
		t.Fatalf("read conflicted file: %v", err)
	}
	if !strings.Contains(string(warpConflictContent), ">>>>>>> "+warpSourceSHA) {
		t.Errorf("warp-only conflict markers = %s; want a trailing \">>>>>>> %s\" label", warpConflictContent, warpSourceSHA)
	}

	weftMarkerPrefix := strings.SplitN(string(weftConflictContent), "\n", 2)[0]
	warpMarkerPrefix := strings.SplitN(string(warpConflictContent), "\n", 2)[0]
	if weftMarkerPrefix != warpMarkerPrefix {
		t.Errorf("conflict marker opening line diverges by side: weft-only = %q, warp-only = %q; want identical style", weftMarkerPrefix, warpMarkerPrefix)
	}
}

// TestMergeIn_SubpathAnchoredHub_PathMapping covers path mapping on a subpath-anchored hub: a
// conflict inside the junctioned path is reported at its unified worktree-root-relative path, and
// the reported file is reachable there through the junction.
func TestMergeIn_SubpathAnchoredHub_PathMapping(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, "backend")

	branchAtCurrentHEAD(t, h.PrimeWorktree(), "feature")
	setupConflictingDivergence(t, h.PrimeWeft(), "feature-weft", "backend/_lyx/anchored-conflict.txt")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	want := "backend/_lyx/anchored-conflict.txt"
	if len(res.Conflicts) != 1 || res.Conflicts[0] != want {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want [%q]", res.Conflicts, want)
	}

	visiblePath := filepath.Join(h.PrimeWorktree(), filepath.FromSlash(want))
	if _, err := os.Stat(visiblePath); err != nil {
		t.Errorf("os.Stat(%s) error = %v; want the conflicted file reachable through the junction from the visible worktree root", visiblePath, err)
	}
}

// TestMergeContinue_InvisibleLandedConclude_AdoptsInsteadOfSticking covers the crash shape where a
// side's conclude-commit landed but the record never learned its SHA — the state a kill between
// `git commit` and the record re-save leaves behind, simulated exactly by resolving a conflicted
// MergeIn and committing each side with plain git while the record still says committed:"".
// Before the adoption arm existed this state was a permanent wedge: MergeContinue re-ran
// `git commit` on a clean tree and failed forever, MergeAbort refused via concludeLandedReason, and
// no fabric verb could ever clear the record.
// The resumed MergeContinue must adopt both landed commits off HEAD — creating no new commit —
// report Committed true, record one KindMergeCommitted per adopted side carrying the adopted SHA,
// and delete the record.
func TestMergeContinue_InvisibleLandedConclude_AdoptsInsteadOfSticking(t *testing.T) {
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

	// Resolve both sides, then land each conclude with plain git — on-disk state now byte-identical
	// to a crash between concludeMergeSides' `git commit` and its record re-save, on both sides.
	writeResolved := func(dir, rel string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), []byte("resolved\n"), 0o644); err != nil {
			t.Fatalf("write resolved %s: %v", rel, err)
		}
	}
	writeResolved(h.PrimeWorktree(), "clash.txt")
	writeResolved(h.PrimeWeft(), "_lyx/weft-clash.txt")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "clash.txt")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "--no-edit")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "add", "_lyx/weft-clash.txt")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "--no-edit")

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	if st.WarpCommitted != "" || st.WeftCommitted != "" {
		t.Fatalf("recorded conclude SHAs = (%q, %q); want both empty — the invisible shape", st.WarpCommitted, st.WeftCommitted)
	}

	// Sanity: MergeAbort must refuse this state (R2's guard), leaving MergeContinue as the recovery.
	fresh := openFreshFabric(t, h.PrimeWorktree())
	if _, err := fresh.MergeAbort(); err == nil {
		t.Fatalf("MergeAbort() on invisible landed conclude: error = nil; want conclude-landed guard refusal")
	}

	warpHEADBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftHEADBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	contRes, err := fresh.MergeContinue("")
	if err != nil {
		t.Fatalf("MergeContinue() on invisible landed conclude: error = %v; want adoption to finish the merge", err)
	}
	if !contRes.Committed {
		t.Errorf("MergeContinue().Committed = false; want true — the pair carries this merge's conclude-commits")
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpHEADBefore {
		t.Errorf("warp HEAD after adoption = %q; want unchanged %q — adoption must not create a commit", got, warpHEADBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftHEADBefore {
		t.Errorf("weft HEAD after adoption = %q; want unchanged %q — adoption must not create a commit", got, weftHEADBefore)
	}

	adopted := map[string]bool{warpHEADBefore: false, weftHEADBefore: false}
	for _, entry := range contRes.Mutated().Entries() {
		if entry.Kind == fabricengine.KindMergeCommitted {
			adopted[entry.Detail] = true
		}
	}
	if !adopted[warpHEADBefore] || !adopted[weftHEADBefore] {
		t.Errorf("MergeContinue() mutations = %v; want KindMergeCommitted carrying both adopted SHAs %q and %q", contRes.Mutated().Entries(), warpHEADBefore, weftHEADBefore)
	}

	if exists, err := fabricengine.MergeRecordExistsForTest(fresh); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after adoption = (%v, %v); want (false, nil)", exists, err)
	}
}

// resolveSHAForTest resolves ref to a full SHA in dir via plain git — the independent read a test
// uses to name a commit fabric itself resolved, rather than trusting fabric's own answer.
func resolveSHAForTest(t *testing.T, dir, ref string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s in %s: %v", ref, dir, err)
	}
	return strings.TrimSpace(string(out))
}

// isAncestorForTest reports whether ancestor is reachable from descendant in dir, via plain
// `git merge-base --is-ancestor` — how a test proves a merge source really did (or really did not)
// get merged.
func isAncestorForTest(t *testing.T, dir, ancestor, descendant string) bool {
	t.Helper()

	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// abortMergeAndLandUnrelatedCommit reproduces the adversarial shape MergeContinue's adoption arm
// must refuse: an operator discards the in-progress git merge with plain `git merge --abort` and
// then lands one commit of their own, while fabric's merge record is still live and still says this
// side's conclude never happened.
// The resulting HEAD satisfies "moved off the recorded pre-merge start, with no live MERGE_HEAD"
// exactly as a real crashed-after-commit conclude does — that ambiguity is the whole point — and it
// returns the unrelated SHA so the caller can assert fabric never claims it.
func abortMergeAndLandUnrelatedCommit(t *testing.T, dir, filename string) string {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "merge", "--abort")
	commitOnCurrentBranch(t, dir, filename, "nothing to do with the merge\n", "unrelated operator commit")
	return fabricengine.CurrentSHAForTest(t, dir)
}

// TestMergeContinue_UnrelatedCommitWhileRecordLive_IsNeverAdopted is the adversarial direction of
// the conclude-adoption arm.
// Adoption is a positive CLAIM about which commit a checkout carries, so it must rest on
// discriminating evidence — git's own parentage — rather than on the same ambiguous "HEAD moved,
// no live MERGE_HEAD" read that MergeAbort's concludeLandedReason uses to REFUSE. Keying adoption
// on that read alone made this exact sequence return ok/committed true naming the operator's
// unrelated commit, delete the record, and leave the merge source un-merged with nothing left to
// inspect: a silent false success.
// The correct disposition is honestly stuck — *ErrMergeIncomplete with the record retained, so the
// operator can still see what fabric thinks is happening.
func TestMergeContinue_UnrelatedCommitWhileRecordLive_IsNeverAdopted(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	setupConflictingDivergence(t, h.PrimeWeft(), "feature-weft", "_lyx/weft-clash.txt")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) != 2 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want both sides conflicted — the scenario needs a real conclude pending on each side", res.Conflicts)
	}

	sourceWarpSHA := resolveSHAForTest(t, h.PrimeWorktree(), "feature")
	unrelatedWarpSHA := abortMergeAndLandUnrelatedCommit(t, h.PrimeWorktree(), "warp-unrelated.txt")
	unrelatedWeftSHA := abortMergeAndLandUnrelatedCommit(t, h.PrimeWeft(), "_lyx/weft-unrelated.txt")

	// Precondition, asserted rather than assumed: the record must still be live and must still show
	// neither side concluded, or the scenario is not the one this test names.
	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	if st.WarpCommitted != "" || st.WeftCommitted != "" {
		t.Fatalf("recorded conclude SHAs = (%q, %q); want both empty", st.WarpCommitted, st.WeftCommitted)
	}
	if st.WarpStart == unrelatedWarpSHA {
		t.Fatalf("warp HEAD did not move off the recorded start %q; the ambiguous signal this test exercises is not present", st.WarpStart)
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	contRes, err := fresh.MergeContinue("")

	var incompleteErr *fabricengine.ErrMergeIncomplete
	if !errors.As(err, &incompleteErr) {
		t.Fatalf("MergeContinue() over an operator's unrelated commit: (committed %v, error %v (%T)); want *fabricengine.ErrMergeIncomplete and no adoption", contRes.Committed, err, err)
	}
	if contRes.Committed {
		t.Errorf("MergeContinue().Committed = true; want false — no conclude for this merge has landed")
	}
	for _, entry := range contRes.Mutated().Entries() {
		if entry.Kind == fabricengine.KindMergeCommitted {
			t.Errorf("MergeContinue() recorded %v; want no KindMergeCommitted — nothing was concluded", entry)
		}
	}

	if exists, err := fabricengine.MergeRecordExistsForTest(fresh); err != nil || !exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (true, nil) — the record is the operator's only remaining handle on this merge", exists, err)
	}
	if isAncestorForTest(t, h.PrimeWorktree(), sourceWarpSHA, unrelatedWarpSHA) {
		t.Fatalf("feature (%q) is an ancestor of the unrelated commit (%q); the fixture did not actually leave the source un-merged", sourceWarpSHA, unrelatedWarpSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != unrelatedWeftSHA {
		t.Errorf("weft HEAD after the refusal = %q; want the operator's own commit %q left untouched", got, unrelatedWeftSHA)
	}
}

// TestMergeContinue_MergeOfSourceOntoWrongBase_IsNeverAdopted pins the first-parent clause of the
// adoption evidence, which no other test exercises: deleting `parents[0] != start` from
// sideConcludeAlreadyLanded left the whole suite green, because the unrelated-commit test's fixture
// is a ONE-parent commit that `len(parents) < 2` refuses on its own.
// The shape only the first-parent clause refuses: the operator plain-git-aborts the recorded merge,
// lands a commit of their own, and then merges the recorded source SHA on top of it — a genuine
// two-parent merge of the RIGHT source on the WRONG base. Adopting it would record correspondence
// against a base the paired side never saw; doc.go names this exact shape as not-adopted, and its
// plain-git recovery instructions (reset to the recorded start first) exist because of it.
func TestMergeContinue_MergeOfSourceOntoWrongBase_IsNeverAdopted(t *testing.T) {
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

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}

	// Warp: abort the recorded merge, land an unrelated base commit, then hand-merge the RECORDED
	// source SHA onto it and resolve — parents [unrelated, recorded source].
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--abort")
	commitOnCurrentBranch(t, h.PrimeWorktree(), "wrong-base.txt", "operator's own base\n", "unrelated base commit")
	wrongBaseSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	runGitExpectingConflict(t, h.PrimeWorktree(), "merge", st.WarpSource)
	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "clash.txt"), []byte("resolved on the wrong base\n"), 0o644); err != nil {
		t.Fatalf("write resolved clash.txt: %v", err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "clash.txt")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "--no-edit")
	handSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	// Weft: abort back to its recorded start, so MergeContinue's unresolved-conflicts guard passes
	// and the call genuinely reaches the warp adoption probe.
	gitkit.MustRun(t, h.PrimeWeft(), "git", "merge", "--abort")

	// Precondition, asserted rather than assumed: the hand-landed commit is a two-parent merge whose
	// SECOND parent is the recorded source and whose FIRST parent is NOT the recorded start — the one
	// shape only the first-parent clause can refuse.
	parents := commitParentsForTest(t, h.PrimeWorktree(), handSHA)
	if len(parents) != 2 || parents[0] != wrongBaseSHA || parents[1] != st.WarpSource {
		t.Fatalf("hand-landed commit parents = %v; want [%s %s] — a merge of the recorded source on the wrong base", parents, wrongBaseSHA, st.WarpSource)
	}
	if parents[0] == st.WarpStart {
		t.Fatalf("hand-landed commit's first parent equals the recorded start %q; the wrong-base shape this test needs is not present", st.WarpStart)
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	contRes, err := fresh.MergeContinue("")
	var incompleteErr *fabricengine.ErrMergeIncomplete
	if !errors.As(err, &incompleteErr) {
		t.Fatalf("MergeContinue() over a wrong-base source merge: (committed %v, error %v (%T)); want *fabricengine.ErrMergeIncomplete and no adoption", contRes.Committed, err, err)
	}
	for _, entry := range contRes.Mutated().Entries() {
		if entry.Kind == fabricengine.KindMergeCommitted {
			t.Errorf("MergeContinue() recorded %v; want no KindMergeCommitted — a wrong-base merge is a merge of a different base", entry)
		}
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(fresh); err != nil || !exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (true, nil)", exists, err)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != handSHA {
		t.Errorf("warp HEAD after the refusal = %q; want the operator's own merge %q left untouched", got, handSHA)
	}
}

// TestMergeContinue_MergeOfWrongSourceOntoStart_IsNeverAdopted pins the source-membership clause of
// the adoption evidence — the mirror of the wrong-base test above: a genuine two-parent merge on
// the RIGHT base whose merged-in tip is NOT the recorded source. Only the "one of the remaining
// parents is the recorded source SHA" clause refuses it; without that clause adoption would claim a
// merge of some other branch as this merge's conclude.
func TestMergeContinue_MergeOfWrongSourceOntoStart_IsNeverAdopted(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	setupConflictingDivergence(t, h.PrimeWeft(), "feature-weft", "_lyx/weft-clash.txt")

	// A decoy branch off the current warp HEAD with one non-conflicting commit, so a plain merge of
	// it onto the recorded start auto-concludes into a two-parent commit.
	commitOnBranch(t, h.PrimeWorktree(), "decoy", "decoy.txt", "nothing to do with feature\n", "decoy commit")
	decoySHA := resolveSHAForTest(t, h.PrimeWorktree(), "decoy")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) != 2 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want both sides conflicted", res.Conflicts)
	}

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}

	// Warp: abort back to the recorded start, then merge the decoy — parents [start, decoy].
	// --no-ff, because the decoy sits one commit ahead of the start and a plain merge would
	// fast-forward without creating the two-parent commit this fixture is about.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--abort")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-ff", "--no-edit", decoySHA)
	handSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	// Weft: abort back to its recorded start so the unresolved-conflicts guard passes.
	gitkit.MustRun(t, h.PrimeWeft(), "git", "merge", "--abort")

	// Precondition, asserted rather than assumed: right base, wrong source.
	parents := commitParentsForTest(t, h.PrimeWorktree(), handSHA)
	if len(parents) != 2 || parents[0] != st.WarpStart || parents[1] != decoySHA {
		t.Fatalf("hand-landed commit parents = %v; want [%s %s] — a merge of the wrong source on the recorded start", parents, st.WarpStart, decoySHA)
	}
	if parents[1] == st.WarpSource {
		t.Fatalf("decoy tip equals the recorded source %q; the wrong-source shape this test needs is not present", st.WarpSource)
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	contRes, err := fresh.MergeContinue("")
	var incompleteErr *fabricengine.ErrMergeIncomplete
	if !errors.As(err, &incompleteErr) {
		t.Fatalf("MergeContinue() over a wrong-source merge: (committed %v, error %v (%T)); want *fabricengine.ErrMergeIncomplete and no adoption", contRes.Committed, err, err)
	}
	for _, entry := range contRes.Mutated().Entries() {
		if entry.Kind == fabricengine.KindMergeCommitted {
			t.Errorf("MergeContinue() recorded %v; want no KindMergeCommitted — a merge of some other branch is not this merge's conclude", entry)
		}
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(fresh); err != nil || !exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (true, nil)", exists, err)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != handSHA {
		t.Errorf("warp HEAD after the refusal = %q; want the operator's own merge %q left untouched", got, handSHA)
	}
}

// runGitExpectingConflict runs git with args in dir, tolerating the nonzero exit a conflicted merge
// returns and failing the test on any spawn-level error.
func runGitExpectingConflict(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("git %v in %s: %v", args, dir, err)
		}
	}
}

// commitParentsForTest reads sha's parent SHAs in dir via plain git, first parent first — the
// independent read the adoption-evidence tests assert their fixture's parentage with.
func commitParentsForTest(t *testing.T, dir, sha string) []string {
	t.Helper()

	cmd := exec.Command("git", "rev-list", "--parents", "-n", "1", sha)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-list --parents -n 1 %s in %s: %v", sha, dir, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 || fields[0] != sha {
		t.Fatalf("git rev-list --parents -n 1 %s in %s = %q; want the commit itself first", sha, dir, out)
	}
	return fields[1:]
}

// TestMergeContinue_SquashConcludeLandedByHand_IsNeverAdopted drives the squash shape of the same
// question, which prior rounds reasoned about but never executed.
// `git merge --squash` writes no MERGE_HEAD and its conclude is an ORDINARY one-parent commit, so
// the parentage evidence a non-squash conclude carries — first parent the pre-merge start, second
// parent the merge source — does not exist for a squash. There is therefore nothing to tell a
// hand-landed squash conclude apart from any other commit, and adoption must refuse rather than
// silently inherit the non-squash predicate.
// The refusal is honest, not lossy: the record is retained and *ErrMergeIncomplete comes back, which
// is exactly the behaviour that held before an adoption arm existed at all.
func TestMergeContinue_SquashConcludeLandedByHand_IsNeverAdopted(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "warp-branch.txt", "warp-current.txt")
	setupCleanNonFastForward(t, h.PrimeWeft(), "feature-weft", "weft-branch.txt", "weft-current.txt")

	// Sabotage the warp conclude so Merge stops with the record retained and warp_outcome staged —
	// the state a crash between `git merge --squash` and the conclude commit leaves.
	installFailingPreCommitHook(t, h.PrimeWorktree())
	_, err := f.Merge("feature", fabricengine.MergeOptions{Squash: true})
	var incompleteErr *fabricengine.ErrMergeIncomplete
	if !errors.As(err, &incompleteErr) {
		t.Fatalf("Merge(feature, squash) with the warp conclude sabotaged: error = %v (%T); want *fabricengine.ErrMergeIncomplete", err, err)
	}
	removePreCommitHook(t, h.PrimeWorktree())

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	if !st.Squash {
		t.Fatalf("merge state Squash = false; the squash shape this test names was not recorded")
	}
	if st.WarpCommitted != "" {
		t.Fatalf("merge state WarpCommitted = %q; want empty — the warp conclude must not have landed yet", st.WarpCommitted)
	}
	warpStart := st.WarpStart

	// The operator finishes the squash by hand, exactly as doc.go's plain-git last resort describes.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "-q", "-m", "hand-landed squash conclude")
	handLandedSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	if handLandedSHA == warpStart {
		t.Fatalf("warp HEAD did not move; the hand-landed squash conclude this test needs is not present")
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	contRes, err := fresh.MergeContinue("")
	if !errors.As(err, &incompleteErr) {
		t.Fatalf("MergeContinue() over a hand-landed squash conclude: (committed %v, error %v (%T)); want *fabricengine.ErrMergeIncomplete — a squash carries no evidence to adopt on", contRes.Committed, err, err)
	}
	if contRes.Committed {
		t.Errorf("MergeContinue().Committed = true; want false")
	}
	for _, entry := range contRes.Mutated().Entries() {
		if entry.Kind == fabricengine.KindMergeCommitted {
			t.Errorf("MergeContinue() recorded %v; want no KindMergeCommitted — a squash conclude is never adopted", entry)
		}
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(fresh); err != nil || !exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (true, nil)", exists, err)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != handLandedSHA {
		t.Errorf("warp HEAD after the refusal = %q; want the operator's hand-landed commit %q left untouched", got, handLandedSHA)
	}
}

// TestMergeContinue_BothSidesAlreadyUpToDate_DerivesAlreadyUpToDate pins that MergeContinue derives
// BOTH result flags from the merge record, the same rule MergeIn and Merge follow.
// Committed was already derived; AlreadyUpToDate was hardcoded to its zero value at MergeContinue's
// single return, so a resumed record whose two sides both recorded up_to_date came back
// already_up_to_date:false where the equivalent single-shot call returns true.
// That record shape is reachable rather than theoretical: MergeIn/Merge's up-to-date probe runs
// before the write lock by design, so a call that loses that race records up_to_date on both sides,
// and a crash before its conclude leaves precisely this record behind. The fixture writes it
// directly, which is the same on-disk state that crash produces.
func TestMergeContinue_BothSidesAlreadyUpToDate_DerivesAlreadyUpToDate(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())
	if err := fabricengine.SaveMergeStateForTest(f, fabricengine.MergeStateForTest{
		Verb:        "merge-in",
		Source:      "feature",
		WarpStart:   warpStart,
		WeftStart:   weftStart,
		WarpSource:  warpStart,
		WeftSource:  weftStart,
		WarpOutcome: "up_to_date",
		WeftOutcome: "up_to_date",
		StartedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("SaveMergeStateForTest() error = %v", err)
	}

	res, err := openFreshFabric(t, h.PrimeWorktree()).MergeContinue("")
	if err != nil {
		t.Fatalf("MergeContinue() over a both-sides-up-to-date record: error = %v", err)
	}
	if !res.AlreadyUpToDate {
		t.Errorf("MergeContinue().AlreadyUpToDate = false; want true — both recorded outcomes are up_to_date, and the flag is read off the record")
	}
	if res.Committed {
		t.Errorf("MergeContinue().Committed = true; want false — no side had anything to conclude")
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStart {
		t.Errorf("warp HEAD = %q; want unchanged %q — an up_to_date side is never concluded", got, warpStart)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after MergeContinue = (%v, %v); want (false, nil)", exists, err)
	}
}
