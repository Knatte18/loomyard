// add_branch_exists_test.go proves that Add's branch-already-exists rejection names a way forward.
// Remove leaves the warp branch behind, since it may carry unmerged work;
// so the everyday remove-then-re-add cycle hits this rejection.
// A bare "already exists" would leave the operator stuck without out-of-band git knowledge.
//
// Package fabricengine_test; shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAdd_ExistingBranchErrorNamesRemedy creates the warp branch a slug would claim and calls Add with that slug.
// It asserts the rejection names all three remedies: checkout onto it, delete the leftover, or adopt it with --adopt-local.
func TestAdd_ExistingBranchErrorNamesRemedy(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	const slug = "leftover-pair"
	gitkit.MustRun(t, l.WorktreePath(), "git", "branch", slug)

	_, err := topology.Add(l, slug, fabricengine.AddOptions{SkipGit: true})
	if err == nil {
		t.Fatalf("Add(%q) error = nil; want branch-already-exists rejection", slug)
	}
	for _, want := range []string{"already exists", "lym fabric checkout " + slug, "git branch -D " + slug, "lym fabric add <slug> --adopt-local"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Add(%q) error = %q; want substring %q", slug, err.Error(), want)
		}
	}
}

// TestAdd_LeftoverWorktreeDirErrorNamesRemedy plants a bare directory at the warp worktree path.
// The slug's branch does not exist, so Add reaches its dir-exists guard.
// It asserts that the error names the leftover-cleanup recovery.
// A stranded directory is invisible to `lym fabric list` and `prune`,
// so a bare "already exists" would leave an operator with no path forward.
func TestAdd_LeftoverWorktreeDirErrorNamesRemedy(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	const slug = "stranded-leftover"
	// Plant only the directory, with no branch, so the branch-exists guard passes and Add reaches the dir-exists guard.
	if err := os.MkdirAll(fabricengine.WorktreePath(l, slug), 0o755); err != nil {
		t.Fatalf("plant leftover directory: %v", err)
	}

	_, err := topology.Add(l, slug, fabricengine.AddOptions{SkipGit: true})
	if err == nil {
		t.Fatalf("Add(%q) error = nil; want worktree-directory-already-exists rejection", slug)
	}
	for _, want := range []string{"already exists", "leftover", "lym fabric prune", "delete it and retry"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Add(%q) error = %q; want substring %q", slug, err.Error(), want)
		}
	}
}
