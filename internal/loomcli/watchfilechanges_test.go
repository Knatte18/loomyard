// watchfilechanges_test.go drives watchFileChanges over a real temporary directory, and the PR-Review wait's RecordChanges seam that watches the run's pull request record through it.

package loomcli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// watchDeadline bounds the wait for a change a test expects; a watch that works delivers it within milliseconds.
	watchDeadline = 10 * time.Second
	// watchQuiet is how long a test waits to see that no change arrives.
	watchQuiet = 200 * time.Millisecond
)

// receiveChange waits for one value on changes and fails the test when none arrives before watchDeadline.
func receiveChange(t *testing.T, changes <-chan struct{}) {
	t.Helper()
	select {
	case <-changes:
	case <-time.After(watchDeadline):
		t.Fatalf("no change delivered within %v; want one", watchDeadline)
	}
}

// assertNoChange fails the test when changes delivers a value within watchQuiet.
func assertNoChange(t *testing.T, changes <-chan struct{}) {
	t.Helper()
	select {
	case <-changes:
		t.Errorf("a change was delivered; want none")
	case <-time.After(watchQuiet):
	}
}

func TestWatchFileChanges(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("AWriteToTheWatchedFileDeliversAChange", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "watched.json")
		changes, stop := watchFileChanges(path, "test waiter")
		t.Cleanup(stop)
		if changes == nil {
			t.Fatalf("watchFileChanges(%q) channel = nil; want a live watch", path)
		}

		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		receiveChange(t, changes)
	})

	t.Run("TheChannelHoldsOneBufferedValue", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "watched.json")
		changes, stop := watchFileChanges(path, "test waiter")
		t.Cleanup(stop)
		if changes == nil {
			t.Fatalf("watchFileChanges(%q) channel = nil; want a live watch", path)
		}

		if got := cap(changes); got != 1 {
			t.Errorf("cap(changes) = %d; want 1, so a burst of changes holds one value", got)
		}
	})

	t.Run("AWriteToASiblingFileDeliversNothing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		changes, stop := watchFileChanges(filepath.Join(dir, "watched.json"), "test waiter")
		t.Cleanup(stop)
		if changes == nil {
			t.Fatalf("watchFileChanges channel = nil; want a live watch")
		}

		sibling := filepath.Join(dir, "sibling.json")
		if err := os.WriteFile(sibling, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", sibling, err)
		}
		assertNoChange(t, changes)
	})

	t.Run("AMissingDirectoryReturnsANilChannelAndAStopThatIsSafeToCall", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "no-such-dir", "watched.json")
		changes, stop := watchFileChanges(path, "test waiter")
		if changes != nil {
			t.Errorf("watchFileChanges(%q) channel is non-nil; want nil so the waiter stays on its timer", path)
		}
		if stop == nil {
			t.Fatalf("watchFileChanges(%q) stop = nil; want a function", path)
		}
		stop()
	})

	t.Run("StopEndsTheWatchAndMayBeCalledAgain", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "watched.json")
		changes, stop := watchFileChanges(path, "test waiter")
		if changes == nil {
			t.Fatalf("watchFileChanges(%q) channel = nil; want a live watch", path)
		}

		stop()
		stop()
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		assertNoChange(t, changes)
	})
}

// TestPullRequestDeps_RecordChangesWatchesTheRunsPullRequestRecord asserts a write to the run's pull request record reaches the RecordChanges channel of the PR-Review wait.
func TestPullRequestDeps_RecordChangesWatchesTheRunsPullRequestRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "task", AnchorRel: "."}
	record := shedrun.PullRequestRecord(location, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(record), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(record), err)
	}

	changes, stop := pullRequestDeps(location, "main", true, &shuttleengine.Runner{}).RecordChanges()
	t.Cleanup(stop)
	if changes == nil {
		t.Fatalf("RecordChanges() channel = nil; want a watch on %s", record)
	}

	if err := os.WriteFile(record, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write %s: %v", record, err)
	}
	receiveChange(t, changes)
}
