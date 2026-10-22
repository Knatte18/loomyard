//go:build integration

// spawn_status_integration_test.go covers SpawnStatusPush, the one entry point for a status push, over a hubforge hub: the prime rule, the slug roles, the board working directory and the local-branch check.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestSpawnStatusPush covers the request SpawnStatusPush builds from the prime, a task pair, a local pair and a pair whose code branch alone is marked and a pair whose records branch alone is marked.
// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
func TestSpawnStatusPush(t *testing.T) {
	const localSlug = "localpair"
	pairSlug := hubforge.TemplatePairSlug

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
	boardDir := fabricengine.BoardDir(h.Location.HubPath)
	recorder := fabricengine.SwapPushRecorderForTest(t)

	resolve := func(t *testing.T, worktree string) *lyxcwd.Location {
		t.Helper()
		l, err := lyxcwd.ResolveWorktree(worktree)
		if err != nil {
			t.Fatalf("ResolveWorktree(%s): %v", worktree, err)
		}
		return l
	}
	requireOneCall := func(t *testing.T, warp, weft, pauseSlug, teardownSlug string) {
		t.Helper()
		calls := recorder.Calls()
		if len(calls) != 1 {
			t.Fatalf("recorded requests = %+v; want exactly one", calls)
		}
		got := calls[0]
		if got.WarpPath != warp || got.WeftPath != weft || got.PauseSlug != pauseSlug || got.TeardownSlug != teardownSlug || got.WorkDir != boardDir || got.ParentLocked {
			t.Errorf("request = %+v; want warp %q, weft %q, pause %q, teardown %q, workdir %q and no parent-locked mark", got, warp, weft, pauseSlug, teardownSlug, boardDir)
		}
	}

	t.Run("PrimePushesRecordsOnlyUnderTheGateSlug", func(t *testing.T) {
		recorder.Reset()
		res, err := fabricengine.SpawnStatusPush(h.Location, "s")
		if err != nil {
			t.Fatalf("SpawnStatusPush: %v", err)
		}
		if !res.Spawned || len(res.LocalSkipped) != 0 {
			t.Errorf("result = %+v; want spawned with nothing skipped", res)
		}
		requireOneCall(t, "", h.PrimeRecords(), "s", "")
	})

	t.Run("TaskPairPushesBothSidesUnderItsOwnSlug", func(t *testing.T) {
		recorder.Reset()
		l := resolve(t, h.PairCodeWorktree(pairSlug))
		if _, err := fabricengine.SpawnStatusPush(l, "ignored"); err != nil {
			t.Fatalf("SpawnStatusPush: %v", err)
		}
		requireOneCall(t, h.PairCodeWorktree(pairSlug), h.PairRecordsSibling(pairSlug), pairSlug, pairSlug)

		recorder.Reset()
		if err := fabricengine.SpawnPairPush(l); err != nil {
			t.Fatalf("SpawnPairPush: %v", err)
		}
		requireOneCall(t, h.PairCodeWorktree(pairSlug), h.PairRecordsSibling(pairSlug), pairSlug, pairSlug)

		spawnErr := errors.New("spawn failed")
		recorder.Err = spawnErr
		t.Cleanup(func() { recorder.Err = nil })
		if err := fabricengine.SpawnPairPush(l); !errors.Is(err, spawnErr) {
			t.Errorf("SpawnPairPush with a failing spawn = %v; want %v", err, spawnErr)
		}
	})

	t.Run("PairWhoseSlugHoldsAParentLockSpawnsParentLocked", func(t *testing.T) {
		recorder.Reset()
		l := resolve(t, h.PairCodeWorktree(pairSlug))
		startTime, _ := proc.StartTime(os.Getpid())
		store := &parentlock.Store{Dir: parentlock.Dir(boardDir)}
		owner := parentlock.Owner{Slug: pairSlug, Pair: pairSlug, RunID: "run", PID: os.Getpid(), StartTime: startTime}
		if attempt, err := store.TryAcquire("main", owner); err != nil || !attempt.Granted {
			t.Fatalf("TryAcquire = (%+v, %v); want granted", attempt, err)
		}
		t.Cleanup(func() { store.Release("main", owner) }) //nolint:errcheck

		if _, err := fabricengine.SpawnStatusPush(l, ""); err != nil {
			t.Fatalf("SpawnStatusPush: %v", err)
		}
		calls := recorder.Calls()
		if len(calls) != 1 || !calls[0].ParentLocked {
			t.Errorf("recorded requests = %+v; want one with the parent-locked mark", calls)
		}
	})

	t.Run("UnreadableParentLockRecordsLeaveItUnmarkedAndWarn", func(t *testing.T) {
		recorder.Reset()
		logged := logcapture.Capture(t)
		l := resolve(t, h.PairCodeWorktree(pairSlug))
		lockDir := parentlock.Dir(boardDir)
		if err := os.RemoveAll(lockDir); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(lockDir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(lockDir, []byte("not a directory"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(lockDir) })

		if _, err := fabricengine.SpawnStatusPush(l, ""); err != nil {
			t.Fatalf("SpawnStatusPush: %v", err)
		}
		requireOneCall(t, h.PairCodeWorktree(pairSlug), h.PairRecordsSibling(pairSlug), pairSlug, pairSlug)
		if got := logged.String(); strings.Count(got, "parent lock records unreadable") != 1 {
			t.Errorf("log = %q; want one warning that the parent lock records are unreadable", got)
		}
	})

	t.Run("OrdinaryPairWithMarkedCodeBranchPushesRecordsOnly", func(t *testing.T) {
		recorder.Reset()
		l := resolve(t, h.PairCodeWorktree(pairSlug))
		code := gitrepo.New(h.PairCodeWorktree(pairSlug))
		branch, err := code.CurrentBranch()
		if err != nil {
			t.Fatalf("CurrentBranch: %v", err)
		}
		if err := code.MarkBranchLocal(branch); err != nil {
			t.Fatalf("MarkBranchLocal: %v", err)
		}
		t.Cleanup(func() { code.ClearBranchLocalMark(branch) }) //nolint:errcheck

		res, err := fabricengine.SpawnStatusPush(l, "")
		if err != nil {
			t.Fatalf("SpawnStatusPush: %v", err)
		}
		if !res.Spawned || !slices.Equal(res.LocalSkipped, []string{branch}) {
			t.Errorf("result = %+v; want spawned with only %q skipped", res, branch)
		}
		requireOneCall(t, "", h.PairRecordsSibling(pairSlug), pairSlug, pairSlug)
	})

	t.Run("OrdinaryPairWithMarkedRecordsBranchSpawnsNothing", func(t *testing.T) {
		recorder.Reset()
		l := resolve(t, h.PairCodeWorktree(pairSlug))
		records := gitrepo.New(h.PairRecordsSibling(pairSlug))
		branch, err := records.CurrentBranch()
		if err != nil {
			t.Fatalf("CurrentBranch: %v", err)
		}
		if err := records.MarkBranchLocal(branch); err != nil {
			t.Fatalf("MarkBranchLocal: %v", err)
		}
		t.Cleanup(func() { records.ClearBranchLocalMark(branch) }) //nolint:errcheck

		res, err := fabricengine.SpawnStatusPush(l, "")
		if err != nil {
			t.Fatalf("SpawnStatusPush: %v", err)
		}
		if res.Spawned || !slices.Equal(res.LocalSkipped, []string{branch}) {
			t.Errorf("result = %+v; want nothing spawned and only %q skipped", res, branch)
		}
		if calls := recorder.Calls(); len(calls) != 0 {
			t.Errorf("recorded requests = %+v; want none without a records side", calls)
		}
	})

	t.Run("LocalPairSpawnsNothingAndItsCommitReportsTheSkip", func(t *testing.T) {
		recorder.Reset()
		if _, err := h.Topology.AddLocal(h.Location, localSlug); err != nil {
			t.Fatalf("AddLocal: %v", err)
		}
		l := resolve(t, h.PairCodeWorktree(localSlug))

		res, err := fabricengine.SpawnStatusPush(l, "")
		if err != nil {
			t.Fatalf("SpawnStatusPush: %v", err)
		}
		wantSkipped := []string{localSlug, fabricengine.RecordsBranchName(localSlug)}
		if res.Spawned || !slices.Equal(res.LocalSkipped, wantSkipped) {
			t.Errorf("result = %+v; want nothing spawned and %q skipped", res, wantSkipped)
		}
		if calls := recorder.Calls(); len(calls) != 0 {
			t.Errorf("recorded requests = %+v; want none for a local pair", calls)
		}

		f, err := fabricengine.Open(l)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		fabricengine.WriteWarpFileForTest(t, h.PairCodeWorktree(localSlug), "local-file.txt", "local change")
		commit, err := f.Commit([]string{"local-file.txt"}, "local commit", nil, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
		if !commit.Committed() {
			t.Errorf("Commit landed nothing: %+v", commit)
		}
		if !slices.Equal(commit.PushLocalSkipped, wantSkipped) {
			t.Errorf("PushLocalSkipped = %q; want %q", commit.PushLocalSkipped, wantSkipped)
		}
		if calls := recorder.Calls(); len(calls) != 0 {
			t.Errorf("recorded requests after the commit = %+v; want none for a local pair", calls)
		}
	})
}
