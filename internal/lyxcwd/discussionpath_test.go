// discussionpath_test.go tests the AnchorPath-anchored DiscussionDir/
// DiscussionDecisionRecord/DiscussionSupportLog accessors on a hand-built
// Location — pure path arithmetic, no spawning, untagged (Tier 1). It mirrors
// loomstatus_test.go's construction and AnchorPath-vs-WorktreePath assertion shape.

package lyxcwd

import (
	"path/filepath"
	"testing"
)

func TestDiscussionDir(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from "." to prove the accessor
		// follows the anchored subpath, not the bare worktree root.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxDirName, "discussion")
	if got := l.DiscussionDir(); got != want {
		t.Errorf("DiscussionDir() = %q; want %q", got, want)
	}
}

func TestDiscussionDecisionRecord(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxDirName, "discussion", "decision-record.md")
	if got := l.DiscussionDecisionRecord(); got != want {
		t.Errorf("DiscussionDecisionRecord() = %q; want %q", got, want)
	}
}

func TestDiscussionSupportLog(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxDirName, "discussion", "support-log.md")
	if got := l.DiscussionSupportLog(); got != want {
		t.Errorf("DiscussionSupportLog() = %q; want %q", got, want)
	}
}

func TestDiscussionDir_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), lyxDirName, "discussion")
	if got := l.DiscussionDir(); got != want {
		t.Errorf("DiscussionDir() = %q; want %q", got, want)
	}
}
