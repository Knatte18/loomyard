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

// TestLogsDir_AnchorsAtAnchorPath pins that LogsDir is rooted at the Location's AnchorPath: for an unanchored Location that equals the worktree path, and for a subpath-anchored one it differs.
//
//testtiming:keep pins LogsDir path arithmetic for unanchored and subpath-anchored Locations without git, which its covering integration scenario reaches only for the unanchored repository root
func TestLogsDir_AnchorsAtAnchorPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		anchorRel          string
		sameAsWorktreePath bool
	}{
		{name: "unanchored equals the worktree-path-based directory", anchorRel: ".", sameAsWorktreePath: true},
		{name: "subpath-anchored differs from the worktree-path-based directory", anchorRel: filepath.Join("sub", "dir"), sameAsWorktreePath: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := &lyxcwd.Location{
				HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
				WorktreeName: "repo",
				AnchorRel:    tt.anchorRel,
			}

			want := filepath.Join(l.AnchorPath(), ".lyx", "logs")
			got := logger.LogsDir(l)
			if got != want {
				t.Errorf("LogsDir(l) = %q; want %q", got, want)
			}
			worktreeBased := filepath.Join(l.WorktreePath(), ".lyx", "logs")
			if (got == worktreeBased) != tt.sameAsWorktreePath {
				t.Errorf("LogsDir(l) = %q equals the WorktreePath-based path %q: %v; want %v", got, worktreeBased, got == worktreeBased, tt.sameAsWorktreePath)
			}
		})
	}
}
