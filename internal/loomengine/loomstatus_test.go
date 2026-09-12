// loomstatus_test.go tests the AnchorPath-anchored LoomStatusFile/LoomStatusLock accessors on a
// hand-built lyxcwd.Location — pure path arithmetic, no spawning, untagged (Tier 1). It pins the
// scratch-dir split: LoomStatusFile (durable) stays under lyxdirs.LyxDirName, while LoomStatusLock
// (never-tracked) resolves under lyxdirs.DotLyxDirName at the same mirrored subpath, for both an
// unanchored and a subpath-anchored *lyxcwd.Location.

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

	want := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "loom", "status.json")
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

// TestLoomSelfreportFiled proves LoomSelfreportFiled follows the anchored subpath, mirroring
// TestLoomRunLock's pair.
func TestLoomSelfreportFiled(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "selfreport-filed.json")
	if got := LoomSelfreportFiled(l); got != want {
		t.Errorf("LoomSelfreportFiled() = %q; want %q", got, want)
	}
}

// TestLoomSelfreportFiled_UnanchoredEqualsWorktreePath proves LoomSelfreportFiled's AnchorPath
// anchoring coincides with WorktreePath at AnchorRel "." — the same unanchored equivalence
// TestLoomRunLock_UnanchoredEqualsWorktreePath pins for the run lock, but for the selfreport-filed
// marker.
func TestLoomSelfreportFiled_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), lyxdirs.DotLyxDirName, "loom", "selfreport-filed.json")
	if got := LoomSelfreportFiled(l); got != want {
		t.Errorf("LoomSelfreportFiled() = %q; want %q", got, want)
	}
}

// TestLoomSelfreportFiledLock proves LoomSelfreportFiledLock follows the anchored subpath, mirroring
// TestLoomRunLock's pair.
func TestLoomSelfreportFiledLock(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "selfreport-filed.json.lock")
	if got := LoomSelfreportFiledLock(l); got != want {
		t.Errorf("LoomSelfreportFiledLock() = %q; want %q", got, want)
	}
}

// TestLoomSelfreportFiledLock_UnanchoredEqualsWorktreePath proves LoomSelfreportFiledLock's
// AnchorPath anchoring coincides with WorktreePath at AnchorRel "." — the same unanchored
// equivalence TestLoomRunLock_UnanchoredEqualsWorktreePath pins for the run lock, but for the
// selfreport-filed marker's lock.
func TestLoomSelfreportFiledLock_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), lyxdirs.DotLyxDirName, "loom", "selfreport-filed.json.lock")
	if got := LoomSelfreportFiledLock(l); got != want {
		t.Errorf("LoomSelfreportFiledLock() = %q; want %q", got, want)
	}
}

func TestLoomStatusFile_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), lyxdirs.LyxDirName, "loom", "status.json")
	if got := LoomStatusFile(l); got != want {
		t.Errorf("LoomStatusFile() = %q; want %q", got, want)
	}
}

// TestLoomStatusLock_UnanchoredEqualsWorktreePath proves LoomStatusLock's AnchorPath anchoring
// coincides with WorktreePath at AnchorRel "." — the same unanchored equivalence
// TestLoomStatusFile_UnanchoredEqualsWorktreePath pins for the durable file, but for the
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
