//go:build tier0

// spawn_test.go — tier 0 tests for StartCapture, driven through its spawn and breakaway seams, and for ProbeLock.

package driftengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/runproc"
)

// spawnFixture is a StartCapture request over temp directories with a recording fake child.
type spawnFixture struct {
	req    SpawnRequest
	spawns []childSpawn
}

// newSpawnFixture returns a fixture whose fake child is the test process, so its runproc record stays alive.
// onSpawn runs inside the seam and returns the child's exit channel.
func newSpawnFixture(t *testing.T, onSpawn func(f *spawnFixture, spec childSpawn) (<-chan struct{}, error)) *spawnFixture {
	t.Helper()
	f := &spawnFixture{}
	root := t.TempDir()
	f.req = SpawnRequest{
		Executable:       "lyx",
		WorktreeRoot:     root,
		Paths:            NewPaths(root),
		RunProcDir:       t.TempDir(),
		Owner:            "slug",
		RecordWait:       5 * time.Second,
		Poll:             time.Millisecond,
		breakawayRefused: func(err error) bool { return strings.Contains(err.Error(), "refused") },
		spawn: func(spec childSpawn) (spawnedChild, error) {
			f.spawns = append(f.spawns, spec)
			exited, err := onSpawn(f, spec)
			return spawnedChild{pid: os.Getpid(), exited: exited}, err
		},
	}
	return f
}

// writeRunning writes the capture record a child writes after its precondition held.
func writeRunning(t *testing.T, p Paths, pid int) {
	t.Helper()
	if err := WriteCapture(p, CaptureRecord{State: CaptureRunning, Pid: pid}); err != nil {
		t.Errorf("WriteCapture() = %v; want nil", err)
	}
}

// TestStartCapture covers the child's argv and run-process record, the wait for the child's own running record, a child that ends or stalls before it, a lock held before the start, and both breakaway branches.
func TestStartCapture(t *testing.T) {
	t.Parallel()

	t.Run("it returns the pid once the child writes running with its pid", func(t *testing.T) {
		t.Parallel()
		release := make(chan struct{})
		spawned := make(chan struct{})
		f := newSpawnFixture(t, func(f *spawnFixture, _ childSpawn) (<-chan struct{}, error) {
			writeRunning(t, f.req.Paths, os.Getpid()+1)
			go func() {
				<-release
				writeRunning(t, f.req.Paths, os.Getpid())
			}()
			close(spawned)
			return make(chan struct{}), nil
		})

		type result struct {
			pid int
			err error
		}
		done := make(chan result, 1)
		go func() {
			pid, err := StartCapture(f.req)
			done <- result{pid, err}
		}()
		<-spawned
		select {
		case got := <-done:
			t.Fatalf("StartCapture() returned %+v before the child wrote its record; want it to wait", got)
		case <-time.After(50 * time.Millisecond):
		}
		close(release)

		got := <-done
		if got.err != nil || got.pid != os.Getpid() {
			t.Fatalf("StartCapture() = %d, %v; want %d, nil", got.pid, got.err, os.Getpid())
		}
		if want := []string{"lyx", "drift", "capture-child"}; len(f.spawns) != 1 || strings.Join(f.spawns[0].argv, " ") != strings.Join(want, " ") || !f.spawns[0].breakaway {
			t.Errorf("spawns = %+v; want one breakaway start of %v", f.spawns, want)
		}
		records, err := runproc.List(f.req.RunProcDir, f.req.Owner)
		if err != nil || len(records) != 1 || records[0].Role != runproc.RoleCapture || records[0].PID != os.Getpid() {
			t.Errorf("runproc.List() = %+v, %v; want one %q record of the child", records, err, runproc.RoleCapture)
		}
	})

	failures := []struct {
		name     string
		fallback bool
		exits    bool
		want     []string
	}{
		{"a child that exits without its record, no fallback", false, true, []string{"ended before it recorded its start", "capture-child-", "lyx loom resume, whose entry captures again"}},
		{"a child that exits without its record, with fallback", true, true, []string{"ended before it recorded its start", "capture-child-", "lyx drift capture again", "lyx drift defer followed by lyx loom resume"}},
		{"a child that stalls past the record bound, no fallback", false, false, []string{"did not record its start within", "lyx loom resume, whose entry captures again"}},
		{"a child that stalls past the record bound, with fallback", true, false, []string{"did not record its start within", "lyx drift capture again"}},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newSpawnFixture(t, func(*spawnFixture, childSpawn) (<-chan struct{}, error) {
				exited := make(chan struct{})
				if tt.exits {
					close(exited)
				}
				return exited, nil
			})
			f.req.FallbackDetach = tt.fallback
			f.req.RecordWait = 30 * time.Millisecond

			_, err := StartCapture(f.req)
			if err == nil {
				t.Fatalf("StartCapture() = nil; want an error")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("StartCapture() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}

	t.Run("a lock held before the start refuses with nothing spawned", func(t *testing.T) {
		t.Parallel()
		f := newSpawnFixture(t, func(*spawnFixture, childSpawn) (<-chan struct{}, error) { return nil, nil })
		if err := os.MkdirAll(filepath.Dir(f.req.Paths.CaptureLock), 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		held, err := lock.AcquireWriteLock(f.req.Paths.CaptureLock)
		if err != nil {
			t.Fatalf("AcquireWriteLock() = %v; want nil", err)
		}
		defer held.Release()

		_, err = StartCapture(f.req)
		if err == nil || !strings.Contains(err.Error(), "a capture already holds") {
			t.Errorf("StartCapture() error = %v; want the held-lock refusal", err)
		}
		if len(f.spawns) != 0 {
			t.Errorf("spawns = %+v; want none", f.spawns)
		}
	})

	t.Run("a breakaway refusal is ErrBreakawayRefused, or retries detached with a fallback", func(t *testing.T) {
		t.Parallel()
		refuseBreakaway := func(f *spawnFixture, spec childSpawn) (<-chan struct{}, error) {
			if spec.breakaway {
				return nil, errors.New("access refused")
			}
			writeRunning(t, f.req.Paths, os.Getpid())
			return make(chan struct{}), nil
		}

		f := newSpawnFixture(t, refuseBreakaway)
		if _, err := StartCapture(f.req); !errors.Is(err, ErrBreakawayRefused) || len(f.spawns) != 1 {
			t.Errorf("StartCapture() without fallback = %v after %d spawns; want ErrBreakawayRefused after 1", err, len(f.spawns))
		}

		f = newSpawnFixture(t, refuseBreakaway)
		f.req.FallbackDetach = true
		if pid, err := StartCapture(f.req); err != nil || pid != os.Getpid() || len(f.spawns) != 2 || f.spawns[1].breakaway {
			t.Errorf("StartCapture() with fallback = %d, %v after %+v; want a plain detached second start", pid, err, f.spawns)
		}
	})

	t.Run("ProbeLock reads a missing directory as free and creates nothing", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "absent", "capture.lock")
		if held, err := ProbeLock(missing); held || err != nil {
			t.Errorf("ProbeLock() = %v, %v; want false, nil", held, err)
		}
		if _, err := os.Stat(filepath.Dir(missing)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("ProbeLock() created %s; want nothing created", filepath.Dir(missing))
		}
	})
}
