// mergein_recovery_integration_test.go covers MergeIn's crash recovery and its guards.
// It tests a fresh Fabric handle resuming a mid-merge record, both conflicted and crashed after clean staging,
// and a conclude-phase partial failure with its idempotent retry.
// It tests refusal of foreign git merge state without touching it.
// It tests the freshness rule's source-resolution outcomes and the fabric-managed guard.
// It tests that dirty-pair guard errors are byte-identical.
// It tests an unmappable weft-root conflict aborting itself.
// It tests that conflict markers never leak a "-weft" name, and path mapping on a subpath-anchored hub.
// It reuses newMergePairFixture and its sibling helpers from mergein_integration_test.go.
// Both files share package fabricengine_test.

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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// openFreshFabric opens a brand-new *fabricengine.Fabric handle over warpPath, independent of any
// handle a fixture already returned.
// Crash-recovery tests use it to model a fresh process resuming a merge.
func openFreshFabric(t *testing.T, warpPath string) *fabricengine.Fabric {
	t.Helper()

	l, err := lymcwd.ResolveWorktree(warpPath)
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s): %v", warpPath, err)
	}
	f, err := fabricengine.Open(l)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}
	return f
}

// installFailingPreCommitHook installs a pre-commit hook in repoDir's .git/hooks that always exits 1,
// so any commit attempted there fails.
func installFailingPreCommitHook(t *testing.T, repoDir string) {
	t.Helper()

	hookPath := filepath.Join(repoDir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write pre-commit hook at %s: %v", hookPath, err)
	}
}

// removePreCommitHook removes the pre-commit hook in repoDir, tolerating its absence.
func removePreCommitHook(t *testing.T, repoDir string) {
	t.Helper()

	hookPath := filepath.Join(repoDir, ".git", "hooks", "pre-commit")
	if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove pre-commit hook at %s: %v", hookPath, err)
	}
}

// gitMergeAllowConflict runs `git merge ref` in dir directly, bypassing fabric, and tolerates a non-zero exit.
// The non-zero exit is the plain-git conflict that foreign-merge-state tests need.
func gitMergeAllowConflict(t *testing.T, dir, ref string) {
	t.Helper()

	cmd := exec.Command("git", "merge", ref)
	cmd.Dir = dir
	_ = cmd.Run()
}

// pushDetachedContentAsRemoteBranch commits filename and content on a detached HEAD in dir.
// It then pushes that commit to origin as branch, so the branch exists only on the remote.
// Then it checks dir back out to the branch that was checked out before.
func pushDetachedContentAsRemoteBranch(t *testing.T, dir, branch, filename, content, msg string) {
	t.Helper()

	original := gitkit.CurrentBranch(t, dir)
	gitkit.MustRun(t, dir, "git", "checkout", "-q", "--detach", "HEAD")
	gitkit.CommitFile(t, dir, filename, content, msg)
	gitkit.MustRun(t, dir, "git", "push", "origin", "HEAD:refs/heads/"+branch)
	gitkit.MustRun(t, dir, "git", "checkout", "-q", original)
}

// TestMergeAbort_FreshHandle_RecoversConflictedRecord covers the crash-recovery case:
// a real conflicted MergeIn leaves its record on disk,
// and a brand-new Fabric handle over the same pair drives MergeAbort to an exact two-sided restore.
func TestMergeAbort_FreshHandle_RecoversConflictedRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergeLayerFixture(t, &conflictLayer)

	warpStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

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
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeRecords()); got != weftStartSHA {
		t.Errorf("weft HEAD after fresh-handle MergeAbort = %q; want %q", got, weftStartSHA)
	}
}

// TestMergeContinue_FreshHandle_RecoversCrashedAfterCleanStaging covers the crash after clean staging.
// Both sides are staged via the gitrepo.Repo.MergeStart seam, never a conflict.
// A record is saved by hand, and nothing is concluded.
// A fresh Fabric handle's MergeContinue then concludes both sides.
func TestMergeContinue_FreshHandle_RecoversCrashedAfterCleanStaging(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "warp-branch.txt", "warp-current.txt")
	setupCleanNonFastForward(t, h.PrimeRecords(), "feature-weft", "weft-branch.txt", "weft-current.txt")

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

