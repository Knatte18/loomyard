//go:build integration

// remove_finish_integration_test.go covers Remove finishing a half-removed pair: the task worktree removed by hand, both worktrees gone with branches left, a sibling branch only on origin, a stray path at the pair location, and a pair of which nothing remains.
// Each case converges on one re-run of Remove.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// removeByHand deletes the directory at path the way an operator's `rm -rf` would, leaving git's worktree registration behind.
func removeByHand(t *testing.T, path string) {
	t.Helper()

	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("remove %s by hand: %v", path, err)
	}
}

// requireSteps fails unless got holds every step in want.
func requireSteps(t *testing.T, got []string, want ...string) {
	t.Helper()

	for _, step := range want {
		if !slices.Contains(got, step) {
			t.Errorf("Steps = %v; want it to contain %q", got, step)
		}
	}
}

// TestRemove_FinishesPairWhoseTaskWorktreeWasRemovedByHand removes the task worktree by hand with a pending record in the sibling worktree, and asserts one Remove commits the record into the archive tag and removes the sibling worktree and both branches.
func TestRemove_FinishesPairWhoseTaskWorktreeWasRemovedByHand(t *testing.T) {
	t.Parallel()

	const slug = "finish-by-hand"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	rel := "_lyx/shed/" + slug + "/drive-reports/stop.md"
	writeFile(t, filepath.Join(fabricengine.WeftWorktreePath(l, slug), filepath.FromSlash(rel)), "stopped\n")
	warpTip := gitkit.RevParse(t, l.WorktreePath(), slug)
	removeByHand(t, fabricengine.WorktreePath(l, slug))

	res, err := h.Topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove error = %v; want the half-removed pair finished", err)
	}
	if !res.Finished {
		t.Errorf("Finished = false; want true for a pair whose task worktree was already gone")
	}
	requireSteps(t, res.Steps, fabricengine.RemoveStepRecordCommit, fabricengine.RemoveStepArchive, fabricengine.RemoveStepSiblingWorktree, fabricengine.RemoveStepSiblingBranch, fabricengine.RemoveStepTaskBranch)
	if slices.Contains(res.Steps, fabricengine.RemoveStepTaskWorktree) {
		t.Errorf("Steps = %v; want no task_worktree step for a worktree that was already gone", res.Steps)
	}

	if res.ArchiveTag == "" {
		t.Fatalf("ArchiveTag is empty; want the tag covering the committed record")
	}
	if got := showAtTag(t, h.WeftBare, res.ArchiveTag, rel); got != "stopped\n" {
		t.Errorf("archived %s = %q; want the committed record", rel, got)
	}
	msg, err := gitexec.Run([]string{"log", "-1", "--format=%B", tagTargetAt(t, h.WeftBare, res.ArchiveTag)}, h.WeftBare)
	if err != nil {
		t.Fatalf("read the archived tip's message: %v", err)
	}
	if !strings.Contains(msg, fabricengine.WarpSHATrailerKey+": "+warpTip) {
		t.Errorf("archived tip message = %q; want the Warp-SHA trailer to name the task branch tip %s", msg, warpTip)
	}

	if _, statErr := os.Lstat(fabricengine.WeftWorktreePath(l, slug)); !os.IsNotExist(statErr) {
		t.Errorf("sibling worktree still present after Remove: %v", statErr)
	}
	if gitkit.BranchExists(t, weftRoot, fabricengine.WeftBranchName(slug)) {
		t.Errorf("sibling branch still exists after Remove")
	}
	if gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("task branch still exists after Remove")
	}
}

// TestRemove_FinishesPairWithBothWorktreesGone removes both worktrees by hand and asserts one Remove deletes both leftover branches, the sibling branch only after its archive tag is on origin.
func TestRemove_FinishesPairWithBothWorktreesGone(t *testing.T) {
	t.Parallel()

	const slug = "finish-both-gone"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	siblingTip := gitkit.RevParse(t, weftRoot, fabricengine.WeftBranchName(slug))
	removeByHand(t, fabricengine.WorktreePath(l, slug))
	removeByHand(t, fabricengine.WeftWorktreePath(l, slug))

	res, err := h.Topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove error = %v", err)
	}
	if gitkit.BranchExists(t, weftRoot, fabricengine.WeftBranchName(slug)) {
		t.Errorf("sibling branch still exists after Remove")
	}
	if gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("task branch still exists after Remove")
	}
	if res.ArchiveTag == "" {
		t.Fatalf("ArchiveTag is empty; want the sibling branch archived before its deletion")
	}
	if got := tagTargetAt(t, h.WeftBare, res.ArchiveTag); got != siblingTip {
		t.Errorf("origin archive tag points at %q; want the sibling tip %s", got, siblingTip)
	}
	requireSteps(t, res.Steps, fabricengine.RemoveStepArchive, fabricengine.RemoveStepSiblingBranch, fabricengine.RemoveStepTaskBranch)
	if slices.Contains(res.Steps, fabricengine.RemoveStepSiblingWorktree) {
		t.Errorf("Steps = %v; want no sibling_worktree step for a worktree that was already gone", res.Steps)
	}
}

// leaveSiblingBranchOnlyOnOrigin removes both worktrees by hand and both local branches, leaving the sibling branch on origin alone locally gone.
func leaveSiblingBranchOnlyOnOrigin(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	removeByHand(t, fabricengine.WorktreePath(l, slug))
	removeByHand(t, fabricengine.WeftWorktreePath(l, slug))
	gitkit.MustRun(t, l.WorktreePath(), "git", "worktree", "prune")
	gitkit.MustRun(t, weftRoot, "git", "worktree", "prune")
	gitkit.MustRun(t, l.WorktreePath(), "git", "branch", "-D", slug)
	gitkit.MustRun(t, weftRoot, "git", "branch", "-D", fabricengine.WeftBranchName(slug))
}

