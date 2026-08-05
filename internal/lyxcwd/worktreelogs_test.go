// worktreelogs_test.go tests the WorktreePath-anchored WorktreeLogsDir
// accessor on a hand-built Location — pure path arithmetic, no spawning,
// untagged (Tier 1).

package lyxcwd

import (
	"path/filepath"
	"testing"
)

func TestWorktreeLogsDir(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
	}

	want := filepath.Join(l.WorktreePath(), ".lyx", "logs")
	if got := l.WorktreeLogsDir(); got != want {
		t.Errorf("WorktreeLogsDir() = %q; want %q", got, want)
	}
}

func TestWorktreeLogsDir_IgnoresAnchorRel(t *testing.T) {
	base := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}
	anchored := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from base's to prove the accessor
		// ignores AnchorRel and stays anchored to WorktreePath.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(base.WorktreePath(), ".lyx", "logs")
	if got := anchored.WorktreeLogsDir(); got != want {
		t.Errorf("WorktreeLogsDir() = %q; want %q", got, want)
	}
}
