// mergeweftlocal_integration_test.go pins the warp-only merge against a real hubforge pair.
// The weft is never a merge participant in either verb or either direction.
// A weft counterpart can rewrite system-file content, diverge, or evolve the same _lym/ path from a shared base.
// Such a merge produces no conflict, no self-abort, and no moved weft HEAD.
// Only a genuine warp-side conflict still reaches `unifyConflictPaths`.

package fabricengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestMergeWeftLocal_TargetWeftRewritesStatusManyTimes_WarpAdvancesWeftUnchanged checks one case:
// a Merge of a source branch whose weft counterpart rewrote _lym/loom/status.json many times.
// The target pair's warp HEAD advances, the target pair's weft HEAD is byte-identical before and after,
// and MergeResult.Conflicts is empty.
// It also asserts MergeResult.AlreadyUpToDate is false while the warp moved.
// The merge record, which would show WeftOutcome as up_to_date, is deleted when a successful call returns.
// That leaves AlreadyUpToDate as the check.
func TestMergeWeftLocal_TargetWeftRewritesStatusManyTimes_WarpAdvancesWeftUnchanged(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	if err := os.MkdirAll(filepath.Join(h.PrimeRecords(), "_lym", "loom"), 0o755); err != nil {
		t.Fatalf("MkdirAll(_lym/loom): %v", err)
	}
	for i := range 5 {
		commitOnSourceWeft("feature-weft", "_lym/loom/status.json", fmt.Sprintf(`{"n":%d}`, i), fmt.Sprintf("weft: status rewrite %d", i))
	}

	targetWarpPath, targetWeftPath := h.PairCodeWorktree("target"), h.PairRecordsSibling("target")
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

// TestMergeWeftLocal_TargetWeftDivergedStatus_ContentUnchanged checks a Merge of a source branch.
// The target pair's own diverged _lym/loom/status.json keeps its content through that Merge.
// No merge runs between the two weft sides, so the target's divergence is never reconciled against the source's.
func TestMergeWeftLocal_TargetWeftDivergedStatus_ContentUnchanged(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)
	if err := os.MkdirAll(filepath.Join(h.PrimeRecords(), "_lym", "loom"), 0o755); err != nil {
		t.Fatalf("MkdirAll(_lym/loom) on source weft: %v", err)
	}
	commitOnSourceWeft("feature-weft", "_lym/loom/status.json", `{"source":true}`, "feature: weft status")

	targetWeftPath := h.PairRecordsSibling("target")
	statusRel := filepath.Join("_lym", "loom", "status.json")
	if err := os.MkdirAll(filepath.Join(targetWeftPath, "_lym", "loom"), 0o755); err != nil {
		t.Fatalf("MkdirAll(_lym/loom) on target weft: %v", err)
	}
	gitkit.CommitFile(t, targetWeftPath, filepath.ToSlash(statusRel), `{"target":true}`, "target: weft status diverge")

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

// TestMergeWeftLocal_BothSidesEvolveLymFromSharedBase_NowCompletes checks a Merge.
// The target's weft and the source's weft counterpart both evolve the same _lym/ path from a shared base.
// It reports Committed true, since the weft is not a merge participant and nothing on that side conflicts.
func TestMergeWeftLocal_BothSidesEvolveLymFromSharedBase_NowCompletes(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	// A target-side warp commit makes the merge of "feature" a non-fast-forward merge that lands a conclude-commit.
	// Without it, a fast-forward reports Committed false, which has nothing to do with this test.
	gitkit.CommitFile(t, h.PairCodeWorktree("target"), "target-progress.txt", "target progress\n", "target: progress")

	targetWeftPath := h.PairRecordsSibling("target")
	gitkit.CommitFile(t, targetWeftPath, "_lym/shared.txt", "target content\n", "target: lym shared")
	commitOnSourceWeft("feature-weft", "_lym/shared.txt", "feature content\n", "feature: lym shared")

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

// TestMergeWeftLocal_MergeIn_ParentLymNeverReachesChildWeft checks a MergeIn of a parent branch.
// The parent's _lym/ content never reaches the child's weft worktree.
// The child's own live _lym/ content is unchanged.
func TestMergeWeftLocal_MergeIn_ParentLymNeverReachesChildWeft(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")

	const childContent = "child content\n"
	gitkit.CommitFile(t, h.PrimeRecords(), "_lym/parent-child.txt", childContent, "child: seed lym content")

	commitOnWarpBranch("parent", "parent-warp.txt", "parent warp content\n", "parent: warp change")
	commitOnWeftBranch("parent-weft", "_lym/parent-only.txt", "parent lym content\n", "parent: weft lym change")

	parentWeftSHA := gitkit.RevParse(t, h.PrimeRecords(), "parent-weft")
	childWeftSHABefore := gitkit.RevParse(t, h.PrimeRecords(), "HEAD")
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

	if _, err := os.Stat(filepath.Join(h.PrimeWorktree(), "_lym", "parent-only.txt")); !os.IsNotExist(err) {
		t.Errorf("Stat(_lym/parent-only.txt) = %v; want not-exist — the parent's weft-only content must never reach the child", err)
	}
	got, err := os.ReadFile(filepath.Join(h.PrimeWorktree(), "_lym", "parent-child.txt"))
	if err != nil {
		t.Fatalf("read _lym/parent-child.txt after MergeIn: %v", err)
	}
	if string(got) != childContent {
		t.Errorf("_lym/parent-child.txt after MergeIn = %q; want unchanged %q", got, childContent)
	}
	if got := gitkit.RevParse(t, h.PrimeRecords(), "HEAD"); got != childWeftSHABefore {
		t.Errorf("child weft HEAD after MergeIn = %q; want unchanged %q", got, childWeftSHABefore)
	}
}

// TestMergeWeftLocal_MergeIn_WarpConflictReachesUnifyConflictPaths checks a genuine warp-side conflict.
// It still reaches `unifyConflictPaths`.
// MergeIn returns a non-empty Conflicts list naming the warp path and leaves the pair mid-merge for MergeContinue.
func TestMergeWeftLocal_MergeIn_WarpConflictReachesUnifyConflictPaths(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	branchAtCurrentHEAD(t, h.PrimeRecords(), "feature-weft")

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
