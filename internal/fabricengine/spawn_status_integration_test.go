// spawn_status_integration_test.go covers SpawnStatusPush, the one entry point for a status push, over a hubforge hub: the prime rule, the slug roles, and the board working directory.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSpawnStatusPush covers the request SpawnStatusPush builds from the prime and a task pair.
// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
func TestSpawnStatusPush(t *testing.T) {
	tiergate.Require(t, 1)
	pairSlug := hubforge.TemplatePairSlug

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
	boardDir := fabricengine.BoardDir(h.Location.HubPath)
	recorder := fabricengine.SwapPushRecorderForTest(t)

	resolve := func(t *testing.T, worktree string) *lymcwd.Location {
		t.Helper()
		l, err := lymcwd.ResolveWorktree(worktree)
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
		if !res.Spawned {
			t.Errorf("result = %+v; want spawned", res)
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
}
