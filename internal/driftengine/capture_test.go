//go:build !windows

// capture_test.go — tier 1 tests for the main and current captures.
// They drive fake sh commands against a temp gate-slot pool.

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
	"sync"
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

// TestFailedCaptureError covers the reason and log the error carries and the capture and defer ways forward it names.
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
			name:    "a capture that wrote no file is a plain failure",
			rec:     CaptureRecord{State: CaptureFailed, Reason: "exit 3; it ran in /w and printed nothing", Log: "/logs/capture.log", WroteNoFile: true},
			want:    []string{"lym drift capture followed by lym loom resume"},
			wantNot: []string{"lym drift scope"},
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

// TestCaptureCurrent covers the current capture's own directory, its lock wait, its slot, and the results that did not pass.
func TestCaptureCurrent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("concurrent captures each write their own directory and leave the baseline and the records alone", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo profile > {baseline_dir}/p.json")
		p := f.req.Paths
		if err := os.MkdirAll(p.BaselineDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		if err := os.WriteFile(filepath.Join(p.BaselineDir, "old.json"), []byte("old"), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}

		results := make([]CurrentCapture, 2)
		var wg sync.WaitGroup
		for i := range results {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := CaptureCurrent(context.Background(), f.req)
				if err != nil {
					t.Errorf("CaptureCurrent() = _, %v; want nil", err)
				}
				results[i] = got
			}()
		}
		wg.Wait()

		if results[0].Dir == results[1].Dir {
			t.Errorf("both captures wrote %s; want a directory each", results[0].Dir)
		}
		for _, got := range results {
			if !got.Passed || filepath.Dir(got.Dir) != p.CurrentDir || readFile(t, filepath.Join(got.Dir, "p.json")) != "profile\n" {
				t.Errorf("CaptureCurrent() = %+v; want a passed capture with its file under %s", got, p.CurrentDir)
			}
		}
		if _, found, _ := ReadCapture(p); found {
			t.Errorf("a current capture wrote a capture record")
		}
		if got := readFile(t, filepath.Join(p.BaselineDir, "old.json")); got != "old" {
			t.Errorf("baseline/old.json = %q; want the baseline untouched", got)
		}
		if _, err := os.Stat(p.BaselineDir + ".1"); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a baseline backup exists; want a current capture to make none")
		}
		if !slotFree(t, f.pool) {
			t.Errorf("the slot is still held after the captures")
		}
	})

	t.Run("a held capture lock is waited for through Sleep from one second doubling", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo profile > {baseline_dir}/p.json")
		if err := os.MkdirAll(filepath.Dir(f.req.Paths.CaptureLock), 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		held, err := lock.AcquireWriteLock(f.req.Paths.CaptureLock)
		if err != nil {
			t.Fatalf("AcquireWriteLock() = %v; want nil", err)
		}
		var waits []time.Duration
		f.req.Sleep = func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			if len(waits) == 2 {
				return held.Release()
			}
			return nil
		}

		got, err := CaptureCurrent(context.Background(), f.req)
		if err != nil || !got.Passed {
			t.Fatalf("CaptureCurrent() = %+v, %v; want a passed capture once the lock was released", got, err)
		}
		if want := []time.Duration{time.Second, 2 * time.Second}; !slices.Equal(waits, want) {
			t.Errorf("Sleep waits = %v; want %v", waits, want)
		}
		if held, _ := ProbeLock(f.req.Paths.CaptureLock); held {
			t.Errorf("the capture lock is still held after the capture")
		}
	})

	t.Run("a capture inside a gate runs in the gate's slot and a second waits for the slot", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo profile > {baseline_dir}/p.json")
		gate, err := f.pool.Acquire(context.Background(), gateslot.Holder{Worktree: f.req.AnchorDir, Site: "test gate"})
		if err != nil {
			t.Fatalf("Acquire() = %v; want nil", err)
		}
		gateSlot, _ := strings.CutPrefix(gate.InheritEnv(nil)[0], gateslot.InheritEnv+"=")

		inside := f.req
		inside.InheritedSlot = gateSlot
		if got, err := CaptureCurrent(context.Background(), inside); err != nil || !got.Passed {
			t.Fatalf("CaptureCurrent() inside the gate = %+v, %v; want a passed capture with no slot of its own", got, err)
		}

		done := make(chan CurrentCapture, 1)
		go func() {
			got, err := CaptureCurrent(context.Background(), f.req)
			if err != nil {
				t.Errorf("CaptureCurrent() = _, %v; want nil", err)
			}
			done <- got
		}()
		eventually(t, "the second capture's wait record", func() bool {
			waits, _ := gateslot.ReadWaits(f.req.WaitDir)
			return len(waits) == 1
		})
		if err := gate.Release(); err != nil {
			t.Fatalf("Release() = %v; want nil", err)
		}
		if got := <-done; !got.Passed {
			t.Errorf("CaptureCurrent() after the gate released = %+v; want a passed capture", got)
		}
	})

	t.Run("a command writing to the meta dir lands beside the capture directory, not in it", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo meta > {meta_dir}/provenance.json; echo profile > {baseline_dir}/p.json")

		got, err := CaptureCurrent(context.Background(), f.req)
		if err != nil || !got.Passed {
			t.Fatalf("CaptureCurrent() = %+v, %v; want a passed capture", got, err)
		}
		if got.MetaDir != got.Dir+".meta" {
			t.Errorf("CaptureCurrent() meta dir = %q; want %q", got.MetaDir, got.Dir+".meta")
		}
		if text := readFile(t, filepath.Join(got.MetaDir, "provenance.json")); text != "meta\n" {
			t.Errorf("meta dir provenance.json = %q; want %q", text, "meta\n")
		}
		if _, err := os.Stat(filepath.Join(got.Dir, "provenance.json")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the meta file is under the capture directory; want it only in the meta dir")
		}
	})

	failures := []struct {
		name    string
		command string
		dir     string
		// wantReason lists text the reason holds; "{dir}" stands for the capture's directory and "{root}" for the worktree.
		wantReason []string
		// wantLog is text the log holds.
		wantLog string
	}{
		{
			name:       "a failing command does not pass and names the exit, where it ran, its last output and its log",
			command:    "echo boom; exit 3",
			wantReason: []string{"exit 3; it ran in {root} and its last output was: boom"},
			wantLog:    "boom",
		},
		{
			name:       "exit 0 with no file does not pass and names the directory",
			command:    "true",
			wantReason: []string{"exit 0 but wrote no file to {dir}"},
		},
		{
			name:       "exit 0 with only a meta file does not pass and names the directory",
			command:    "echo meta > {meta_dir}/provenance.json",
			wantReason: []string{"exit 0 but wrote no file to {dir}"},
		},
		{
			name:       "a step whose dir is no directory is a refusal naming the dir write",
			command:    "true",
			dir:        "gone",
			wantReason: []string{"lym config drift --set capture.dir=<dir> from the prime"},
		},
		{
			name:       "a command holding the retired skip file placeholder is a refusal naming the edit",
			command:    "cat {skip_file}",
			wantReason: []string{"the drift current capture command holds {skip_file}, which drift no longer renders", "the operator or the hub orch runs lym config reconcile --layer default --apply from the hub's prime worktree", "any other use of it from capture"},
		},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCaptureFixture(t, tt.command)
			f.req.Step.Dir = tt.dir

			got, err := CaptureCurrent(context.Background(), f.req)
			if err != nil || got.Passed {
				t.Fatalf("CaptureCurrent() = %+v, %v; want a result that did not pass and no error", got, err)
			}
			for _, want := range tt.wantReason {
				want = strings.ReplaceAll(strings.ReplaceAll(want, "{dir}", got.Dir), "{root}", f.req.WorktreeDir)
				if !strings.Contains(got.Reason, want) {
					t.Errorf("CaptureCurrent() reason = %q; want it to name %q", got.Reason, want)
				}
			}
			if tt.wantLog != "" && !strings.Contains(readFile(t, got.Log), tt.wantLog) {
				t.Errorf("log %s = %q; want it to hold %q", got.Log, readFile(t, got.Log), tt.wantLog)
			}
			if !slotFree(t, f.pool) {
				t.Errorf("the slot is still held after the capture")
			}
		})
	}
}

