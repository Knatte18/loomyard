//go:build integration

// add_branch_exists_test.go proves Add's branch-already-exists rejection names
// a way forward. Remove deliberately leaves the warp branch behind (it may
// carry unmerged work), so the everyday remove-then-re-add cycle hits this
// rejection — a bare "already exists" left the operator stuck without
// out-of-band git knowledge.
//
// Package fabricengine_test to reuse newFabricFixture from
// reconcile_stale_registration_test.go; shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestAdd_ExistingBranchErrorNamesRemedy creates the warp branch a slug would claim, calls Add with
// that slug, and asserts the rejection names both remedies (checkout onto it, or delete the
// leftover).
func TestAdd_ExistingBranchErrorNamesRemedy(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	const slug = "leftover-pair"
	gitkit.MustRun(t, l.WorktreePath(), "git", "branch", slug)

	_, err := topology.Add(l, slug, fabricengine.AddOptions{SkipGit: true})
	if err == nil {
		t.Fatalf("Add(%q) error = nil; want branch-already-exists rejection", slug)
	}
	for _, want := range []string{"already exists", "lyx fabric checkout " + slug, "git branch -D " + slug} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Add(%q) error = %q; want substring %q", slug, err.Error(), want)
		}
	}
}

// TestAdd_LeftoverWorktreeDirErrorNamesRemedy plants a bare directory at the warp worktree path for a
// slug whose branch does not exist, so Add reaches its dir-exists guard, and asserts the error names
// the leftover-cleanup recovery (F4): a stranded directory is invisible to `lyx fabric list`/`prune`,
// so a bare "already exists" left an operator with no path forward.
func TestAdd_LeftoverWorktreeDirErrorNamesRemedy(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	const slug = "stranded-leftover"
	// Plant only the directory — no branch — so the branch-exists guard passes and Add reaches the
	// dir-exists guard.
	if err := os.MkdirAll(fabricengine.WorktreePath(l, slug), 0o755); err != nil {
		t.Fatalf("plant leftover directory: %v", err)
	}

	_, err := topology.Add(l, slug, fabricengine.AddOptions{SkipGit: true})
	if err == nil {
		t.Fatalf("Add(%q) error = nil; want worktree-directory-already-exists rejection", slug)
	}
	for _, want := range []string{"already exists", "leftover", "remove the directory and retry"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Add(%q) error = %q; want substring %q", slug, err.Error(), want)
		}
	}
}
