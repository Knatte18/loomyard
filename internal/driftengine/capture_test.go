//go:build !windows

// capture_test.go — tier 1 tests for RunCapture and RunSideCapture, driven by fake sh commands against a temp gate-slot pool.

package driftengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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
			Paths:       NewPaths(root),
			WorktreeDir: root,
			AnchorDir:   root,
			Step:        CommandStep{Command: command, TimeoutMin: 1},
			Pool:        pool,
			WaitDir:     t.TempDir(),
			Check:       func() (string, error) { return "head1", nil },
			Timeout:     time.Minute,
			Now:         time.Now,
			LockRetry:   time.Second,
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

// TestFailedCaptureError covers the reason and log the error carries and the ways forward by whether the capture wrote no file under skipped tests.
func TestFailedCaptureError(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name    string
		rec     CaptureRecord
		want    []string
		wantNot []string
	}{
		{
			name:    "a plain failure names capture followed by resume",
			rec:     CaptureRecord{State: CaptureFailed, Reason: "exit 3; it ran in /w and printed nothing", Log: "/logs/capture.log"},
			want:    []string{"lym drift capture followed by lym loom resume"},
			wantNot: []string{"lym drift scope"},
		},
		{
			name:    "no file under an empty skip list is a plain failure",
			rec:     CaptureRecord{State: CaptureFailed, Reason: "exit 3; it ran in /w and printed nothing", Log: "/logs/capture.log", WroteNoFile: true},
			want:    []string{"lym drift capture followed by lym loom resume"},
			wantNot: []string{"lym drift scope"},
		},
		{
			name: "no file under skipped tests names the scope changes",
			rec:  CaptureRecord{State: CaptureFailed, Reason: "exit 3; it ran in /w and printed nothing", Log: "/logs/capture.log", WroteNoFile: true, Skip: []string{"TestA"}},
			want: []string{"lym drift scope --scope full", "lym drift scope --scope off"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := FailedCaptureError(tt.rec)
			if err == nil {
				t.Fatalf("FailedCaptureError() = nil; want an error")
			}
			wantOpen := "drift: the baseline capture ended failed (" + tt.rec.Reason + "), log /logs/capture.log; way forward: "
			if !strings.HasPrefix(err.Error(), wantOpen) {
				t.Errorf("FailedCaptureError() = %q; want it to open %q", err, wantOpen)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("FailedCaptureError() = %q; want it to name %q", err, want)
				}
			}
			for _, unwanted := range tt.wantNot {
				if strings.Contains(err.Error(), unwanted) {
					t.Errorf("FailedCaptureError() = %q; want no %q", err, unwanted)
				}
			}
		})
	}
}

