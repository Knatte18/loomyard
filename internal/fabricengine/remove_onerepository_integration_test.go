// remove_onerepository_integration_test.go covers Remove of a pair in a one-repository hub.
// The hub is built by hubforge.NewOneRepoHub, so the pair is one worktree on one branch.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRemove_OneRepository covers the removal of a pair whose one branch carries its records and its code.
// The removal tags the tip, deletes the one worktree and branch, and then deletes the branch and the view ref on origin.
func TestRemove_OneRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name         string
		plantViewRef bool
	}{
		{"TagIsPushedTheBranchAndTheViewRefGo", true},
		{"ViewRefAlreadyGoneIsNoFailure", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			const slug = "removed-one"
			p := newOneRepoPair(t, slug)
			tip := gitkit.RevParse(t, p.path, "HEAD")
			viewRef := "refs/heads/" + fabricengine.ViewRefPrefix + slug
			if tt.plantViewRef {
				gitkit.Git(t, p.path, "push", "origin", tip+":"+viewRef)
			}
			onOrigin := func(ref string) string {
				return gitkit.Git(t, p.hub.CodeBare, "for-each-ref", "--format=%(objectname)", ref)
			}

			res, err := p.hub.Topology.Remove(p.hub.Location, slug, false, true)

			if err != nil {
				t.Fatalf("Remove() error = %v; want nil", err)
			}
			if res.RemoteBranchError != "" {
				t.Errorf("Remove() remote branch error = %q; want none", res.RemoteBranchError)
			}
			if got := tagTargetAt(t, p.hub.CodeBare, wantArchiveTag(slug, tip)); got != tip {
				t.Errorf("origin archive tag points at %q; want the tip %s", got, tip)
			}
			if got := onOrigin("refs/heads/" + p.branch); got != "" {
				t.Errorf("origin branch = %q; want it deleted", got)
			}
			if got := onOrigin(viewRef); got != "" {
				t.Errorf("origin view ref = %q; want it deleted", got)
			}
			if _, err := os.Stat(p.path); !os.IsNotExist(err) {
				t.Errorf("the pair's worktree still exists (stat err %v); want it removed", err)
			}
			if gitkit.BranchExists(t, p.hub.PrimeWorktree(), p.branch) {
				t.Errorf("local branch %s exists; want it deleted", p.branch)
			}
		})
	}
}
