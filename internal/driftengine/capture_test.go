//go:build tier1 && !windows

// capture_test.go — tier 1 tests for RunCapture and RunSideCapture, driven by fake sh commands against a temp gate-slot pool.

package driftengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/proc"
)

// captureFixture is a CaptureRequest over a temp worktree and a one-slot pool.
type captureFixture struct {
	req  CaptureRequest
	pool *gateslot.Pool
}

// newCaptureFixture returns a fixture whose capture runs command in the temp worktree.
func newCaptureFixture(t *testing.T, command string) *captureFixture {
	t.Helper()
	root := t.TempDir()
	pool := &gateslot.Pool{
		Dir:    t.TempDir(),
		Limits: func() (gateslot.Limits, error) { return gateslot.Limits{Slots: 1}, nil },
		Poll:   5 * time.Millisecond,
	}
	return &captureFixture{
		pool: pool,
		req: CaptureRequest{
			Paths:        NewPaths(root),
			WorktreeRoot: root,
			Step:         CommandStep{Command: command, Dir: ".", TimeoutMin: 1},
			Pool:         pool,
			WaitDir:      t.TempDir(),
			Check:        func() (string, error) { return "head1", nil },
			Timeout:      time.Minute,
			Now:          time.Now,
			LockRetry:    time.Second,
		},
	}
}

// eventually polls cond until it holds or the deadline passes.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// readFile returns a file's text, failing the test when it cannot be read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v; want nil", path, err)
	}
	return string(data)
}

// slotFree reports whether the pool has no holder.
func slotFree(t *testing.T, pool *gateslot.Pool) bool {
	t.Helper()
	holders, err := pool.Holders()
	if err != nil {
		t.Fatalf("Holders() = %v; want nil", err)
	}
	return len(holders) == 0
}

