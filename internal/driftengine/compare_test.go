//go:build !windows

// compare_test.go — tier 1 tests for RunCompare, driven by fake sh commands against a temp gate-slot pool.

package driftengine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newCompareFixture returns a compare request, and the pool it takes a slot of, that runs command in a temp worktree.
func newCompareFixture(t *testing.T, command string) (CompareRequest, *gateslot.Pool) {
	t.Helper()
	f := newCaptureFixture(t, command)
	return CompareRequest{
		Paths:       f.req.Paths,
		WorktreeDir: f.req.WorktreeDir,
		AnchorDir:   f.req.AnchorDir,
		Step:        f.req.Step,
		Pool:        f.pool,
		WaitDir:     f.req.WaitDir,
		Timeout:     time.Minute,
		Now:         time.Now,
		LockRetry:   time.Second,
	}, f.pool
}

// TestRunCompare covers a pass, a failure naming its log, the rendered baseline placeholder, the retired skip file placeholder's refusal, the marker read as held while the run waits for a slot, the timeout tree kill and a held marker.
func TestRunCompare(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("exit 0 passes", func(t *testing.T) {
		t.Parallel()
		req, pool := newCompareFixture(t, "true")
		got, err := RunCompare(context.Background(), req)
		if err != nil || !got.Passed || got.ExitCode != 0 || got.Reason != "" {
			t.Errorf("RunCompare() = %+v, %v; want a pass", got, err)
		}
		if !slotFree(t, pool) {
			t.Errorf("the slot is still held after the compare")
		}
	})

	t.Run("a non-zero exit fails and names the directory, the last output and the log", func(t *testing.T) {
		t.Parallel()
		req, _ := newCompareFixture(t, "echo drifted; exit 4")
		got, err := RunCompare(context.Background(), req)
		wantReason := "exit 4; it ran in " + req.AnchorDir + " and its last output was: drifted; log " + got.Log
		if err != nil || got.Passed || got.ExitCode != 4 || got.Reason != wantReason {
			t.Fatalf("RunCompare() = %+v, %v; want a failure with reason %q", got, err, wantReason)
		}
		if text := readFile(t, got.Log); text != "drifted\n" {
			t.Errorf("log = %q; want the command's output", text)
		}
	})

	t.Run("the command sees the baseline directory", func(t *testing.T) {
		t.Parallel()
		req, _ := newCompareFixture(t, "echo {baseline_dir} > seen.txt")
		if got, err := RunCompare(context.Background(), req); err != nil || !got.Passed {
			t.Fatalf("RunCompare() = %+v, %v; want a pass", got, err)
		}
		want := req.Paths.BaselineDir + "\n"
		if text := readFile(t, filepath.Join(req.AnchorDir, "seen.txt")); text != want {
			t.Errorf("seen.txt = %q; want %q", text, want)
		}
	})

	t.Run("a command holding the retired skip file placeholder is refused naming the edit, with the slot and the marker free", func(t *testing.T) {
		t.Parallel()
		req, pool := newCompareFixture(t, "cat {skip_file}")
		_, err := RunCompare(context.Background(), req)
		for _, want := range []string{"the drift compare command holds {skip_file}, which drift no longer renders", "the operator or the hub orch runs lym config reconcile --layer default --apply from the hub's prime worktree", "any other use of it from compare"} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("RunCompare() = %v; want an error naming %q", err, want)
			}
		}
		if !slotFree(t, pool) {
			t.Errorf("the slot is held after a refused command")
		}
		if held, err := ProbeLock(req.Paths.CompareMarker); held || err != nil {
			t.Errorf("ProbeLock(CompareMarker) after the refusal = %v, %v; want free", held, err)
		}
	})

	t.Run("the marker reads held while the run waits for a slot and free once it ends", func(t *testing.T) {
		t.Parallel()
		req, pool := newCompareFixture(t, "true")
		lease, err := pool.Acquire(context.Background(), gateslot.Holder{Worktree: req.AnchorDir, Site: "test"})
		if err != nil {
			t.Fatalf("Acquire() = %v; want nil", err)
		}

		done := make(chan CompareResult, 1)
		go func() {
			got, err := RunCompare(context.Background(), req)
			if err != nil {
				t.Errorf("RunCompare() = _, %v; want nil", err)
			}
			done <- got
		}()
		eventually(t, "the compare's wait record", func() bool {
			waits, _ := gateslot.ReadWaits(req.WaitDir)
			return len(waits) == 1
		})
		if held, err := ProbeLock(req.Paths.CompareMarker); !held || err != nil {
			t.Errorf("ProbeLock(CompareMarker) while queued = %v, %v; want held", held, err)
		}

		if err := lease.Release(); err != nil {
			t.Fatalf("Release() = %v; want nil", err)
		}
		if got := <-done; !got.Passed {
			t.Errorf("RunCompare() = %+v; want a pass once the slot is free", got)
		}
		if held, err := ProbeLock(req.Paths.CompareMarker); held || err != nil {
			t.Errorf("ProbeLock(CompareMarker) after the run = %v, %v; want free", held, err)
		}
	})

	t.Run("a missing dir is refused naming the compare key and resume as the way forward", func(t *testing.T) {
		t.Parallel()
		req, pool := newCompareFixture(t, "true")
		req.Step.Dir = "gone"
		_, err := RunCompare(context.Background(), req)
		for _, want := range []string{filepath.Join(req.WorktreeDir, "gone"), `drift.yaml key "compare.dir"`, "lym config drift --set compare.dir=<dir> from the prime, then lym loom resume"} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("RunCompare() = %v; want an error naming %q", err, want)
			}
		}
		if err != nil && strings.Contains(err.Error(), "lym drift capture") {
			t.Errorf("RunCompare() = %v; want no capture in a compare's way forward", err)
		}
		if !slotFree(t, pool) {
			t.Errorf("the slot is held after a refused dir")
		}
	})

	t.Run("a relative dir joins the worktree root, not the anchor", func(t *testing.T) {
		t.Parallel()
		req, _ := newCompareFixture(t, "pwd -P > ran.txt")
		req.AnchorDir = filepath.Join(req.WorktreeDir, "dotnet")
		if err := os.MkdirAll(req.AnchorDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		req.Step.Dir = "dotnet"
		if got, err := RunCompare(context.Background(), req); err != nil || !got.Passed {
			t.Fatalf("RunCompare() = %+v, %v; want a pass", got, err)
		}
		want, err := filepath.EvalSymlinks(req.AnchorDir)
		if err != nil {
			t.Fatalf("EvalSymlinks() = %v; want nil", err)
		}
		if got := strings.TrimSpace(readFile(t, filepath.Join(req.AnchorDir, "ran.txt"))); got != want {
			t.Errorf("the command ran in %s; want %s", got, want)
		}
	})

	t.Run("a shortened timeout kills the tree and fails", func(t *testing.T) {
		t.Parallel()
		req, pool := newCompareFixture(t, "sleep 60")
		req.Timeout = 300 * time.Millisecond
		got, err := RunCompare(context.Background(), req)
		if err != nil || got.Passed || !strings.HasPrefix(got.Reason, "timed out after ") {
			t.Errorf("RunCompare() = %+v, %v; want a failure naming the timeout", got, err)
		}
		if want := "; it ran in " + req.WorktreeDir + " and printed nothing; log " + got.Log; !strings.HasSuffix(got.Reason, want) {
			t.Errorf("RunCompare() reason = %q; want it to end %q", got.Reason, want)
		}
		if !slotFree(t, pool) {
			t.Errorf("the slot is still held after the timeout")
		}
	})

	t.Run("a marker held past LockRetry refuses, and one held briefly inside it does not", func(t *testing.T) {
		t.Parallel()
		req, _ := newCompareFixture(t, "true")
		held := holdLock(t, req.Paths.CompareMarker)
		req.LockRetry = 30 * time.Millisecond
		if _, err := RunCompare(context.Background(), req); err == nil || !strings.Contains(err.Error(), "a compare is already running") {
			t.Errorf("RunCompare() with the marker held = %v; want the refusal", err)
		}

		req.LockRetry = 10 * time.Second
		time.AfterFunc(50*time.Millisecond, func() { _ = held.Release() })
		if got, err := RunCompare(context.Background(), req); err != nil || !got.Passed {
			t.Errorf("RunCompare() with a brief hold = %+v, %v; want a pass", got, err)
		}
	})
}

// holdLock takes the lock at path, creating its directory.
func holdLock(t *testing.T, path string) *lock.FileLock {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v; want nil", err)
	}
	held, err := lock.AcquireWriteLock(path)
	if err != nil {
		t.Fatalf("AcquireWriteLock() = %v; want nil", err)
	}
	return held
}
