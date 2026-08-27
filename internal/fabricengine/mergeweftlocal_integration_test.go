//go:build integration

// mergeweftlocal_integration_test.go pins the warp-only merge against a real hubforge pair: the weft
// is never a merge participant in either verb or either direction, so a weft counterpart that
// rewrites system-file content, diverges independently, or evolves the same _lyx/ path from a shared
// base can no longer produce a merge conflict, a self-abort, or a moved weft HEAD — only a genuine
// warp-side conflict still reaches unifyConflictPaths.
// Reuses newMergeTargetFixture, seedSourceAndTarget, newMergePairFixture, commitOnCurrentBranch,
// advanceRemoteBranch, openFreshFabric, gitRevParse and fabricengine.CurrentSHAForTest from the
// package's existing test files rather than adding new fixture helpers.

package fabricengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestMergeWeftLocal_TargetWeftRewritesStatusManyTimes_WarpAdvancesWeftUnchanged covers scenario 1: a
// Merge of a source branch whose weft counterpart rewrote _lyx/loom/status.json many times. The
// target pair's warp HEAD advances, the target pair's weft HEAD is byte-identical before and after,
// and MergeResult.Conflicts is empty.
// It additionally asserts MergeResult.AlreadyUpToDate is false while the warp genuinely moved — the
// alternative way (per the plan) of pinning that the merge record's WeftOutcome was written as
// up_to_date, since the record itself is deleted by the time a successful call returns.
func TestMergeWeftLocal_TargetWeftRewritesStatusManyTimes_WarpAdvancesWeftUnchanged(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	if err := os.MkdirAll(filepath.Join(h.PrimeWeft(), "_lyx", "loom"), 0o755); err != nil {
		t.Fatalf("MkdirAll(_lyx/loom): %v", err)
	}
	// The weft counterpart rewrites the same system file many times on the source branch.
	for i := range 5 {
		commitOnSourceWeft("feature-weft", "_lyx/loom/status.json", fmt.Sprintf(`{"n":%d}`, i), fmt.Sprintf("weft: status rewrite %d", i))
	}

	targetWarpPath, targetWeftPath := h.PairWarpWorktree("target"), h.PairWeftSibling("target")
	warpBefore := fabricengine.CurrentSHAForTest(t, targetWarpPath)
	weftBefore := fabricengine.CurrentSHAForTest(t, targetWeftPath)

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	if err != nil {
		t.Fatalf("Merge(feature) error = %v", err)
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("Merge(feature).Conflicts = %v; want empty", res.Conflicts)
	}
	if res.AlreadyUpToDate {
		t.Error("Merge(feature).AlreadyUpToDate = true; want false — the warp side genuinely moved")
	}

	if got := fabricengine.CurrentSHAForTest(t, targetWarpPath); got == warpBefore {
		t.Errorf("target warp HEAD = %q, unchanged; want it to have advanced", got)
	}
	if got := fabricengine.CurrentSHAForTest(t, targetWeftPath); got != weftBefore {
		t.Errorf("target weft HEAD = %q; want byte-identical %q — the weft is not a merge participant", got, weftBefore)
	}
}

// TestMergeWeftLocal_TargetWeftDivergedStatus_ContentUnchanged covers scenario 2: the same shape as
// above, but the target pair carries its own diverged _lyx/loom/status.json. The target's weft file
// content is unchanged after the merge — proving the target's own divergence is never reconciled
// against the source's, since there is no merge between them to reconcile it.
func TestMergeWeftLocal_TargetWeftDivergedStatus_ContentUnchanged(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)
	if err := os.MkdirAll(filepath.Join(h.PrimeWeft(), "_lyx", "loom"), 0o755); err != nil {
		t.Fatalf("MkdirAll(_lyx/loom) on source weft: %v", err)
	}
	commitOnSourceWeft("feature-weft", "_lyx/loom/status.json", `{"source":true}`, "feature: weft status")

	targetWeftPath := h.PairWeftSibling("target")
	statusRel := filepath.Join("_lyx", "loom", "status.json")
	if err := os.MkdirAll(filepath.Join(targetWeftPath, "_lyx", "loom"), 0o755); err != nil {
		t.Fatalf("MkdirAll(_lyx/loom) on target weft: %v", err)
	}
	commitOnCurrentBranch(t, targetWeftPath, filepath.ToSlash(statusRel), `{"target":true}`, "target: weft status diverge")

	wantContent, err := os.ReadFile(filepath.Join(targetWeftPath, statusRel))
	if err != nil {
		t.Fatalf("read target's own diverged status.json: %v", err)
	}

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	if err != nil {
		t.Fatalf("Merge(feature) error = %v", err)
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("Merge(feature).Conflicts = %v; want empty — the diverged content lives on a side the merge never touches", res.Conflicts)
	}

	gotContent, err := os.ReadFile(filepath.Join(targetWeftPath, statusRel))
	if err != nil {
		t.Fatalf("read status.json after merge: %v", err)
	}
	if string(gotContent) != string(wantContent) {
		t.Errorf("target's status.json after merge = %q; want unchanged %q", gotContent, wantContent)
	}
}

