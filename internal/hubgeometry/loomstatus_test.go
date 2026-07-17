// loomstatus_test.go tests the WorktreeRoot-anchored LoomStatusFile/LoomStatusLock
// accessors on a hand-built Layout — pure path arithmetic, no spawning, untagged
// (Tier 1).

package hubgeometry

import (
	"path/filepath"
	"testing"
)

func TestLoomStatusFile(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, LyxDirName, "status.json")
	if got := l.LoomStatusFile(); got != want {
		t.Errorf("LoomStatusFile() = %q; want %q", got, want)
	}
}

func TestLoomStatusLock(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, LyxDirName, "status.json.lock")
	if got := l.LoomStatusLock(); got != want {
		t.Errorf("LoomStatusLock() = %q; want %q", got, want)
	}
}

func TestLoomStatusFile_CwdEqualsWorktreeRoot(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		Cwd:          filepath.Join("home", "user", "repo"),
	}

	want := filepath.Join(l.WorktreeRoot, LyxDirName, "status.json")
	if got := l.LoomStatusFile(); got != want {
		t.Errorf("LoomStatusFile() = %q; want %q", got, want)
	}
}
