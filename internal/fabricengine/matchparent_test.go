// matchparent_test.go covers matchParentBranch, the pure worktree-entry matcher OpenParent's
// resolution chain builds on — no fixture needed, since the function takes a hand-built
// []fabricengine.WorktreeEntry slice.

package fabricengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestMatchParentBranch covers matchParentBranch's matching, skipping, and normalization behavior
// over hand-built WorktreeEntry slices.
func TestMatchParentBranch(t *testing.T) {
	tests := []struct {
		name         string
		entries      []fabricengine.WorktreeEntry
		parentBranch string
		wantPath     string
		wantOK       bool
	}{
		{
			name: "SingleMatch",
			entries: []fabricengine.WorktreeEntry{
				{Path: "/hub/wt1", Branch: "main"},
			},
			parentBranch: "main",
			wantPath:     "/hub/wt1",
			wantOK:       true,
		},
		{
			name: "NoMatch",
			entries: []fabricengine.WorktreeEntry{
				{Path: "/hub/wt1", Branch: "other"},
			},
			parentBranch: "main",
			wantPath:     "",
			wantOK:       false,
		},
		{
			name: "DetachedCandidateNoPanic",
			entries: []fabricengine.WorktreeEntry{
				{Path: "/hub/wt1", Branch: "(detached)"},
			},
			parentBranch: "main",
			wantPath:     "",
			wantOK:       false,
		},
		{
			// matchParentBranch special-cases nothing: a match whose Branch equals the given
			// parentBranch still matches even though nothing here distinguishes it as the
			// "acting worktree's own" branch. That refusal is loom policy, not fabric policy
			// (self-parent-is-loom-policy-not-fabric-policy), so a future edit must not "fix"
			// this case into an error.
			name: "SelfParentBranchStillMatches",
			entries: []fabricengine.WorktreeEntry{
				{Path: "/hub/wt-self", Branch: "task-branch"},
			},
			parentBranch: "task-branch",
			wantPath:     "/hub/wt-self",
			wantOK:       true,
		},
		{
			name: "ForwardSlashPathNormalized",
			entries: []fabricengine.WorktreeEntry{
				{Path: "C:/hub/wt1", Branch: "main"},
			},
			parentBranch: "main",
			wantPath:     filepath.FromSlash("C:/hub/wt1"),
			wantOK:       true,
		},
		{
			name: "MainFieldPlaysNoRole",
			entries: []fabricengine.WorktreeEntry{
				{Path: "/hub/wt-main", Branch: "other", Main: true},
				{Path: "/hub/wt-match", Branch: "main", Main: false},
			},
			parentBranch: "main",
			wantPath:     "/hub/wt-match",
			wantOK:       true,
		},
		{
			name: "PrunableEntryNeverMatches",
			entries: []fabricengine.WorktreeEntry{
				{Path: "/hub/wt1", Branch: "main", Prunable: true},
			},
			parentBranch: "main",
			wantPath:     "",
			wantOK:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotOK := fabricengine.MatchParentBranchForTest(tt.entries, tt.parentBranch)
			if gotPath != tt.wantPath || gotOK != tt.wantOK {
				t.Errorf("matchParentBranch(%v, %q) = (%q, %v); want (%q, %v)",
					tt.entries, tt.parentBranch, gotPath, gotOK, tt.wantPath, tt.wantOK)
			}
		})
	}
}