// TestMergeContinue_ConcludeFailureThenRetryConcludes covers a conclude-phase failure on the warp side.
// The conclude-commit is sabotaged to fail, so MergeIn returns *ErrMergeIncomplete,
// and the record stays retained with no WarpCommitted recorded.
// Removing the sabotage and re-running MergeContinue then lands the conclude-commit.
func TestMergeContinue_ConcludeFailureThenRetryConcludes(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// The layer forks the branch off the pre-divergence HEAD and advances current separately.
	// The target is a genuine non-fast-forward merge, so it lands a real conclude commit.
	// A fast-forward would make concludeMergeSides skip the conclude commit.
	h, f, _, _, _, _ := newMergeLayerFixture(t, &nonFastForwardLayer)

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

// TestMergeVerbs_ForeignMergeState_RefuseWithoutTouching covers foreign git merge state:
// a plain-git conflicted merge staged directly in the warp checkout leaves MergeInProgress() false with no error.
// MergeIn, MergeContinue, and MergeAbort all refuse with *ErrForeignMergeState,
// and they leave the foreign state untouched (same MERGE_HEAD, same conflicted files).
func TestMergeVerbs_ForeignMergeState_RefuseWithoutTouching(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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
	tiergate.Require(t, 1)
	t.Parallel()

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
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "feature")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "push", "origin", "feature")
	gitkit.MustRun(t, h.PrimeRecords(), "git", "branch", "feature-weft")
	gitkit.MustRun(t, h.PrimeRecords(), "git", "push", "origin", "feature-weft")

	// Advance origin's copy of "feature" past the local branch, via a separate clone,
	// so the local "feature" ref itself never moves.
	remoteClone := t.TempDir()
	gitkit.MustRun(t, t.TempDir(), "git", "clone", h.CodeBare, remoteClone)
	gitkit.Git(t, remoteClone, "config", "user.email", "test@test.com")
	gitkit.Git(t, remoteClone, "config", "user.name", "Test")
	gitkit.MustRun(t, remoteClone, "git", "checkout", "feature")
	gitkit.CommitFile(t, remoteClone, "remote-tip.txt", "remote tip content\n", "advance origin/feature")
	gitkit.MustRun(t, remoteClone, "git", "push", "origin", "feature")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	// Both sides fast-forward onto the remote-tracking tip, so no conclude-commit is fabricated and
	// Committed is false.
	// The merge landing is asserted by the merged content below, not by the Committed flag.
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
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	pushDetachedContentAsRemoteBranch(t, h.PrimeWorktree(), "feature", "remote-only.txt", "remote only content\n", "warp: remote-only feature")
	pushDetachedContentAsRemoteBranch(t, h.PrimeRecords(), "feature-weft", "remote-only-weft.txt", "remote only weft content\n", "weft: remote-only feature")

	if gitkit.BranchExists(t, h.PrimeWorktree(), "feature") {
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

// TestMergeIn_Freshness_SourceResolvableNowhere covers a source resolvable on neither local nor remote.
// Such a source refuses with the fixed "source branch not found" reason.
// The weft counterpart gets a local branch, so the warp-side check is the only guard that can fire.
func TestMergeIn_Freshness_SourceResolvableNowhere(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	gitkit.MustRun(t, h.PrimeRecords(), "git", "branch", "feature-weft")

	_, err := f.MergeIn("feature")
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("MergeIn(feature) error = %v (%T); want *fabricengine.MergeGuardError", err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "source branch not found" {
		t.Errorf("MergeIn(feature) guard reasons = %v; want exactly [\"source branch not found\"]", guardErr.Reasons)
	}
}

// TestMergeIn_NoWeftCounterpart_NothingMutated covers a source branch with no "-weft" counterpart, locally or remotely.
// The merge resolves the weft counterpart's SHA best-effort;
// an unresolvable one leaves that field empty rather than blocking the merge.
// This fixture's "feature" branch is created at the current HEAD, so nothing mutates:
// the call reaches the AlreadyUpToDate case.
func TestMergeIn_NoWeftCounterpart_NothingMutated(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "feature")

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v; want nil — a source with no weft counterpart must no longer refuse", err)
	}
	if !res.AlreadyUpToDate {
		t.Errorf("MergeIn(feature).AlreadyUpToDate = false; want true — feature was branched at the current HEAD")
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpBefore {
		t.Errorf("warp HEAD changed to %q; want unchanged %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeRecords()); got != weftBefore {
		t.Errorf("weft HEAD changed to %q; want unchanged %q", got, weftBefore)
	}
}

// TestMergeIn_DirtyWarpRefuses covers a dirty warp-only pair.
// The merge refuses with the fixed "worktree dirty" reason.
func TestMergeIn_DirtyWarpRefuses(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "feature")
	gitkit.MustRun(t, h.PrimeRecords(), "git", "branch", "feature-weft")
	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "dirty.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "dirty.txt")

	_, err := f.MergeIn("feature")
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("warp-dirty MergeIn() error = %v (%T); want *fabricengine.MergeGuardError", err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "worktree dirty" {
		t.Errorf("warp-dirty guard reasons = %v; want exactly [\"worktree dirty\"]", guardErr.Reasons)
	}
	assertGuardDirt(t, guardErr, h.PrimeWorktree(), []string{"dirty.txt"})
}

// TestMergeIn_DirtyWeftDoesNotRefuse pins that a dirty weft does not block a warp-only merge.
// The weft is not a merge participant, so its dirtiness cannot affect the merge's correctness.
// This fixture's "feature" branch is created at the current HEAD;
// the call reaches the AlreadyUpToDate case rather than landing a merge commit.
func TestMergeIn_DirtyWeftDoesNotRefuse(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "feature")
	gitkit.MustRun(t, h.PrimeRecords(), "git", "branch", "feature-weft")
	if err := os.WriteFile(filepath.Join(h.PrimeRecords(), "dirty.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}
	gitkit.MustRun(t, h.PrimeRecords(), "git", "add", "dirty.txt")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("weft-dirty MergeIn() error = %v; want nil — a dirty weft must no longer refuse", err)
	}
	if !res.AlreadyUpToDate {
		t.Errorf("weft-dirty MergeIn().AlreadyUpToDate = false; want true — feature was branched at the current HEAD")
	}
	if out := gitkit.GitStatusPorcelain(t, h.PrimeRecords()); !strings.Contains(out, "dirty.txt") {
		t.Errorf("weft git status --porcelain after MergeIn = %q; want it to still mention dirty.txt — MergeIn never touches the weft", out)
	}
}

// The merge no longer reaches unifyConflictPaths' unmappable outcome, so no test in this file covers it.
// mergepaths_test.go covers that outcome at the unit level.

// TestMergeIn_ConflictMarkers_NeverLeakWeftName covers conflict-marker content on a warp-only conflict.
// The markers never show a "-weft"-suffixed branch name.
func TestMergeIn_ConflictMarkers_NeverLeakWeftName(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "marker-conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeRecords(), "feature-weft")
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

// A warp path is already worktree-relative, so unifyConflictPaths passes it through unchanged.
// The hub's subpath anchor does not change that.
// The weftPathVisible mapping is covered at the unit level by mergepaths_test.go.

// TestMergeContinue_InvisibleLandedConclude_AdoptsInsteadOfSticking covers an invisible landed conclude.
// The warp side's conclude commit has landed, but the record holds no SHA for it.
// A crash between `git commit` and the record re-save leaves this state behind.
// The test reproduces it by resolving a conflicted MergeIn and committing the warp side with plain git.
// The record still says committed:"" at that point.
// MergeContinue must adopt the landed commit off HEAD, creating no new commit.
// It reports Committed true, records a KindMergeCommitted carrying the adopted SHA, and deletes the record.
func TestMergeContinue_InvisibleLandedConclude_AdoptsInsteadOfSticking(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergeLayerFixture(t, &clashLayer)

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want the warp-side conflict", res.Conflicts)
	}

	// Resolve, then land the conclude with plain git.
	// The on-disk state is now byte-identical to a crash between concludeMergeSides' `git commit` and its record re-save.
	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "clash.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolved clash.txt: %v", err)
	}
	gitkit.Git(t, h.PrimeWorktree(), "add", "clash.txt")
	gitkit.Git(t, h.PrimeWorktree(), "commit", "--no-edit")

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	if st.WarpCommitted != "" {
		t.Fatalf("recorded warp conclude SHA = %q; want empty — the invisible shape", st.WarpCommitted)
	}

	// MergeAbort must refuse this state, leaving MergeContinue as the recovery.
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

// abortMergeAndLandUnrelatedCommit moves HEAD with no live MERGE_HEAD, the state a crashed conclude leaves.
// It discards the in-progress git merge with `git merge --abort`, then lands one commit of its own.
// The merge record stays live and still says this side's conclude never happened.
// MergeContinue must therefore not adopt that commit.
// It returns the unrelated SHA, so the caller can assert fabric never claims it.
func abortMergeAndLandUnrelatedCommit(t *testing.T, dir, filename string) string {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "merge", "--abort")
	gitkit.CommitFile(t, dir, filename, "nothing to do with the merge\n", "unrelated operator commit")
	return fabricengine.CurrentSHAForTest(t, dir)
}

