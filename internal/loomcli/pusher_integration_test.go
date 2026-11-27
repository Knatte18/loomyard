// pusher_integration_test.go drives the pusher verb and the board target's seams over a hubforge hub.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// holdPusherLock takes the pusher lock of target for the rest of the test.
func holdPusherLock(t *testing.T, target runpusher.Target) {
	t.Helper()
	path := runpusher.LockPath(target)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the pusher lock directory: %v", err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(path)
	if err != nil || !acquired {
		t.Fatalf("TryAcquireWriteLock(%s) = %v, %v; want the lock", path, acquired, err)
	}
	t.Cleanup(func() { _ = held.Release() })
}

// TestPusherCmd_PushNowWakesAndExitsAtAHeldLock pins that --push-now marks the target and exits at a held lock,
// and that the line a board write starts exits at the board's held lock with no mark written.
func TestPusherCmd_PushNowWakesAndExitsAtAHeldLock(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("task1"))
	if err != nil {
		t.Fatalf("ResolveWorktree(task1) = %v; want nil", err)
	}
	target, err := runpusher.TargetOf(pair, "task1")
	if err != nil {
		t.Fatalf("TargetOf(task1) = %v; want nil", err)
	}
	holdPusherLock(t, target)
	holdPusherLock(t, runpusher.BoardTarget(h.Location))
	boardDir := fabriclayout.BoardDir(h.Location.HubPath)

	run := func(flags fabriclayout.PusherFlags) {
		t.Helper()
		var out bytes.Buffer
		line := fabriclayout.PusherCommand(flags)
		// The loom command is the root here, so the line's leading "loom" is dropped.
		if code := clihelp.ExecuteIn(Command(), boardDir, &out, line[1:]); code != 0 {
			t.Fatalf("lym %q = %d, %s; want 0", line, code, out.String())
		}
	}

	records := fabricengine.RecordsWorktree(pair)
	if err := os.WriteFile(filepath.Join(records, "_lym", "unpushed.txt"), []byte("unpushed\n"), 0o644); err != nil {
		t.Fatalf("write a records file: %v", err)
	}
	if _, committed, err := fabricengine.NewBolt(records).Commit("unpushed", fabricengine.SyncOptions{}); err != nil || !committed {
		t.Fatalf("commit the records = %v, %v; want a commit", committed, err)
	}

	run(fabriclayout.PusherFlags{Worktree: pair.WorktreePath(), Slug: "task1", PushNow: true})
	entry, found, err := pushledger.Read(h.Location.HubPath, pushledger.RecordsKey(records))
	if err != nil || !found || !entry.PushNow || entry.Worktree != pair.WorktreePath() || entry.Slug != "task1" {
		t.Errorf("entry = %+v, %v, %v; want the push-now mark on task1's entry", entry, found, err)
	}
	if unpushed, err := gitrepo.New(records).HasUnpushed(); err != nil || !unpushed {
		t.Errorf("HasUnpushed() = %v, %v; want the commit still unpushed", unpushed, err)
	}

	run(fabriclayout.PusherFlags{Worktree: boardDir, Board: true})
	if entry, found, err := pushledger.Read(h.Location.HubPath, pushledger.BoardKey); err != nil || found {
		t.Errorf("board entry = %+v, %v, %v; want none", entry, found, err)
	}
}

// TestPusherDeps_BoardPushCommitsTheBoardWrite pins the board target's level read and push over a bare origin:
// the push commits an uncommitted board write, and an unreachable origin leaves it owed with no failed-push record.
// It is not parallel: it sets BOARD_SKIP_PUSH, which is process-global state.
func TestPusherDeps_BoardPushCommitsTheBoardWrite(t *testing.T) {
	tiergate.Require(t, 1)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	boardDir := fabriclayout.BoardDir(h.Location.HubPath)
	board := gitrepo.New(boardDir)
	deps := pusherDeps(h.Location, runpusher.BoardTarget(h.Location), prePushCheck{}, nil)

	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(boardDir, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatalf("write %s to the board: %v", name, err)
		}
	}
	requireAhead := func(want bool, when string) {
		t.Helper()
		if ahead, err := deps.Ahead(); ahead != want || err != nil {
			t.Fatalf("Ahead() %s = %v, %v; want %v, nil", when, ahead, err, want)
		}
	}

	write("first.txt")
	requireAhead(true, "with an uncommitted write")
	if err := deps.Push(); err != nil {
		t.Fatalf("Push() = %v; want nil", err)
	}
	requireAhead(false, "after the push")
	head, err := board.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() = %v; want nil", err)
	}
	branch, err := board.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() = %v; want nil", err)
	}
	if got, err := gitrepo.New(h.RecordsBare).ResolveSHA("refs/heads/" + branch); err != nil || got != head {
		t.Errorf("origin's board = %q, %v; want the pushed commit %q", got, err, head)
	}

	// git refuses this URL with `connection refused`, which reads as a transport failure.
	gitkit.Git(t, boardDir, "remote", "set-url", "origin", "http://127.0.0.1:1/x")
	write("second.txt")
	if err := deps.Push(); !fabricengine.IsPushOwed(err) {
		t.Fatalf("Push() over an unreachable origin = %v; want an owed failure", err)
	}
	if uncommitted, err := fabricengine.NewBolt(boardDir).HasUncommitted(); err != nil || uncommitted {
		t.Errorf("HasUncommitted() = %v, %v; want the write committed", uncommitted, err)
	}
	failed, err := fabricengine.ListFailedPushes(h.Location.HubPath)
	if err != nil || slices.ContainsFunc(failed, func(record fabriclayout.FailedPush) bool { return record.Repo == fabriclayout.RepoBolt }) {
		t.Errorf("failed-push records = %+v, %v; want none of the board", failed, err)
	}
	requireAhead(true, "with the commit owed")

	t.Setenv("BOARD_SKIP_PUSH", "1")
	requireAhead(false, "with pushes off and the commit unpushed")
}

// TestStartRunPusher pins the target a loom resume's pusher start hands over.
// A pair's target carries its slug, and the prime's carries none.
func TestStartRunPusher(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("task1"))
	if err != nil {
		t.Fatalf("ResolveWorktree(task1) = %v; want nil", err)
	}

	var started []runpusher.Target
	start := func(target runpusher.Target) (bool, error) {
		started = append(started, target)
		return false, nil
	}
	startRunPusher(pair, "task1", start)
	startRunPusher(h.Location, "task1", start)

	want := []runpusher.Target{{Location: pair, Slug: "task1", Pair: true}, {Location: h.Location}}
	if !slices.Equal(started, want) {
		t.Errorf("targets = %+v; want %+v", started, want)
	}
}
