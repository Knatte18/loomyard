// loomstatus_test.go tests the AnchorPath-anchored LoomSelfreportFiled/LoomSelfreportFiledLock
// accessors on a hand-built lyxcwd.Location — pure path arithmetic, no spawning, untagged (Tier 1).
// It pins the scratch-dir split: LoomSelfreportFiled (durable-adjacent marker) and
// LoomSelfreportFiledLock (never-tracked lock) both resolve under lyxdirs.DotLyxDirName at the same
// mirrored subpath, for both an unanchored and a subpath-anchored *lyxcwd.Location.
//
// loom's own status.json and its two locks (status.json.lock, run.lock) moved onto
// internal/shedrun's run directory: internal/shedrun/paths_test.go already covers
// shedrun.StatusFile/StatusLock/RunLock, so this file no longer pins them.

package loomengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestLoomSelfreportFiled proves LoomSelfreportFiled follows the anchored subpath, mirroring
// TestLoomSelfreportFiledLock's pair.
func TestLoomSelfreportFiled(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "selfreport-filed.json")
	if got := LoomSelfreportFiled(l); got != want {
		t.Errorf("LoomSelfreportFiled() = %q; want %q", got, want)
	}
}

// TestLoomSelfreportFiled_UnanchoredEqualsWorktreePath proves LoomSelfreportFiled's AnchorPath
// anchoring coincides with WorktreePath at AnchorRel "." -- the same unanchored equivalence
// TestLoomSelfreportFiledLock_UnanchoredEqualsWorktreePath pins for the marker's lock.
func TestLoomSelfreportFiled_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), lyxdirs.DotLyxDirName, "loom", "selfreport-filed.json")
	if got := LoomSelfreportFiled(l); got != want {
		t.Errorf("LoomSelfreportFiled() = %q; want %q", got, want)
	}
}

// TestLoomSelfreportFiledLock proves LoomSelfreportFiledLock follows the anchored subpath,
// mirroring TestLoomSelfreportFiled's pair.
func TestLoomSelfreportFiledLock(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "selfreport-filed.json.lock")
	if got := LoomSelfreportFiledLock(l); got != want {
		t.Errorf("LoomSelfreportFiledLock() = %q; want %q", got, want)
	}
}

// TestLoomSelfreportFiledLock_UnanchoredEqualsWorktreePath proves LoomSelfreportFiledLock's
// AnchorPath anchoring coincides with WorktreePath at AnchorRel "." -- the same unanchored
// equivalence TestLoomSelfreportFiled_UnanchoredEqualsWorktreePath pins for the marker itself.
func TestLoomSelfreportFiledLock_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), lyxdirs.DotLyxDirName, "loom", "selfreport-filed.json.lock")
	if got := LoomSelfreportFiledLock(l); got != want {
		t.Errorf("LoomSelfreportFiledLock() = %q; want %q", got, want)
	}
}
