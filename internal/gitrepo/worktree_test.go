//go:build tier1

// worktree_test.go covers WorktreeChangedFiles against a real git repository built under t.TempDir(), reusing gitrepo_test.go's newRepo/writeFile/ commitAll fixture helpers rather than redeclaring them — this file lives in the same external package (gitrepo_test) as gitrepo_test.go, so a same-named helper here would collide at compile time.

package gitrepo_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestWorktreeChangedFiles covers WorktreeChangedFiles over one repository: a clean one reports nothing, then the three uncommitted-change shapes it must catch — a modification to an already-tracked file, a brand-new untracked file, and a separately-staged file — are all reported together, none more than once.
// The steps run serially in that order and share the repository's state.
// The top-level test calls t.Parallel; no step does, because the steps share the repository.
func TestWorktreeChangedFiles(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	writeFile(t, dir, "b.txt", "initial")
	commitAll(t, dir, "init")

	if !t.Run("a clean repo returns empty", func(t *testing.T) {
		got, err := repo.WorktreeChangedFiles()
		if err != nil {
			t.Fatalf("WorktreeChangedFiles() error = %v; want nil", err)
		}
		if len(got) != 0 {
			t.Errorf("WorktreeChangedFiles() = %v; want empty on a clean repo", got)
		}
	}) {
		return
	}

	t.Run("reports modified, untracked and staged files together", func(t *testing.T) {
		writeFile(t, dir, "a.txt", "changed")
		writeFile(t, dir, "b.txt", "changed")
		gitkit.MustRun(t, dir, "git", "add", "b.txt")
		writeFile(t, dir, "c.txt", "new file")

		got, err := repo.WorktreeChangedFiles()
		if err != nil {
			t.Fatalf("WorktreeChangedFiles() error = %v; want nil", err)
		}
		requireSameFiles(t, "WorktreeChangedFiles()", got, "a.txt", "b.txt", "c.txt")
	})
}

// TestTrackedRootEntries covers TrackedRootEntries: committed and staged-only root entries count, an untracked one does not, and an absent index is an empty result.
func TestTrackedRootEntries(t *testing.T) {
	t.Parallel()

	t.Run("committed and staged roots are tracked and an untracked one is not", func(t *testing.T) {
		t.Parallel()
		dir, repo := newRepo(t)
		writeFile(t, dir, "top.md", "x")
		if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, dir, "a/x.go", "x")
		commitAll(t, dir, "init")
		for _, name := range []string{"b", "c"} {
			if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		writeFile(t, dir, "b/y.go", "y")
		if _, _, code, err := runGit(t, dir, "add", "b/y.go"); err != nil || code != 0 {
			t.Fatalf("git add: code %d, err %v", code, err)
		}
		writeFile(t, dir, "c/z.go", "z")

		got, err := repo.TrackedRootEntries()
		if want := []string{"a", "b", "top.md"}; err != nil || !slices.Equal(got, want) {
			t.Errorf("TrackedRootEntries() = %v, %v; want %v", got, err, want)
		}
	})

	t.Run("an empty index is an empty result", func(t *testing.T) {
		t.Parallel()
		_, repo := newRepo(t)
		got, err := repo.TrackedRootEntries()
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("TrackedRootEntries() = %#v, %v; want an empty non-nil result and no error", got, err)
		}
	})
}
