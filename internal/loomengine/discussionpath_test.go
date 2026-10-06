// discussionpath_test.go tests the AnchorPath-anchored DiscussionDir/
// DiscussionDecisionRecord/DiscussionSupportLog accessors on a hand-built lyxcwd.Location — pure
// path arithmetic, no spawning, untagged (Tier 1).

package loomengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestDiscussionPaths pins the discussion directory, the decision record and the support log at the
// anchor, and that DiscussionDirRel is the relative form DiscussionDir composes from.
// The anchored row's AnchorRel differs from "." to prove the accessors follow the anchored subpath,
// not the bare worktree root; the unanchored row's anchor path equals the worktree path.
//
//testtiming:keep pins the exact discussion directory, decision record and support log paths at an anchored and an unanchored location, which the covering spec tests only pass through unasserted
func TestDiscussionPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		anchorRel string
	}{
		{"anchored", filepath.Join("sub", "dir")},
		{"unanchored", "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := &lyxcwd.Location{
				HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
				WorktreeName: "repo",
				AnchorRel:    tt.anchorRel,
			}

			wantDir := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "discussion")
			if got := DiscussionDir(l); got != wantDir {
				t.Errorf("DiscussionDir() = %q; want %q", got, wantDir)
			}
			if tt.anchorRel == "." {
				if want := filepath.Join(l.WorktreePath(), lyxdirs.LyxDirName, "discussion"); DiscussionDir(l) != want {
					t.Errorf("DiscussionDir() = %q; want the worktree-rooted %q for an unanchored location", DiscussionDir(l), want)
				}
			}
			if got, want := DiscussionDecisionRecord(l), filepath.Join(wantDir, "decision-record.md"); got != want {
				t.Errorf("DiscussionDecisionRecord() = %q; want %q", got, want)
			}
			if got, want := DiscussionSupportLog(l), filepath.Join(wantDir, "support-log.md"); got != want {
				t.Errorf("DiscussionSupportLog() = %q; want %q", got, want)
			}
			if got, want := DiscussionDir(l), filepath.Join(l.AnchorPath(), DiscussionDirRel()); got != want {
				t.Errorf("DiscussionDir() = %q; want it to equal the anchor joined with DiscussionDirRel(), %q", got, want)
			}
			if filepath.IsAbs(DiscussionDirRel()) {
				t.Errorf("DiscussionDirRel() = %q; want a relative path", DiscussionDirRel())
			}
		})
	}
}
