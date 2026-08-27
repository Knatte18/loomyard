//go:build integration

// mergestateactive_integration_test.go covers MergeStateActive against a real hubforge pair: a clean
// weft reports false; a weft carrying a live MERGE_HEAD (no conflicts) reports true; a weft carrying
// a conflicted `git merge --squash` (no MERGE_HEAD) reports true, pinning that neither probe kind is
// redundant; and a warp-alone mid-merge with a clean weft reports false, pinning the weft-only scope.
// Reuses mergestate_integration_test.go's writeConflictFile and driveConflictedMergeStart fixture
// helpers.

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
	writeConflictFile(t, dir, "no-conflict-file.txt", "no conflict content")
	gitkit.MustRun(t, dir, "git", "add", "no-conflict-file.txt")
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", "no-conflict branch commit")

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-")
	gitkit.MustRun(t, dir, "git", "merge", "--no-commit", "--no-ff", "no-conflict-branch")
}

// driveConflictedSquashMerge builds a divergent, conflicting branch in dir and runs
// `git merge --squash` against it, leaving a non-empty conflicted index with no MERGE_HEAD at
// all — the squash form writes none — the shape only the conflicted-index probe catches.
func driveConflictedSquashMerge(t *testing.T, dir string) {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-b", "squash-conflict-branch")
	writeConflictFile(t, dir, "conflict-target.txt", "branch content")
	gitkit.MustRun(t, dir, "git", "add", "conflict-target.txt")
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", "branch content commit")

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-")
	writeConflictFile(t, dir, "conflict-target.txt", "main content")
	gitkit.MustRun(t, dir, "git", "add", "conflict-target.txt")
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", "main content commit")

	mergeCmd := exec.Command("git", "merge", "--squash", "squash-conflict-branch")
	mergeCmd.Dir = dir
	_, _ = mergeCmd.CombinedOutput() // conflicted squash merge exits non-zero, intentionally ignored
}

// TestMergeStateActive_CleanWeft_ReportsFalse covers the baseline case: a freshly cloned pair's weft
// sibling is not mid-merge.
func TestMergeStateActive_CleanWeft_ReportsFalse(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	active, err := fabricengine.MergeStateActive(h.Location)
	if err != nil {
		t.Fatalf("MergeStateActive() on clean weft error = %v", err)
	}
	if active {
		t.Error("MergeStateActive() on clean weft = true; want false")
	}
}

// TestMergeStateActive_WeftMergeHeadPresent_ReportsTrue covers the MERGE_HEAD-only shape: a live,
// non-conflicting merge staged in the weft sibling.
func TestMergeStateActive_WeftMergeHeadPresent_ReportsTrue(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	driveMergeHeadOnlyNoConflicts(t, h.PrimeWeft())

	active, err := fabricengine.MergeStateActive(h.Location)
	if err != nil {
		t.Fatalf("MergeStateActive() with live weft MERGE_HEAD error = %v", err)
	}
	if !active {
		t.Error("MergeStateActive() with live weft MERGE_HEAD = false; want true")
	}
}

// TestMergeStateActive_WeftConflictedSquashNoMergeHead_ReportsTrue covers the conflicted-index-only
// shape a squash merge produces: no MERGE_HEAD is ever written for `git merge --squash`, so this
// scenario is unreachable by the MERGE_HEAD probe alone, pinning that neither probe kind is
// redundant.
func TestMergeStateActive_WeftConflictedSquashNoMergeHead_ReportsTrue(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	driveConflictedSquashMerge(t, h.PrimeWeft())

	active, err := fabricengine.MergeStateActive(h.Location)
	if err != nil {
		t.Fatalf("MergeStateActive() with conflicted weft squash merge error = %v", err)
	}
	if !active {
		t.Error("MergeStateActive() with conflicted weft squash merge (no MERGE_HEAD) = false; want true")
	}
}

// TestMergeStateActive_WarpAloneMidMerge_WeftClean_ReportsFalse covers the pinning case for the
// weft-only scope: a foreign conflicted merge running in the warp checkout alone must not make
// MergeStateActive report true, since it probes only the weft's independent .git state.
func TestMergeStateActive_WarpAloneMidMerge_WeftClean_ReportsFalse(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	f := fabricengine.NewFabricForTest(t, h.PrimeWorktree(), h.PrimeWeft())

	driveConflictedMergeStart(t, h.PrimeWorktree(), fabricengine.WarpForTest(f))

	active, err := fabricengine.MergeStateActive(h.Location)
	if err != nil {
		t.Fatalf("MergeStateActive() with warp-alone mid-merge error = %v", err)
	}
	if active {
		t.Error("MergeStateActive() with warp-alone mid-merge (weft clean) = true; want false — the probe is weft-only")
	}
}
