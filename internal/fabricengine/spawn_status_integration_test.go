// spawn_status_integration_test.go covers SpawnStatusPush, the entry point for a status push, over a hubforge hub,
// and the pusher lines the commit and add verbs start.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"bytes"
	"errors"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// firstAttemptDeadline bounds the wait for the pusher child's first attempt, far above what load adds to it.
const firstAttemptDeadline = 2 * time.Minute

// TestSpawnStatusPush covers the request SpawnStatusPush builds from the prime and a task pair.
// It also covers what the commit and add verbs start.
// It runs serially because SwapPushRecorderForTest replaces the package-level spawnDetachedPushFn.
func TestSpawnStatusPush(t *testing.T) {
	tiergate.Require(t, 1)
	pairSlug := hubforge.TemplatePairSlug

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
	boardDir := fabriclayout.BoardDir(h.Location.HubPath)
	recorder := fabricengine.SwapPushRecorderForTest(t)

	resolve := func(t *testing.T, worktree string) *lymcwd.Location {
		t.Helper()
		l, err := lymcwd.ResolveWorktree(worktree)
		if err != nil {
			t.Fatalf("ResolveWorktree(%s): %v", worktree, err)
		}
		return l
	}
	requireOneCall := func(t *testing.T, warp, weft string) {
		t.Helper()
		calls := recorder.Calls()
		if len(calls) != 1 {
			t.Fatalf("recorded requests = %+v; want exactly one", calls)
		}
		got := calls[0]
		if got.WarpPath != warp || got.WeftPath != weft || got.PauseSlug != "" || got.TeardownSlug != "" || got.WorkDir != boardDir || got.ParentLocked {
			t.Errorf("request = %+v; want warp %q, weft %q, workdir %q and no pause, teardown or parent-locked mark", got, warp, weft, boardDir)
		}
	}

	t.Run("PrimeNamesRecordsOnly", func(t *testing.T) {
		recorder.Reset()
		res, err := fabricengine.SpawnStatusPush(h.Location, "s")
		if err != nil {
			t.Fatalf("SpawnStatusPush: %v", err)
		}
		if !res.Spawned {
			t.Errorf("result = %+v; want spawned", res)
		}
		requireOneCall(t, "", h.PrimeRecords())
	})

	t.Run("TaskPairNamesBothSides", func(t *testing.T) {
		recorder.Reset()
		l := resolve(t, h.PairCodeWorktree(pairSlug))
		if _, err := fabricengine.SpawnStatusPush(l, "ignored"); err != nil {
			t.Fatalf("SpawnStatusPush: %v", err)
		}
		requireOneCall(t, h.PairCodeWorktree(pairSlug), h.PairRecordsSibling(pairSlug))

		recorder.Reset()
		if err := fabricengine.SpawnPairPush(l); err != nil {
			t.Fatalf("SpawnPairPush: %v", err)
		}
		requireOneCall(t, h.PairCodeWorktree(pairSlug), h.PairRecordsSibling(pairSlug))

		spawnErr := errors.New("spawn failed")
		recorder.Err = spawnErr
		t.Cleanup(func() { recorder.Err = nil })
		if err := fabricengine.SpawnPairPush(l); !errors.Is(err, spawnErr) {
			t.Errorf("SpawnPairPush with a failing spawn = %v; want %v", err, spawnErr)
		}
	})

	t.Run("AddStartsThePusherOfTheNewPair", func(t *testing.T) {
		recorder.Reset()
		var out bytes.Buffer
		if code := clihelp.ExecuteIn(fabriccli.Command(), h.PrimeWorktree(), &out, []string{"add", "added"}); code != 0 {
			t.Fatalf("fabric add = %d, %s; want 0", code, out.String())
		}
		calls := recorder.Calls()
		if len(calls) == 0 {
			t.Fatal("fabric add recorded no spawn; want the new pair's pusher")
		}
		want := []string{"loom", "pusher", "--worktree", h.PairCodeWorktree("added"), "--slug", "added"}
		for _, call := range calls {
			if got := fabricengine.PushSpawnCommandForTest(call); !slices.Equal(got, want) {
				t.Errorf("spawned line = %q; want %q", got, want)
			}
		}
	})

	// A commit over an unreachable origin leaves the push to the pusher, which keeps it owed with no failed-push record.
	// It ends the hub's remotes, so it runs last.
	t.Run("CommitOverAnUnreachableOriginWritesNoFailedPushRecord", func(t *testing.T) {
		bin := lymbin.Build(t)
		records := h.PairRecordsSibling(pairSlug)
		for _, worktree := range []string{records, h.PairCodeWorktree(pairSlug)} {
			gitkit.Git(t, worktree, "remote", "set-url", "origin", refusedOriginURL)
		}

		recorder.Reset()
		pairLocation := resolve(t, h.PairCodeWorktree(pairSlug))
		pair, err := fabricengine.Open(pairLocation)
		if err != nil {
			t.Fatalf("Open(%s) = %v; want nil", pairSlug, err)
		}
		fabricengine.WriteWarpFileForTest(t, records, "_lym/owed.txt", "owed\n")
		if _, err := pair.Commit([]string{"_lym/owed.txt"}, "owed", nil, fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("Commit() = %v; want nil", err)
		}
		calls := recorder.Calls()
		if len(calls) != 1 {
			t.Fatalf("recorded requests = %+v; want exactly one", calls)
		}
		line := fabricengine.PushSpawnCommandForTest(calls[0])
		if slices.Contains(line, "--push-now") || len(line) < 2 || line[0] != "loom" || line[1] != "pusher" {
			t.Fatalf("spawned line = %q; want the pusher's, without --push-now", line)
		}

		var childOutput bytes.Buffer
		child := exec.Command(bin, line...)
		child.Dir = calls[0].WorkDir
		child.Stdout, child.Stderr = &childOutput, &childOutput
		if err := child.Start(); err != nil {
			t.Fatalf("start the pusher: %v", err)
		}
		exited := make(chan error, 1)
		go func() { exited <- child.Wait() }()
		stop := func() {
			_ = child.Process.Kill()
			<-exited
		}

		key := pushledger.RecordsKey(records)
		deadline := time.After(firstAttemptDeadline)
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for attempted := false; !attempted; {
			select {
			case err := <-exited:
				t.Fatalf("the pusher exited before its first attempt: %v, %s", err, childOutput.String())
			case <-deadline:
				stop()
				t.Fatalf("the pusher made no attempt; output %s", childOutput.String())
			case <-tick.C:
				entry, _, err := pushledger.Read(h.Location.HubPath, key)
				if err != nil {
					stop()
					t.Fatalf("read the ledger: %v", err)
				}
				attempted = !entry.LastAttempt.IsZero()
			}
		}
		stop()

		failed, err := fabricengine.ListFailedPushes(h.Location.HubPath)
		if err != nil {
			t.Fatalf("ListFailedPushes() = %v; want nil", err)
		}
		if len(failed) != 0 {
			t.Errorf("failed-push records = %+v; want none for an owed push", failed)
		}
	})
}
