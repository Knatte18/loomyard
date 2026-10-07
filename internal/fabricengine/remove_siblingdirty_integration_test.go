//go:build integration

// remove_siblingdirty_integration_test.go pins that an uncommitted run record in a pair's other worktree is committed and archived rather than refused, and that a pair dirty only on the task side is refused without satisfying errors.Is(err, fabricengine.ErrPairSiblingDirty).
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestRemove_UntrackedDriveReportIsCommittedAndArchived leaves a new file inside a not-yet-tracked _lyx/shed/<slug>/drive-reports/ directory — the uncommitted stop report the done-wait rests on — and asserts a no-force Remove commits it and the archive tag contains it, with no sibling-dirty refusal.
func TestRemove_UntrackedDriveReportIsCommittedAndArchived(t *testing.T) {
	t.Parallel()

	const slug = "sibling-dirty"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	topology := h.Topology

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	reports := filepath.Join(fabricengine.WeftWorktreePath(l, slug), "_lyx", "shed", slug, "drive-reports")
	if err := os.MkdirAll(reports, 0o755); err != nil {
		t.Fatalf("create %s: %v", reports, err)
	}
	if err := os.WriteFile(filepath.Join(reports, "stop.md"), []byte("stopped\n"), 0o644); err != nil {
		t.Fatalf("write stop report: %v", err)
	}

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove(force=false) on a pair with an uncommitted drive report error = %v; want it committed and archived", err)
	}
	if res.ArchiveTag == "" {
		t.Fatalf("ArchiveTag is empty; want the tag covering the committed report")
	}
	rel := "_lyx/shed/" + slug + "/drive-reports/stop.md"
	if got := showAtTag(t, h.RecordsBare, res.ArchiveTag, rel); got != "stopped\n" {
		t.Errorf("archived %s = %q; want the committed report", rel, got)
	}
}

// TestRemove_TaskSideDirtyDoesNotSatisfySiblingDirty dirties only the task-side worktree and asserts its refusal is not the sibling-dirty sentinel.
func TestRemove_TaskSideDirtyDoesNotSatisfySiblingDirty(t *testing.T) {
	t.Parallel()

	const slug = "task-side-dirty"
	h := hubforge.NewHub(t, ".")
	l := h.Location
	topology := h.Topology

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	warpPath := fabricengine.WorktreePath(l, slug)
	tracked := filepath.Join(warpPath, "tracked.md")
	gitkit.CommitFile(t, warpPath, "tracked.md", "committed\n", "seed tracked file")
	if err := os.WriteFile(tracked, []byte("committed\nuncommitted\n"), 0o644); err != nil {
		t.Fatalf("dirty %s: %v", tracked, err)
	}

	_, err := topology.Remove(l, slug, false, false)
	if err == nil {
		t.Fatalf("Remove(force=false) on a task-side-dirty pair returned nil error; want a refusal")
	}
	if errors.Is(err, fabricengine.ErrPairSiblingDirty) {
		t.Errorf("task-side refusal %v must not satisfy ErrPairSiblingDirty", err)
	}
}