// TestMergeContinue_UnrelatedCommitWhileRecordLive_IsNeverAdopted checks that an unrelated commit is never adopted.
// Adoption claims which commit a checkout carries, so it must rest on git's own parentage.
// Adopting on the ambiguous "HEAD moved, no live MERGE_HEAD" signal alone is a silent false success:
// it reports committed true, deletes the record, and leaves the merge source un-merged.
// The correct result is *ErrMergeIncomplete with the record kept, so the operator can see the merge state.
func TestMergeContinue_UnrelatedCommitWhileRecordLive_IsNeverAdopted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergeLayerFixture(t, &clashLayer)

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want the warp-side conflict — the scenario needs a real conclude pending", res.Conflicts)
	}

	sourceWarpSHA := gitkit.RevParse(t, h.PrimeWorktree(), "feature")
	unrelatedWarpSHA := abortMergeAndLandUnrelatedCommit(t, h.PrimeWorktree(), "warp-unrelated.txt")

	// The record must still be live with the warp side unconcluded, or the scenario this test names is absent.
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
	if gitkit.IsAncestor(t, h.PrimeWorktree(), sourceWarpSHA, unrelatedWarpSHA) {
		t.Fatalf("feature (%q) is an ancestor of the unrelated commit (%q); the fixture did not actually leave the source un-merged", sourceWarpSHA, unrelatedWarpSHA)
	}
}

