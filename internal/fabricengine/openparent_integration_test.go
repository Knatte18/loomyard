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
	"slices"
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
	t.Parallel()

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
	if want := filepath.ToSlash(h.CodeBare); origin != want {
		t.Errorf("f.OriginURL() = %q; want %q", origin, want)
	}
}

// TestOpenParent_NoLivePairForBranch asserts OpenParent errors, naming the branch, when the branch
// has no live worktree at all.
//
//testtiming:keep OpenParent naming the branch when it has no live worktree at all; coverage of its blocks by other tests does not show an assertion of this
func TestOpenParent_NoLivePairForBranch(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	res := hubforge.AddPair(t, h, "task1")
	taskLoc, err := lyxcwd.ResolveWorktree(res.Path)
	if err != nil {
		t.Fatalf("ResolveWorktree(%q): %v", res.Path, err)
	}

	if err := os.RemoveAll(h.PrimeRecords()); err != nil {
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
	if missingPath.Path != h.PrimeRecords() {
		t.Errorf("OpenParent() error path = %q; want %q", missingPath.Path, h.PrimeRecords())
	}
}

// TestOpenParent_PrunableParentDirRemoved asserts that deleting a pair's directory without pruning
// makes OpenParent report "no live pair" naming the branch — never an *fabricengine.ErrMissingPath —
// since the prunable entry is skipped by matchParentBranch before it is ever opened.
//
//testtiming:keep a deleted but unpruned pair reporting "no live pair" naming the branch and never an *ErrMissingPath; coverage of its blocks by other tests does not show an assertion of this
func TestOpenParent_PrunableParentDirRemoved(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

// TestCodeWorktrees_ListOpenAndPairComplete pins the path-based reads a told-geometry caller walks a hub with, on a root-anchored and a subpath-anchored hub:
// the listing from either pair names the prime first and both pairs with their anchors, a pair whose directory is deleted by hand drops out and reads as missing,
// and a pair whose warp `_lyx` junction is removed is listed but not complete.
func TestCodeWorktrees_ListOpenAndPairComplete(t *testing.T) {
	t.Parallel()

	for _, anchor := range []string{".", "backend"} {
		t.Run("anchor "+anchor, func(t *testing.T) {
			t.Parallel()

			h := hubforge.NewHub(t, anchor)
			kept := hubforge.AddPair(t, h, "kept")
			gone := hubforge.AddPair(t, h, "gone")
			anchorOf := func(worktree string) string { return filepath.Join(worktree, h.Location.AnchorRel) }

			want := []fabricengine.CodeWorktree{
				{Path: h.PrimeWorktree(), Anchor: h.Location.AnchorPath(), Main: true},
				{Path: kept.Path, Anchor: anchorOf(kept.Path)},
				{Path: gone.Path, Anchor: anchorOf(gone.Path)},
			}
			for _, from := range []string{kept.Path, gone.Path} {
				got, err := fabricengine.CodeWorktrees(from)
				if err != nil {
					t.Fatalf("CodeWorktrees(%q) error = %v", from, err)
				}
				if !sameCodeWorktrees(got, want) {
					t.Errorf("CodeWorktrees(%q) = %+v; want %+v", from, got, want)
				}
			}
			if want[1].Path != fabricengine.WorktreePath(h.Location, "kept") {
				t.Errorf("listed pair path %q; want WorktreePath %q", want[1].Path, fabricengine.WorktreePath(h.Location, "kept"))
			}

			if err := os.RemoveAll(gone.Path); err != nil {
				t.Fatalf("RemoveAll(%q): %v", gone.Path, err)
			}
			got, err := fabricengine.CodeWorktrees(kept.Path)
			if err != nil {
				t.Fatalf("CodeWorktrees after delete error = %v", err)
			}
			if !sameCodeWorktrees(got, want[:2]) {
				t.Errorf("CodeWorktrees after delete = %+v; want %+v", got, want[:2])
			}

			if _, err := fabricengine.OpenCodeWorktree(kept.Path); err != nil {
				t.Errorf("OpenCodeWorktree(kept) error = %v", err)
			}
			var missing *fabricengine.ErrMissingPath
			if _, err := fabricengine.OpenCodeWorktree(gone.Path); !errors.As(err, &missing) || missing.Path != gone.Path {
				t.Errorf("OpenCodeWorktree(gone) error = %v; want *ErrMissingPath naming %q", err, gone.Path)
			}

			if ok, reason, err := fabricengine.PairCompleteAt(kept.Path); err != nil || !ok {
				t.Errorf("PairCompleteAt(kept) = %v, %q, %v; want complete", ok, reason, err)
			}
			if err := os.Remove(fabricengine.CodeLyxLink(h.Location, "kept")); err != nil {
				t.Fatalf("remove warp _lyx junction: %v", err)
			}
			if ok, _, err := fabricengine.PairCompleteAt(kept.Path); err != nil || ok {
				t.Errorf("PairCompleteAt(kept) without its _lyx junction = %v, %v; want not complete", ok, err)
			}
			if _, _, err := fabricengine.PairCompleteAt(gone.Path); !errors.As(err, &missing) || missing.Path != gone.Path {
				t.Errorf("PairCompleteAt(gone) error = %v; want *ErrMissingPath naming %q", err, gone.Path)
			}
		})
	}
}

// sameCodeWorktrees reports whether got is want's main worktree first followed by the same pairs in any order.
func sameCodeWorktrees(got, want []fabricengine.CodeWorktree) bool {
	if len(got) != len(want) || len(got) == 0 || got[0] != want[0] || !got[0].Main {
		return false
	}
	byPath := func(a, b fabricengine.CodeWorktree) int { return strings.Compare(a.Path, b.Path) }
	gotPairs, wantPairs := slices.Clone(got[1:]), slices.Clone(want[1:])
	slices.SortFunc(gotPairs, byPath)
	slices.SortFunc(wantPairs, byPath)
	return slices.Equal(gotPairs, wantPairs)
}
