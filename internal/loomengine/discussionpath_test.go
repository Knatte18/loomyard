// discussionpath_test.go tests the AnchorPath-anchored DiscussionDir/ DiscussionDecisionRecord/DiscussionSupportLog accessors on a hand-built lyxcwd.Location — pure path arithmetic, no spawning, untagged (Tier 1).
// It mirrors loomstatus_test.go's construction and AnchorPath-vs-WorktreePath assertion shape.

package loomengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestDiscussionDir(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from "." to prove the accessor
		// follows the anchored subpath, not the bare worktree root.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), configengine.LyxDirName, "discussion")
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

	want := filepath.Join(l.AnchorPath(), configengine.LyxDirName, "discussion", "decision-record.md")
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

	want := filepath.Join(l.AnchorPath(), configengine.LyxDirName, "discussion", "support-log.md")
	if got := DiscussionSupportLog(l); got != want {
		t.Errorf("DiscussionSupportLog() = %q; want %q", got, want)
	}
}

func TestDiscussionDir_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), configengine.LyxDirName, "discussion")
	if got := DiscussionDir(l); got != want {
		t.Errorf("DiscussionDir() = %q; want %q", got, want)
	}
}