// TestMergeContinue_MergeOfSourceOntoWrongBase_IsNeverAdopted checks the refusal of a wrong base.
// The operator aborts the recorded merge, lands a commit of their own, and merges the recorded source onto it.
// The result is a genuine two-parent merge of the right source on the wrong base.
// Adopting it would record correspondence against a base the paired side never saw.
func TestMergeContinue_MergeOfSourceOntoWrongBase_IsNeverAdopted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergeLayerFixture(t, &clashLayer)

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

	// Warp: abort the recorded merge, land an unrelated base commit, then merge the recorded source onto it.
	// The resolved merge has parents [unrelated base, recorded source].
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--abort")
	gitkit.CommitFile(t, h.PrimeWorktree(), "wrong-base.txt", "operator's own base\n", "unrelated base commit")
	wrongBaseSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	runGitExpectingConflict(t, h.PrimeWorktree(), "merge", st.WarpSource)
	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "clash.txt"), []byte("resolved on the wrong base\n"), 0o644); err != nil {
		t.Fatalf("write resolved clash.txt: %v", err)
	}
	gitkit.Git(t, h.PrimeWorktree(), "add", "clash.txt")
	gitkit.Git(t, h.PrimeWorktree(), "commit", "--no-edit")
	handSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	// The hand-landed commit must be a two-parent merge: its second parent is the recorded source;
	// its first parent is not the recorded start.
	// Only the first-parent check refuses this shape.
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