// TestRunCapture covers the command's outcome rows, the skip file, the timeout tree kill, the kept backup, the capture lock, the slot wait, Check's refusal and the side capture.
func TestRunCapture(t *testing.T) {
	t.Parallel()

	outcomes := []struct {
		name       string
		command    string
		skip       []string
		wantState  string
		wantReason string
		wantFile   string
		wantText   string
	}{
		{name: "a command writing a file passes", command: "echo profile > {baseline_dir}/p.json", wantState: CapturePassed, wantFile: "p.json", wantText: "profile\n"},
		{name: "the command sees the skip file with one test per line", command: "cat {skip_file} > {baseline_dir}/seen", skip: []string{"TestA", "TestB"}, wantState: CapturePassed, wantFile: "seen", wantText: "TestA\nTestB\n"},
		{name: "the command sees an empty skip file when nothing is skipped", command: "cat {skip_file} > {baseline_dir}/seen", wantState: CapturePassed, wantFile: "seen", wantText: ""},
		{name: "exit 0 with no file fails", command: "true", wantState: CaptureFailed, wantReason: "exit 0 but wrote no file to "},
		{name: "a non-zero exit fails", command: "exit 3", wantState: CaptureFailed, wantReason: "exit 3"},
	}
	for _, tt := range outcomes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCaptureFixture(t, tt.command)
			f.req.Skip = tt.skip

			rec, err := RunCapture(context.Background(), f.req)
			if err != nil {
				t.Fatalf("RunCapture() = _, %v; want nil", err)
			}
			if rec.State != tt.wantState || !strings.HasPrefix(rec.Reason, tt.wantReason) {
				t.Errorf("RunCapture() record = %+v; want state %q and a reason opening %q", rec, tt.wantState, tt.wantReason)
			}
			if tt.wantFile != "" {
				if got := readFile(t, filepath.Join(f.req.Paths.BaselineDir, tt.wantFile)); got != tt.wantText {
					t.Errorf("baseline file %s = %q; want %q", tt.wantFile, got, tt.wantText)
				}
			}
			stored, found, err := ReadCapture(f.req.Paths)
			if err != nil || !found || stored != rec {
				t.Errorf("ReadCapture() = %+v, %v, %v; want the returned record", stored, found, err)
			}
			if rec.Pid != os.Getpid() || rec.Head != "head1" || rec.SlotAcquiredAt == "" || rec.CommandStartedAt == "" || rec.EndedAt == "" {
				t.Errorf("record = %+v; want the pid, head and the slot, start and end times", rec)
			}
			if _, err := os.Stat(rec.Log); err != nil {
				t.Errorf("record log %q: %v; want the log file", rec.Log, err)
			}
			if !slotFree(t, f.pool) {
				t.Errorf("the slot is still held after the capture")
			}
			if held, _ := ProbeLock(f.req.Paths.CaptureLock); held {
				t.Errorf("the capture lock is still held after the capture")
			}
		})
	}

	t.Run("a command past the timeout has its tree killed and fails", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "sleep 60 & echo $! > {baseline_dir}/child.pid; wait")
		f.req.Timeout = 500 * time.Millisecond

		rec, err := RunCapture(context.Background(), f.req)
		if err != nil || rec.State != CaptureFailed || !strings.HasPrefix(rec.Reason, "timed out after ") {
			t.Fatalf("RunCapture() = %+v, %v; want a failed record that timed out", rec, err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(readFile(t, filepath.Join(f.req.Paths.BaselineDir, "child.pid"))))
		if err != nil {
			t.Fatalf("child.pid is no pid: %v", err)
		}
		eventually(t, "the sleeping grandchild to die", func() bool { return !proc.IsAlive(pid) })
		if !slotFree(t, f.pool) {
			t.Errorf("the slot is still held after the timeout")
		}
		if held, _ := ProbeLock(f.req.Paths.CaptureLock); held {
			t.Errorf("the capture lock is still held after the timeout")
		}
	})

	t.Run("an existing non-empty baseline is moved to a numbered backup and kept", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo new > {baseline_dir}/p.json")
		if err := os.MkdirAll(f.req.Paths.BaselineDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		if err := os.WriteFile(filepath.Join(f.req.Paths.BaselineDir, "old.json"), []byte("old"), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}

		if rec, err := RunCapture(context.Background(), f.req); err != nil || rec.State != CapturePassed {
			t.Fatalf("RunCapture() = %+v, %v; want a passed record", rec, err)
		}
		if got := readFile(t, f.req.Paths.BaselineDir+".1/old.json"); got != "old" {
			t.Errorf("baseline.1/old.json = %q; want the kept old baseline", got)
		}
		if _, err := os.Stat(filepath.Join(f.req.Paths.BaselineDir, "old.json")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the old file is still in the new baseline directory")
		}
	})

	t.Run("a capture lock held past LockRetry refuses, and one held briefly inside it does not", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo x > {baseline_dir}/p.json")
		if err := os.MkdirAll(filepath.Dir(f.req.Paths.CaptureLock), 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		held, err := lock.AcquireWriteLock(f.req.Paths.CaptureLock)
		if err != nil {
			t.Fatalf("AcquireWriteLock() = %v; want nil", err)
		}
		f.req.LockRetry = 30 * time.Millisecond
		if _, err := RunCapture(context.Background(), f.req); err == nil || !strings.Contains(err.Error(), "a capture already holds") {
			t.Errorf("RunCapture() with the lock held = %v; want the held-lock refusal", err)
		}

		f.req.LockRetry = 10 * time.Second
		time.AfterFunc(50*time.Millisecond, func() { _ = held.Release() })
		if rec, err := RunCapture(context.Background(), f.req); err != nil || rec.State != CapturePassed {
			t.Errorf("RunCapture() with a brief hold = %+v, %v; want a passed record", rec, err)
		}
	})

	t.Run("with the only slot held the record stays running without a slot time", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo x > {baseline_dir}/p.json")
		lease, err := f.pool.Acquire(context.Background(), gateslot.Holder{Worktree: f.req.WorktreeRoot, Site: "test"})
		if err != nil {
			t.Fatalf("Acquire() = %v; want nil", err)
		}

		done := make(chan CaptureRecord, 1)
		go func() {
			rec, err := RunCapture(context.Background(), f.req)
			if err != nil {
				t.Errorf("RunCapture() = _, %v; want nil", err)
			}
			done <- rec
		}()
		eventually(t, "the capture's wait record", func() bool {
			waits, _ := gateslot.ReadWaits(f.req.WaitDir)
			return len(waits) == 1
		})
		rec, found, err := ReadCapture(f.req.Paths)
		if err != nil || !found || rec.State != CaptureRunning || rec.SlotAcquiredAt != "" {
			t.Errorf("ReadCapture() while waiting = %+v, %v, %v; want running with no slot_acquired_at", rec, found, err)
		}

		if err := lease.Release(); err != nil {
			t.Fatalf("Release() = %v; want nil", err)
		}
		if final := <-done; final.State != CapturePassed || final.SlotAcquiredAt == "" {
			t.Errorf("final record = %+v; want passed with a slot time", final)
		}
		if waits, _ := gateslot.ReadWaits(f.req.WaitDir); len(waits) != 0 {
			t.Errorf("wait records after the capture = %+v; want none", waits)
		}
	})

	t.Run("Check's refusal is returned untouched and writes no record", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo x > {baseline_dir}/p.json")
		refusal := errors.New("drift capture refused: test")
		f.req.Check = func() (string, error) { return "", refusal }

		if _, err := RunCapture(context.Background(), f.req); err != refusal {
			t.Errorf("RunCapture() = %v; want Check's error untouched", err)
		}
		if _, found, _ := ReadCapture(f.req.Paths); found {
			t.Errorf("a capture record was written for a refused capture")
		}
	})

	t.Run("a side capture writes the next numbered directory and leaves the main capture's files", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo side > {baseline_dir}/p.json")

		for _, dir := range []string{"1", "2"} {
			got, err := RunSideCapture(context.Background(), f.req)
			if err != nil || got.ExitCode != 0 || got.Dir != filepath.Join(f.req.Paths.SideDir, dir) {
				t.Fatalf("RunSideCapture() = %+v, %v; want exit 0 into side/%s", got, err, dir)
			}
			if text := readFile(t, filepath.Join(got.Dir, "p.json")); text != "side\n" {
				t.Errorf("side/%s/p.json = %q; want the command's output", dir, text)
			}
			if _, err := os.Stat(got.Log); err != nil {
				t.Errorf("side log %q: %v; want the log file", got.Log, err)
			}
		}
		if _, found, _ := ReadCapture(f.req.Paths); found {
			t.Errorf("a side capture wrote a capture record")
		}
		if _, err := os.Stat(f.req.Paths.BaselineDir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a side capture touched the baseline directory")
		}
		if !slotFree(t, f.pool) {
			t.Errorf("the slot is still held after the side capture")
		}
	})
}