// TestMergeWeftLocal_BothSidesEvolveLyxFromSharedBase_NowCompletes covers scenario 3: both the
// target's own weft and the source's weft counterpart evolve the same _lyx/ path independently from
// a shared base — the shape that returned *ErrMergeInRequired before this task, since the weft side
// would conflict. It must now complete and report Committed true, since the weft is not a merge
// participant and there is nothing left on that side to conflict.
func TestMergeWeftLocal_BothSidesEvolveLyxFromSharedBase_NowCompletes(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	// A target-side warp commit, so merging "feature" is a genuine (non-fast-forward) merge that
	// lands a real conclude-commit — otherwise a plain fast-forward reports Committed false on its
	// own, for a reason unrelated to what this scenario is about.
	commitOnCurrentBranch(t, h.PairWarpWorktree("target"), "target-progress.txt", "target progress\n", "target: progress")

	targetWeftPath := h.PairWeftSibling("target")
	commitOnCurrentBranch(t, targetWeftPath, "_lyx/shared.txt", "target content\n", "target: lyx shared")
	commitOnSourceWeft("feature-weft", "_lyx/shared.txt", "feature content\n", "feature: lyx shared")

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	if err != nil {
		t.Fatalf("Merge(feature) error = %v; want it to complete now that the weft is not a merge participant", err)
	}
	if !res.Committed {
		t.Errorf("Merge(feature).Committed = false; want true")
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("Merge(feature).Conflicts = %v; want empty", res.Conflicts)
	}
}

// TestMergeWeftLocal_MergeIn_ParentLyxNeverReachesChildWeft covers scenario 4: MergeIn in the
// opposite direction — a parent branch's _lyx/ content never reaches the child's weft worktree, and
// the child's own live _lyx/ content is unchanged.
func TestMergeWeftLocal_MergeIn_ParentLyxNeverReachesChildWeft(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")

	const childContent = "child content\n"
	commitOnCurrentBranch(t, h.PrimeWeft(), "_lyx/parent-child.txt", childContent, "child: seed lyx content")

	commitOnWarpBranch("parent", "parent-warp.txt", "parent warp content\n", "parent: warp change")
	commitOnWeftBranch("parent-weft", "_lyx/parent-only.txt", "parent lyx content\n", "parent: weft lyx change")

	parentWeftSHA := gitRevParse(t, h.PrimeWeft(), "parent-weft")
	childWeftSHABefore := gitRevParse(t, h.PrimeWeft(), "HEAD")
	if parentWeftSHA == childWeftSHABefore {
		t.Fatalf("parent-weft (%s) equals the child's own weft HEAD (%s); the fixture must diverge them", parentWeftSHA, childWeftSHABefore)
	}

	res, err := f.MergeIn("parent")
	if err != nil {
		t.Fatalf("MergeIn(parent) error = %v", err)
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("MergeIn(parent).Conflicts = %v; want empty", res.Conflicts)
	}

	if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), "_lyx", "parent-only.txt")); !os.IsNotExist(err) {
		t.Errorf("Stat(_lyx/parent-only.txt) = %v; want not-exist — the parent's weft-only content must never reach the child", err)
	}
	got, err := os.ReadFile(filepath.Join(h.PrimeWorktree(), "_lyx", "parent-child.txt"))
	if err != nil {
		t.Fatalf("read _lyx/parent-child.txt after MergeIn: %v", err)
	}
	if string(got) != childContent {
		t.Errorf("_lyx/parent-child.txt after MergeIn = %q; want unchanged %q", got, childContent)
	}
	if got := gitRevParse(t, h.PrimeWeft(), "HEAD"); got != childWeftSHABefore {
		t.Errorf("child weft HEAD after MergeIn = %q; want unchanged %q", got, childWeftSHABefore)
	}
}

// TestMergeWeftLocal_MergeIn_WarpConflictReachesUnifyConflictPaths covers scenario 5: a genuine
// warp-side conflict still reaches unifyConflictPaths. MergeIn returns a non-empty Conflicts list
// naming the warp path, leaving the pair mid-merge for MergeContinue.
func TestMergeWeftLocal_MergeIn_WarpConflictReachesUnifyConflictPaths(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	want := []string{"clash.txt"}
	if len(res.Conflicts) != 1 || res.Conflicts[0] != want[0] {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want %v", res.Conflicts, want)
	}

	inProgress, err := f.MergeInProgress()
	if err != nil {
		t.Fatalf("MergeInProgress() error = %v", err)
	}
	if !inProgress {
		t.Error("MergeInProgress() = false; want true — the pair stays mid-merge for MergeContinue")
	}

	fresh := openFreshFabric(t, h.PrimeWorktree())
	if inProgress, err := fresh.MergeInProgress(); err != nil || !inProgress {
		t.Errorf("fresh handle MergeInProgress() = (%v, %v); want (true, nil)", inProgress, err)
	}
}