// TestMergeContinue_MergeOfWrongSourceOntoStart_IsNeverAdopted checks that a merge of the wrong source is refused.
// The merge is a genuine two-parent merge on the right base,
// but its merged-in tip is not the recorded source.
// Only the check that a parent is the recorded source refuses it;
// without that check, adoption would claim another branch's merge as this merge's conclude.
func TestMergeContinue_MergeOfWrongSourceOntoStart_IsNeverAdopted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergeLayerFixture(t, &clashLayer)

	// A decoy branch off the current warp HEAD, carrying one non-conflicting commit.
	commitOnBranch(t, h.PrimeWorktree(), "decoy", "decoy.txt", "nothing to do with feature\n", "decoy commit")
	decoySHA := gitkit.RevParse(t, h.PrimeWorktree(), "decoy")

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

	// Warp: abort back to the recorded start, then merge the decoy, giving parents [start, decoy].
	// The merge uses --no-ff: the decoy is one commit ahead of the start,
	// and a plain merge would fast-forward instead of creating the two-parent commit.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--abort")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-ff", "--no-edit", decoySHA)
	handSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	// Right base, wrong source: the second parent is not the recorded source.
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

// rootCommitForTest returns the oldest root commit of dir.
// A branch created there is outside the history of the recorded start;
// git therefore keeps the recorded start as a parent of an octopus merge instead of discarding it as redundant.
func rootCommitForTest(t *testing.T, dir string) string {
	t.Helper()

	lines := strings.Fields(gitkit.Git(t, dir, "rev-list", "--max-parents=0", "HEAD"))
	if len(lines) == 0 {
		t.Fatalf("git rev-list --max-parents=0 HEAD in %s returned nothing", dir)
	}
	return lines[len(lines)-1]
}

// TestMergeContinue_OctopusMergeCarryingTheSource_IsNeverAdopted checks that adoption requires exactly two parents.
// The operator discards the staged merge and then merges the recorded source with an unrelated branch.
// That is an ordinary action, and git builds the commit without complaint.
// Adopting that commit would report committed true, record correspondence, and delete the record.
// The checkout would then carry the unrelated branch's content, which no side of this merge brought in;
// no merge_staged entry accounts for it.
// Fabric never produces an octopus.
// It starts each non-squash merge with a single `git merge --ff --no-commit <sourceSHA>`,
// so its conclude has exactly two parents.
func TestMergeContinue_OctopusMergeCarryingTheSource_IsNeverAdopted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, st := setupWarpStagedPendingConclude(t)

	// The operator discards the staged merge and merges the recorded source alongside an unrelated
	// branch. The decoy is rooted outside the recorded start's history, or git would drop the start as
	// a redundant parent and build a two-parent commit instead of the octopus this test needs.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--abort")
	decoyBase := rootCommitForTest(t, h.PrimeWorktree())
	warpBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", "-b", "decoy", decoyBase)
	gitkit.CommitFile(t, h.PrimeWorktree(), "decoy.txt", "content nobody asked this merge for\n", "decoy: unrelated work")
	decoySHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", warpBranch)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-edit", st.WarpSource, decoySHA)
	octopusSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	// Precondition, asserted rather than assumed: a three-parent commit whose first parent is the
	// recorded start and whose second is the recorded source.
	// Every adoption clause holds for it except the exact-arity check, so only that check refuses it.
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

// setupWarpStagedPendingConclude builds the fixture several conclude-evidence tests share.
// The warp side merges cleanly: a real non-fast-forward merge, outcome staged, MERGE_HEAD live.
// No conclude has landed yet.
// The merge-state record on disk reflects that: WarpOutcome "staged" and WarpCommitted empty.
// WeftOutcome is "up_to_date", since the weft is not a merge participant.
// It builds this state directly through gitrepo.Repo.MergeStart and SaveMergeStateForTest.
// A MergeIn call cannot leave this window open.
// When the warp side merges cleanly, MergeIn concludes and deletes its record at once.
// It returns the hub and the manufactured record, after asserting the fixture has the described shape.
func setupWarpStagedPendingConclude(t *testing.T) (*hubforge.Hub, fabricengine.MergeStateForTest) {
	t.Helper()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "warp-branch.txt", "warp-current.txt")

	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())
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

// assertConcludeRefusedWithoutCommitting asserts the disposition the conclude-evidence tests expect.
// The conclude is refused with the aggregated guard error naming the recorded-merge-gone reason.
// No conclude commit is recorded or landed.
// The record stays on disk, and the operator's own warp state is left exactly as it was.
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

