//go:build integration

// reset_test.go covers Repo.ResetHard against real git repositories, reusing
// gitrepo_test.go's fixture helpers (newRepo, writeFile, commitAll, runGit).

package gitrepo_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// TestResetHard covers ResetHard against one repository with two commits.
// A well-formed but fabricated hex SHA — one that passes validSHA but names no commit in this repo's history — surfaces as a genuine git failure, not ErrInvalidSHA; ResetHard then restores the earlier commit's file state and moves CurrentSHA back to it.
// The steps run serially in that order: the failed reset must leave the repository untouched for the successful one.
// The top-level test calls t.Parallel; no step does, because the steps share the repository.
func TestResetHard(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "v1")
	commitAll(t, dir, "v1")
	earlier := requireCurrentSHA(t, repo)
	writeFile(t, dir, "a.txt", "v2")
	commitAll(t, dir, "v2")

	if !t.Run("a well-formed sha not in history returns a git failure", func(t *testing.T) {
		err := repo.ResetHard("0123456789abcdef0123456789abcdef01234567")
		if err == nil {
			t.Fatal("ResetHard(fabricated sha) error = nil; want an error")
		}
		if errors.Is(err, gitrepo.ErrInvalidSHA) {
			t.Errorf("ResetHard(fabricated sha) error = %v; want a git-failure error, not ErrInvalidSHA (the sha is well-formed)", err)
		}
	}) {
		return
	}

	t.Run("moves to an earlier commit", func(t *testing.T) {
		if err := repo.ResetHard(earlier); err != nil {
			t.Fatalf("ResetHard(%q) error = %v; want nil", earlier, err)
		}

		if got := requireCurrentSHA(t, repo); got != earlier {
			t.Errorf("CurrentSHA() after ResetHard() = %q; want %q", got, earlier)
		}

		content, err := os.ReadFile(filepath.Join(dir, "a.txt"))
		if err != nil {
			t.Fatalf("read a.txt error = %v", err)
		}
		if string(content) != "v1" {
			t.Errorf("a.txt content after ResetHard() = %q; want %q", content, "v1")
		}
	})
}

// TestResetHard_InvalidSHA_RejectedBeforeGitSpawn asserts that an option-shaped or empty sha is
// rejected with ErrInvalidSHA before any git spawn.
// The check is run against a path with no .git directory at all: if ResetHard spawned git before
// validating sha, the result would be a git failure (not a repository) rather than ErrInvalidSHA,
// so a passing assertion here doubles as proof validation happens first.
func TestResetHard_InvalidSHA_RejectedBeforeGitSpawn(t *testing.T) {
	t.Parallel()

	repo := gitrepo.New(t.TempDir())

	tests := []struct {
		name string
		sha  string
	}{
		{"OptionShaped", "--hard"},
		{"Empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := repo.ResetHard(tt.sha)
			if !errors.Is(err, gitrepo.ErrInvalidSHA) {
				t.Errorf("ResetHard(%q) error = %v; want errors.Is(err, ErrInvalidSHA)", tt.sha, err)
			}
		})
	}
}
