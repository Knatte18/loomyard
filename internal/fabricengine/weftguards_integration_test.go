//go:build integration

// weftguards_integration_test.go pins the weft-guards-drop batch's own regression surface: the four
// guard arms mergeguards.go and destroy.go dropped from the weft (dirty, detached, not-fabric-managed,
// and the abort-time reset), each paired with its surviving warp-side twin so the same test proves
// both halves of the claim — the weft lost the power to block a merge, the warp did not.
// Reuses newMergePairFixture and its sibling fixture helpers from mergein_integration_test.go and
// assertSoleGuardReason from mergecrucible_integration_test.go, since all three files share package
// fabricengine_test.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestWeftGuards_DirtyWeftDoesNotRefuseWarpDirtyStillDoes covers pairDirtyReason's narrowed guard: a
// weft worktree carrying uncommitted tracked changes no longer refuses a MergeIn the warp alone can
// complete, while a dirty warp worktree still refuses with mergeReasonWorktreeDirty.
func TestWeftGuards_DirtyWeftDoesNotRefuseWarpDirtyStillDoes(t *testing.T) {
	h1, f1, _, _, _, _ := newMergePairFixture(t, ".")
	setupCleanNonFastForward(t, h1.PrimeWorktree(), "feature", "feature.txt", "warp-progress.txt")
	branchAtCurrentHEAD(t, h1.PrimeWeft(), "feature-weft")
	if err := os.WriteFile(filepath.Join(h1.PrimeWorktree(), "dirty.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}
	gitkit.MustRun(t, h1.PrimeWorktree(), "git", "add", "dirty.txt")

	_, err := f1.MergeIn("feature")
	assertSoleGuardReason(t, "MergeIn(feature) [warp dirty]", err, "worktree dirty")

	h2, f2, _, _, _, _ := newMergePairFixture(t, ".")
	setupCleanNonFastForward(t, h2.PrimeWorktree(), "feature", "feature.txt", "warp-progress.txt")
	branchAtCurrentHEAD(t, h2.PrimeWeft(), "feature-weft")
	weftBefore2 := fabricengine.CurrentSHAForTest(t, h2.PrimeWeft())
	if err := os.WriteFile(filepath.Join(h2.PrimeWeft(), "dirty.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}
	gitkit.MustRun(t, h2.PrimeWeft(), "git", "add", "dirty.txt")

	res, err := f2.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) [weft dirty] error = %v; want nil — a dirty weft must no longer refuse a merge the warp alone can complete", err)
	}
	if !res.Committed {
		t.Errorf("MergeIn(feature) [weft dirty].Committed = false; want true — a real (non-fast-forward) merge")
	}
	if got := fabricengine.CurrentSHAForTest(t, h2.PrimeWeft()); got != weftBefore2 {
		t.Errorf("weft HEAD changed to %q; want unchanged %q — MergeIn never touches the weft", got, weftBefore2)
	}
	if out := gitkit.GitStatusPorcelain(t, h2.PrimeWeft()); out == "" {
		t.Error("weft git status --porcelain after MergeIn = clean; want it to still mention the uncommitted dirty.txt")
	}
}

// TestWeftGuards_DetachedWeftDoesNotRefuseWarpDetachedStillDoes covers detachedHeadReason's narrowed
// guard: a weft checkout on a detached HEAD no longer refuses, while a detached warp HEAD still
// refuses with mergeReasonDetachedHead.
func TestWeftGuards_DetachedWeftDoesNotRefuseWarpDetachedStillDoes(t *testing.T) {
	h1, f1, commitOnWarpBranch1, commitOnWeftBranch1, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch1("feature", "feature.txt", "feature\n", "feature: warp")
	commitOnWeftBranch1("feature-weft", "feature.txt", "feature\n", "feature: weft")
	gitkit.MustRun(t, h1.PrimeWorktree(), "git", "checkout", "-q", "--detach", "HEAD")

	_, err := f1.MergeIn("feature")
	assertSoleGuardReason(t, "MergeIn(feature) [warp detached]", err, "checkout is not on a branch")

	h2, f2, commitOnWarpBranch2, commitOnWeftBranch2, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch2("feature", "feature.txt", "feature\n", "feature: warp")
	commitOnWeftBranch2("feature-weft", "feature.txt", "feature\n", "feature: weft")
	gitkit.MustRun(t, h2.PrimeWeft(), "git", "checkout", "-q", "--detach", "HEAD")
	weftBefore2 := fabricengine.CurrentSHAForTest(t, h2.PrimeWeft())

	if _, err := f2.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) [weft detached] error = %v; want nil — a detached weft HEAD must no longer refuse a merge the warp alone can complete", err)
	}
	if got := fabricengine.CurrentSHAForTest(t, h2.PrimeWeft()); got != weftBefore2 {
		t.Errorf("weft HEAD changed to %q; want unchanged %q — MergeIn never touches the weft", got, weftBefore2)
	}
}