// TestMergeContinue_DifferentMergeLiveAtConcludeTime_IsNeverCommitted checks the commit side of a merge.
// The commit arm must refuse a MERGE_HEAD that is not this merge's.
//
// concludeMergeSides finishes a pending side by running `git commit`, which commits whatever MERGE_HEAD names.
// The adoption arm demands exact parentage before it claims a landed commit as this merge's conclude.
// The Fabric Git Invariant permits an operator to discard the staged warp merge with plain `git merge --abort`.
// The operator may then start an unrelated merge instead.
// Committing that merge would record it as this merge's conclude and pair it in the correspondence index.
// The record would then be erased, leaving a permanently non-corresponding pair.
// The commit arm must refuse it: the record stays on disk, and MergeAbort stays available.
//
// The adoption arm never sees this shape, because the operator did not commit.
// HEAD stays on the recorded start and MERGE_HEAD is live.
// So sideConcludeAlreadyLanded reports the side as not landed, and the commit runs.
func TestMergeContinue_DifferentMergeLiveAtConcludeTime_IsNeverCommitted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, st := setupWarpStagedPendingConclude(t)

	// The operator discards fabric's staged merge and starts an unrelated one, leaving it uncommitted.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--abort")
	warpBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", "-b", "other", st.WarpStart)
	gitkit.CommitFile(t, h.PrimeWorktree(), "other.txt", "work that has nothing to do with feature\n", "other: unrelated work")
	otherSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", warpBranch)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-commit", "--no-ff", otherSHA)

	// Preconditions, asserted rather than assumed: HEAD is back on the recorded start, so the adoption arm cannot fire,
	// and the live merge is a different one.
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

// TestMergeContinue_UncommittedOctopusCarryingTheSource_IsNeverCommitted covers this case:
// an uncommitted merge whose MERGE_HEAD holds two SHAs, the recorded source first.
// Evidence must be read from every SHA in MERGE_HEAD.
// Reading only the first, as `git rev-parse --verify --quiet MERGE_HEAD` does, would accept this merge:
// fabric would commit an octopus it could never produce itself.
// The decoy's content would land under this merge's name, brought in by no side and named by no merge_staged entry.
// The test asserts that the first entry is the source, so a truncating read fails here instead of passing.
func TestMergeContinue_UncommittedOctopusCarryingTheSource_IsNeverCommitted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, st := setupWarpStagedPendingConclude(t)

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--abort")
	decoyBase := rootCommitForTest(t, h.PrimeWorktree())
	warpBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", "-b", "decoy", decoyBase)
	gitkit.CommitFile(t, h.PrimeWorktree(), "decoy.txt", "content nobody asked this merge for\n", "decoy: unrelated work")
	decoySHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", warpBranch)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-commit", st.WarpSource, decoySHA)

	// Precondition, asserted rather than assumed: the first-head read reports exactly the recorded source.
	// Only reading every head tells this state apart from a legitimate one.
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

// TestMergeContinue_StagedContentWithNoLiveMergeAtConcludeTime_IsNeverCommitted covers a conclude:
// the merge is gone, but tracked content is staged in the index.
// A clean index with no live merge refuses by itself, because a plain `git commit` fails there.
// With content staged, `git commit --no-edit` succeeds and lands an ordinary one-parent commit of whatever was staged;
// the conclude then records that commit as this merge's conclude SHA and reports it as committed.
// The commit is then paired into the correspondence index, and the merge record is erased.
// The outcome is a silent false success, reached with no second merge: one `git merge --abort` and one `git add`.
func TestMergeContinue_StagedContentWithNoLiveMergeAtConcludeTime_IsNeverCommitted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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

// stagedContentPresentForTest reports whether dir's index differs from HEAD.
// It reads the answer through `git diff --cached --quiet`,
// so a fixture can assert its precondition without trusting the code under test.
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

// firstMergeHeadForTest returns what `git rev-parse --verify --quiet MERGE_HEAD` reports in dir.
// The answer keeps only the first SHA of a MERGE_HEAD that lists several,
// so a test can check that a fixture's first head is the source.
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

// runGitExpectingConflict runs git with args in dir;
// it tolerates the nonzero exit a conflicted merge returns and fails the test on any spawn-level error.
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

// commitParentsForTest reads sha's parent SHAs in dir via plain git, first parent first.
// It reads them without fabric, so the adoption tests check their fixture's parentage independently.
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

