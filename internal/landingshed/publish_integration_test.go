//go:build integration

// publish_integration_test.go drives Merge-In against a real pair built by the
// standard hub fixture (internal/hubforge), with only the conflict-resolution session faked.
// The byte-identical conflict shape across both sides is covered exhaustively by internal/fabricengine's own
// mergein_integration_test.go and this file's own sibling (finalize_integration_test.go). No test
// in this file contacts a real service or a real model.

package landingshed_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// newMergeInDepsAt builds Merge-In's Deps over the task worktree at taskWorktree: its real pair opener, a fake conflict-resolution session that must never spawn, a require-PR config for main, the real pair's tip, path and head reads, and a temporary lock store.
// The push is told skipped, so the parent tip is read locally with no fetch.
func newMergeInDepsAt(t *testing.T, taskWorktree string) landingshed.Deps {
	t.Helper()
	deps := landingshed.NewTestDeps(t)
	deps.WorktreeRoot = taskWorktree
	deps.OpenFabric = func() (*fabricengine.Fabric, error) { return openFabricAtLanding(t, taskWorktree), nil }
	// These scenarios stage no conflict, so the conflict-resolution session must never spawn.
	deps.Shuttle = &shedfake.MergeShuttle{RunFn: func(shuttleengine.Spec) (shuttleengine.Result, error) {
		t.Fatal("fake shuttle Run() called; want the clean merge-in to need no conflict-resolution session")
		return shuttleengine.Result{}, nil
	}}
	deps.Config = landingshed.Config{
		RequirePRToBase:    []string{"main"},
		Conflict:           "claude:test-model",
		ConflictTimeoutMin: 1,
	}
	fabricHandle := openFabricAtLanding(t, taskWorktree)
	owner := parentlock.Owner{Slug: "task", Pair: taskWorktree, RunID: "r1", PID: os.Getpid(), StartTime: "t1"}
	scratch := t.TempDir()
	deps.PushSkipped = true
	deps.OpenParentFabric = func() (*fabricengine.Fabric, error) { return fabricHandle, nil }
	deps.ReadParentTips = func(fetch bool) (fabricengine.ParentTips, error) { return fabricHandle.ReadParentTips("main", fetch) }
	deps.ChangedPaths = fabricHandle.ChangedPaths
	deps.MergeBase = fabricHandle.MergeBase
	deps.TaskHead = fabricHandle.HeadSHA
	deps.Lock = landingshed.ParentLockDeps{
		Store:    &parentlock.Store{Dir: filepath.Join(scratch, "locks"), Alive: func(int, string) bool { return true }},
		Owner:    func() (parentlock.Owner, error) { return owner, nil },
		WaitPath: parentlock.WaitRecordPath(scratch),
	}
	deps.MergeRecordPath = landingshed.MergeRecordPath(scratch)
	// The parent is told to add nothing over the fork point, so Merge-In aligns with no fixer session.
	deps.HoldsWork = func(string, string) (bool, error) { return true, nil }
	deps.RowEntry = func(string) (int, error) { return 1, nil }
	deps.Config.MergeFixAttempts = 1
	return deps
}

// TestMergeIn_ConflictSessionLoadsLanguageSkills drives Merge-In over a real pair whose branches conflict on one file, and asserts the one conflict session it spawns loads the told language skills after its own.
// A second row stages a local parent branch holding a commit its upstream lacks, and asserts the merge brings in the upstream tip and leaves the local parent branch's ref alone.
func TestMergeIn_ConflictSessionLoadsLanguageSkills(t *testing.T) {
	t.Run("a conflict session loads the told language skills", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		taskWorktree := h.PrimeWorktree()

		gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "-b", "task-branch")
		gitkit.CommitFile(t, taskWorktree, "conflict.txt", "task content\n", "task: add conflict.txt")
		gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "main")
		gitkit.CommitFile(t, taskWorktree, "conflict.txt", "parent content\n", "main: add conflict.txt")
		gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "task-branch")

		shuttle := resolutionShuttle(taskWorktree, "resolved content\n", "conflict.txt")
		deps := newMergeInDepsAt(t, taskWorktree)
		deps.StencilsDir = seedConflictStencil(t)
		deps.Shuttle = shuttle
		deps.LanguageSkills = []string{"scribe:golang-testing"}

		m, err := landingshed.NewMergeIn(deps)
		if err != nil {
			t.Fatalf("NewMergeIn() error = %v; want nil", err)
		}
		shedfake.RequireOutcome(t, m, shedengine.Done)

		if len(shuttle.Specs) != 1 {
			t.Fatalf("fake shuttle Run() called %d time(s); want exactly 1 conflict-resolution session", len(shuttle.Specs))
		}
		if want := []string{"scribe:prose", "scribe:code-quality", "scribe:golang-testing"}; !slices.Equal(shuttle.Specs[0].Skills, want) {
			t.Errorf("conflict session Skills = %v; want %v", shuttle.Specs[0].Skills, want)
		}
	})

	t.Run("a local parent ahead of its upstream merges the upstream tip", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		taskWorktree := h.PrimeWorktree()

		gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "-b", "task-branch")
		gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "main")
		upstreamTip := gitkit.CommitFile(t, taskWorktree, "pushed.txt", "pushed\n", "main: pushed")
		gitkit.Git(t, taskWorktree, "push", "origin", "main")
		gitkit.CommitFile(t, taskWorktree, "local-only.txt", "local only\n", "main: local only")
		localTip := gitkit.RevParse(t, taskWorktree, "main")
		gitkit.MustRun(t, taskWorktree, "git", "checkout", "-q", "task-branch")

		deps := newMergeInDepsAt(t, taskWorktree)
		deps.PushSkipped = false
		m, err := landingshed.NewMergeIn(deps)
		if err != nil {
			t.Fatalf("NewMergeIn() error = %v; want nil", err)
		}
		shedfake.RequireOutcome(t, m, shedengine.Done)

		rec, found, err := landingshed.ReadMergeRecord(deps.MergeRecordPath)
		if err != nil || !found || rec.MergedTip != upstreamTip {
			t.Errorf("merge record = %+v, %v, %v; want merged_tip %s, the upstream tip", rec, found, err, upstreamTip)
		}
		if _, err := os.Stat(filepath.Join(taskWorktree, "pushed.txt")); err != nil {
			t.Errorf("pushed.txt missing from the task tree: %v; want the upstream tip merged in", err)
		}
		if _, err := os.Stat(filepath.Join(taskWorktree, "local-only.txt")); err == nil {
			t.Error("local-only.txt is in the task tree; want the local-only commit left out")
		}
		if got := gitkit.RevParse(t, taskWorktree, "main"); got != localTip {
			t.Errorf("local main = %s; want it unchanged at %s", got, localTip)
		}
	})
}
