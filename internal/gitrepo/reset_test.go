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

// TestResetHard_MovesToEarlierCommit asserts the ordinary case: resetting to
// an earlier commit's SHA restores that commit's file state on disk and
// moves CurrentSHA back to it.
func TestResetHard_MovesToEarlierCommit(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "v1")
	commitAll(t, dir, "v1")
	earlier, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	writeFile(t, dir, "a.txt", "v2")
	commitAll(t, dir, "v2")

	if err := repo.ResetHard(earlier); err != nil {
		t.Fatalf("ResetHard(%q) error = %v; want nil", earlier, err)
	}

	got, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if got != earlier {
		t.Errorf("CurrentSHA() after ResetHard() = %q; want %q", got, earlier)
	}

	content, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt error = %v", err)
	}
	if string(content) != "v1" {
		t.Errorf("a.txt content after ResetHard() = %q; want %q", content, "v1")
	}
}

// TestResetHard_InvalidSHA_RejectedBeforeGitSpawn asserts that an
// option-shaped or empty sha is rejected with ErrInvalidSHA before any git
// spawn. The check is run against a path with no .git directory at all: if
// ResetHard spawned git before validating sha, the result would be a git
// failure (not a repository) rather than ErrInvalidSHA, so a passing
// assertion here doubles as proof validation happens first.
func TestResetHard_InvalidSHA_RejectedBeforeGitSpawn(t *testing.T) {
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
			err := repo.ResetHard(tt.sha)
			if !errors.Is(err, gitrepo.ErrInvalidSHA) {
				t.Errorf("ResetHard(%q) error = %v; want errors.Is(err, ErrInvalidSHA)", tt.sha, err)
			}
		})
	}
}

// TestResetHard_WellFormedSHANotInHistory_ReturnsError asserts that a
// well-formed but fabricated hex SHA — one that passes validSHA but names no
// commit in this repo's history — surfaces as a genuine git failure, not
// ErrInvalidSHA.
func TestResetHard_WellFormedSHANotInHistory_ReturnsError(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "v1")
	commitAll(t, dir, "v1")

	err := repo.ResetHard("0123456789abcdef0123456789abcdef01234567")
	if err == nil {
		t.Fatal("ResetHard(fabricated sha) error = nil; want an error")
	}
	if errors.Is(err, gitrepo.ErrInvalidSHA) {
		t.Errorf("ResetHard(fabricated sha) error = %v; want a git-failure error, not ErrInvalidSHA (the sha is well-formed)", err)
	}
}
