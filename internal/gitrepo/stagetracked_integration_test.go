//go:build integration

// stagetracked_integration_test.go covers StageTrackedChanges and UntrackedFiles against real git repositories, reusing gitrepo_test.go's fixture helpers.

package gitrepo_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestStageTrackedAndUntracked drives StageTrackedChanges and UntrackedFiles through one repository.
// A clean repository first yields an empty, non-nil untracked slice.
// StageTrackedChanges then follows `git add -u` semantics: a modified and a deleted tracked file are
// staged, an untracked one is left alone.
// Last, a new file is listed as untracked while a path matched by .git/info/exclude and a tracked
// file are not.
// The steps run serially in that order and share the repository's state: the last step relies on
// the untracked file the second one creates and leaves untracked.
// The top-level test calls t.Parallel; no step does, because the steps share the repository.
func TestStageTrackedAndUntracked(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "modified.txt", "base\n")
	writeFile(t, dir, "deleted.txt", "base\n")
	commitAll(t, dir, "base")

	if !t.Run("a clean repository yields an empty non-nil untracked slice", func(t *testing.T) {
		got, err := repo.UntrackedFiles()
		if err != nil {
			t.Fatalf("UntrackedFiles() error = %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("UntrackedFiles() = %#v; want empty non-nil slice", got)
		}
	}) {
		return
	}

	if !t.Run("StageTrackedChanges stages a modification and a deletion, not an untracked file", func(t *testing.T) {
		writeFile(t, dir, "modified.txt", "changed\n")
		if err := os.Remove(filepath.Join(dir, "deleted.txt")); err != nil {
			t.Fatalf("Remove(deleted.txt): %v", err)
		}
		writeFile(t, dir, "untracked.txt", "new\n")

		if err := repo.StageTrackedChanges(); err != nil {
			t.Fatalf("StageTrackedChanges() error = %v", err)
		}

		staged := strings.Fields(gitkit.Git(t, dir, "diff", "--cached", "--name-only"))
		slices.Sort(staged)
		if want := []string{"deleted.txt", "modified.txt"}; !slices.Equal(staged, want) {
			t.Errorf("staged = %v; want %v", staged, want)
		}
		if status := gitkit.GitStatusPorcelain(t, dir); !strings.Contains(status, "?? untracked.txt") {
			t.Errorf("git status = %q; want untracked.txt left untracked", status)
		}
	}) {
		return
	}

	t.Run("UntrackedFiles lists a new file and omits an excluded and a tracked one", func(t *testing.T) {
		excludePath := filepath.Join(dir, ".git", "info", "exclude")
		if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(info): %v", err)
		}
		if err := os.WriteFile(excludePath, []byte("excluded.txt\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(exclude): %v", err)
		}
		writeFile(t, dir, "excluded.txt", "x\n")

		got, err := repo.UntrackedFiles()
		if err != nil {
			t.Fatalf("UntrackedFiles() error = %v", err)
		}
		if want := []string{"untracked.txt"}; !slices.Equal(got, want) {
			t.Errorf("UntrackedFiles() = %v; want %v", got, want)
		}
	})
}
