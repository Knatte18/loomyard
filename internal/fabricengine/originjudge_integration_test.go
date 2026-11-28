// originjudge_integration_test.go proves what the origin reconcile's judging reads from a hub, and its seam refusal.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestReconcileOrigin_RefusesAnUnsuppliedSeam is not parallel: it unsets a process-global seam.
func TestReconcileOrigin_RefusesAnUnsuppliedSeam(t *testing.T) {
	tiergate.Require(t, 1)
	hub, worktree, branch := newOriginTaskHub(t)
	other := otherMachine(t, hub.CodeBare, branch)
	gitkit.CommitFile(t, other, "moved.txt", "moved", "origin moved code")
	gitkit.Git(t, other, "push", "origin", branch)
	refsBefore := refsOf(t, worktree)
	restore := fabricengine.SetRunRecordPaths(nil)
	defer restore()

	report, err := fabricengine.ReconcileOrigin(hub.Location, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{})

	var refusal *fabriccore.Refusal
	if !errors.As(err, &refusal) || !strings.Contains(refusal.Reason, string(fabriccore.SeamRunRecordPaths)) {
		t.Fatalf("ReconcileOrigin() error = %v; want a refusal naming %s", err, fabriccore.SeamRunRecordPaths)
	}
	if len(report.Branches) != 0 || len(report.Entries) != 0 || len(report.Mutations.Entries()) != 0 {
		t.Errorf("report = %+v; want no branch, no entry and no mutation", report)
	}
	if got := refsOf(t, worktree); got != refsBefore {
		t.Errorf("refs after the refusal:\n%s\nwant them unchanged:\n%s", got, refsBefore)
	}
}

func TestReconcileOrigin_Snapshot(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	hub, worktree, branch := newOriginTaskHub(t)
	local := gitkit.RevParse(t, worktree, "HEAD")
	other := otherMachine(t, hub.CodeBare, branch)
	moved := gitkit.CommitFile(t, other, "moved.txt", "moved", "origin moved code")
	gitkit.Git(t, other, "push", "origin", branch)
	scope := fabriclayout.ReconcileScope{Run: "task", JudgeOnly: true}

	snapshot, err := fabricengine.ReconcileJudgeSnapshotForTest(hub.Location, scope, fabricengine.ReconcileOptions{})
	if err != nil {
		t.Fatalf("ReconcileJudgeSnapshotForTest() error = %v; want nil", err)
	}

	wantPhases := []fabriccore.Phase{fabriccore.PhasePreLock, fabriccore.PhaseOrigin}
	if !slices.Equal(snapshot.Phases, wantPhases) {
		t.Errorf("Phases = %v; want %v", snapshot.Phases, wantPhases)
	}
	facts := snapshot.Reconcile
	if !facts.Fetched || !facts.Judged || !slices.Equal(facts.Pairs, []string{"task"}) {
		t.Errorf("Reconcile = %+v; want the moved heads fetched, the branches read and the pair task", facts)
	}
	if got := snapshot.Stores[fabriccore.StoreCode].Origin["refs/heads/"+branch]; got != moved {
		t.Errorf("origin's head of %s = %q; want %q", branch, got, moved)
	}
	i := slices.IndexFunc(facts.Targets, func(f fabriccore.BranchFacts) bool { return f.Repo == fabriclayout.RepoCode && f.Branch == branch })
	if i < 0 {
		t.Fatalf("Targets = %+v; want the code branch %s", facts.Targets, branch)
	}
	target := facts.Targets[i]
	if target.Tracking != local || target.OriginHead != moved || target.LocalTip != local || !target.Behind || target.Ahead || target.Rewound {
		t.Errorf("branch facts = %+v; want tracking and local tip %s, origin head %s, behind alone", target, local, moved)
	}

	report := reconcile(t, hub, scope, fabricengine.ReconcileOptions{})
	requireState(t, report, "code", branch, fabriclayout.BranchFastForward)
	requireState(t, report, "code", hub.primeBranch, fabriclayout.BranchLevel)

	// Origin's branch replaced by a history that shares no commit reads rewritten, through the tracking ref's reflog.
	gitkit.Git(t, other, "checkout", "--orphan", "replaced")
	gitkit.CommitFile(t, other, "replaced.txt", "replaced", "replaced history")
	gitkit.Git(t, other, "push", "--force", "origin", "replaced:"+branch)

	requireState(t, reconcile(t, hub, scope, fabricengine.ReconcileOptions{}), "code", branch, fabriclayout.BranchRewritten)
}
