//go:build integration

// beginbatch_git_test.go pins BeginBatch's wiring to the real repository: with no Git told, the batch's start commit is the worktree's actual HEAD.
// Every other BeginBatch behavior is tested over a fakeGit in beginbatch_test.go.

package websterengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

//testtiming:keep pins a nil Git defaulting to the real repository's HEAD as the start commit; every other begin-batch test runs over a fake git
func TestBeginBatch_StartSHAIsTheRealHead(t *testing.T) {
	fx := newBeginFixture(t)
	repo := newScratchRepo(t)
	head := gitkit.CommitFile(t, repo, "base.txt", "base", "base commit")
	fx.Deps.Geom.Git = nil
	fx.Deps.Geom.WorktreeRoot = repo
	fx.Deps.State.AssertedModel = "master-model"

	result, err := websterengine.BeginBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("BeginBatch(1) error = %v; want nil", err)
	}
	if result.StartSHA != head {
		t.Errorf("result.StartSHA = %q; want the repository's HEAD %q", result.StartSHA, head)
	}
}