// TestRunCapture covers the command's outcome rows, the skip file, the timeout tree kill, the kept backup, the capture lock, the slot wait, Check's refusal and the side capture.
func TestRunCapture(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	outcomes := []struct {
		name       string
		command    string
		skip       []string
		wantState  string
		wantReason string
		wantFile   string
		wantText   string
		// timeout, when set, replaces the fixture's timeout.
		timeout time.Duration
		// wantNamed lists text the reason holds past its opening.
		wantNamed []string
		// wantEnds is the exact end of the reason; "{dir}" stands for the directory the command ran in.
		wantEnds        string
		wantWroteNoFile bool
	}{
		{name: "a command writing a file passes", command: "echo profile > {baseline_dir}/p.json", wantState: CapturePassed, wantFile: "p.json", wantText: "profile\n"},
		{name: "the command sees the skip file with one test per line", command: "cat {skip_file} > {baseline_dir}/seen", skip: []string{"TestA", "TestB"}, wantState: CapturePassed, wantFile: "seen", wantText: "TestA\nTestB\n"},
		{name: "the command sees an empty skip file when nothing is skipped", command: "cat {skip_file} > {baseline_dir}/seen", wantState: CapturePassed, wantFile: "seen", wantText: ""},
		{
			name:    "exit 0 with no file and no output fails and says the command printed nothing",
			command: "true", wantState: CaptureFailed, wantReason: "exit 0 but wrote no file to ", wantWroteNoFile: true,
			wantEnds: "; it ran in {dir} and printed nothing",
		},
		{
			name:    "exit 0 with no file under skipped tests names the scope, the count and the skips",
			command: "true", skip: []string{"TestA", "TestB"}, wantState: CaptureFailed, wantReason: "exit 0 but wrote no file to ", wantWroteNoFile: true,
			wantNamed: []string{"scope partial skipped 2 tests", "every test the capture command ran may have been skipped"},
		},
		{
			name:    "exit 0 with no file under one skipped test names the singular count",
			command: "true", skip: []string{"TestA"}, wantState: CaptureFailed, wantReason: "exit 0 but wrote no file to ", wantWroteNoFile: true,
			wantNamed: []string{"scope partial skipped 1 test,"},
		},
		{
			name:    "exit 0 with no file counts a repeated skip once",
			command: "true", skip: []string{"TestA", "TestA"}, wantState: CaptureFailed, wantReason: "exit 0 but wrote no file to ", wantWroteNoFile: true,
			wantNamed: []string{"scope partial skipped 1 test,"},
		},
		{name: "a non-zero exit fails", command: "exit 3", wantState: CaptureFailed, wantReason: "exit 3"},
		{
			name:    "a non-zero exit after several lines quotes the directory and only the last three non-empty lines",
			command: "echo one; echo two; echo; echo three; echo four; exit 5", wantState: CaptureFailed, wantReason: "exit 5",
			wantEnds: "; it ran in {dir} and its last output was: two / three / four",
		},
		{
			name:    "an over-long last line is cut to the output bound with an ellipsis",
			command: "printf '%0500d\\n' 0; exit 2", wantState: CaptureFailed, wantReason: "exit 2",
			wantEnds: "and its last output was: …" + strings.Repeat("0", outputTailMaxRunes-1),
		},
		{
			name:    "a short last line after two long lines is quoted whole",
			command: "printf '%0500d\\n' 0; printf '%0500d\\n' 0; echo cause; exit 2", wantState: CaptureFailed, wantReason: "exit 2",
			wantEnds: "and its last output was: …" + strings.Repeat("0", outputTailMaxRunes-1-len(" / cause")) + " / cause",
		},
		{
			name:    "a log past the read window quotes the lines inside it and drops the line the window begins in",
			command: "head -c 100000 /dev/zero | tr '\\0' x; echo; echo cause; exit 2", wantState: CaptureFailed, wantReason: "exit 2",
			wantEnds: "and its last output was: cause",
		},
		{
			name:    "a timeout fails and says the directory and that the command printed nothing",
			command: "sleep 60", timeout: 300 * time.Millisecond, wantState: CaptureFailed, wantReason: "timed out after ",
			wantEnds: "; it ran in {dir} and printed nothing",
		},
	}
	for _, tt := range outcomes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCaptureFixture(t, tt.command)
			f.req.Skip = tt.skip
			if tt.timeout > 0 {
				f.req.Timeout = tt.timeout
			}

			rec, err := RunCapture(context.Background(), f.req)
			if err != nil {
				t.Fatalf("RunCapture() = _, %v; want nil", err)
			}
			if rec.State != tt.wantState || !strings.HasPrefix(rec.Reason, tt.wantReason) {
				t.Errorf("RunCapture() record = %+v; want state %q and a reason opening %q", rec, tt.wantState, tt.wantReason)
			}
			for _, want := range tt.wantNamed {
				if !strings.Contains(rec.Reason, want) {
					t.Errorf("RunCapture() reason = %q; want it to name %q", rec.Reason, want)
				}
			}
			if wantEnds := strings.ReplaceAll(tt.wantEnds, "{dir}", f.req.WorktreeDir); !strings.HasSuffix(rec.Reason, wantEnds) {
				t.Errorf("RunCapture() reason = %q; want it to end %q", rec.Reason, wantEnds)
			}
			if len(tt.wantNamed) == 0 && strings.Contains(rec.Reason, "skipped") {
				t.Errorf("RunCapture() reason = %q; want no skips named when none were skipped", rec.Reason)
			}
			if rec.WroteNoFile != tt.wantWroteNoFile {
				t.Errorf("RunCapture() wrote_no_file = %v; want %v", rec.WroteNoFile, tt.wantWroteNoFile)
			}
			if tt.wantFile != "" {
				if got := readFile(t, filepath.Join(f.req.Paths.BaselineDir, tt.wantFile)); got != tt.wantText {
					t.Errorf("baseline file %s = %q; want %q", tt.wantFile, got, tt.wantText)
				}
			}
			stored, found, err := ReadCapture(f.req.Paths)
			if err != nil || !found || !reflect.DeepEqual(stored, rec) {
				t.Errorf("ReadCapture() = %+v, %v, %v; want the returned record", stored, found, err)
			}
			if rec.Pid != os.Getpid() || rec.Head != "head1" || rec.SlotAcquiredAt == "" || rec.CommandStartedAt == "" || rec.EndedAt == "" {
				t.Errorf("record = %+v; want the pid, head and the slot, start and end times", rec)
			}
			if !slices.Equal(rec.Skip, tt.skip) {
				t.Errorf("record skip = %v; want the skipped tests %v the capture ran under", rec.Skip, tt.skip)
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

	t.Run("the step's dir", func(t *testing.T) {
		t.Parallel()
		// Each row runs under an anchor at the worktree's dotnet subdirectory, the geometry where the two bases differ.
		dirs := []struct {
			name string
			dir  string
			// ranIn is where the command ran, relative to the worktree root; empty when the dir is refused.
			ranIn string
		}{
			{name: "NoDirRunsInTheAnchor", dir: "", ranIn: "dotnet"},
			{name: "ARelativeDirJoinsTheWorktreeRootNotTheAnchor", dir: "dotnet", ranIn: "dotnet"},
			{name: "AMissingDirIsRefusedBeforeTheCommand", dir: "dotnet/dotnet"},
		}
		for _, tt := range dirs {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newCaptureFixture(t, "pwd -P > {baseline_dir}/ran")
				root := f.req.WorktreeDir
				f.req.AnchorDir = filepath.Join(root, "dotnet")
				if err := os.MkdirAll(f.req.AnchorDir, 0o755); err != nil {
					t.Fatalf("MkdirAll() = %v; want nil", err)
				}
				f.req.Step.Dir = tt.dir
				if tt.ranIn == "" {
					f.req.OnEnd = func(CaptureRecord) { t.Errorf("OnEnd was called for a capture whose command never ran") }
				}

				rec, err := RunCapture(context.Background(), f.req)
				if tt.ranIn == "" {
					resolved := filepath.Join(root, filepath.FromSlash(tt.dir))
					for _, want := range []string{resolved, `drift.yaml key "capture.dir"`, "lym config drift --set capture.dir=<dir> from the prime", "then lym drift capture and lym loom resume"} {
						if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(rec.Reason, want) {
							t.Errorf("RunCapture() = %+v, %v; want an error and a reason naming %q", rec, err, want)
						}
					}
					if rec.State != CaptureFailed {
						t.Errorf("RunCapture() state = %q; want %q", rec.State, CaptureFailed)
					}
					if _, statErr := os.Stat(rec.Log); !errors.Is(statErr, os.ErrNotExist) {
						t.Errorf("the log %s exists; want no command started", rec.Log)
					}
					if !slotFree(t, f.pool) {
						t.Errorf("the slot is held after a refused dir")
					}
					return
				}
				if err != nil || rec.State != CapturePassed {
					t.Fatalf("RunCapture() = %+v, %v; want a passed record", rec, err)
				}
				want, err := filepath.EvalSymlinks(filepath.Join(root, tt.ranIn))
				if err != nil {
					t.Fatalf("EvalSymlinks() = %v; want nil", err)
				}
				if got := strings.TrimSpace(readFile(t, filepath.Join(f.req.Paths.BaselineDir, "ran"))); got != want {
					t.Errorf("the command ran in %s; want %s", got, want)
				}
			})
		}
	})

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

	t.Run("a log the command removed is reported unreadable beside the directory", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "true")
		f.req.Step.Command = "rm -f " + f.req.Paths.LogsDir + "/capture-*.log; exit 4"

		rec, err := RunCapture(context.Background(), f.req)
		wantEnds := "exit 4; it ran in " + f.req.WorktreeDir + " and its log could not be read: "
		if err != nil || rec.State != CaptureFailed || !strings.Contains(rec.Reason, wantEnds) {
			t.Errorf("RunCapture() = %+v, %v; want a failed record whose reason holds %q", rec, err, wantEnds)
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
		lease, err := f.pool.Acquire(context.Background(), gateslot.Holder{Worktree: f.req.AnchorDir, Site: "test"})
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
		f.req.OnEnd = func(CaptureRecord) { t.Errorf("OnEnd was called for a refused capture") }
		if _, err := RunCapture(context.Background(), f.req); err != refusal {
			t.Errorf("RunCapture() with OnEnd = %v; want Check's error untouched", err)
		}
	})

	t.Run("OnEnd is called once with the final record while the capture lock is held", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "exit 3")
		var calls []CaptureRecord
		var heldInside bool
		f.req.OnEnd = func(rec CaptureRecord) {
			calls = append(calls, rec)
			heldInside, _ = ProbeLock(f.req.Paths.CaptureLock)
		}

		rec, err := RunCapture(context.Background(), f.req)
		if err != nil {
			t.Fatalf("RunCapture() = _, %v; want nil", err)
		}
		if len(calls) != 1 || !reflect.DeepEqual(calls[0], rec) || calls[0].State != CaptureFailed || calls[0].Reason != rec.Reason {
			t.Errorf("OnEnd calls = %+v; want one call with the returned failed record %+v", calls, rec)
		}
		if !heldInside {
			t.Errorf("ProbeLock(CaptureLock) inside OnEnd = false; want held")
		}
		if held, _ := ProbeLock(f.req.Paths.CaptureLock); held {
			t.Errorf("the capture lock is still held after the capture")
		}
	})

	acceptedRows := []struct {
		name    string
		command string
		// stale seeds a file in the accepted directory before the capture.
		stale bool
		// timeout, when set, replaces the fixture's timeout.
		timeout    time.Duration
		wantPassed bool
		// wantFile is the file the command wrote under the accepted directory.
		wantFile     string
		wantReasonIn []string
	}{
		{name: "an accepted capture past the timeout does not pass and names the timeout and the log", command: "sleep 60", timeout: 300 * time.Millisecond, wantReasonIn: []string{"timed out after ", "; it ran in {dir} and printed nothing; log "}},
		{name: "an accepted capture writing a file passes and leaves its output under the accepted directory", command: "echo profile > {baseline_dir}/p.json", wantPassed: true, wantFile: "p.json"},
		{name: "an accepted capture empties a file left in the accepted directory first", command: "echo profile > {baseline_dir}/p.json", stale: true, wantPassed: true, wantFile: "p.json"},
		{
			name:    "an accepted capture exiting non-zero does not pass and names the exit, the directory, the last output and the log",
			command: "echo boom; exit 3", wantReasonIn: []string{"exit 3; it ran in {dir} and its last output was: boom; log "},
		},
		{
			name:    "an accepted capture exiting 0 with no file does not pass and names the missing file, the directory and the log",
			command: "true", wantReasonIn: []string{"wrote no file", "; it ran in {dir} and printed nothing; log "},
		},
		{name: "a file left in the accepted directory does not make an empty capture pass", command: "true", stale: true, wantReasonIn: []string{"wrote no file"}},
	}
	for _, tt := range acceptedRows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCaptureFixture(t, tt.command)
			if tt.timeout > 0 {
				f.req.Timeout = tt.timeout
			}
			dir := f.req.Paths.AcceptedDir
			if tt.stale {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("MkdirAll() = %v; want nil", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "stale.json"), []byte("old"), 0o644); err != nil {
					t.Fatalf("WriteFile() = %v; want nil", err)
				}
			}

			got, err := RunAcceptedCapture(context.Background(), f.req)
			if err != nil {
				t.Fatalf("RunAcceptedCapture() = _, %v; want nil", err)
			}
			if got.Passed != tt.wantPassed {
				t.Errorf("RunAcceptedCapture() = %+v; want passed = %v", got, tt.wantPassed)
			}
			if tt.wantPassed && got.Reason != "" {
				t.Errorf("RunAcceptedCapture() reason = %q; want none for a pass", got.Reason)
			}
			for _, want := range tt.wantReasonIn {
				want = strings.ReplaceAll(want, "{dir}", f.req.WorktreeDir)
				if !strings.Contains(got.Reason, want) {
					t.Errorf("RunAcceptedCapture() reason = %q; want it to name %q", got.Reason, want)
				}
			}
			if !tt.wantPassed && !strings.Contains(got.Reason, got.Log) {
				t.Errorf("RunAcceptedCapture() reason = %q; want it to name the log %q", got.Reason, got.Log)
			}
			if _, err := os.Stat(got.Log); err != nil {
				t.Errorf("accepted capture log %q: %v; want the log file", got.Log, err)
			}
			if tt.wantFile != "" {
				if text := readFile(t, filepath.Join(dir, tt.wantFile)); text != "profile\n" {
					t.Errorf("accepted/%s = %q; want the command's output", tt.wantFile, text)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "stale.json")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the stale file is still in the accepted directory")
			}
			if _, found, _ := ReadCapture(f.req.Paths); found {
				t.Errorf("an accepted capture wrote a capture record")
			}
			if !slotFree(t, f.pool) {
				t.Errorf("the slot is still held after the accepted capture")
			}
		})
	}

	t.Run("a side capture's relative dir joins the worktree root, not the anchor", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "pwd -P > {baseline_dir}/ran")
		f.req.AnchorDir = filepath.Join(f.req.WorktreeDir, "dotnet")
		if err := os.MkdirAll(f.req.AnchorDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		f.req.Step.Dir = "dotnet"

		got, err := RunSideCapture(context.Background(), f.req)
		if err != nil || got.ExitCode != 0 {
			t.Fatalf("RunSideCapture() = %+v, %v; want exit 0", got, err)
		}
		want, err := filepath.EvalSymlinks(f.req.AnchorDir)
		if err != nil {
			t.Fatalf("EvalSymlinks() = %v; want nil", err)
		}
		if ran := strings.TrimSpace(readFile(t, filepath.Join(got.Dir, "ran"))); ran != want {
			t.Errorf("the command ran in %s; want %s", ran, want)
		}
	})

	t.Run("a side capture of a missing dir is refused with the side capture as its way forward", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "true")
		f.req.Step.Dir = "gone"

		_, err := RunSideCapture(context.Background(), f.req)
		for _, want := range []string{filepath.Join(f.req.WorktreeDir, "gone"), `drift.yaml key "capture.dir"`, "lym config drift --set capture.dir=<dir> from the prime, then lym drift capture --side"} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("RunSideCapture() = %v; want an error naming %q", err, want)
			}
		}
		if err != nil && strings.Contains(err.Error(), "lym loom resume") {
			t.Errorf("RunSideCapture() = %v; want no loom resume in a side capture's way forward", err)
		}
		if !slotFree(t, f.pool) {
			t.Errorf("the slot is held after a refused dir")
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
