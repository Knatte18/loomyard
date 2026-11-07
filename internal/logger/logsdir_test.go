// logsdir_test.go tests that LogsDir is rooted at the Location's AnchorPath.
// It builds hand-made Locations and does pure path arithmetic, without spawning a process.

package logger_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLogsDir_AnchorsAtAnchorPath pins that LogsDir is rooted at the Location's AnchorPath.
// For an unanchored Location that is the worktree path; for a subpath-anchored one it differs.
//
//testtiming:keep pins LogsDir path arithmetic for unanchored and subpath-anchored Locations without git, which its covering integration scenario reaches only for the unanchored repository root
func TestLogsDir_AnchorsAtAnchorPath(t *testing.T) {
	tiergate.Require(t, 0)
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
			l := &lymcwd.Location{
				HubPath:      filepath.Join("home", "user", "repo-LYMHUB"),
				WorktreeName: "repo",
				AnchorRel:    tt.anchorRel,
			}

			want := filepath.Join(l.AnchorPath(), ".lym", "logs")
			got := logger.LogsDir(l)
			if got != want {
				t.Errorf("LogsDir(l) = %q; want %q", got, want)
			}
			if atAnchor := logger.LogsDirAt(l.AnchorPath()); got != atAnchor {
				t.Errorf("LogsDir(l) = %q; want LogsDirAt(l.AnchorPath()) = %q", got, atAnchor)
			}
			worktreeBased := filepath.Join(l.WorktreePath(), ".lym", "logs")
			if (got == worktreeBased) != tt.sameAsWorktreePath {
				t.Errorf("LogsDir(l) = %q equals the WorktreePath-based path %q: %v; want %v", got, worktreeBased, got == worktreeBased, tt.sameAsWorktreePath)
			}
		})
	}
}