// TestRemove_ArchivesAndDeletesSiblingBranchOnlyOnOrigin leaves the sibling branch only on origin and asserts Remove archives it from there and, with remote, deletes it there.
func TestRemove_ArchivesAndDeletesSiblingBranchOnlyOnOrigin(t *testing.T) {
	t.Parallel()

	const slug = "finish-origin-only"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	siblingTip := gitkit.RevParse(t, h.WeftBare, fabricengine.WeftBranchName(slug))
	leaveSiblingBranchOnlyOnOrigin(t, h, slug)

	res, err := h.Topology.Remove(l, slug, false, true)
	if err != nil {
		t.Fatalf("Remove(remote=true) error = %v", err)
	}
	if res.ArchiveTag == "" {
		t.Fatalf("ArchiveTag is empty; want the origin copy archived")
	}
	if got := tagTargetAt(t, h.WeftBare, res.ArchiveTag); got != siblingTip {
		t.Errorf("origin archive tag points at %q; want the origin copy's tip %s", got, siblingTip)
	}
	if !res.RemoteBranchDeleted || res.RemoteBranchError != "" {
		t.Errorf("RemoteBranchDeleted = %v, RemoteBranchError = %q; want the origin copy deleted", res.RemoteBranchDeleted, res.RemoteBranchError)
	}
	if gitkit.BranchExists(t, h.WeftBare, fabricengine.WeftBranchName(slug)) {
		t.Errorf("sibling branch still on origin after Remove(remote=true)")
	}
	requireSteps(t, res.Steps, fabricengine.RemoveStepArchive, fabricengine.RemoveStepSiblingBranchOnOrigin)
}

// TestRemove_KeepsOriginOnlySiblingBranchWithoutRemote asserts the same state without remote keeps the origin copy, archives it, and says so on the result.
func TestRemove_KeepsOriginOnlySiblingBranchWithoutRemote(t *testing.T) {
	t.Parallel()

	const slug = "finish-origin-kept"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	leaveSiblingBranchOnlyOnOrigin(t, h, slug)

	res, err := h.Topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove(remote=false) error = %v", err)
	}
	if !gitkit.BranchExists(t, h.WeftBare, fabricengine.WeftBranchName(slug)) {
		t.Errorf("sibling branch gone from origin after Remove(remote=false); want it kept")
	}
	if res.RemoteSkippedReason == "" {
		t.Errorf("RemoteSkippedReason is empty; want it to say the origin-only copy was kept")
	}
	if res.ArchiveTag == "" {
		t.Errorf("ArchiveTag is empty; want the origin copy archived even though it is kept")
	}
	if slices.Contains(res.Steps, fabricengine.RemoveStepSiblingBranchOnOrigin) {
		t.Errorf("Steps = %v; want no sibling_branch_on_origin step without remote", res.Steps)
	}
}

// TestRemove_ReportsStrayPathAndFinishesBranchTeardown puts a plain directory at the task worktree's location beside leftover branches, and asserts it is reported in StrayPath and left on disk while the branch teardown completes.
func TestRemove_ReportsStrayPathAndFinishesBranchTeardown(t *testing.T) {
	t.Parallel()

	const slug = "finish-stray"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	weftRoot := mustWeftRepoRoot(t, l)
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	target := fabricengine.WorktreePath(l, slug)
	removeByHand(t, target)
	gitkit.MustRun(t, l.WorktreePath(), "git", "worktree", "prune")
	marker := filepath.Join(target, "keep-me.txt")
	writeFile(t, marker, "not a worktree\n")

	res, err := h.Topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove error = %v", err)
	}
	if res.StrayPath != target {
		t.Errorf("StrayPath = %q; want %q", res.StrayPath, target)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Errorf("stray directory content was deleted: %v", statErr)
	}
	if gitkit.BranchExists(t, weftRoot, fabricengine.WeftBranchName(slug)) {
		t.Errorf("sibling branch still exists after Remove")
	}
	if gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("task branch still exists after Remove")
	}
}

// TestRemove_ReturnsPairNotFoundWhenNothingRemains finishes a pair and asserts a second Remove wraps ErrPairNotFound, and that a bare stray directory is named in the error.
func TestRemove_ReturnsPairNotFoundWhenNothingRemains(t *testing.T) {
	t.Parallel()

	const slug = "finish-nothing-left"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	if _, err := h.Topology.Remove(l, slug, false, true); err != nil {
		t.Fatalf("first Remove error = %v", err)
	}

	_, err := h.Topology.Remove(l, slug, false, true)
	if !errors.Is(err, fabricengine.ErrPairNotFound) {
		t.Fatalf("second Remove error = %v; want it to wrap ErrPairNotFound", err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q; want it to contain %q", err.Error(), "not found")
	}

	stray := fabricengine.WorktreePath(l, slug)
	writeFile(t, filepath.Join(stray, "keep-me.txt"), "not a worktree\n")
	_, err = h.Topology.Remove(l, slug, false, true)
	if !errors.Is(err, fabricengine.ErrPairNotFound) {
		t.Fatalf("Remove with only a stray directory left: error = %v; want it to wrap ErrPairNotFound", err)
	}
	if !strings.Contains(err.Error(), stray) {
		t.Errorf("error = %q; want it to name the stray path %s", err.Error(), stray)
	}
	if _, statErr := os.Stat(stray); statErr != nil {
		t.Errorf("stray directory was deleted: %v", statErr)
	}
}
