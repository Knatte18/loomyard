// logsdir_test.go tests the AnchorPath-anchored LogsDir constructor on hand-built Locations — pure
// path arithmetic, no spawning, untagged (Tier 1).
// It replaces worktreelogs_test.go: every assertion there pinned the old WorktreePath-anchored
// behaviour, which this batch inverts.

package logger_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestLogsDir_UnanchoredEqualsWorktreePathBased(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), ".lyx", "logs")
	if got := logger.LogsDir(l); got != want {
		t.Errorf("LogsDir(l) = %q; want %q", got, want)
	}
}

func TestLogsDir_SubpathAnchoredDiffersFromWorktreePathBased(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), ".lyx", "logs")
	if got := logger.LogsDir(l); got != want {
		t.Errorf("LogsDir(l) = %q; want %q", got, want)
	}

	worktreeBased := filepath.Join(l.WorktreePath(), ".lyx", "logs")
	if got := logger.LogsDir(l); got == worktreeBased {
		t.Errorf("LogsDir(l) = %q; want it to differ from the WorktreePath-based path %q for a subpath-anchored Location", got, worktreeBased)
	}
}
