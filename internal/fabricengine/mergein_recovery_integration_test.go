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
	"github.com/Knatte18/loomyard/internal/hubforge"
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

// TestMergeContinue_ConcludeFailureThenRetryConcludes covers a conclude-phase failure on the warp
// side — the only side concludeMergeSides now has anything to conclude on: the conclude-commit is
// sabotaged to fail, so MergeIn returns *ErrMergeIncomplete with the record retained and no
// WarpCommitted recorded; removing the sabotage and re-running MergeContinue then lands the
// conclude-commit for the first time.
// This test used to sabotage the weft side specifically, pinning a partial failure where warp had
// already landed and only the weft side needed a retry. That shape has no warp-only analogue and is
// deleted along with it: concludeMergeSides has only the warp side left to conclude, so a conclude
// failure is total, not partial — see the merge-drops-weft task. The already-landed-then-retried
// idempotency this test also used to cover is pinned instead by
// TestMergeContinue_InvisibleLandedConclude_AdoptsInsteadOfSticking.
func TestMergeContinue_ConcludeFailureThenRetryConcludes(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	// setupCleanNonFastForward forks the branch off the pre-divergence HEAD, then advances current
	// separately — a genuine (non-fast-forward) merge target, so the merge lands a real conclude
	// commit rather than a fast-forward (which concludeMergeSides skips outright).
	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "warp-branch.txt", "warp-current.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	installFailingPreCommitHook(t, h.PrimeWorktree())

	_, err := f.MergeIn("feature")
	var incompleteErr *fabricengine.ErrMergeIncomplete
	if !errors.As(err, &incompleteErr) {
		t.Fatalf("MergeIn(feature) with warp conclude sabotaged: error = %v (%T); want *fabricengine.ErrMergeIncomplete", err, err)
	}

	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || !exists {
		t.Errorf("MergeRecordExistsForTest() after conclude failure = (%v, %v); want (true, nil)", exists, err)
	}
	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	if st.WarpCommitted != "" {
		t.Fatalf("merge state WarpCommitted = %q; want empty — the sabotaged conclude must not have landed anything", st.WarpCommitted)
	}

	removePreCommitHook(t, h.PrimeWorktree())

	res, err := f.MergeContinue("")
	if err != nil {
		t.Fatalf("MergeContinue() after removing sabotage: error = %v", err)
	}
	if !res.Committed {
		t.Errorf("MergeContinue().Committed = false; want true")
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after the retry = (%v, %v); want (false, nil)", exists, err)
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
// TestMergeIn_UnmappablePathConflict_SelfAbortsBothSides used to cover a weft-root conflict outside
// the wired name set, which unifyConflictPaths' own weftPathVisible test flagged unmappable and
// self-aborted the merge with *ErrUnmergeableState. MergeIn now calls unifyConflictPaths with a
// literal nil weft conflict list — the weft can no longer conflict at all — so that loop, and the
// unmappable outcome it alone could produce, is unreachable from MergeIn: a warp path (already
// worktree-relative) never sets unmappable, and the same-path-from-both-sides collision needs a
// non-empty weft list to occur. This test is deleted rather than adapted, since there is no warp-side
// shape left that reaches the branch under test — see the merge-drops-weft task.
// unifyConflictPaths' own unmappable behavior stays covered directly, at the unit level, by
// mergepaths_test.go, which is exactly the plumbing-is-retained decision's point: the function stays
// correct for a caller that still passes it a real weft list, even though MergeIn is no longer one.

// TestMergeIn_ConflictMarkers_NeverLeakWeftName used to cover conflict-marker content on both a
// weft-only conflict and a warp-only conflict, asserting the two were styled identically so neither
// ever leaked a "-weft"-suffixed branch name. The weft arm is deleted along with the weft's ability to
// conflict at all — see the merge-drops-weft task — leaving the warp-only assertion, which needs no
// cross-side comparison any more since there is no other side to compare against.
func TestMergeIn_ConflictMarkers_NeverLeakWeftName(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "marker-conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")
	warpSourceSHA, err := fabricengine.WarpForTest(f).ResolveSHA("feature")
	if err != nil {
		t.Fatalf("ResolveSHA(feature) error = %v", err)
	}
	if _, err := f.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) [warp conflict] error = %v", err)
	}
	warpConflictContent, err := os.ReadFile(filepath.Join(h.PrimeWorktree(), "marker-conflict.txt"))
	if err != nil {
		t.Fatalf("read conflicted file: %v", err)
	}
	if strings.Contains(string(warpConflictContent), "-weft") {
		t.Errorf("warp conflict markers leak a \"-weft\" name: %s", warpConflictContent)
	}
	if !strings.Contains(string(warpConflictContent), ">>>>>>> "+warpSourceSHA) {
		t.Errorf("warp conflict markers = %s; want a trailing \">>>>>>> %s\" label", warpConflictContent, warpSourceSHA)
	}
}

