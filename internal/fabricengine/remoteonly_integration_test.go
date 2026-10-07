//go:build integration

// remoteonly_integration_test.go proves Fabric.RemoteOnlyCommits reports the remote task branch's tip and the commits the warp HEAD lacks over a real hub with its origin.

package fabricengine_test

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestFabricRemoteOnlyCommits covers the three answers over the prime warp worktree: a remote branch ahead of HEAD, a remote branch at or behind HEAD, and no remote branch or no remote at all.
func TestFabricRemoteOnlyCommits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// arrange shapes the hub and returns the tip and commits RemoteOnlyCommits must report.
		arrange func(t *testing.T, dir string) (wantTip string, wantCommits []string)
	}{
		{
			name: "remote branch holding a commit HEAD lacks",
			arrange: func(t *testing.T, dir string) (string, []string) {
				behind := gitkit.RevParse(t, dir, "HEAD")
				ahead := gitkit.CommitFile(t, dir, "remoteonly.txt", "x", "remote-only commit")
				gitkit.Git(t, dir, "push", "origin", "HEAD")
				gitkit.Git(t, dir, "reset", "--hard", behind)
				return ahead, []string{ahead}
			},
		},
		{
			name: "remote branch at HEAD",
			arrange: func(t *testing.T, dir string) (string, []string) {
				gitkit.Git(t, dir, "push", "origin", "HEAD")
				return gitkit.RevParse(t, dir, "HEAD"), nil
			},
		},
		{
			name: "remote branch behind HEAD",
			arrange: func(t *testing.T, dir string) (string, []string) {
				gitkit.Git(t, dir, "push", "origin", "HEAD")
				tip := gitkit.RevParse(t, dir, "HEAD")
				gitkit.CommitFile(t, dir, "local.txt", "x", "local-only commit")
				return tip, nil
			},
		},
		{
			name: "branch absent on the remote",
			arrange: func(t *testing.T, dir string) (string, []string) {
				gitkit.Git(t, dir, "checkout", "-b", "unpushed-task")
				gitkit.CommitFile(t, dir, "local.txt", "x", "local-only commit")
				return "", nil
			},
		},
		{
			name: "repository with no remote",
			arrange: func(t *testing.T, dir string) (string, []string) {
				gitkit.Git(t, dir, "remote", "remove", "origin")
				return "", nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.NewHub(t, ".")
			f, err := fabricengine.Open(h.Location)
			if err != nil {
				t.Fatalf("fabricengine.Open: %v", err)
			}
			wantTip, wantCommits := tt.arrange(t, h.PrimeWorktree())

			tip, commits, err := f.RemoteOnlyCommits()
			if err != nil {
				t.Fatalf("RemoteOnlyCommits: %v", err)
			}
			if tip != wantTip {
				t.Errorf("tip = %q; want %q", tip, wantTip)
			}
			if !slices.Equal(commits, wantCommits) {
				t.Errorf("commits = %v; want %v", commits, wantCommits)
			}
		})
	}
}
