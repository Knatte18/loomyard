// originreconcile_onerepository_integration_test.go covers the origin reconcile of a one-repository pair's branch.
// The branch carries records and code, so a local commit that changes a code path is never replayed.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReconcileOrigin_OneRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("LocalCodeCommitOverAMovedOriginIsDivergedNamingTheCommit", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "diverged")
		gitkit.Git(t, p.path, "branch", "--set-upstream-to=origin/"+p.branch)
		localCode := gitkit.CommitFile(t, p.path, "code.txt", "local\n", "local code")
		p.originCommit(t, "_lym/origin.txt", "origin\n")

		report, err := fabricengine.ReconcileOrigin(p.hub.Location, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{})
		if err != nil {
			t.Fatalf("ReconcileOrigin() error = %v; want nil", err)
		}

		outcome := requireState(t, report, "records", p.branch, fabricengine.BranchDiverged)
		if !slices.Equal(outcome.Commits, []string{localCode}) || outcome.Moved {
			t.Errorf("outcome = %+v; want the local code commit %s named and nothing moved", outcome, localCode)
		}
		if got := gitkit.RevParse(t, p.path, "HEAD"); got != localCode {
			t.Errorf("HEAD = %s; want the branch unmoved at %s", got, localCode)
		}
	})

	t.Run("RecordsOnlyDivergenceIsReplayedOntoOrigin", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "replayed")
		gitkit.Git(t, p.path, "branch", "--set-upstream-to=origin/"+p.branch)
		p.commitTransition(t, "local")
		originTip := p.originCommit(t, "_lym/origin.txt", "origin\n")

		report, err := fabricengine.ReconcileOrigin(p.hub.Location, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{})
		if err != nil {
			t.Fatalf("ReconcileOrigin() error = %v; want nil", err)
		}

		outcome := requireState(t, report, "records", p.branch, fabricengine.BranchAhead)
		if !outcome.Moved {
			t.Errorf("outcome = %+v; want the records-only commit replayed", outcome)
		}
		gitkit.Git(t, p.path, "merge-base", "--is-ancestor", originTip, "HEAD")
	})
}
