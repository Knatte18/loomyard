// discussionpath_test.go tests the WorktreeRoot-anchored DiscussionDir/
// DiscussionDecisionRecord/DiscussionSupportLog accessors on a hand-built
// Layout — pure path arithmetic, no spawning, untagged (Tier 1). It mirrors
// loomstatus_test.go's construction and WorktreeRoot-vs-Cwd assertion shape.

package hubgeometry

import (
	"path/filepath"
	"testing"
)

func TestDiscussionDir(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, LyxDirName, "discussion")
	if got := l.DiscussionDir(); got != want {
		t.Errorf("DiscussionDir() = %q; want %q", got, want)
	}
}

func TestDiscussionDecisionRecord(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, LyxDirName, "discussion", "decision-record.md")
	if got := l.DiscussionDecisionRecord(); got != want {
		t.Errorf("DiscussionDecisionRecord() = %q; want %q", got, want)
	}
}

func TestDiscussionSupportLog(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, LyxDirName, "discussion", "support-log.md")
	if got := l.DiscussionSupportLog(); got != want {
		t.Errorf("DiscussionSupportLog() = %q; want %q", got, want)
	}
}

func TestDiscussionDir_CwdEqualsWorktreeRoot(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		Cwd:          filepath.Join("home", "user", "repo"),
	}

	want := filepath.Join(l.WorktreeRoot, LyxDirName, "discussion")
	if got := l.DiscussionDir(); got != want {
		t.Errorf("DiscussionDir() = %q; want %q", got, want)
	}
}
