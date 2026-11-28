// worktreelist_parse_test.go covers, with no git, how worktree records become WorktreeEntry values:
// the `git worktree list --porcelain` parser and the mapping from dotgit's worktrees.

package fabricengine

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestParseWorktreePorcelain covers the porcelain parser.
// The first record is Main, and a prunable line with a reason marks the entry prunable.
// A detached record gets the detached branch name, and a bare record is an error.
func TestParseWorktreePorcelain(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		out     string
		want    []WorktreeEntry
		wantErr string
	}{
		{
			name: "SingleWorktreeIsMain",
			out:  "worktree /hub/main\nHEAD aaa\nbranch refs/heads/main\n\n",
			want: []WorktreeEntry{{Path: "/hub/main", Head: "aaa", Branch: "main", Main: true}},
		},
		{
			name: "LaterWorktreesAreNotMain",
			out:  "worktree /hub/main\nHEAD aaa\nbranch refs/heads/main\n\nworktree /hub/wt1\nHEAD bbb\nbranch refs/heads/feature/x\n",
			want: []WorktreeEntry{
				{Path: "/hub/main", Head: "aaa", Branch: "main", Main: true},
				{Path: "/hub/wt1", Head: "bbb", Branch: "feature/x"},
			},
		},
		{
			name: "DetachedHead",
			out:  "worktree /hub/main\nHEAD aaa\nbranch refs/heads/main\n\nworktree /hub/wt1\nHEAD bbb\ndetached\n",
			want: []WorktreeEntry{
				{Path: "/hub/main", Head: "aaa", Branch: "main", Main: true},
				{Path: "/hub/wt1", Head: "bbb", Branch: detachedBranch},
			},
		},
		{
			name: "PrunableWithReason",
			out:  "worktree /hub/main\nHEAD aaa\nbranch refs/heads/main\n\nworktree /hub/gone\nHEAD bbb\nbranch refs/heads/gone\nprunable gitdir file points to non-existent location\n",
			want: []WorktreeEntry{
				{Path: "/hub/main", Head: "aaa", Branch: "main", Main: true},
				{Path: "/hub/gone", Head: "bbb", Branch: "gone", Prunable: true},
			},
		},
		{
			name: "EmptyOutputListsNothing",
			out:  "",
			want: nil,
		},
		{
			name:    "BareRepositoryIsRejected",
			out:     "worktree /hub/bare.git\nbare\n\n",
			wantErr: "bare",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseWorktreePorcelain(tt.out)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseWorktreePorcelain(%q) error = %v; want substring %q", tt.out, err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("parseWorktreePorcelain(%q) entries = %v; want nil", tt.out, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWorktreePorcelain(%q) error = %v; want nil", tt.out, err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parseWorktreePorcelain(%q) mismatch (-want +got):\n%s", tt.out, diff)
			}
		})
	}
}

// TestEntriesFromWorktrees covers the mapping from dotgit's worktrees:
// the first worktree is Main, a branch loses its refs/heads/ prefix and an empty branch reads as detached.
func TestEntriesFromWorktrees(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		worktrees []dotgit.Worktree
		want      []WorktreeEntry
	}{
		{
			name:      "SingleWorktreeIsMain",
			worktrees: []dotgit.Worktree{{Path: "/hub/main", Head: "aaa", Branch: "refs/heads/main"}},
			want:      []WorktreeEntry{{Path: "/hub/main", Head: "aaa", Branch: "main", Main: true}},
		},
		{
			name: "LaterWorktreesAreNotMainAndCarryPrunable",
			worktrees: []dotgit.Worktree{
				{Path: "/hub/main", Head: "aaa", Branch: "refs/heads/main"},
				{Path: "/hub/gone", Head: "bbb", Branch: "refs/heads/feature/x", Prunable: true},
			},
			want: []WorktreeEntry{
				{Path: "/hub/main", Head: "aaa", Branch: "main", Main: true},
				{Path: "/hub/gone", Head: "bbb", Branch: "feature/x", Prunable: true},
			},
		},
		{
			name:      "EmptyBranchIsDetached",
			worktrees: []dotgit.Worktree{{Path: "/hub/main", Head: "aaa", Branch: "refs/heads/main"}, {Path: "/hub/wt1", Head: "bbb"}},
			want: []WorktreeEntry{
				{Path: "/hub/main", Head: "aaa", Branch: "main", Main: true},
				{Path: "/hub/wt1", Head: "bbb", Branch: detachedBranch},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(tt.want, entriesFromWorktrees(tt.worktrees)); diff != "" {
				t.Errorf("entriesFromWorktrees() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
