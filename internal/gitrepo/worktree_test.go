//go:build integration

// worktree_test.go covers WorktreeChangedFiles against real git repositories
// built fresh under t.TempDir(), reusing gitrepo_test.go's newRepo/writeFile/
// commitAll fixture helpers rather than redeclaring them — this file lives in
// the same external package (gitrepo_test) as gitrepo_test.go, so a
// same-named helper here would collide at compile time.

package gitrepo_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

func TestWorktreeChangedFiles_CleanRepo_ReturnsEmpty(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	got, err := repo.WorktreeChangedFiles()
	if err != nil {
		t.Fatalf("WorktreeChangedFiles() error = %v; want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("WorktreeChangedFiles() = %v; want empty on a clean repo", got)
	}
}

// TestWorktreeChangedFiles_ReportsModifiedUntrackedAndStaged asserts the
// three uncommitted-change shapes WorktreeChangedFiles must catch: a
// modification to an already-tracked file, a brand-new untracked file, and a
// separately-staged file — all reported together, none more than once.
func TestWorktreeChangedFiles_ReportsModifiedUntrackedAndStaged(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	writeFile(t, dir, "b.txt", "initial")
	commitAll(t, dir, "init")

	writeFile(t, dir, "a.txt", "changed")
	writeFile(t, dir, "b.txt", "changed")
	lyxtest.MustRun(t, dir, "git", "add", "b.txt")
	writeFile(t, dir, "c.txt", "new file")

	got, err := repo.WorktreeChangedFiles()
	if err != nil {
		t.Fatalf("WorktreeChangedFiles() error = %v; want nil", err)
	}

	want := map[string]bool{"a.txt": true, "b.txt": true, "c.txt": true}
	if len(got) != len(want) {
		t.Fatalf("WorktreeChangedFiles() = %v; want exactly %v", got, want)
	}
	for _, f := range got {
		if !want[f] {
			t.Errorf("WorktreeChangedFiles() contains unexpected file %q", f)
		}
	}
}
