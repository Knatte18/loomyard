//go:build integration

// reset_test.go covers Repo.ResetHard and Repo.ResetKeep against real git repositories, reusing
// gitrepo_test.go's fixture helpers (newRepo, writeFile, commitAll, runGit).

package gitrepo_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
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

// TestResetKeep covers ResetKeep against one repository with two commits.
// An invalid sha is rejected before any git spawn, checked against a path with no .git directory;
// a move back over a committed change keeps an unrelated uncommitted file;
// and a move that would rewrite an uncommitted path is refused with git's error, leaving HEAD and that file untouched.
// The steps run serially in that order, each starting from the one before's state.
// The top-level test calls t.Parallel; no step does, because the steps share the repository.
func TestResetKeep(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "v1")
	writeFile(t, dir, "b.txt", "b1")
	commitAll(t, dir, "v1")
	earlier := requireCurrentSHA(t, repo)
	writeFile(t, dir, "a.txt", "v2")
	commitAll(t, dir, "v2")
	later := requireCurrentSHA(t, repo)

	readFile := func(t *testing.T, name string) string {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s error = %v", name, err)
		}
		return string(content)
	}

	t.Run("an invalid sha is rejected before any git spawn", func(t *testing.T) {
		unversioned := gitrepo.New(t.TempDir())
		for _, sha := range []string{"--keep", ""} {
			if err := unversioned.ResetKeep(sha); !errors.Is(err, gitrepo.ErrInvalidSHA) {
				t.Errorf("ResetKeep(%q) error = %v; want errors.Is(err, ErrInvalidSHA)", sha, err)
			}
		}
	})

	if !t.Run("a move back keeps an unrelated uncommitted file", func(t *testing.T) {
		writeFile(t, dir, "b.txt", "dirty")

		if err := repo.ResetKeep(earlier); err != nil {
			t.Fatalf("ResetKeep(%q) error = %v; want nil", earlier, err)
		}

		if got := requireCurrentSHA(t, repo); got != earlier {
			t.Errorf("CurrentSHA() after ResetKeep() = %q; want %q", got, earlier)
		}
		if got := readFile(t, "a.txt"); got != "v1" {
			t.Errorf("a.txt after ResetKeep() = %q; want %q", got, "v1")
		}
		if got := readFile(t, "b.txt"); got != "dirty" {
			t.Errorf("b.txt after ResetKeep() = %q; want the uncommitted %q kept", got, "dirty")
		}
	}) {
		return
	}

	t.Run("a move over an uncommitted path is refused and changes nothing", func(t *testing.T) {
		if err := repo.ResetKeep(later); err != nil {
			t.Fatalf("ResetKeep(%q) error = %v; want nil", later, err)
		}
		writeFile(t, dir, "a.txt", "dirty-a")

		err := repo.ResetKeep(earlier)

		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) {
			t.Fatalf("ResetKeep(%q) over a dirty a.txt error = %v; want a wrapped *gitexec.GitError", earlier, err)
		}
		if got := requireCurrentSHA(t, repo); got != later {
			t.Errorf("CurrentSHA() after a refused ResetKeep() = %q; want it unmoved at %q", got, later)
		}
		if got := readFile(t, "a.txt"); got != "dirty-a" {
			t.Errorf("a.txt after a refused ResetKeep() = %q; want the uncommitted %q untouched", got, "dirty-a")
		}
	})
}