// TestRunCapture covers the command's outcome rows and the timeout tree kill.
// It also covers the kept backup, the capture lock, the slot wait and Check's refusal.
// It also covers a command holding the retired skip file placeholder.
func TestRunCapture(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	outcomes := []struct {
		name       string
		command    string
		wantState  string
		wantReason string
		wantFile   string
		wantText   string
		// timeout, when set, replaces the fixture's timeout.
		timeout time.Duration
		// wantEnds is the exact end of the reason; "{dir}" stands for the directory the command ran in.
		wantEnds        string
		wantWroteNoFile bool
	}{
		{name: "a command writing a file passes", command: "echo profile > {baseline_dir}/p.json", wantState: CapturePassed, wantFile: "p.json", wantText: "profile\n"},
		{
			name:    "exit 0 with no file and no output fails and says the command printed nothing",
			command: "true", wantState: CaptureFailed, wantReason: "exit 0 but wrote no file to ", wantWroteNoFile: true,
			wantEnds: "; it ran in {dir} and printed nothing",
		},
		{
			name:    "exit 0 with only a meta file fails as a capture that wrote no file",
			command: "echo meta > {meta_dir}/provenance.json", wantState: CaptureFailed, wantReason: "exit 0 but wrote no file to ", wantWroteNoFile: true,
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
			if wantEnds := strings.ReplaceAll(tt.wantEnds, "{dir}", f.req.WorktreeDir); !strings.HasSuffix(rec.Reason, wantEnds) {
				t.Errorf("RunCapture() reason = %q; want it to end %q", rec.Reason, wantEnds)
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

	t.Run("a command writing to the meta dir lands in the baseline's meta dir, not in the baseline", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo meta > {meta_dir}/provenance.json; echo profile > {baseline_dir}/p.json")
		p := f.req.Paths

		if rec, err := RunCapture(context.Background(), f.req); err != nil || rec.State != CapturePassed {
			t.Fatalf("RunCapture() = %+v, %v; want a passed record", rec, err)
		}
		if p.BaselineMetaDir != p.BaselineDir+".meta" {
			t.Errorf("BaselineMetaDir = %q; want %q", p.BaselineMetaDir, p.BaselineDir+".meta")
		}
		if text := readFile(t, filepath.Join(p.BaselineMetaDir, "provenance.json")); text != "meta\n" {
			t.Errorf("meta dir provenance.json = %q; want %q", text, "meta\n")
		}
		if _, err := os.Stat(filepath.Join(p.BaselineDir, "provenance.json")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the meta file is under the baseline directory; want it only in the meta dir")
		}
	})

	t.Run("a backup moves the baseline's meta dir to the backup's meta dir", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo new > {baseline_dir}/p.json; echo new > {meta_dir}/provenance.json")
		p := f.req.Paths
		writeTree(t, p.BaselineDir, map[string]string{"old.json": "old"})
		writeTree(t, p.BaselineMetaDir, map[string]string{"provenance.json": "old"})

		if rec, err := RunCapture(context.Background(), f.req); err != nil || rec.State != CapturePassed {
			t.Fatalf("RunCapture() = %+v, %v; want a passed record", rec, err)
		}
		if text := readFile(t, p.BaselineDir+".1.meta/provenance.json"); text != "old" {
			t.Errorf("baseline.1.meta/provenance.json = %q; want the kept old metadata", text)
		}
		if text := readFile(t, filepath.Join(p.BaselineMetaDir, "provenance.json")); text != "new\n" {
			t.Errorf("meta dir provenance.json = %q; want the new capture's metadata", text)
		}
	})

	t.Run("a backup skips a number whose meta dir is taken and keeps what is there", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo new > {baseline_dir}/p.json")
		p := f.req.Paths
		writeTree(t, p.BaselineDir, map[string]string{"old.json": "old"})
		writeTree(t, p.BaselineMetaDir, map[string]string{"provenance.json": "old"})
		writeTree(t, p.BaselineDir+".1.meta", map[string]string{"keep.json": "kept"})

		if rec, err := RunCapture(context.Background(), f.req); err != nil || rec.State != CapturePassed {
			t.Fatalf("RunCapture() = %+v, %v; want a passed record", rec, err)
		}
		if text := readFile(t, p.BaselineDir+".1.meta/keep.json"); text != "kept" {
			t.Errorf("baseline.1.meta/keep.json = %q; want the taken meta dir untouched", text)
		}
		if text := readFile(t, p.BaselineDir+".2/old.json"); text != "old" {
			t.Errorf("baseline.2/old.json = %q; want the old baseline under the next free number", text)
		}
		if text := readFile(t, p.BaselineDir+".2.meta/provenance.json"); text != "old" {
			t.Errorf("baseline.2.meta/provenance.json = %q; want the old metadata beside its baseline", text)
		}
	})

	t.Run("a meta dir beside an empty baseline is cleared before the capture", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "echo new > {baseline_dir}/p.json")
		p := f.req.Paths
		writeTree(t, p.BaselineDir, nil)
		writeTree(t, p.BaselineMetaDir, map[string]string{"stale.json": "stale"})

		if rec, err := RunCapture(context.Background(), f.req); err != nil || rec.State != CapturePassed {
			t.Fatalf("RunCapture() = %+v, %v; want a passed record", rec, err)
		}
		if _, err := os.Stat(filepath.Join(p.BaselineMetaDir, "stale.json")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the stale metadata is still beside the new baseline; want it cleared")
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

	t.Run("with the only slot held the capture holds no lock and writes no record while it waits", func(t *testing.T) {
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
		if rec, found, err := ReadCapture(f.req.Paths); err != nil || found {
			t.Errorf("ReadCapture() while waiting = %+v, %v, %v; want no record before the slot is taken", rec, found, err)
		}
		if held, _ := ProbeLock(f.req.Paths.CaptureLock); held {
			t.Errorf("the capture lock is held while the capture waits for its slot")
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

	t.Run("a command holding the retired skip file placeholder is refused naming the edit, with the slot free and the baseline untouched", func(t *testing.T) {
		t.Parallel()
		f := newCaptureFixture(t, "cat {skip_file} > {baseline_dir}/seen")
		if err := os.MkdirAll(f.req.Paths.BaselineDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		if err := os.WriteFile(filepath.Join(f.req.Paths.BaselineDir, "old.json"), []byte("old"), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}

		rec, err := RunCapture(context.Background(), f.req)
		for _, want := range []string{"the drift capture command holds {skip_file}, which drift no longer renders", "the operator or the hub orch runs lym config reconcile --layer default --apply from the hub's prime worktree", "any other use of it from capture"} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("RunCapture() = %v; want an error naming %q", err, want)
			}
		}
		if !reflect.DeepEqual(rec, CaptureRecord{}) {
			t.Errorf("RunCapture() record = %+v; want none for a refused command", rec)
		}
		if _, found, _ := ReadCapture(f.req.Paths); found {
			t.Errorf("a capture record was written for a refused command")
		}
		if got := readFile(t, filepath.Join(f.req.Paths.BaselineDir, "old.json")); got != "old" {
			t.Errorf("baseline/old.json = %q; want the baseline untouched", got)
		}
		if _, err := os.Stat(f.req.Paths.BaselineDir + ".1"); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a baseline backup exists; want a refused command to move no baseline")
		}
		if !slotFree(t, f.pool) {
			t.Errorf("the slot is held after a refused command")
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
}
