//go:build integration

// mergestateactive_integration_test.go covers MergeStateActive against a real hubforge pair: a clean
// weft reports false; a weft carrying a live MERGE_HEAD (no conflicts) reports true; a weft carrying
// a conflicted `git merge --squash` (no MERGE_HEAD) reports true, pinning that neither probe kind is
// redundant; and a warp-alone mid-merge with a clean weft reports false, pinning the weft-only scope.
// Reuses mergestate_integration_test.go's driveConflictedMergeStart fixture helper.

package fabricengine_test

import (
	"os/exec"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// driveMergeHeadOnlyNoConflicts builds a divergent, non-conflicting branch in dir and runs
// `git merge --no-commit --no-ff` against it, leaving MERGE_HEAD live with a clean, fully-staged
// merge — the "resolved but not concluded" shape neither the conflicted-index probe nor a bare
// worktree-dirty check would catch.
func driveMergeHeadOnlyNoConflicts(t *testing.T, dir string) {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-b", "no-conflict-branch")
	gitkit.CommitFile(t, dir, "no-conflict-file.txt", "no conflict content", "no-conflict branch commit")

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-")
	gitkit.MustRun(t, dir, "git", "merge", "--no-commit", "--no-ff", "no-conflict-branch")
}

// driveConflictedSquashMerge builds a divergent, conflicting branch in dir and runs
// `git merge --squash` against it, leaving a non-empty conflicted index with no MERGE_HEAD at
// all — the squash form writes none — the shape only the conflicted-index probe catches.
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

// TestMergeStateActive builds one hubforge pair and drives it through the shapes MergeStateActive
// distinguishes, in this order:
// a freshly cloned pair's clean weft is not mid-merge (reports false);
// a foreign conflicted merge running in the warp checkout alone, with the weft clean, must not make
// it report true, since it probes only the weft's independent .git state;
// a live, non-conflicting merge staged in the weft sibling (MERGE_HEAD only) reports true, and is
// aborted so the weft is clean again;
// and a conflicted `git merge --squash` in the weft, which never writes a MERGE_HEAD, reports true —
// unreachable by the MERGE_HEAD probe alone, pinning that neither probe kind is redundant.
// The steps run serially on one pair.
// Each leaves the weft clean or conflicted exactly as the next step expects.
func TestMergeStateActive(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	f := fabricengine.NewFabricForTest(t, h.PrimeWorktree(), h.PrimeWeft())

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
		driveMergeHeadOnlyNoConflicts(t, h.PrimeWeft())

		requireMergeStateActive(t, true, "a live weft MERGE_HEAD")

		gitkit.MustRun(t, h.PrimeWeft(), "git", "merge", "--abort")
	})

	t.Run("weft conflicted squash without MERGE_HEAD reports true", func(t *testing.T) {
		driveConflictedSquashMerge(t, h.PrimeWeft())

		requireMergeStateActive(t, true, "a conflicted weft squash merge (no MERGE_HEAD)")
	})
}
