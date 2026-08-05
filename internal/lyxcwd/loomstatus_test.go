// loomstatus_test.go tests the AnchorPath-anchored LoomStatusFile/LoomStatusLock
// accessors on a hand-built Location — pure path arithmetic, no spawning, untagged
// (Tier 1).

package lyxcwd

import (
	"path/filepath"
	"testing"
)

func TestLoomStatusFile(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from "." to prove the accessor
		// follows the anchored subpath, not the bare worktree root.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxDirName, "status.json")
	if got := l.LoomStatusFile(); got != want {
		t.Errorf("LoomStatusFile() = %q; want %q", got, want)
	}
}

func TestLoomStatusLock(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxDirName, "status.json.lock")
	if got := l.LoomStatusLock(); got != want {
		t.Errorf("LoomStatusLock() = %q; want %q", got, want)
	}
}

func TestLoomStatusFile_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), lyxDirName, "status.json")
	if got := l.LoomStatusFile(); got != want {
		t.Errorf("LoomStatusFile() = %q; want %q", got, want)
	}
}