// TestMergeContinue_SquashConcludeLandedByHand_IsNeverAdopted runs the squash shape of the adoption question.
// `git merge --squash` writes no MERGE_HEAD, and its conclude is an ordinary one-parent commit.
// A non-squash conclude carries a first parent (the pre-merge start) and a second (the merge source).
// A squash conclude has no such parentage.
// Nothing tells a hand-landed squash conclude apart from any other commit.
// Adoption must therefore refuse rather than inherit the non-squash predicate.
// The refusal is not lossy: the record is retained and *ErrMergeIncomplete comes back.
// That is the behaviour from before an adoption arm existed.
func TestMergeContinue_SquashConcludeLandedByHand_IsNeverAdopted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergeLayerFixture(t, &nonFastForwardLayer)

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
	gitkit.Git(t, h.PrimeWorktree(), "commit", "-q", "-m", "hand-landed squash conclude")
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

// TestMergeContinue_SecondMergeStartedOverALandedConclude_LeavesNoLiveMergeHead tests one clause:
// the live-MERGE_HEAD clause of sideConcludeAlreadyLanded, which no other test reaches.
// The operator hand-lands this merge's conclude, so HEAD moves off the recorded start and MERGE_HEAD clears.
// That is the adoptable shape.
// Before MergeContinue runs, the operator starts a second merge in the same checkout, and it merges cleanly.
// The index carries no conflicts, so MergeContinue's own guard passes.
// Control then reaches the adoption probe with HEAD moved and no recorded conclude SHA,
// and a live MERGE_HEAD is present that fabric never started.
// Adopting there is the disaster shape.
// Fabric would report the merge concluded, record correspondence, delete its record, and leave a live MERGE_HEAD.
// No fabric verb can clear it, MergeAbort included, because the state reads as foreign without the record.
// That is the wedge TestMergeCrucible_EmptyResultMergeIsConcludedNotAbandoned guards against.
// The discriminating assertion is the same one:
// a verb that returns without error must never leave git-level merge state behind on either side.
func TestMergeContinue_SecondMergeStartedOverALandedConclude_LeavesNoLiveMergeHead(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, commitOnWarpBranch, _, _, _ := newMergePairFixture(t, ".")

	// The decoy is built first, off the pre-merge tip, so merging it later is a clean non-fast-forward.
	commitOnWarpBranch("decoy", "decoy.txt", "the operator's own second merge\n", "decoy: unrelated branch")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	branchAtCurrentHEAD(t, h.PrimeRecords(), "feature-weft")

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
	gitkit.Git(t, h.PrimeWorktree(), "add", "clash.txt")
	gitkit.Git(t, h.PrimeWorktree(), "commit", "-q", "--no-edit")
	concludeSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	// The operator now starts a second merge of their own and leaves it uncommitted.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-commit", "--no-ff", "decoy")

	// Preconditions, asserted rather than assumed: the hand-landed conclude has the adoptable shape,
	// and a live MERGE_HEAD from the operator's own second merge is present with a conflict-free index.
	// So nothing but the MERGE_HEAD clause can change the outcome.
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
	if mergeHeadPresentInCheckout(t, h.PrimeRecords()) {
		t.Error("weft checkout still carries a live MERGE_HEAD after MergeContinue returned without error")
	}
	if !gitkit.IsAncestor(t, h.PrimeWorktree(), st.WarpSource, fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())) {
		t.Errorf("recorded source %q is not an ancestor of warp HEAD; the merge did not actually land", st.WarpSource)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(fresh); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after MergeContinue = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMergeContinue_SquashRecordCarryingATwoParentMerge_IsNeverAdopted pins the squash refusal of MergeContinue.
// Only the squash clause refuses this shape:
// a side that carries a genuine two-parent merge of the recorded source on the recorded start.
// An operator reaches it by the documented route.
// A squash writes no MERGE_HEAD, so `git merge --abort` is unavailable;
// the way back is `git reset --hard <recorded start>`, followed by a plain `git merge <recorded source>`.
// That produces exactly the parentage a non-squash conclude carries.
// Adopting it would record a MERGE commit as a SQUASH record's conclude.
// It would then record correspondence for a pair whose two sides carry structurally different history:
// one squashed, one not.
// Refusing leaves the merge honestly stuck.
func TestMergeContinue_SquashRecordCarryingATwoParentMerge_IsNeverAdopted(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergeLayerFixture(t, &nonFastForwardLayer)

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

	// A squash writes no MERGE_HEAD, so `git merge --abort` is unavailable;
	// the side is reset to the recorded start and finished with a plain, non-squash merge.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "reset", "--hard", st.WarpStart)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-edit", st.WarpSource)
	handSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	// Precondition: every non-squash adoption clause is satisfied:
	// HEAD moved, no live MERGE_HEAD, a resolved source SHA on the record, and two parents in the right order.
	// Only `squash` stands between this commit and adoption.
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

// TestMergeContinue_BothSidesAlreadyUpToDate_DerivesAlreadyUpToDate pins the derivation of both result flags.
// MergeContinue derives both flags from the merge record.
// A resumed record whose two sides both recorded up_to_date must report already_up_to_date:true.
// The single-shot call reports the same.
// Such a record is reachable: the up-to-date probe in MergeIn and Merge runs before the write lock by design,
// so a call that loses that race records up_to_date on both sides,
// and a crash before its conclude leaves the same record behind.
// The fixture writes it directly to the same on-disk state that crash produces.
func TestMergeContinue_BothSidesAlreadyUpToDate_DerivesAlreadyUpToDate(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())
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

// gitUnmergedPaths lists dir's unmerged index entries: the paths git reports as conflicted.
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

// foreignShape names one of three git-level merge states that fabric did not start.
// Each state is seen by a different subset of the probes that detect a foreign merge:
// the MERGE_HEAD file and the unmerged index entries.
// The states shapeMergeHeadOnly and shapeConflictedIndexOnly are each seen by one probe alone,
// so they are what make each probe load-bearing.
type foreignShape int

const (
	// shapeConflictedAndMergeHead is an ordinary conflicted `git merge`: MERGE_HEAD live and unmerged index entries.
	// Either probe alone sees it, which is why it cannot pin either one.
	shapeConflictedAndMergeHead foreignShape = iota
	// shapeMergeHeadOnly is a foreign merge resolved but not concluded: MERGE_HEAD live, no unmerged entries.
	// Only the MERGE_HEAD probe sees it.
	// It is the most dangerous shape, because nothing about the worktree looks wrong.
	shapeMergeHeadOnly
	// shapeConflictedIndexOnly is a conflicted `git merge --squash`: unmerged index entries, no MERGE_HEAD.
	// --squash writes no MERGE_HEAD, so only the conflicted-index probe sees it.
	// Plain cherry-pick and `checkout -m` conflicts land in this same shape.
	shapeConflictedIndexOnly
)

// TestMergeVerbs_ForeignMergeState_EverySideAndShapeRefuses checks that every merge verb refuses foreign merge state.
// It does so on either checkout, in each of three shapes.
// The shapes separate the two probe kinds.
// The sides matter: the weft checkout is the hidden half an operator is told never to touch.
// Foreign state there is the state nobody watches for.
// Every helper on this path is per-side.
//
// One conflicted plain-git merge in the warp checkout sets `warpMergeHead` and `warpConflicted` together.
// So either warp probe alone passes the assertion, and no single-side fixture exercises the weft probes.
// Each row asserts its own shape with t.Fatal before asserting any refusal,
// so a fixture that stops producing the state under test fails on its precondition instead of passing vacuously.
func TestMergeVerbs_ForeignMergeState_EverySideAndShapeRefuses(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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
				dir = h.PrimeRecords()
				branch, conflictPath = "other-weft", "_lym/plain-conflict.txt"
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

			// Precondition: the checkout is in exactly this row's shape, so the row rests on the probe it pins.
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

			// A read-only probe reports fabric's own state.
			// Foreign plain-git state does not make that state true, on either side and in every shape.
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

// gitMergeSquashAllowConflict runs a plain `git merge --squash ref` in dir.
// It ignores the nonzero exit that a conflicted squash returns.
// The result is a conflicted index with no MERGE_HEAD, because --squash writes none.
// Only the conflicted-index probe sees that state.
func gitMergeSquashAllowConflict(t *testing.T, dir, ref string) {
	t.Helper()

	cmd := exec.Command("git", "merge", "--squash", ref)
	cmd.Dir = dir
	_ = cmd.Run()
}
