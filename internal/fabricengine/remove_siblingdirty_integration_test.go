//go:build integration

// remove_siblingdirty_integration_test.go pins that Remove's no-force refusal of a pair whose other worktree is dirty satisfies errors.Is(err, fabricengine.ErrPairSiblingDirty), and that a pair dirty only on the task side does not.
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

// TestRemove_UntrackedDriveReportRefusesWithSiblingDirty leaves a new file inside a not-yet-tracked _lyx/shed/<slug>/drive-reports/ directory — the uncommitted stop report the done-wait rests on — and asserts the refusal satisfies ErrPairSiblingDirty.
func TestRemove_UntrackedDriveReportRefusesWithSiblingDirty(t *testing.T) {
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

	_, err := topology.Remove(l, slug, false, false)
	if err == nil {
		t.Fatalf("Remove(force=false) on a pair with an uncommitted drive report returned nil error; want a refusal")
	}
	if !errors.Is(err, fabricengine.ErrPairSiblingDirty) {
		t.Errorf("refusal = %v; want errors.Is(err, ErrPairSiblingDirty)", err)
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
