// loomstatus_test.go tests the AnchorPath-anchored LoomStatusFile/LoomStatusLock accessors on a
// hand-built lyxcwd.Location — pure path arithmetic, no spawning, untagged (Tier 1). It pins every
// loom status-directory accessor as a never-tracked transient under lyxdirs.DotLyxDirName, for both
// an unanchored and a subpath-anchored *lyxcwd.Location.

package loomengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

func TestLoomStatusFile(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from "." to prove the accessor
		// follows the anchored subpath, not the bare worktree root.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "status.json")
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

	want := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "status.json.lock")
	if got := LoomStatusLock(l); got != want {
		t.Errorf("LoomStatusLock() = %q; want %q", got, want)
	}
}

func TestLoomRunLock(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "run.lock")
	if got := LoomRunLock(l); got != want {
		t.Errorf("LoomRunLock() = %q; want %q", got, want)
	}
}

func TestLoomStatusFile_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), lyxdirs.DotLyxDirName, "loom", "status.json")
	if got := LoomStatusFile(l); got != want {
		t.Errorf("LoomStatusFile() = %q; want %q", got, want)
	}
}

// TestLoomStatusLock_UnanchoredEqualsWorktreePath proves LoomStatusLock's AnchorPath anchoring
// coincides with WorktreePath at AnchorRel "." — the same unanchored equivalence
// TestLoomStatusFile_UnanchoredEqualsWorktreePath pins for the status file, but for its own
// never-tracked .lyx sibling.
func TestLoomStatusLock_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), lyxdirs.DotLyxDirName, "loom", "status.json.lock")
	if got := LoomStatusLock(l); got != want {
		t.Errorf("LoomStatusLock() = %q; want %q", got, want)
	}
}

// TestLoomRunLock_UnanchoredEqualsWorktreePath proves LoomRunLock's AnchorPath anchoring
// coincides with WorktreePath at AnchorRel "." — the same unanchored equivalence
// TestLoomStatusLock_UnanchoredEqualsWorktreePath pins for the status lock, but for the run lock.
func TestLoomRunLock_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), lyxdirs.DotLyxDirName, "loom", "run.lock")
	if got := LoomRunLock(l); got != want {
		t.Errorf("LoomRunLock() = %q; want %q", got, want)
	}
}