// TestMergeIn_SubpathAnchoredHub_PathMapping used to cover path mapping on a subpath-anchored hub for
// a weft-side conflict: a conflict inside the junctioned path had to be reported at its unified
// worktree-root-relative path via weftPathVisible's anchor/wired-name join. That mapping step only
// ever ran for weft paths — a warp path already IS worktree-relative and passes through
// unifyConflictPaths unchanged, subpath anchor or not, so there is no warp-side version of this test
// that would exercise anything the plain warp-conflict tests do not already cover. Deleted along with
// the weft's ability to conflict at all — see the merge-drops-weft task. The mapping function itself
// stays covered at the unit level by mergepaths_test.go.

// TestMergeContinue_InvisibleLandedConclude_AdoptsInsteadOfSticking covers the crash shape where the
// warp side's conclude-commit landed but the record never learned its SHA — the state a kill between
// `git commit` and the record re-save leaves behind, simulated exactly by resolving a conflicted
// MergeIn and committing the warp side with plain git while the record still says committed:"".
// Before the adoption arm existed this state was a permanent wedge: MergeContinue re-ran
// `git commit` on a clean tree and failed forever, MergeAbort refused via concludeLandedReason, and
// no fabric verb could ever clear the record.
// The resumed MergeContinue must adopt the landed commit off HEAD — creating no new commit — report
// Committed true, record a KindMergeCommitted carrying the adopted SHA, and delete the record.
// This test used to also land and adopt a weft-side conclude, since a weft-side conflict was how the
// two-sided design kept BOTH sides pending. With the weft no longer able to conflict, there is only
// the warp side left to adopt — see the merge-drops-weft task.
func TestMergeContinue_InvisibleLandedConclude_AdoptsInsteadOfSticking(t *testing.T) {
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

	// Resolve, then land the conclude with plain git — on-disk state now byte-identical to a crash
	// between concludeMergeSides' `git commit` and its record re-save.
	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "clash.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolved clash.txt: %v", err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "clash.txt")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "--no-edit")

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	if st.WarpCommitted != "" {
		t.Fatalf("recorded warp conclude SHA = %q; want empty — the invisible shape", st.WarpCommitted)
	}

	// Sanity: MergeAbort must refuse this state (R2's guard), leaving MergeContinue as the recovery.
	fresh := openFreshFabric(t, h.PrimeWorktree())
	if _, err := fresh.MergeAbort(); err == nil {
		t.Fatalf("MergeAbort() on invisible landed conclude: error = nil; want conclude-landed guard refusal")
	}

	warpHEADBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	contRes, err := fresh.MergeContinue("")
	if err != nil {
		t.Fatalf("MergeContinue() on invisible landed conclude: error = %v; want adoption to finish the merge", err)
	}
	if !contRes.Committed {
		t.Errorf("MergeContinue().Committed = false; want true — the pair carries this merge's conclude-commit")
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpHEADBefore {
		t.Errorf("warp HEAD after adoption = %q; want unchanged %q — adoption must not create a commit", got, warpHEADBefore)
	}

	adopted := false
	for _, entry := range contRes.Mutated().Entries() {
		if entry.Kind == fabricengine.KindMergeCommitted && entry.Detail == warpHEADBefore {
			adopted = true
		}
	}
	if !adopted {
		t.Errorf("MergeContinue() mutations = %v; want a KindMergeCommitted carrying the adopted SHA %q", contRes.Mutated().Entries(), warpHEADBefore)
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
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want the warp-side conflict — the scenario needs a real conclude pending", res.Conflicts)
	}

	sourceWarpSHA := resolveSHAForTest(t, h.PrimeWorktree(), "feature")
	unrelatedWarpSHA := abortMergeAndLandUnrelatedCommit(t, h.PrimeWorktree(), "warp-unrelated.txt")

	// Precondition, asserted rather than assumed: the record must still be live and must still show
	// the warp side unconcluded, or the scenario is not the one this test names.
	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	if st.WarpCommitted != "" {
		t.Fatalf("recorded warp conclude SHA = %q; want empty", st.WarpCommitted)
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
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want the warp-side conflict", res.Conflicts)
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
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	// A decoy branch off the current warp HEAD with one non-conflicting commit, so a plain merge of
	// it onto the recorded start auto-concludes into a two-parent commit.
	commitOnBranch(t, h.PrimeWorktree(), "decoy", "decoy.txt", "nothing to do with feature\n", "decoy commit")
	decoySHA := resolveSHAForTest(t, h.PrimeWorktree(), "decoy")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want the warp-side conflict", res.Conflicts)
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

// rootCommitForTest returns dir's oldest root commit — a commit that is an ancestor of nothing the
// merge fixtures build on, so a branch created there stays outside the recorded start's history and
// git therefore keeps the recorded start as a parent of an octopus merge rather than discarding it
// as redundant.
func rootCommitForTest(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-list --max-parents=0 HEAD in %s: %v", dir, err)
	}
	lines := strings.Fields(string(out))
	if len(lines) == 0 {
		t.Fatalf("git rev-list --max-parents=0 HEAD in %s returned nothing", dir)
	}
	return lines[len(lines)-1]
}

// TestMergeContinue_OctopusMergeCarryingTheSource_IsNeverAdopted pins the parent-ARITY half of the
// adoption evidence, which no other test exercises: before this test, `len(parents) < 2` was a lower
// bound and the source was searched across ALL remaining parents, so a three-parent octopus whose
// first parent was the recorded start and whose second was the recorded source was adopted as this
// merge's own conclude.
// The shape only the exact-arity clause refuses: the operator discards the staged merge and then
// merges the recorded source TOGETHER with an unrelated branch — `git merge <source> <other>` — an
// ordinary thing to do and a commit git builds happily. Adopting it made fabric report
// committed:true naming that commit, record correspondence, and delete the record, while the
// checkout carried <other>'s content that no side of this merge brought in and that no merge_staged
// entry accounts for. fabric can never produce an octopus: it starts every non-squash merge with a
// single `git merge --ff --no-commit <sourceSHA>`, so its conclude has exactly two parents.
func TestMergeContinue_OctopusMergeCarryingTheSource_IsNeverAdopted(t *testing.T) {
	h, st := setupWarpStagedPendingConclude(t)

	// The operator discards the staged merge and merges the recorded source alongside an unrelated
	// branch. The decoy is rooted outside the recorded start's history, or git would drop the start as
	// a redundant parent and build a two-parent commit instead of the octopus this test needs.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--abort")
	decoyBase := rootCommitForTest(t, h.PrimeWorktree())
	warpBranch := currentBranchName(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", "-b", "decoy", decoyBase)
	commitOnCurrentBranch(t, h.PrimeWorktree(), "decoy.txt", "content nobody asked this merge for\n", "decoy: unrelated work")
	decoySHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", warpBranch)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-edit", st.WarpSource, decoySHA)
	octopusSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	// Precondition, asserted rather than assumed: a THREE-parent commit whose first parent is the
	// recorded start and whose second is the recorded source — every clause of the old loose test
	// satisfied, and only the exact-arity clause standing between it and adoption.
	parents := commitParentsForTest(t, h.PrimeWorktree(), octopusSHA)
	if len(parents) != 3 {
		t.Fatalf("hand-landed commit has %d parents (%v); want the 3-parent octopus this test is about", len(parents), parents)
	}
	if parents[0] != st.WarpStart || parents[1] != st.WarpSource || parents[2] != decoySHA {
		t.Fatalf("octopus parents = %v; want [%s %s %s]", parents, st.WarpStart, st.WarpSource, decoySHA)
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	contRes, err := fresh.MergeContinue("")

	var incompleteErr *fabricengine.ErrMergeIncomplete
	if !errors.As(err, &incompleteErr) {
		t.Fatalf("MergeContinue() over an octopus carrying the source: (committed %v, error %v (%T)); want *fabricengine.ErrMergeIncomplete and no adoption", contRes.Committed, err, err)
	}
	if contRes.Committed {
		t.Errorf("MergeContinue().Committed = true; want false — fabric never produced this commit")
	}
	for _, entry := range contRes.Mutated().Entries() {
		if entry.Kind == fabricengine.KindMergeCommitted {
			t.Errorf("MergeContinue() recorded %v; want no KindMergeCommitted — an octopus is not a conclude fabric can make", entry)
		}
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(fresh); err != nil || !exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (true, nil)", exists, err)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != octopusSHA {
		t.Errorf("warp HEAD after the refusal = %q; want the operator's own octopus %q left untouched", got, octopusSHA)
	}
}

// setupWarpStagedPendingConclude builds the one fixture several conclude-evidence tests in this file
// share: the warp side merges CLEANLY (a real non-fast-forward merge, outcome staged, MERGE_HEAD
// live) with no conclude landed yet, and the merge-state record already reflects that on disk —
// WarpOutcome "staged", WarpCommitted empty, WeftOutcome already "up_to_date" since the weft is not a
// merge participant.
// It builds this state directly via gitrepo.Repo.MergeStart and SaveMergeStateForTest rather than
// through a real MergeIn call. Before this batch, a weft-side conflict was what kept MergeIn from
// auto-concluding a clean warp side, isolating the tests below on the warp side alone; with the weft
// no longer able to conflict, a MergeIn whose warp side merges cleanly now concludes and deletes its
// own record immediately, so there is no other way left to leave this pending-conclude window open —
// see the merge-drops-weft task.
// It returns the hub and the manufactured record, asserting the fixture really is the shape described
// rather than assuming it.
func setupWarpStagedPendingConclude(t *testing.T) (*hubforge.Hub, fabricengine.MergeStateForTest) {
	t.Helper()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "warp-branch.txt", "warp-current.txt")

	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())
	warpRepo := fabricengine.WarpForTest(f)
	warpSourceSHA, err := warpRepo.ResolveSHA("feature")
	if err != nil {
		t.Fatalf("ResolveSHA(feature) error = %v", err)
	}
	if _, err := warpRepo.MergeStart(warpSourceSHA, false); err != nil {
		t.Fatalf("warp MergeStart(%s) error = %v", warpSourceSHA, err)
	}

	st := fabricengine.MergeStateForTest{
		Verb:        "merge-in",
		Source:      "feature",
		WarpStart:   warpStart,
		WeftStart:   weftStart,
		WarpSource:  warpSourceSHA,
		WarpOutcome: "staged",
		WeftOutcome: "up_to_date",
		StartedAt:   time.Now(),
	}
	if err := fabricengine.SaveMergeStateForTest(f, st); err != nil {
		t.Fatalf("SaveMergeStateForTest() error = %v", err)
	}
	return h, st
}

