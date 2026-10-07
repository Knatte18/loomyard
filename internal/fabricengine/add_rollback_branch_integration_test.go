//go:build integration

// add_rollback_branch_integration_test.go proves Add's rollback deletes the warp branch this Add created, locally under any branch_prefix and on origin only under the probe, attempt and lease conditions.
// Push failures are injected through the Topology's push seam; every hub is built through hubforge with the default empty branch_prefix.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// failedRefusal is the synthetic bare "(failed)" rejection of branch that Add's push retry treats as transient.
func failedRefusal(args []string, cwd, branch string) error {
	return &gitexec.GitError{Args: args, Dir: cwd, ExitCode: 1, Stderr: " ! [remote rejected] " + branch + " -> " + branch + " (failed)\n"}
}

func TestAddRollback_DeletesTheWarpBranchItCreated(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		opts fabricengine.AddOptions
		// prepare arranges the hub before Add runs.
		prepare func(t *testing.T, h *hubforge.Hub, slug string)
		// runner builds the push runner; attempts counts the pushes it saw of the branch it sabotages.
		runner func(slug, weftBranch string, attempts *int) func(args []string, cwd string) (string, error)
		// wantAttempts is the sabotaged branch's push count, 0 to skip the check.
		wantAttempts int
		// wantOriginAtHead is whether origin's warp branch must remain, at the prime's HEAD.
		wantOriginAtHead bool
		wantRemoteDelete bool
		wantReAdd        bool
	}{
		{
			name: "weft push refused until the attempt bound rolls back without stranding the branch",
			runner: func(slug, weftBranch string, attempts *int) func([]string, string) (string, error) {
				return func(args []string, cwd string) (string, error) {
					if args[len(args)-1] == weftBranch {
						*attempts++
						return "", failedRefusal(args, cwd, weftBranch)
					}
					return gitexec.Run(args, cwd)
				}
			},
			wantAttempts:     4,
			wantRemoteDelete: true,
			wantReAdd:        true,
		},
		{
			name: "warp push refused without landing leaves origin without the branch",
			runner: func(slug, weftBranch string, attempts *int) func([]string, string) (string, error) {
				return func(args []string, cwd string) (string, error) {
					if args[len(args)-1] == slug {
						*attempts++
						return "", failedRefusal(args, cwd, slug)
					}
					return gitexec.Run(args, cwd)
				}
			},
			wantAttempts: 4,
		},
		{
			name: "warp push that landed despite the refusals is deleted from origin",
			runner: func(slug, weftBranch string, attempts *int) func([]string, string) (string, error) {
				return func(args []string, cwd string) (string, error) {
					if args[len(args)-1] == slug {
						if _, err := gitexec.Run(args, cwd); err != nil {
							return "", err
						}
						return "", failedRefusal(args, cwd, slug)
					}
					return gitexec.Run(args, cwd)
				}
			},
			wantRemoteDelete: true,
		},
		{
			name: "pre-existing origin branch behind HEAD survives the rollback",
			prepare: func(t *testing.T, h *hubforge.Hub, slug string) {
				gitkit.MustRun(t, h.PrimeWorktree(), "git", "push", "--quiet", "origin", "HEAD:refs/heads/"+slug)
				gitkit.CommitFile(t, h.PrimeWorktree(), "ahead.txt", "ahead\n", "prime moves past the leftover")
			},
			runner: func(slug, weftBranch string, attempts *int) func([]string, string) (string, error) {
				return func(args []string, cwd string) (string, error) {
					if args[len(args)-1] == weftBranch {
						return "", failedRefusal(args, cwd, weftBranch)
					}
					return gitexec.Run(args, cwd)
				}
			},
			wantOriginAtHead: true,
		},
		{
			name: "SkipPush deletes only the local branch",
			opts: fabricengine.AddOptions{SkipPush: true},
			runner: func(slug, weftBranch string, attempts *int) func([]string, string) (string, error) {
				return func(args []string, cwd string) (string, error) {
					if args[len(args)-1] == slug {
						if _, err := gitexec.Run(args, cwd); err != nil {
							return "", err
						}
						return "", errors.New("connection lost after the push")
					}
					return gitexec.Run(args, cwd)
				}
			},
			wantOriginAtHead: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			const slug = "rollback-created-branch"
			weftBranch := fabricengine.RecordsBranchName(slug)
			h := hubforge.NewHub(t, ".")
			if tc.prepare != nil {
				tc.prepare(t, h, slug)
			}
			var attempts int
			fabricengine.SetPushSeamForTest(h.Topology, tc.runner(slug, weftBranch, &attempts), func(time.Duration) {})

			res, err := h.Topology.Add(h.Location, slug, tc.opts)

			if err == nil {
				t.Fatalf("Add should have failed")
			}
			if tc.wantAttempts != 0 && attempts != tc.wantAttempts {
				t.Errorf("sabotaged pushes = %d; want %d", attempts, tc.wantAttempts)
			}
			if gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
				t.Errorf("local warp branch %q survived the rollback", slug)
			}
			originHasBranch := gitkit.BranchExists(t, h.CodeBare, slug)
			if originHasBranch != tc.wantOriginAtHead {
				t.Fatalf("origin has warp branch = %v; want %v", originHasBranch, tc.wantOriginAtHead)
			}
			if tc.wantOriginAtHead && !tc.opts.SkipPush {
				if got, want := gitkit.RevParse(t, h.CodeBare, slug), gitkit.RevParse(t, h.PrimeWorktree(), "HEAD"); got != want {
					t.Errorf("origin warp branch = %s; want the pushed %s", got, want)
				}
			}
			var deleted bool
			for _, entry := range res.Mutated().Entries() {
				deleted = deleted || entry.Kind == fabricengine.KindRemoteBranchDeleted
			}
			if deleted != tc.wantRemoteDelete {
				t.Errorf("record carries a remote-branch deletion = %v; want %v", deleted, tc.wantRemoteDelete)
			}
			if tc.wantReAdd {
				fabricengine.SetPushSeamForTest(h.Topology, nil, nil)
				hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
			}
		})
	}
}
