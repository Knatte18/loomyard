//go:build integration

// mergein_recovery_integration_test.go covers MergeIn's recovery, freshness, and illusion-integrity
// matrix against a real hubforge pair: crash recovery on a fresh Fabric handle, a conclude-phase
// partial failure and its idempotent retry, foreign merge-state disposition, the freshness rule,
// the fabric-managed guard, the dirty-pair guard, an unmappable-path self-abort, conflict-marker
// content, and path mapping on a subpath-anchored hub.

package fabricengine_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// gitRevParse resolves ref to a full SHA in dir, failing the test on error.
func gitRevParse(t *testing.T, dir, ref string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s in %s: %v", ref, dir, err)
	}
	return strings.TrimSpace(string(out))
}

// reopenFabric opens a fresh *fabricengine.Fabric handle on the same pair warpPath sits in — the
// "crashed process, new process picks the record back up" shape every recovery scenario in this
// file needs.
func reopenFabric(t *testing.T, warpPath string) *fabricengine.Fabric {
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

// installFailingPreCommitHook installs a pre-commit hook in weftDir's gitdir that always exits 1,
// so the next `git commit` there fails — the conclude-phase partial-failure fixture.
func installFailingPreCommitHook(t *testing.T, f *fabricengine.Fabric) {
	t.Helper()

	gitDir, err := fabricengine.WeftGitDirForTest(f)
	if err != nil {
		t.Fatalf("WeftGitDirForTest() error = %v", err)
	}
	hookPath := filepath.Join(gitDir, "hooks", "pre-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(pre-commit hook): %v", err)
	}
}

// removeFailingPreCommitHook removes the hook installFailingPreCommitHook installed.
func removeFailingPreCommitHook(t *testing.T, f *fabricengine.Fabric) {
	t.Helper()

	gitDir, err := fabricengine.WeftGitDirForTest(f)
	if err != nil {
		t.Fatalf("WeftGitDirForTest() error = %v", err)
	}
	if err := os.Remove(filepath.Join(gitDir, "hooks", "pre-commit")); err != nil {
		t.Fatalf("Remove(pre-commit hook): %v", err)
	}
}