// assertConcludeRefusedWithoutCommitting asserts the disposition both conclude-evidence tests demand:
// the aggregated guard refusal naming the recorded-merge-gone reason, no conclude commit recorded or
// landed, the record still on disk, and the operator's own warp state left exactly as it was.
func assertConcludeRefusedWithoutCommitting(t *testing.T, h *hubforge.Hub, warpHEADBefore string, res fabricengine.MergeResult, err error) {
	t.Helper()

	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("MergeContinue() over a checkout carrying a different merge: (committed %v, error %v (%T)); want *fabricengine.MergeGuardError", res.Committed, err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "checkout no longer carries the recorded merge" {
		t.Errorf("guard reasons = %v; want exactly [\"checkout no longer carries the recorded merge\"]", guardErr.Reasons)
	}
	if res.Committed {
		t.Error("MergeContinue().Committed = true; want false — nothing was concluded")
	}
	for _, entry := range res.Mutated().Entries() {
		if entry.Kind == fabricengine.KindMergeCommitted {
			t.Errorf("MergeContinue() recorded %v; want no KindMergeCommitted — fabric must not claim a merge it did not start", entry)
		}
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpHEADBefore {
		t.Errorf("warp HEAD after the refusal = %q; want %q — the operator's own merge must be left uncommitted and untouched", got, warpHEADBefore)
	}
}

// TestMergeContinue_DifferentMergeLiveAtConcludeTime_IsNeverCommitted pins the commit-side twin of the
// adoption arm's evidence rule, and closes a silent false success the adoption hardening did not reach.
//
// concludeMergeSides finishes a pending side by running `git commit`, which commits whatever MERGE_HEAD
// names. The adoption arm demands exact parentage before CLAIMING a landed commit as this merge's
// conclude; the commit arm three lines below it demanded nothing at all. So an operator who — as the
// Fabric Git Invariant explicitly permits — discarded fabric's staged warp merge with plain
// `git merge --abort` and started an unrelated one instead had that unrelated merge committed by
// fabric, written into the record as this merge's conclude SHA, reported as a merge_committed mutation
// and an ok/committed:true result, paired into the correspondence index, and then erased along with the
// record. The merge source stayed un-merged on that side while the other side's half landed for good,
// leaving a permanently non-corresponding pair and nothing left to inspect.
//
// The adoption arm never sees this shape precisely because the operator did not commit: HEAD is still
// on the recorded start and a MERGE_HEAD is live, so sideConcludeAlreadyLanded correctly reports "not
// landed" and hands straight to the commit that was the defect.
func TestMergeContinue_DifferentMergeLiveAtConcludeTime_IsNeverCommitted(t *testing.T) {
	h, st := setupWarpStagedPendingConclude(t)

	// The operator discards fabric's staged merge and starts an unrelated one, leaving it uncommitted.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--abort")
	warpBranch := currentBranchName(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", "-b", "other", st.WarpStart)
	commitOnCurrentBranch(t, h.PrimeWorktree(), "other.txt", "work that has nothing to do with feature\n", "other: unrelated work")
	otherSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", warpBranch)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-commit", "--no-ff", otherSHA)

	// Preconditions, asserted rather than assumed: HEAD is back on the recorded start (so the adoption
	// arm cannot fire), and the live merge is genuinely a DIFFERENT one.
	warpHEADBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	if warpHEADBefore != st.WarpStart {
		t.Fatalf("warp HEAD = %q; want the recorded start %q, or this test is exercising the adoption arm instead", warpHEADBefore, st.WarpStart)
	}
	if otherSHA == st.WarpSource {
		t.Fatalf("the planted merge head equals the recorded source %q; the fixture must plant a DIFFERENT merge", st.WarpSource)
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	res, err := fresh.MergeContinue("")
	assertConcludeRefusedWithoutCommitting(t, h, warpHEADBefore, res, err)

	if exists, err := fabricengine.MergeRecordExistsForTest(fresh); err != nil || !exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (true, nil) — the record is the only evidence left", exists, err)
	}

	// The refusal must not wedge the pair: MergeAbort is still the documented way out of this state, and
	// it restores both sides from the recorded pre-merge SHAs.
	if _, err := fresh.MergeAbort(); err != nil {
		t.Errorf("MergeAbort() after the refusal error = %v; want the abort route to stay open", err)
	}
}

// TestMergeContinue_UncommittedOctopusCarryingTheSource_IsNeverCommitted is the uncommitted twin of
// TestMergeContinue_OctopusMergeCarryingTheSource_IsNeverAdopted, and it is what forces the evidence to
// be read from the WHOLE of MERGE_HEAD rather than from `git rev-parse --verify --quiet MERGE_HEAD`.
// The operator discards the staged merge and starts a merge of the recorded source TOGETHER with an
// unrelated decoy, leaving it uncommitted. MERGE_HEAD then carries two SHAs whose FIRST is the recorded
// source, so an equality test written against rev-parse's single-value answer passes and fabric commits
// an octopus it can never itself produce — the decoy's content landing under this merge's name, brought
// in by no side of it and named by no merge_staged entry.
// The test asserts that first-entry-equals-the-source precondition explicitly, so a regression to the
// truncating read fails here instead of passing.
func TestMergeContinue_UncommittedOctopusCarryingTheSource_IsNeverCommitted(t *testing.T) {
	h, st := setupWarpStagedPendingConclude(t)

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--abort")
	decoyBase := rootCommitForTest(t, h.PrimeWorktree())
	warpBranch := currentBranchName(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", "-b", "decoy", decoyBase)
	commitOnCurrentBranch(t, h.PrimeWorktree(), "decoy.txt", "content nobody asked this merge for\n", "decoy: unrelated work")
	decoySHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", warpBranch)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-commit", st.WarpSource, decoySHA)

	// Precondition, asserted rather than assumed: the truncating read reports exactly the recorded
	// source, so only reading every head can tell this state apart from a legitimate one.
	warpHEADBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	if got := firstMergeHeadForTest(t, h.PrimeWorktree()); got != st.WarpSource {
		t.Fatalf("git rev-parse --verify --quiet MERGE_HEAD = %q; want the recorded source %q — the octopus this test needs is not present", got, st.WarpSource)
	}
	if decoySHA == st.WarpSource {
		t.Fatalf("decoy SHA equals the recorded source %q; the fixture must plant a second, unrelated head", st.WarpSource)
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	res, err := fresh.MergeContinue("")
	assertConcludeRefusedWithoutCommitting(t, h, warpHEADBefore, res, err)
}

// TestMergeContinue_StagedContentWithNoLiveMergeAtConcludeTime_IsNeverCommitted pins the third arm of
// recordedMergeGoneReason — the one the no-live-merge exemption must NOT swallow.
// With fabric's merge discarded and the checkout clean, `git commit` fails by itself and the conclude
// already refuses honestly, which is why that state is exempt (refusing there would decide every
// adoption-evidence test before it reached the clause it pins). But the same discarded state with
// tracked content staged is not honest at all: `git commit --no-edit` succeeds, landing an ORDINARY
// one-parent commit of whatever the operator staged, which fabric then writes into the record as this
// merge's conclude SHA, reports as committed:true with a merge_committed mutation, pairs into the
// correspondence index, and erases the record behind.
// It is the same silent false success as the live-different-merge shape, reached without any second
// merge at all — one `git merge --abort` and one `git add`.
func TestMergeContinue_StagedContentWithNoLiveMergeAtConcludeTime_IsNeverCommitted(t *testing.T) {
	h, st := setupWarpStagedPendingConclude(t)

	// The operator discards fabric's staged merge and stages unrelated content of their own.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--abort")
	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "operator-staged.txt"), []byte("staged by hand\n"), 0o644); err != nil {
		t.Fatalf("write operator-staged.txt: %v", err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "operator-staged.txt")

	// Preconditions, asserted rather than assumed: no merge is live, HEAD is back on the recorded
	// start, and a plain `git commit` really WOULD succeed here — without that last fact the conclude
	// would fail on its own and this test would prove nothing.
	warpHEADBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	if warpHEADBefore != st.WarpStart {
		t.Fatalf("warp HEAD = %q; want the recorded start %q", warpHEADBefore, st.WarpStart)
	}
	if mergeHeadPresentInCheckout(t, h.PrimeWorktree()) {
		t.Fatal("warp checkout still carries a live MERGE_HEAD; this test needs the no-live-merge arm")
	}
	if !stagedContentPresentForTest(t, h.PrimeWorktree()) {
		t.Fatal("warp index carries nothing staged; `git commit` would fail on its own and this test would prove nothing")
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	res, err := fresh.MergeContinue("")
	assertConcludeRefusedWithoutCommitting(t, h, warpHEADBefore, res, err)

	if exists, err := fabricengine.MergeRecordExistsForTest(fresh); err != nil || !exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (true, nil)", exists, err)
	}
}

// stagedContentPresentForTest reports whether dir's index differs from HEAD, via
// `git diff --cached --quiet`'s exit status — the independent read the staged-content test asserts its
// fixture's "a commit would actually succeed here" precondition with.
func stagedContentPresentForTest(t *testing.T, dir string) bool {
	t.Helper()

	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = dir
	err := cmd.Run()
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("git diff --cached --quiet in %s: %v", dir, err)
	}
	return true
}

// firstMergeHeadForTest returns what `git rev-parse --verify --quiet MERGE_HEAD` reports in dir — the
// truncating, first-head-only answer the octopus test asserts its fixture against, read through plain
// git rather than through the method under test.
func firstMergeHeadForTest(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse --verify --quiet MERGE_HEAD in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
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
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

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

// TestMergeContinue_SecondMergeStartedOverALandedConclude_LeavesNoLiveMergeHead pins
// sideConcludeAlreadyLanded's live-MERGE_HEAD clause, which no other test reaches: replacing that
// clause with a no-op left the entire integration suite green.
// The state it guards is reachable. The operator hand-lands this merge's conclude (HEAD moves off
// the recorded start, MERGE_HEAD clears — the adoptable shape), and then, before running
// MergeContinue, starts a SECOND merge of their own in the same checkout that merges cleanly. The
// index carries no conflicts, so MergeContinue's own guard passes and control reaches the adoption
// probe with HEAD moved, no recorded conclude SHA, and a live MERGE_HEAD belonging to a merge fabric
// never started.
// Adopting there is the documented disaster shape: fabric would report the merge concluded, record
// correspondence, delete its record, and walk away leaving a live MERGE_HEAD in the checkout that no
// fabric verb can then clear — MergeAbort included, since with the record gone the state reads as
// foreign. That is exactly the wedge TestMergeCrucible_EmptyResultMergeIsConcludedNotAbandoned
// exists to prevent, and the assertion that discriminates is the same one: a verb that returns
// without error must never leave git-level merge state behind on either side.
func TestMergeContinue_SecondMergeStartedOverALandedConclude_LeavesNoLiveMergeHead(t *testing.T) {
	h, f, commitOnWarpBranch, _, _, _ := newMergePairFixture(t, ".")

	// The decoy is built first, off the pre-merge tip, so merging it later is a clean non-fast-forward.
	commitOnWarpBranch("decoy", "decoy.txt", "the operator's own second merge\n", "decoy: unrelated branch")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want the single warp-side conflict", res.Conflicts)
	}

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}

	// Resolve and hand-land this merge's conclude with plain git — the adoptable shape.
	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "clash.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolved clash.txt: %v", err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "clash.txt")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "-q", "--no-edit")
	concludeSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	// The operator now starts a second merge of their own and leaves it uncommitted.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-commit", "--no-ff", "decoy")

	// Preconditions, asserted rather than assumed: the hand-landed conclude really is the adoptable
	// shape, and a live MERGE_HEAD from the operator's own second merge really is present with a
	// conflict-free index — so nothing but the MERGE_HEAD clause can change the outcome.
	parents := commitParentsForTest(t, h.PrimeWorktree(), concludeSHA)
	if len(parents) != 2 || parents[0] != st.WarpStart || parents[1] != st.WarpSource {
		t.Fatalf("hand-landed conclude parents = %v; want [%s %s] — the adoptable shape", parents, st.WarpStart, st.WarpSource)
	}
	if !mergeHeadPresentInCheckout(t, h.PrimeWorktree()) {
		t.Fatal("warp checkout carries no live MERGE_HEAD; the second-merge state this test needs is not present")
	}
	if st.WarpCommitted != "" {
		t.Fatalf("recorded WarpCommitted = %q; want empty — the record must not have learned the conclude", st.WarpCommitted)
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	if _, err := fresh.MergeContinue(""); err != nil {
		t.Fatalf("MergeContinue() over a landed conclude with a second merge live: error = %v", err)
	}

	if mergeHeadPresentInCheckout(t, h.PrimeWorktree()) {
		t.Error("warp checkout still carries a live MERGE_HEAD after MergeContinue returned without error; the pair is wedged — no fabric verb can clear it once the record is gone")
	}
	if mergeHeadPresentInCheckout(t, h.PrimeWeft()) {
		t.Error("weft checkout still carries a live MERGE_HEAD after MergeContinue returned without error")
	}
	if !isAncestorForTest(t, h.PrimeWorktree(), st.WarpSource, fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())) {
		t.Errorf("recorded source %q is not an ancestor of warp HEAD; the merge did not actually land", st.WarpSource)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(fresh); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after MergeContinue = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMergeContinue_SquashRecordCarryingATwoParentMerge_IsNeverAdopted is the test the squash
// refusal actually needs, and the reason it exists is a proof gap rather than a new behaviour.
// TestMergeContinue_SquashConcludeLandedByHand_IsNeverAdopted above looks like it pins the `squash`
// clause but does not: its hand-landed squash conclude is an ORDINARY one-parent commit, so the
// parent-arity clause refuses it first. Deleting `squash ||` from sideConcludeAlreadyLanded left
// that test — and the entire integration suite — green.
// The shape only the squash clause can refuse is a squash record whose side carries a genuine
// TWO-parent merge of the recorded source on the recorded start. An operator reaches it by the
// documented route: a squash writes no MERGE_HEAD, so `git merge --abort` is unavailable and the
// way back is `git reset --hard <recorded start>`, after which finishing with a plain `git merge
// <recorded source>` produces exactly the parentage a non-squash conclude carries.
// Adopting it would record a MERGE commit as a SQUASH record's conclude and then record
// correspondence for a pair whose two sides carry structurally different history — one squashed,
// one not. Refusing keeps it honestly stuck, which is the pre-adoption-arm behaviour.
func TestMergeContinue_SquashRecordCarryingATwoParentMerge_IsNeverAdopted(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "warp-branch.txt", "warp-current.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	// Sabotage the warp conclude so Merge stops with the record retained, squash recorded, and the
	// warp side staged with no conclude SHA.
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
	if st.WarpSource == "" {
		t.Fatalf("merge state WarpSource is empty; the source-SHA clause would refuse before the squash clause is ever reached")
	}

	// The operator returns the side to the recorded start — `git merge --abort` is unavailable, since
	// a squash writes no MERGE_HEAD — and finishes with a plain, non-squash merge.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "reset", "--hard", st.WarpStart)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-edit", st.WarpSource)
	handSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	// Precondition, asserted rather than assumed: EVERY non-squash adoption clause is now satisfied —
	// HEAD moved, no live MERGE_HEAD, a resolved source SHA on the record, and exactly two parents in
	// exactly the right order. Only `squash` stands between this commit and adoption.
	parents := commitParentsForTest(t, h.PrimeWorktree(), handSHA)
	if len(parents) != 2 || parents[0] != st.WarpStart || parents[1] != st.WarpSource {
		t.Fatalf("hand-landed commit parents = %v; want [%s %s] — the two-parent shape only the squash clause refuses", parents, st.WarpStart, st.WarpSource)
	}
	if mergeHeadPresentInCheckout(t, h.PrimeWorktree()) {
		t.Fatalf("warp checkout still carries a live MERGE_HEAD; the MERGE_HEAD clause would refuse before the squash clause is reached")
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	contRes, err := fresh.MergeContinue("")
	if !errors.As(err, &incompleteErr) {
		t.Fatalf("MergeContinue() over a squash record carrying a two-parent merge: (committed %v, error %v (%T)); want *fabricengine.ErrMergeIncomplete — a squash conclude is never adopted", contRes.Committed, err, err)
	}
	if contRes.Committed {
		t.Errorf("MergeContinue().Committed = true; want false")
	}
	for _, entry := range contRes.Mutated().Entries() {
		if entry.Kind == fabricengine.KindMergeCommitted {
			t.Errorf("MergeContinue() recorded %v; want no KindMergeCommitted — a merge commit is not a squash record's conclude", entry)
		}
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(fresh); err != nil || !exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (true, nil)", exists, err)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != handSHA {
		t.Errorf("warp HEAD after the refusal = %q; want the operator's own merge %q left untouched", got, handSHA)
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

// gitMergeHeadRaw reads dir's MERGE_HEAD file, reporting absence as ("", false) rather than an error,
// so a test can assert either presence or absence and compare the exact bytes afterwards.
// A linked worktree keeps MERGE_HEAD in its own gitdir, so the path is resolved via
// `git rev-parse --git-path MERGE_HEAD` rather than assumed to be <dir>/.git/MERGE_HEAD.
func gitMergeHeadRaw(t *testing.T, dir string) (string, bool) {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--git-path", "MERGE_HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse --git-path MERGE_HEAD in %s: %v", dir, err)
	}
	mergeHeadPath := strings.TrimSpace(string(out))
	if !filepath.IsAbs(mergeHeadPath) {
		mergeHeadPath = filepath.Join(dir, mergeHeadPath)
	}

	content, err := os.ReadFile(mergeHeadPath)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read %s: %v", mergeHeadPath, err)
	}
	return string(content), true
}

// gitUnmergedPaths lists dir's unmerged index entries, the same set gitrepo.ConflictedFiles reads.
func gitUnmergedPaths(t *testing.T, dir string) []string {
	t.Helper()

	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git diff --name-only --diff-filter=U in %s: %v", dir, err)
	}
	return strings.Fields(strings.TrimSpace(string(out)))
}

// foreignShape names one of the three distinguishable git-level merge states fabric did not start.
// They are not three spellings of one condition: each is seen by a different subset of
// foreignMergeStatePresent's probes, and only shapeConflictedIndexOnly and shapeMergeHeadOnly make
// any individual probe load-bearing.
type foreignShape int

const (
	// shapeConflictedAndMergeHead is an ordinary conflicted `git merge`: MERGE_HEAD live AND unmerged
	// index entries. Either probe alone sees it, which is why it cannot pin either one.
	shapeConflictedAndMergeHead foreignShape = iota
	// shapeMergeHeadOnly is a foreign merge resolved but not concluded: MERGE_HEAD live, unmerged set
	// EMPTY. Only the MERGE_HEAD probe sees it, and it is the most dangerous shape because nothing
	// about the worktree looks wrong.
	shapeMergeHeadOnly
	// shapeConflictedIndexOnly is a conflicted `git merge --squash`: unmerged index entries with NO
	// MERGE_HEAD, since --squash writes none. Only the conflicted-index probe sees it. Plain cherry-pick
	// and `checkout -m` conflicts land in this same shape.
	shapeConflictedIndexOnly
)

// TestMergeVerbs_ForeignMergeState_EverySideAndShapeRefuses is the foreign-state matrix, and it exists
// because the single warp-conflicted fixture that covered this before could not fail for three of the
// four probes foreignMergeStatePresent runs.
//
// Sabotage-measured, not assumed: deleting BOTH weft probes left the whole suite green, and so did
// deleting either warp probe on its own. The reason is that one conflicted plain-git merge in the warp
// checkout sets `warpMergeHead` and `warpConflicted` simultaneously, so either alone carried the
// assertion, and no fixture ever populated the weft side at all.
//
// The matrix is three shapes on each of the two sides. The shapes separate the two probe KINDS (see
// foreignShape). The SIDES matter for a different reason: the weft checkout is the hidden half an
// operator is told never to touch, so state appearing there is precisely the state nobody is watching
// for, and every helper on this path is per-side.
// Each row asserts its own shape with t.Fatal before asserting any refusal, so a fixture that silently
// stopped producing the state under test fails on its precondition instead of passing vacuously.
func TestMergeVerbs_ForeignMergeState_EverySideAndShapeRefuses(t *testing.T) {
	tests := []struct {
		name string
		// onWeft selects which checkout carries the foreign state.
		onWeft bool
		shape  foreignShape
	}{
		{name: "WarpConflictedIndexAndMergeHead", onWeft: false, shape: shapeConflictedAndMergeHead},
		{name: "WarpMergeHeadOnlyResolvedNotConcluded", onWeft: false, shape: shapeMergeHeadOnly},
		{name: "WarpConflictedIndexOnlyFromSquash", onWeft: false, shape: shapeConflictedIndexOnly},
		{name: "WeftConflictedIndexAndMergeHead", onWeft: true, shape: shapeConflictedAndMergeHead},
		{name: "WeftMergeHeadOnlyResolvedNotConcluded", onWeft: true, shape: shapeMergeHeadOnly},
		{name: "WeftConflictedIndexOnlyFromSquash", onWeft: true, shape: shapeConflictedIndexOnly},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, f, _, _, _, _ := newMergePairFixture(t, ".")

			dir := h.PrimeWorktree()
			branch, conflictPath := "other", "plain-conflict.txt"
			if tt.onWeft {
				dir = h.PrimeWeft()
				branch, conflictPath = "other-weft", "_lyx/plain-conflict.txt"
			}

			setupConflictingDivergence(t, dir, branch, conflictPath)
			if tt.shape == shapeConflictedIndexOnly {
				gitMergeSquashAllowConflict(t, dir, branch)
			} else {
				gitMergeAllowConflict(t, dir, branch)
			}
			if tt.shape == shapeMergeHeadOnly {
				gitkit.MustRun(t, dir, "git", "add", "--", conflictPath)
			}

			// Precondition: exactly the shape this row is named for, so the row really does rest on the
			// probe it is meant to pin.
			mergeHeadBefore, mergeHeadPresent := gitMergeHeadRaw(t, dir)
			unmergedBefore := gitUnmergedPaths(t, dir)
			wantMergeHead := tt.shape != shapeConflictedIndexOnly
			wantUnmerged := tt.shape != shapeMergeHeadOnly
			if mergeHeadPresent != wantMergeHead {
				t.Fatalf("MERGE_HEAD present in %s = %v; want %v for this shape", dir, mergeHeadPresent, wantMergeHead)
			}
			if (len(unmergedBefore) > 0) != wantUnmerged {
				t.Fatalf("unmerged entries in %s = %v; want non-empty = %v for this shape", dir, unmergedBefore, wantUnmerged)
			}
			statusBefore := gitkit.GitStatusPorcelain(t, dir)

			// A read-only probe reports fabric's own state, which foreign plain-git state does not make
			// true, on either side and in every shape.
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

			_, err := f.MergeIn("feature")
			assertForeign("MergeIn", err)
			_, err = f.Merge("feature", fabricengine.MergeOptions{})
			assertForeign("Merge", err)
			_, err = f.MergeContinue("")
			assertForeign("MergeContinue", err)
			_, err = f.MergeAbort()
			assertForeign("MergeAbort", err)
			_, err = f.MergeStageResolved([]string{conflictPath})
			assertForeign("MergeStageResolved", err)

			mergeHeadAfter, stillPresent := gitMergeHeadRaw(t, dir)
			if stillPresent != mergeHeadPresent || mergeHeadAfter != mergeHeadBefore {
				t.Errorf("MERGE_HEAD in %s: before = (%q, present %v), after = (%q, present %v); want untouched", dir, mergeHeadBefore, mergeHeadPresent, mergeHeadAfter, stillPresent)
			}
			if got := gitkit.GitStatusPorcelain(t, dir); got != statusBefore {
				t.Errorf("git status in %s changed: before = %q, after = %q; want untouched", dir, statusBefore, got)
			}
		})
	}
}

// gitMergeSquashAllowConflict runs a plain `git merge --squash ref` in dir, tolerating the nonzero
// exit a conflicted squash returns. The result is the conflicted-index-with-no-MERGE_HEAD shape:
// --squash deliberately writes no MERGE_HEAD, so this is the state only the conflicted-index probe
// can see.
func gitMergeSquashAllowConflict(t *testing.T, dir, ref string) {
	t.Helper()

	cmd := exec.Command("git", "merge", "--squash", ref)
	cmd.Dir = dir
	_ = cmd.Run()
}
