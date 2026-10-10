//go:build integration

// publishfailure_integration_test.go checks the merge-commit field of a Publish failure record against a real scratch repository.

package loomshed

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

func TestCheckedPublishFailure_MergeCommit(t *testing.T) {
	t.Parallel()

	worktree := t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")
	gitkit.Git(t, worktree, "checkout", "-b", "side")
	gitkit.CommitFile(t, worktree, "b.txt", "b\n", "side")
	gitkit.Git(t, worktree, "checkout", "main")
	gitkit.CommitFile(t, worktree, "c.txt", "c\n", "main")
	gitkit.Git(t, worktree, "merge", "--no-ff", "-m", "merge side", "side")
	mergeCommit := gitkit.Git(t, worktree, "rev-parse", "HEAD")
	plainCommit := gitkit.Git(t, worktree, "rev-parse", "HEAD^")

	tests := []struct {
		name string
		sha  string
		want string
	}{
		{"full name of a merge commit is kept", mergeCommit, mergeCommit},
		{"full name of a non-merge commit is dropped", plainCommit, ""},
	}
	for _, tt := range tests {
		paths := verifytree.NewPaths(worktree, t.TempDir())
		writeRecord(t, paths, verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify, MergeCommit: tt.sha})
		got, ok := checkedPublishFailure(paths, worktree)
		if !ok || got.MergeCommit != tt.want {
			t.Errorf("%s: merge commit = %q, kept = %v; want %q", tt.name, got.MergeCommit, ok, tt.want)
		}
	}
}
