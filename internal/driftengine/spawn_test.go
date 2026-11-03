// spawn_test.go — tier 0 tests for StartCapture, driven through its spawn and breakaway seams, and for ProbeLock.

package driftengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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
		Executable:       "lym",
		AnchorDir:        root,
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

// TestStartCapture covers the child's argv and run-process record, the wait for the child's own running record, a child that ends or stalls before it with the error its log ends on, a lock held before the start, and both breakaway branches.
func TestStartCapture(t *testing.T) {
	tiergate.Require(t, 0)
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
		if want := []string{"lym", "drift", "capture-child"}; len(f.spawns) != 1 || strings.Join(f.spawns[0].argv, " ") != strings.Join(want, " ") || !f.spawns[0].breakaway || f.spawns[0].dir != f.req.AnchorDir {
			t.Errorf("spawns = %+v; want one breakaway start of %v in the anchored directory %s", f.spawns, want, f.req.AnchorDir)
		}
		records, err := runproc.List(f.req.RunProcDir, f.req.Owner)
		if err != nil || len(records) != 1 || records[0].Role != runproc.RoleCapture || records[0].PID != os.Getpid() {
			t.Errorf("runproc.List() = %+v, %v; want one %q record of the child", records, err, runproc.RoleCapture)
		}
	})

	lastingEnvelope := fmt.Sprintf(`{"error":"cwd /w does not equal the anchored directory /w/backend","%s":true,"ok":false}`, LastingFaultKey)
	failures := []struct {
		name     string
		fallback bool
		exits    bool
		// log is what the child writes to its log before it ends.
		log         string
		want        []string
		wantNot     []string
		wantLasting bool
	}{
		{name: "a child that exits without its record, no fallback", exits: true, want: []string{"ended before it recorded its start", "its log is", "capture-child-"}},
		{name: "a child that exits without its record, with fallback", fallback: true, exits: true, want: []string{"ended before it recorded its start", "capture-child-", "run lym drift capture again", "lym drift defer followed by lym loom resume"}},
		{name: "a child that stalls past the record bound, no fallback", want: []string{"did not record its start within", "its log is"}},
		{name: "a child that stalls past the record bound, with fallback", fallback: true, want: []string{"did not record its start within", "lym drift capture again"}},
		{
			name:  "a child that ends on an error envelope carries its error on one line",
			exits: true,
			log:   "time=t level=INFO msg=start\n" + `{"error":"drift config key \"capture\":\n  missing","ok":false}` + "\n",
			want:  []string{`ended before it recorded its start: drift config key "capture": missing; its log is`},
		},
		{
			name:        "a lasting error offers no fresh capture, with fallback",
			fallback:    true,
			exits:       true,
			log:         lastingEnvelope + "\n",
			want:        []string{"does not equal the anchored directory", "cannot change this error", "lym drift defer followed by lym loom resume", "fix of the error's cause"},
			wantNot:     []string{"run lym drift capture again"},
			wantLasting: true,
		},
		{
			name:        "a lasting error is marked for the calling row, no fallback",
			exits:       true,
			log:         lastingEnvelope + "\n",
			want:        []string{"does not equal the anchored directory", "its log is"},
			wantLasting: true,
		},
		{
			name:  "the last error envelope wins and an ok envelope is skipped",
			exits: true,
			log: fmt.Sprintf(`{"error":"first error","%s":true,"ok":false}`, LastingFaultKey) + "\n" +
				`{"error":"second error","ok":false}` + "\n" +
				`{"error":"not an error","ok":true}` + "\n",
			want:    []string{"ended before it recorded its start: second error;"},
			wantNot: []string{"first error", "not an error"},
		},
		{
			name:    "an overlong error is cut to the bound on a rune",
			exits:   true,
			log:     `{"error":"` + strings.Repeat("å", 2*childErrorMaxRunes) + `","ok":false}`,
			want:    []string{strings.Repeat("å", childErrorMaxRunes-1) + "…"},
			wantNot: []string{strings.Repeat("å", childErrorMaxRunes)},
		},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newSpawnFixture(t, func(_ *spawnFixture, spec childSpawn) (<-chan struct{}, error) {
				if _, err := spec.log.WriteString(tt.log); err != nil {
					t.Errorf("write the child log: %v", err)
				}
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
			for _, unwanted := range tt.wantNot {
				if strings.Contains(err.Error(), unwanted) {
					t.Errorf("StartCapture() error = %q; want it without %q", err.Error(), unwanted)
				}
			}
			var noRecord childNoRecordError
			if !errors.As(err, &noRecord) || noRecord.lasting != tt.wantLasting {
				t.Errorf("StartCapture() error = %#v; want a childNoRecordError with lasting = %v", err, tt.wantLasting)
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
