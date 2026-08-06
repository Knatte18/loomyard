// loomstatus_test.go tests the AnchorPath-anchored LoomStatusFile/LoomStatusLock accessors on a
// hand-built lyxcwd.Location — pure path arithmetic, no spawning, untagged (Tier 1).

package loomengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestLoomStatusFile(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from "." to prove the accessor
		// follows the anchored subpath, not the bare worktree root.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), configengine.LyxDirName, "status.json")
	if got := LoomStatusFile(l); got != want {
		t.Errorf("LoomStatusFile() = %q; want %q", got, want)
	}
}

func TestLoomStatusLock(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), configengine.LyxDirName, "status.json.lock")
	if got := LoomStatusLock(l); got != want {
		t.Errorf("LoomStatusLock() = %q; want %q", got, want)
	}
}

func TestLoomStatusFile_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), configengine.LyxDirName, "status.json")
	if got := LoomStatusFile(l); got != want {
		t.Errorf("LoomStatusFile() = %q; want %q", got, want)
	}
}
