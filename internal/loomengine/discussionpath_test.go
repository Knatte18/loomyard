// discussionpath_test.go tests the AnchorPath-anchored DiscussionDir/
// DiscussionDecisionRecord/DiscussionSupportLog accessors on a hand-built lyxcwd.Location — pure
// path arithmetic, no spawning, untagged (Tier 1).
// It mirrors loomstatus_test.go's construction and AnchorPath-vs-WorktreePath assertion shape.

package loomengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

func TestDiscussionDir(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from "." to prove the accessor
		// follows the anchored subpath, not the bare worktree root.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "discussion")
	if got := DiscussionDir(l); got != want {
		t.Errorf("DiscussionDir() = %q; want %q", got, want)
	}
}

func TestDiscussionDecisionRecord(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "discussion", "decision-record.md")
	if got := DiscussionDecisionRecord(l); got != want {
		t.Errorf("DiscussionDecisionRecord() = %q; want %q", got, want)
	}
}

func TestDiscussionSupportLog(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "discussion", "support-log.md")
	if got := DiscussionSupportLog(l); got != want {
		t.Errorf("DiscussionSupportLog() = %q; want %q", got, want)
	}
}

func TestDiscussionDirRel(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from "." to prove the accessor's
		// relative value composes correctly against DiscussionDir's absolute
		// one.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), DiscussionDirRel())
	if got := DiscussionDir(l); got != want {
		t.Errorf("DiscussionDir() = %q; want %q", got, want)
	}

	if filepath.IsAbs(DiscussionDirRel()) {
		t.Errorf("DiscussionDirRel() = %q; want a relative path", DiscussionDirRel())
	}
}

func TestDiscussionDir_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), lyxdirs.LyxDirName, "discussion")
	if got := DiscussionDir(l); got != want {
		t.Errorf("DiscussionDir() = %q; want %q", got, want)
	}
}