// TestWeftGuards_NoWeftCounterpartMergesSourceNotFoundStillWarpOnly covers resolveMergeSources'
// dropped refusal arm from two angles.
// First, a source branch whose weft counterpart exists on neither the local weft repo nor origin no
// longer refuses at all: the attempt proceeds (here, into a deliberately conflicted state, so the
// merge-state record survives long enough to inspect), leaving mergeState.WeftSource empty and
// appending neither mergeReasonNotFabricManaged nor mergeReasonSourceNotFound.
// Second, and separately, a source resolvable on NEITHER side still reports mergeReasonSourceNotFound
// alone, from the warp arm — the one reason resolveMergeSources can still produce.
func TestWeftGuards_NoWeftCounterpartMergesSourceNotFoundStillWarpOnly(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "conflict.txt")
	// Deliberately no "-weft" counterpart branch anywhere, locally or on origin.

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v; want nil — a source with no weft counterpart must no longer refuse", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want at least one — setupConflictingDivergence must produce a real warp-side conflict, which is what leaves the record in place to inspect", res.Conflicts)
	}

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (%+v, %v, %v); want (_, true, nil) — a conflicted MergeIn leaves its record in place", st, found, err)
	}
	if st.WeftSource != "" {
		t.Errorf("mergeState.WeftSource = %q; want empty — no weft counterpart exists to resolve", st.WeftSource)
	}

	if _, err := f.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort() cleanup error = %v", err)
	}

	_, err = f.MergeIn("does-not-exist-anywhere")
	assertSoleGuardReason(t, "MergeIn(does-not-exist-anywhere)", err, "source branch not found")
}

// TestWeftGuards_DirtyAndDetachedWeftTogetherStillMerges drives the narrowed guards IN COMBINATION,
// which the per-guard rows above deliberately do not.
// The distinction is not cosmetic: the guard stage aggregates every reason before it decides, so a
// single retained weft conjunct inside any one helper is invisible while the other weft states are
// happy — a per-guard row would keep passing and the merge would still refuse once a real weft went
// bad in more than one way at a time, which is the ordinary shape for a weft carrying a live loom
// run. Here the weft is simultaneously on a detached HEAD, carrying uncommitted tracked changes, and
// sitting on a branch with no upstream, while the warp side is clean.
func TestWeftGuards_DirtyAndDetachedWeftTogetherStillMerges(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "feature.txt", "warp-progress.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	gitkit.MustRun(t, h.PrimeWeft(), "git", "checkout", "-q", "--detach", "HEAD")
	if err := os.WriteFile(filepath.Join(h.PrimeWeft(), "dirty.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}
	gitkit.MustRun(t, h.PrimeWeft(), "git", "add", "dirty.txt")
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) [weft dirty AND detached] error = %v; want nil — combined weft state must not refuse a merge the warp alone can complete", err)
	}
	if !res.Committed {
		t.Errorf("MergeIn(feature) [weft dirty AND detached].Committed = false; want true — a real (non-fast-forward) merge")
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftBefore {
		t.Errorf("weft HEAD changed to %q; want unchanged %q — MergeIn never touches the weft", got, weftBefore)
	}
	if out := gitkit.GitStatusPorcelain(t, h.PrimeWeft()); out == "" {
		t.Error("weft git status --porcelain after MergeIn = clean; want the uncommitted dirty.txt still there — the merge must not have tidied the weft")
	}
}

// TestWeftGuards_EveryRecordThisBinaryWritesIsResumable pins the invariant that made the second,
// redundant saveMergeState call in MergeIn/Merge worth removing: every merge-state record this
// binary persists carries a non-empty WeftOutcome from its very first on-disk appearance, so
// mergeAttemptIncompleteReason — which refuses a MergeContinue resume on an empty outcome — can
// never fire on a record fabric wrote itself.
// It asserts the property at the one observable moment: a conflicted MergeIn leaves the record in
// place, and a MergeContinue over that record must refuse on the conflicts alone, never on
// mergeReasonAttemptIncomplete.
func TestWeftGuards_EveryRecordThisBinaryWritesIsResumable(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v; want nil with conflicts reported", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want at least one — the record only survives a conflicted attempt", res.Conflicts)
	}

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (%+v, %v, %v); want (_, true, nil)", st, found, err)
	}
	if st.WeftOutcome != "up_to_date" {
		t.Errorf("mergeState.WeftOutcome = %q; want %q — the weft is recorded as unmoved from the record's first write, never left empty", st.WeftOutcome, "up_to_date")
	}

	_, continueErr := f.MergeContinue("")
	if continueErr == nil {
		t.Fatal("MergeContinue() error = nil; want a guard refusal — the conflict is still unresolved")
	}
	assertSoleGuardReason(t, "MergeContinue() over an unresolved conflicted record", continueErr, "unresolved conflicts remain")

	if _, err := f.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort() cleanup error = %v", err)
	}
}

// TestWeftGuards_AbortLeavesWeftCommitsDuringAttemptWindowIntact covers resetMergeSides' dropped
// weft arm at MergeAbort: an attempt whose weft gains its own commits during the attempt window
// (the shape a per-transition status push produces, independent of any merge in progress on the warp
// side) is reset to WarpStart on the warp side alone, leaving the weft HEAD — and the commits it
// carries — exactly where MergeAbort found them.
func TestWeftGuards_AbortLeavesWeftCommitsDuringAttemptWindowIntact(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	warpStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	if _, err := f.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}

	// The weft gains its own commit during the attempt window — its own advance, independent of the
	// merge attempt in progress on the warp side.
	commitOnCurrentBranch(t, h.PrimeWeft(), "weft-progress.txt", "progress\n", "weft: progress during attempt")
	weftDuringAttempt := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	if _, err := f.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort() error = %v", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStartSHA {
		t.Errorf("warp HEAD after MergeAbort = %q; want restored pre-merge SHA %q", got, warpStartSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftDuringAttempt {
		t.Errorf("weft HEAD after MergeAbort = %q; want unchanged %q — the weft is not a reset target", got, weftDuringAttempt)
	}
	if _, err := os.Stat(filepath.Join(h.PrimeWeft(), "weft-progress.txt")); err != nil {
		t.Errorf("Stat(weft-progress.txt) after MergeAbort = %v; want present — the weft commit made during the attempt window must survive intact", err)
	}
}
