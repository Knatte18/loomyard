// worktreelogs_test.go tests the WorktreePath-anchored WorktreeLogsDir constructor on a hand-built Location — pure path arithmetic, no spawning, untagged (Tier 1).

package logger_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestWorktreeLogsDir(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
	}

	want := filepath.Join(l.WorktreePath(), ".lyx", "logs")
	if got := logger.WorktreeLogsDir(l); got != want {
		t.Errorf("WorktreeLogsDir(l) = %q; want %q", got, want)
	}
}

func TestWorktreeLogsDir_IgnoresAnchorRel(t *testing.T) {
	base := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}
	anchored := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from base's to prove the constructor
		// ignores AnchorRel and stays anchored to WorktreePath.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(base.WorktreePath(), ".lyx", "logs")
	if got := logger.WorktreeLogsDir(anchored); got != want {
		t.Errorf("WorktreeLogsDir(anchored) = %q; want %q", got, want)
	}
}