// TestMergeAbort_CrashRecovery_FreshHandleRestoresBothSides drives a real conflicted MergeIn,
// opens a fresh Fabric handle on the same pair (simulating a crashed-and-restarted process), and
// asserts MergeAbort on that fresh handle restores both sides exactly.
func TestMergeAbort_CrashRecovery_FreshHandleRestoresBothSides(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	warpStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	if _, err := f.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}

	fresh := reopenFabric(t, h.PrimeWorktree())

	if _, err := fresh.MergeAbort(); err != nil {
		t.Fatalf("fresh handle MergeAbort() error = %v", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStartSHA {
		t.Errorf("warp HEAD after fresh-handle MergeAbort = %q; want restored pre-merge SHA %q", got, warpStartSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftStartSHA {
		t.Errorf("weft HEAD after fresh-handle MergeAbort = %q; want restored pre-merge SHA %q", got, weftStartSHA)
	}
	if exists, err := fresh.MergeInProgress(); err != nil || exists {
		t.Errorf("fresh handle MergeInProgress() after abort = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMergeContinue_CrashedAfterCleanStaging_FreshHandleConcludes builds a crashed-after-clean-
// staging state by hand — driving MergeStart directly on both sides and saving the record manually
// — then asserts MergeContinue on a fresh Fabric handle concludes both.
func TestMergeContinue_CrashedAfterCleanStaging_FreshHandleConcludes(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "warp-branch.txt", "warp-current.txt")
	setupCleanNonFastForward(t, h.PrimeWeft(), "feature-weft", "weft-branch.txt", "weft-current.txt")

	warpSourceSHA := gitRevParse(t, h.PrimeWorktree(), "feature")
	weftSourceSHA := gitRevParse(t, h.PrimeWeft(), "feature-weft")

	// setupCleanNonFastForward guarantees a genuine (non-fast-forward, non-conflicting) merge on
	// both sides, so both outcomes are deterministically "staged" — the on-disk mergeState outcome
	// string mirrors mergestate.go's own mergeOutcomeString mapping for gitrepo.MergeStaged.
	if _, err := fabricengine.WarpForTest(f).MergeStart(warpSourceSHA, false); err != nil {
		t.Fatalf("warp MergeStart(%s) error = %v", warpSourceSHA, err)
	}
	if _, err := fabricengine.WeftForTest(f).MergeStart(weftSourceSHA, false); err != nil {
		t.Fatalf("weft MergeStart(%s) error = %v", weftSourceSHA, err)
	}

	if err := fabricengine.SaveMergeStateForTest(f, fabricengine.MergeStateForTest{
		Verb:        "merge-in",
		Source:      "feature",
		WarpStart:   warpStart,
		WeftStart:   weftStart,
		WarpOutcome: "staged",
		WeftOutcome: "staged",
	}); err != nil {
		t.Fatalf("SaveMergeStateForTest() error = %v", err)
	}

	fresh := reopenFabric(t, h.PrimeWorktree())

	res, err := fresh.MergeContinue("")
	if err != nil {
		t.Fatalf("fresh handle MergeContinue(\"\") error = %v", err)
	}
	if !res.Committed {
		t.Errorf("fresh handle MergeContinue(\"\").Committed = false; want true")
	}
	if exists, err := fresh.MergeInProgress(); err != nil || exists {
		t.Errorf("fresh handle MergeInProgress() after MergeContinue = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMergeContinue_ConcludePartialFailure_RetryLandsRemainingSideOnly forces the weft side's
// conclude-commit to fail via a pre-commit hook, asserting *ErrMergeIncomplete, that the record is
// retained with warp's landed SHA recorded, and that removing the sabotage and retrying concludes
// only the remaining (weft) side — idempotency pinned by SHA comparison on the already-landed warp
// side.
func TestMergeContinue_ConcludePartialFailure_RetryLandsRemainingSideOnly(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "warp-branch.txt", "warp-current.txt")
	setupCleanNonFastForward(t, h.PrimeWeft(), "feature-weft", "weft-branch.txt", "weft-current.txt")

	installFailingPreCommitHook(t, f)

	_, err := f.MergeIn("feature")
	var incomplete *fabricengine.ErrMergeIncomplete
	if !errors.As(err, &incomplete) {
		t.Fatalf("MergeIn(feature) error = %v (%T); want *ErrMergeIncomplete", err, err)
	}
	if err.Error() != "fabricengine: merge conclude did not finish; run MergeContinue again" {
		t.Errorf("MergeIn(feature) error text = %q; want the fixed ErrMergeIncomplete message", err.Error())
	}

	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || !exists {
		t.Fatalf("MergeRecordExistsForTest() after partial conclude failure = (%v, %v); want (true, nil)", exists, err)
	}

	loaded, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() error = (%v, found=%v)", err, found)
	}
	if loaded.WarpCommitted == "" {
		t.Error("record's WarpCommitted is empty; want the landed warp conclude SHA recorded")
	}
	if loaded.WeftCommitted != "" {
		t.Error("record's WeftCommitted is non-empty; want empty since the weft conclude failed")
	}
	warpLandedSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	if loaded.WarpCommitted != warpLandedSHA {
		t.Errorf("record's WarpCommitted = %q; want the actual warp HEAD %q", loaded.WarpCommitted, warpLandedSHA)
	}

	removeFailingPreCommitHook(t, f)

	continued, err := f.MergeContinue("")
	if err != nil {
		t.Fatalf("MergeContinue() after removing sabotage error = %v", err)
	}
	if !continued.Committed {
		t.Errorf("MergeContinue().Committed = false; want true")
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpLandedSHA {
		t.Errorf("warp HEAD after retry = %q; want unchanged %q (idempotent: warp side already landed)", got, warpLandedSHA)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after successful retry = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMerge_ForeignMergeState_AllFourMutatingVerbsRefuse covers a plain-git conflicted merge staged
// directly in the warp checkout: MergeInProgress reports false with no error, and MergeIn,
// MergeContinue, and MergeAbort all refuse with *ErrForeignMergeState, leaving the foreign state
// untouched.
func TestMerge_ForeignMergeState_AllFourMutatingVerbsRefuse(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	driveConflictedMergeStart(t, h.PrimeWorktree(), fabricengine.WarpForTest(f))

	if inProgress, err := f.MergeInProgress(); err != nil || inProgress {
		t.Errorf("MergeInProgress() with foreign state = (%v, %v); want (false, nil)", inProgress, err)
	}

	beforeConflicts, err := fabricengine.WarpForTest(f).ConflictedFiles()
	if err != nil {
		t.Fatalf("ConflictedFiles() before refusals error = %v", err)
	}

	var foreignErr *fabricengine.ErrForeignMergeState
	if _, err := f.MergeIn("feature"); !errors.As(err, &foreignErr) {
		t.Errorf("MergeIn(feature) error = %v (%T); want *ErrForeignMergeState", err, err)
	}
	if _, err := f.MergeContinue(""); !errors.As(err, &foreignErr) {
		t.Errorf("MergeContinue(\"\") error = %v (%T); want *ErrForeignMergeState", err, err)
	}
	if _, err := f.MergeAbort(); !errors.As(err, &foreignErr) {
		t.Errorf("MergeAbort() error = %v (%T); want *ErrForeignMergeState", err, err)
	}

	if present, err := fabricengine.WarpForTest(f).MergeHeadPresent(); err != nil || !present {
		t.Errorf("MergeHeadPresent() after refusals = (%v, %v); want (true, nil) (foreign state left untouched)", present, err)
	}
	afterConflicts, err := fabricengine.WarpForTest(f).ConflictedFiles()
	if err != nil {
		t.Fatalf("ConflictedFiles() after refusals error = %v", err)
	}
	if !reflect.DeepEqual(beforeConflicts, afterConflicts) {
		t.Errorf("ConflictedFiles() before/after refusals = %v / %v; want identical (foreign state untouched)", beforeConflicts, afterConflicts)
	}
}

// TestMergeContinue_MergeAbort_NeitherRecordNorForeignState_ErrNoMergeInProgress covers the
// genuinely-nothing-in-progress case: with neither a fabric record nor foreign git merge state,
// MergeContinue and MergeAbort both return *ErrNoMergeInProgress — pinning that this and
// *ErrForeignMergeState are not interchangeable.
func TestMergeContinue_MergeAbort_NeitherRecordNorForeignState_ErrNoMergeInProgress(t *testing.T) {
	_, f, _, _, _, _ := newMergePairFixture(t, ".")

	var noMerge *fabricengine.ErrNoMergeInProgress
	if _, err := f.MergeContinue(""); !errors.As(err, &noMerge) {
		t.Errorf("MergeContinue(\"\") error = %v (%T); want *ErrNoMergeInProgress", err, err)
	}
	if _, err := f.MergeAbort(); !errors.As(err, &noMerge) {
		t.Errorf("MergeAbort() error = %v (%T); want *ErrNoMergeInProgress", err, err)
	}
}

// TestMergeIn_Freshness_LocalBehindRemote_MergesRemoteTrackingSHA covers the freshness rule: a local
// source branch behind its remote-tracking ref merges the remote-tracking SHA, asserted via the
// merged content actually landing.
func TestMergeIn_Freshness_LocalBehindRemote_MergesRemoteTrackingSHA(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")

	commitOnWarpBranch("feature", "freshness.txt", "v1\n", "warp: feature v1")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "push", "origin", "feature")
	commitOnWeftBranch("feature-weft", "weft-marker.txt", "weft marker\n", "weft: add feature-weft")

	// Advance origin's feature ref independently of the local checkout, via a throwaway clone of the
	// warp bare remote — the local "feature" branch stays at v1.
	scratch := t.TempDir()
	gitkit.MustRun(t, scratch, "git", "clone", "-q", h.WarpBare, ".")
	gitkit.MustRun(t, scratch, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, scratch, "git", "config", "user.name", "Test")
	gitkit.MustRun(t, scratch, "git", "checkout", "-q", "feature")
	if err := os.WriteFile(filepath.Join(scratch, "freshness.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, scratch, "git", "add", "freshness.txt")
	gitkit.MustRun(t, scratch, "git", "commit", "-q", "-m", "advance feature to v2")
	gitkit.MustRun(t, scratch, "git", "push", "origin", "feature")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if !res.Committed {
		t.Fatalf("MergeIn(feature).Committed = false; want true")
	}

	got, err := os.ReadFile(filepath.Join(h.PrimeWorktree(), "freshness.txt"))
	if err != nil {
		t.Fatalf("ReadFile(freshness.txt): %v", err)
	}
	if string(got) != "v2\n" {
		t.Errorf("freshness.txt content after merge = %q; want %q (the remote-tracking tip, not the stale local branch)", got, "v2\n")
	}
}

// TestMergeIn_Freshness_SourceOnlyRemote_Merges covers the freshness rule's other half: a source
// branch existing only on origin (never checked out locally) still resolves and merges.
func TestMergeIn_Freshness_SourceOnlyRemote_Merges(t *testing.T) {
	h, f, _, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")

	commitOnWeftBranch("remote-only-weft", "weft-marker.txt", "weft marker\n", "weft: add remote-only-weft")

	scratch := t.TempDir()
	gitkit.MustRun(t, scratch, "git", "clone", "-q", h.WarpBare, ".")
	gitkit.MustRun(t, scratch, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, scratch, "git", "config", "user.name", "Test")
	gitkit.MustRun(t, scratch, "git", "checkout", "-q", "-b", "remote-only")
	if err := os.WriteFile(filepath.Join(scratch, "remote-only.txt"), []byte("remote content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, scratch, "git", "add", "remote-only.txt")
	gitkit.MustRun(t, scratch, "git", "commit", "-q", "-m", "remote-only branch")
	gitkit.MustRun(t, scratch, "git", "push", "origin", "remote-only")

	res, err := f.MergeIn("remote-only")
	if err != nil {
		t.Fatalf("MergeIn(remote-only) error = %v", err)
	}
	if !res.Committed {
		t.Fatalf("MergeIn(remote-only).Committed = false; want true")
	}
	if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), "remote-only.txt")); err != nil {
		t.Errorf("remote-only.txt missing after merge: %v", err)
	}
}

// TestMergeIn_Freshness_SourceResolvableNowhere_GuardError covers a source resolvable on neither
// local nor remote: *MergeGuardError with the fixed "source branch not found" reason, isolated by
// giving the weft counterpart a legitimate fabric-managed presence.
func TestMergeIn_Freshness_SourceResolvableNowhere_GuardError(t *testing.T) {
	_, f, _, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")

	commitOnWeftBranch("ghost-weft", "weft-marker.txt", "weft marker\n", "weft: add ghost-weft")

	_, err := f.MergeIn("ghost")
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("MergeIn(ghost) error = %v (%T); want *MergeGuardError", err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "source branch not found" {
		t.Errorf("MergeIn(ghost) guard reasons = %v; want exactly [\"source branch not found\"]", guardErr.Reasons)
	}
}

// TestMergeIn_NotFabricManaged_GuardError covers a source with no fabric weft counterpart (absent
// both locally and remotely): *MergeGuardError with the fixed "source branch is not fabric-managed"
// reason, and neither side's HEAD moves.
func TestMergeIn_NotFabricManaged_GuardError(t *testing.T) {
	h, f, commitOnWarpBranch, _, _, _ := newMergePairFixture(t, ".")

	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	commitOnWarpBranch("orphan", "orphan.txt", "orphan content\n", "warp: add orphan")

	_, err := f.MergeIn("orphan")
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("MergeIn(orphan) error = %v (%T); want *MergeGuardError", err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "source branch is not fabric-managed" {
		t.Errorf("MergeIn(orphan) guard reasons = %v; want exactly [\"source branch is not fabric-managed\"]", guardErr.Reasons)
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStart {
		t.Errorf("warp HEAD after refused MergeIn = %q; want unchanged %q", got, warpStart)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftStart {
		t.Errorf("weft HEAD after refused MergeIn = %q; want unchanged %q", got, weftStart)
	}
}

// TestMergeIn_DirtyPair_GuardError_ByteIdenticalRegardlessOfSide covers the dirty-pair guard: a
// dirty-warp-only pair and a dirty-weft-only pair both refuse with *MergeGuardError carrying
// exactly "worktree dirty", and the two error values are byte-identical (never revealing which side
// was dirty).
func TestMergeIn_DirtyPair_GuardError_ByteIdenticalRegardlessOfSide(t *testing.T) {
	hWarpDirty, fWarpDirty, commitOnWarpBranch1, commitOnWeftBranch1, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch1("feature", "clean-warp.txt", "clean\n", "warp: clean branch")
	commitOnWeftBranch1("feature-weft", "clean-weft.txt", "clean\n", "weft: clean branch")
	if err := os.WriteFile(filepath.Join(hWarpDirty.PrimeWorktree(), "README"), []byte("dirtied\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README): %v", err)
	}
	_, errWarpDirty := fWarpDirty.MergeIn("feature")

	hWeftDirty, fWeftDirty, commitOnWarpBranch2, commitOnWeftBranch2, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch2("feature", "clean-warp.txt", "clean\n", "warp: clean branch")
	commitOnWeftBranch2("feature-weft", "clean-weft.txt", "clean\n", "weft: clean branch")
	// The weft primary's own bootstrap commit is empty (no tracked files at all — only the untracked
	// _lyx/ junction target), so a tracked file must be committed on the current weft branch first,
	// before dirtying it, for scopeTracked's dirtiness probe to see a real modification.
	weftTrackedPath := filepath.Join(hWeftDirty.PrimeWeft(), "weft-tracked.txt")
	commitOnCurrentBranch(t, hWeftDirty.PrimeWeft(), "weft-tracked.txt", "clean\n", "weft: seed tracked file")
	if err := os.WriteFile(weftTrackedPath, []byte("dirtied\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(weft tracked file): %v", err)
	}
	_, errWeftDirty := fWeftDirty.MergeIn("feature")

	var guardWarp, guardWeft *fabricengine.MergeGuardError
	if !errors.As(errWarpDirty, &guardWarp) {
		t.Fatalf("dirty-warp-only MergeIn error = %v (%T); want *MergeGuardError", errWarpDirty, errWarpDirty)
	}
	if !errors.As(errWeftDirty, &guardWeft) {
		t.Fatalf("dirty-weft-only MergeIn error = %v (%T); want *MergeGuardError", errWeftDirty, errWeftDirty)
	}
	if len(guardWarp.Reasons) != 1 || guardWarp.Reasons[0] != "worktree dirty" {
		t.Errorf("dirty-warp-only guard reasons = %v; want exactly [\"worktree dirty\"]", guardWarp.Reasons)
	}
	if !reflect.DeepEqual(guardWarp, guardWeft) {
		t.Errorf("dirty-warp-only and dirty-weft-only guard errors differ: %+v vs %+v; want byte-identical", guardWarp, guardWeft)
	}
}

// TestMergeIn_UnmappableWeftConflict_SelfAbortsBothSides covers an unmappable-path conflict: a
// weft-side conflict on a repo-root file outside the wired name set self-aborts both sides,
// returning *ErrUnmergeableState with the pair restored to its pre-merge SHAs and no record left.
func TestMergeIn_UnmappableWeftConflict_SelfAbortsBothSides(t *testing.T) {
	h, f, commitOnWarpBranch, _, _, _ := newMergePairFixture(t, ".")

	commitOnWarpBranch("feature", "clean-warp.txt", "clean\n", "warp: clean branch")
	setupConflictingDivergence(t, h.PrimeWeft(), "feature-weft", "unmapped-root-file.txt")

	// Captured after the divergence above lands, since those are the actual pre-merge SHAs MergeIn's
	// own record captures and the self-abort reset must restore.
	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	_, err := f.MergeIn("feature")
	var unmergeable *fabricengine.ErrUnmergeableState
	if !errors.As(err, &unmergeable) {
		t.Fatalf("MergeIn(feature) error = %v (%T); want *ErrUnmergeableState", err, err)
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStart {
		t.Errorf("warp HEAD after unmappable self-abort = %q; want restored pre-merge SHA %q", got, warpStart)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftStart {
		t.Errorf("weft HEAD after unmappable self-abort = %q; want restored pre-merge SHA %q", got, weftStart)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after unmappable self-abort = (%v, %v); want (false, nil)", exists, err)
	}
}

// assertConflictMarkers asserts path's content carries git's three-way conflict markers with a
// >>>>>>> label naming mergedSHA, and never mentions a "-weft"-suffixed branch name — conflict
// markers name a raw SHA, never a branch (merges-name-a-sha-never-a-branch), so a weft-only
// conflict's markers are indistinguishable from a warp-only conflict's.
func assertConflictMarkers(t *testing.T, path, mergedSHA string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	content := string(data)

	if !strings.Contains(content, "<<<<<<<") || !strings.Contains(content, "=======") {
		t.Errorf("conflict markers missing from %s: %q", path, content)
	}
	if !strings.Contains(content, ">>>>>>> "+mergedSHA) {
		t.Errorf("%s does not contain '>>>>>>> %s'; content = %q", path, mergedSHA, content)
	}
	if strings.Contains(content, "-weft") {
		t.Errorf("%s conflict markers mention a \"-weft\"-suffixed name; want a raw SHA only: %q", path, content)
	}
}

// TestMergeIn_ConflictMarkerContent_WeftAndWarpAreIndistinguishable covers conflict-marker content:
// a weft-only conflict's markers name the merged source SHA and never a "-weft"-suffixed branch
// name, and the same marker style appears on a warp-only conflict, so the two are indistinguishable.
func TestMergeIn_ConflictMarkerContent_WeftAndWarpAreIndistinguishable(t *testing.T) {
	hWeft, fWeft, commitOnWarpBranch1, _, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch1("feature", "clean-warp.txt", "clean\n", "warp: clean branch")
	setupConflictingDivergence(t, hWeft.PrimeWeft(), "feature-weft", "_lyx/conflict-marker-test.txt")
	weftMergedSHA := gitRevParse(t, hWeft.PrimeWeft(), "feature-weft")

	if _, err := fWeft.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) [weft conflict] error = %v", err)
	}
	assertConflictMarkers(t, filepath.Join(hWeft.PrimeWorktree(), "_lyx", "conflict-marker-test.txt"), weftMergedSHA)

	hWarp, fWarp, _, commitOnWeftBranch2, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, hWarp.PrimeWorktree(), "feature", "conflict.txt")
	commitOnWeftBranch2("feature-weft", "clean-weft.txt", "clean\n", "weft: clean branch")
	warpMergedSHA := gitRevParse(t, hWarp.PrimeWorktree(), "feature")

	if _, err := fWarp.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) [warp conflict] error = %v", err)
	}
	assertConflictMarkers(t, filepath.Join(hWarp.PrimeWorktree(), "conflict.txt"), warpMergedSHA)
}

// TestMergeIn_SubpathAnchoredHub_ConflictPathMapping covers path mapping on a subpath-anchored hub:
// a conflict in a junctioned weft path is reported at its unified worktree-root-relative path
// (backend/<name>/...), and the reported file is reachable at that path through the junction from
// the visible worktree root.
func TestMergeIn_SubpathAnchoredHub_ConflictPathMapping(t *testing.T) {
	h, f, commitOnWarpBranch, _, _, _ := newMergePairFixture(t, "backend")

	commitOnWarpBranch("feature", "backend/clean-warp.txt", "clean\n", "warp: clean branch")
	setupConflictingDivergence(t, h.PrimeWeft(), "feature-weft", "backend/_lyx/subpath-conflict.txt")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	wantPath := "backend/_lyx/subpath-conflict.txt"
	found := false
	for _, p := range res.Conflicts {
		if p == wantPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want it to contain %q", res.Conflicts, wantPath)
	}

	visiblePath := filepath.Join(h.PrimeWorktree(), filepath.FromSlash(wantPath))
	if _, err := os.Stat(visiblePath); err != nil {
		t.Errorf("os.Stat(%s) = %v; want the reported conflict path reachable through the junction from the visible worktree root", visiblePath, err)
	}
}
