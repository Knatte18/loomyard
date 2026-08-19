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
	if !res.Committed {
		t.Fatalf("MergeIn(feature).Committed = false; want true")
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
	if !res.Committed {
		t.Fatalf("MergeIn(feature).Committed = false; want true")
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
