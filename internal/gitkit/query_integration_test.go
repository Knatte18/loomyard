//go:build integration

package gitkit

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestGit_ReturnsTrimmedStdout(t *testing.T) {
	t.Parallel()

	fx := CopyRepo(t)
	if got := Git(t, fx.Repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("Git rev-parse --abbrev-ref HEAD = %q; want main", got)
	}
}

func TestQueryHelpers_BranchesAndAncestry(t *testing.T) {
	t.Parallel()

	repo := CopyRepo(t).Repo
	base := RevParse(t, repo, "HEAD")

	if got := CurrentBranch(t, repo); got != "main" {
		t.Errorf("CurrentBranch = %q; want main", got)
	}
	if !BranchExists(t, repo, "main") {
		t.Error("BranchExists(main) = false; want true")
	}
	if BranchExists(t, repo, "missing") {
		t.Error("BranchExists(missing) = true; want false")
	}

	Git(t, repo, "branch", "side")
	tip := CommitFileOnBranch(t, repo, "side", "a/b.txt", "x", "side commit")

	if got := CurrentBranch(t, repo); got != "side" {
		t.Errorf("CurrentBranch after CommitFileOnBranch = %q; want side", got)
	}
	if !IsAncestor(t, repo, base, tip) {
		t.Error("IsAncestor(base, tip) = false; want true")
	}
	if IsAncestor(t, repo, tip, base) {
		t.Error("IsAncestor(tip, base) = true; want false")
	}
}

func TestQueryHelpers_CommitFile(t *testing.T) {
	t.Parallel()

	repo := CopyRepo(t).Repo
	base := RevParse(t, repo, "HEAD")

	sha := CommitFile(t, repo, "dir/file.txt", "content", "add file")
	if got := RevParse(t, repo, "HEAD"); got != sha {
		t.Errorf("RevParse(HEAD) = %q; CommitFile returned %q", got, sha)
	}
	if got := RevListCount(t, repo, base+"..HEAD"); got != 1 {
		t.Errorf("RevListCount(base..HEAD) = %d; want 1", got)
	}
	if got := LsFiles(t, repo, "dir"); !slices.Equal(got, []string{"dir/file.txt"}) {
		t.Errorf("LsFiles(dir) = %v; want [dir/file.txt]", got)
	}
}

func TestExcludeLines_LinkedWorktree(t *testing.T) {
	t.Parallel()

	repo := CopyRepo(t).Repo
	wt := filepath.Join(t.TempDir(), "wt")
	Git(t, repo, "worktree", "add", "-b", "linked", wt)

	infoDir := filepath.Join(repo, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", infoDir, err)
	}
	if err := os.WriteFile(filepath.Join(infoDir, "exclude"), []byte("one\n\ntwo\n"), 0o644); err != nil {
		t.Fatalf("write exclude: %v", err)
	}

	if got := ExcludeLines(t, wt); !slices.Equal(got, []string{"one", "two"}) {
		t.Errorf("ExcludeLines(linked worktree) = %v; want [one two]", got)
	}
}
