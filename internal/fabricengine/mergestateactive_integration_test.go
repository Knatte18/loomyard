// mergestateactive_integration_test.go covers MergeStateActive against a real hubforge pair.
// A clean weft reports false, and a weft with a live MERGE_HEAD and no conflicts reports true.
// A conflicted `git merge --squash` leaves no MERGE_HEAD yet reports true, so neither probe kind is redundant.
// A mid-merge warp checkout with a clean weft reports false, which pins the weft-only scope.
// It reuses driveConflictedMergeStart from mergestate_integration_test.go.

package fabricengine_test

import (
	"os/exec"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// driveMergeHeadOnlyNoConflicts leaves dir mid-merge with MERGE_HEAD set and no conflicts.
// It runs `git merge --no-commit --no-ff` against a divergent, non-conflicting branch.
// The state is a resolved, unconcluded merge that a conflicted-index check and a dirty-worktree check both miss.
func driveMergeHeadOnlyNoConflicts(t *testing.T, dir string) {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-b", "no-conflict-branch")
	gitkit.CommitFile(t, dir, "no-conflict-file.txt", "no conflict content", "no-conflict branch commit")

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-")
	gitkit.MustRun(t, dir, "git", "merge", "--no-commit", "--no-ff", "no-conflict-branch")
}

// driveConflictedSquashMerge leaves dir with a non-empty conflicted index and no MERGE_HEAD.
// It runs `git merge --squash` against a divergent branch that conflicts with the original branch;
// the squash form writes no MERGE_HEAD.
func driveConflictedSquashMerge(t *testing.T, dir string) {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-b", "squash-conflict-branch")
	gitkit.CommitFile(t, dir, "conflict-target.txt", "branch content", "branch content commit")

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-")
	gitkit.CommitFile(t, dir, "conflict-target.txt", "main content", "main content commit")

	mergeCmd := exec.Command("git", "merge", "--squash", "squash-conflict-branch")
	mergeCmd.Dir = dir
	_, _ = mergeCmd.CombinedOutput() // conflicted squash merge exits non-zero, intentionally ignored
}

// TestMergeStateActive drives one hubforge pair through the shapes MergeStateActive distinguishes.
// A freshly cloned pair's clean weft is not mid-merge, so it reports false.
// A foreign conflicted merge in the warp checkout alone, with the weft clean, must not make it report true.
// The probe reads only the weft's own .git state.
// A live, non-conflicting merge staged in the weft (MERGE_HEAD only) reports true;
// it is then aborted, so the weft is clean again.
// A conflicted `git merge --squash` in the weft writes no MERGE_HEAD and reports true;
// the MERGE_HEAD probe alone cannot reach it, so neither probe kind is redundant.
// The steps run serially on one pair.
// Each leaves the weft clean or conflicted exactly as the next step expects.
func TestMergeStateActive(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, h.PrimeWorktree(), h.PrimeRecords())

	requireMergeStateActive := func(t *testing.T, want bool, shape string) {
		t.Helper()
		active, err := fabricengine.MergeStateActive(h.Location)
		if err != nil {
			t.Fatalf("MergeStateActive() with %s error = %v", shape, err)
		}
		if active != want {
			t.Errorf("MergeStateActive() with %s = %v; want %v", shape, active, want)
		}
	}

	t.Run("clean weft reports false", func(t *testing.T) {
		requireMergeStateActive(t, false, "a clean weft")
	})

	t.Run("warp-alone mid-merge with a clean weft reports false", func(t *testing.T) {
		driveConflictedMergeStart(t, h.PrimeWorktree(), fabricengine.WarpForTest(f))

		requireMergeStateActive(t, false, "a warp-alone mid-merge (weft clean; the probe is weft-only)")
	})

	t.Run("weft MERGE_HEAD present reports true", func(t *testing.T) {
		driveMergeHeadOnlyNoConflicts(t, h.PrimeRecords())

		requireMergeStateActive(t, true, "a live weft MERGE_HEAD")

		gitkit.MustRun(t, h.PrimeRecords(), "git", "merge", "--abort")
	})

	t.Run("weft conflicted squash without MERGE_HEAD reports true", func(t *testing.T) {
		driveConflictedSquashMerge(t, h.PrimeRecords())

		requireMergeStateActive(t, true, "a conflicted weft squash merge (no MERGE_HEAD)")
	})
}
