//go:build integration

// openparent_integration_test.go covers OpenParent end to end against real hubforge fixtures: the
// happy path (including the folded-in OriginURL delegation assertion), no live pair for the given
// branch, the parent's own weft sibling missing, a prunable parent directory removed without pruning,
// and a resolve-time failure that must still name both the branch and the resolved path.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestOpenParent_HappyPath asserts OpenParent, called from a task pair, resolves and opens the
// parent's (prime) fabric pair rather than the task's own — and that f.OriginURL() delegates to the
// warp side's configured origin remote.
func TestOpenParent_HappyPath(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	res := hubforge.AddPair(t, h, "task1")

	taskLoc, err := lyxcwd.ResolveWorktree(res.Path)
	if err != nil {
		t.Fatalf("ResolveWorktree(%q): %v", res.Path, err)
	}

	f, err := fabricengine.OpenParent(taskLoc, "main")
	if err != nil {
		t.Fatalf("OpenParent() error = %v", err)
	}
	if f == nil {
		t.Fatal("OpenParent() = nil; want non-nil handle")
	}

	got, err := fabricengine.WarpForTest(f).CurrentBranch()
	if err != nil {
		t.Fatalf("WarpForTest(f).CurrentBranch() error = %v", err)
	}
	if got != "main" {
		t.Errorf("OpenParent() opened branch %q; want %q (the parent, not the task pair)", got, "main")
	}

	origin, err := f.OriginURL()
	if err != nil {
		t.Fatalf("f.OriginURL() error = %v", err)
	}
	if want := filepath.ToSlash(h.WarpBare); origin != want {
		t.Errorf("f.OriginURL() = %q; want %q", origin, want)
	}
}

// TestOpenParent_NoLivePairForBranch asserts OpenParent errors, naming the branch, when the branch
// has no live worktree at all.
func TestOpenParent_NoLivePairForBranch(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "orphan-branch")

	_, err := fabricengine.OpenParent(h.Location, "orphan-branch")
	if err == nil {
		t.Fatal("OpenParent() error = nil; want error naming the branch")
	}
	if !strings.Contains(err.Error(), "orphan-branch") {
		t.Errorf("OpenParent() error = %q; want substring %q", err.Error(), "orphan-branch")
	}
}

// TestOpenParent_ParentSiblingMissing asserts OpenParent errors with an *fabricengine.ErrMissingPath
// naming the hub's own weft sibling when that sibling is deleted, leaving the task pair itself
// untouched.
func TestOpenParent_ParentSiblingMissing(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	res := hubforge.AddPair(t, h, "task1")
	taskLoc, err := lyxcwd.ResolveWorktree(res.Path)
	if err != nil {
		t.Fatalf("ResolveWorktree(%q): %v", res.Path, err)
	}

	if err := os.RemoveAll(h.PrimeWeft()); err != nil {
		t.Fatalf("RemoveAll(hub weft sibling): %v", err)
	}

	_, err = fabricengine.OpenParent(taskLoc, "main")
	if err == nil {
		t.Fatal("OpenParent() error = nil; want error naming the missing weft sibling")
	}
	var missingPath *fabricengine.ErrMissingPath
	if !errors.As(err, &missingPath) {
		t.Fatalf("OpenParent() error = %v; want *ErrMissingPath", err)
	}
	if missingPath.Path != h.PrimeWeft() {
		t.Errorf("OpenParent() error path = %q; want %q", missingPath.Path, h.PrimeWeft())
	}
}

// TestOpenParent_PrunableParentDirRemoved asserts that deleting a pair's directory without pruning
// makes OpenParent report "no live pair" naming the branch — never an *fabricengine.ErrMissingPath —
// since the prunable entry is skipped by matchParentBranch before it is ever opened.
func TestOpenParent_PrunableParentDirRemoved(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	res := hubforge.AddPair(t, h, "task2")

	if err := os.RemoveAll(res.Path); err != nil {
		t.Fatalf("RemoveAll(%q): %v", res.Path, err)
	}

	_, err := fabricengine.OpenParent(h.Location, res.Branch)
	if err == nil {
		t.Fatal("OpenParent() error = nil; want error naming the branch")
	}
	if !strings.Contains(err.Error(), res.Branch) {
		t.Errorf("OpenParent() error = %q; want substring %q", err.Error(), res.Branch)
	}
	var missingPath *fabricengine.ErrMissingPath
	if errors.As(err, &missingPath) {
		t.Errorf("OpenParent() error unwraps to *ErrMissingPath; want a plain \"no live pair\" error, since the prunable entry must never be reported as a broken pair")
	}
}

// TestOpenParent_ResolveFailureNamesBranchAndPath asserts that a step-3 lyxcwd.ResolveWorktree
// failure — the matched path existed at List time but is gone by ResolveWorktree time — surfaces
// through OpenParent's wrap naming both the branch and the resolved path, never git's own bare "not a
// git repository" text unqualified.
//
// The path is locked before its directory is removed: `git worktree lock` suppresses git's own
// "prunable" porcelain line even once the directory is gone, which is the deterministic way to
// construct "matched at List time, gone by ResolveWorktree time" without depending on a real race.
func TestOpenParent_ResolveFailureNamesBranchAndPath(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	res := hubforge.AddPair(t, h, "task3")

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "lock", res.Path)
	if err := os.RemoveAll(res.Path); err != nil {
		t.Fatalf("RemoveAll(%q): %v", res.Path, err)
	}

	_, err := fabricengine.OpenParent(h.Location, res.Branch)
	if err == nil {
		t.Fatal("OpenParent() error = nil; want error naming the branch and the resolved path")
	}
	if !strings.Contains(err.Error(), res.Branch) {
		t.Errorf("OpenParent() error = %q; want substring %q (branch)", err.Error(), res.Branch)
	}
	if !strings.Contains(err.Error(), res.Path) {
		t.Errorf("OpenParent() error = %q; want substring %q (resolved path)", err.Error(), res.Path)
	}
}
