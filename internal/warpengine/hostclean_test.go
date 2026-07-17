//go:build integration

// hostclean_test.go covers HostClean's untracked-is-dirty policy over real,
// lightweight `git init` worktrees (no host+weft pairing needed — HostClean
// only inspects a single worktree's status). The package's testmain_test.go
// already installs the hermetic git TestMain this needs.

package warpengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// newCommittedRepo builds a minimal, committed git repository in a fresh
// t.TempDir() and resolves its Layout, mirroring the lightweight fixture
// pattern used elsewhere in this package's integration tests.
func newCommittedRepo(t *testing.T) *hubgeometry.Layout {
	t.Helper()

	dir := t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init")

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	lyxtest.MustRun(t, dir, "git", "add", "README.md")
	lyxtest.MustRun(t, dir, "git", "commit", "-m", "initial commit")

	l, err := hubgeometry.Resolve(dir)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(%q): %v", dir, err)
	}
	return l
}

// TestHostClean covers the three cleanliness states HostClean must
// distinguish: a fully clean committed worktree, a tracked-modified file,
// and an untracked-only file (the strict-policy case that add.go's
// --untracked-files=no check would miss).
func TestHostClean(t *testing.T) {
	t.Run("Clean", func(t *testing.T) {
		t.Parallel()

		l := newCommittedRepo(t)

		clean, reason, err := HostClean(l)
		if err != nil {
			t.Fatalf("HostClean(): unexpected error: %v", err)
		}
		if !clean {
			t.Errorf("HostClean() = (false, %q); want (true, \"\")", reason)
		}
		if reason != "" {
			t.Errorf("HostClean() reason = %q; want empty", reason)
		}
	})

	t.Run("TrackedModified", func(t *testing.T) {
		t.Parallel()

		l := newCommittedRepo(t)

		readme := filepath.Join(l.WorktreeRoot, "README.md")
		if err := os.WriteFile(readme, []byte("hello, modified\n"), 0o644); err != nil {
			t.Fatalf("modify README.md: %v", err)
		}

		clean, reason, err := HostClean(l)
		if err != nil {
			t.Fatalf("HostClean(): unexpected error: %v", err)
		}
		if clean {
			t.Errorf("HostClean() = (true, \"\"); want dirty due to tracked modification")
		}
		if !strings.Contains(reason, "README.md") {
			t.Errorf("HostClean() reason = %q; want it to mention README.md", reason)
		}
	})

	t.Run("UntrackedOnly", func(t *testing.T) {
		t.Parallel()

		l := newCommittedRepo(t)

		untracked := filepath.Join(l.WorktreeRoot, "scratch.txt")
		if err := os.WriteFile(untracked, []byte("stray\n"), 0o644); err != nil {
			t.Fatalf("write scratch.txt: %v", err)
		}

		clean, reason, err := HostClean(l)
		if err != nil {
			t.Fatalf("HostClean(): unexpected error: %v", err)
		}
		// This is the strict-policy case: an untracked-only file must still be
		// reported as dirty, deliberately stricter than add.go's pre-Add check.
		if clean {
			t.Errorf("HostClean() = (true, \"\"); want dirty due to untracked file (strict policy)")
		}
		if !strings.Contains(reason, "scratch.txt") {
			t.Errorf("HostClean() reason = %q; want it to mention scratch.txt", reason)
		}
	})
}
