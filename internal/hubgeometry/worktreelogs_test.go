// worktreelogs_test.go tests the WorktreeRoot-anchored WorktreeLogsDir
// accessor on a hand-built Layout — pure path arithmetic, no spawning,
// untagged (Tier 1).

package hubgeometry

import (
	"path/filepath"
	"testing"
)

func TestWorktreeLogsDir(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, ".lyx", "logs")
	if got := l.WorktreeLogsDir(); got != want {
		t.Errorf("WorktreeLogsDir() = %q; want %q", got, want)
	}
}

func TestWorktreeLogsDir_CwdEqualsWorktreeRoot(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		Cwd:          filepath.Join("home", "user", "repo"),
	}

	want := filepath.Join(l.WorktreeRoot, ".lyx", "logs")
	if got := l.WorktreeLogsDir(); got != want {
		t.Errorf("WorktreeLogsDir() = %q; want %q", got, want)
	}
}
