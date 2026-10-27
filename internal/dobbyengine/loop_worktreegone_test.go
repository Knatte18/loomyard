//go:build tier0

// loop_worktreegone_test.go pins the exit of a beam dobby whose worktree a teardown removed: when the loop tests it, what it writes to the prime's log and which tidy agent it stops.

package dobbyengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// goneRig is a loop harness over a worktree on disk, with its raw log writes landing in files and a recording tidy shuttle.
type goneRig struct {
	h        *loopHarness
	root     string
	primeLog string
	shuttle  *tidyShuttleFake
	events   chan struct{}
	checks   int
}

// newGoneRig builds a rig for the dobby of slug, empty for the prime, whose state record holds startTidy.
func newGoneRig(t *testing.T, slug string, startTidy *TidyState) *goneRig {
	t.Helper()
	base := t.TempDir()
	r := &goneRig{
		h:        newLoopHarness(t, 3),
		root:     filepath.Join(base, "worktree"),
		primeLog: filepath.Join(base, "prime", "dobby.log"),
		shuttle:  &tidyShuttleFake{},
		events:   make(chan struct{}, 1),
	}
	beamDir := filepath.Join(r.root, ".lym", "beam")
	for _, dir := range []string{beamDir, filepath.Dir(r.primeLog)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
	}
	if err := os.WriteFile(filepath.Join(r.root, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() = %v; want nil", err)
	}
	r.h.geom = Geometry{
		WorktreeName: "beam", Slug: slug, WorktreeRoot: r.root, PrimeLogPath: r.primeLog,
		LockPath: filepath.Join(beamDir, LockFileName), StatePath: filepath.Join(beamDir, StateFileName), LogPath: filepath.Join(beamDir, "dobby.log"),
	}
	r.h.deps.Log = func(path, line string, rotate bool) { _ = AppendLog(path, line, rotate) }
	r.h.deps.PrimeTidyShuttle = r.shuttle
	r.h.deps.Watch = func() (<-chan struct{}, func(), error) { return r.events, func() {}, nil }
	r.h.checkBinary = func(func(), func(error)) { r.checks++ }
	if startTidy != nil {
		r.setTidy(t, startTidy)
	}
	return r
}

// setTidy rewrites the state record's tidy in flight.
func (r *goneRig) setTidy(t *testing.T, tidy *TidyState) {
	t.Helper()
	if err := UpdateState(r.h.geom.StatePath, func(s *State) error { s.Tidy = tidy; return nil }); err != nil {
		t.Fatalf("UpdateState() = %v; want nil", err)
	}
}

// removeWorktree removes the whole worktree, as a teardown does.
func (r *goneRig) removeWorktree(t *testing.T) {
	t.Helper()
	if err := os.RemoveAll(r.root); err != nil {
		t.Fatalf("RemoveAll() = %v; want nil", err)
	}
}

// removeOnFirstSleep makes the loop's first wait remove the worktree and send one watch event, then return.
func (r *goneRig) removeOnFirstSleep(t *testing.T) {
	t.Helper()
	var once sync.Once
	fake := r.h.deps.Sleep
	r.h.deps.Sleep = func(ctx context.Context, d time.Duration) {
		once.Do(func() {
			r.removeWorktree(t)
			r.events <- struct{}{}
		})
		fake(ctx, d)
	}
}

// primeLogText returns what the prime's log holds.
func (r *goneRig) primeLogText(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(r.primeLog)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadFile() = %v; want nil", err)
	}
	return string(data)
}

func TestRun_WorktreeGoneExits(t *testing.T) {
	t.Parallel()
	const exitLine = "stop: the worktree of beam beam-a is gone"
	tidy := &TidyState{StrandGUID: "s1"}

	t.Run("a tidy in flight is stopped and both lines land in the prime's log, with no re-exec check", func(t *testing.T) {
		t.Parallel()
		r := newGoneRig(t, "beam-a", tidy)
		r.h.perTick = func(n int) error {
			if n == 1 {
				r.removeWorktree(t)
				r.events <- struct{}{}
			}
			return nil
		}
		if err := r.h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		text := r.primeLogText(t)
		if len(r.h.ticks) != 1 || len(r.shuttle.stopped) != 1 || r.shuttle.stopped[0] != "s1" {
			t.Errorf("ticks, stopped = %d, %v; want 1, [s1]", len(r.h.ticks), r.shuttle.stopped)
		}
		if !strings.Contains(text, exitLine) || !strings.Contains(text, "stopped the board-tidy agent s1 of beam beam-a") {
			t.Errorf("prime log = %q; want the exit line and the stop line", text)
		}
		if _, err := os.Stat(r.root); !os.IsNotExist(err) {
			t.Errorf("Stat(worktree) = %v; want it absent", err)
		}
		if r.checks != 0 {
			t.Errorf("CheckBinary calls = %d; want 0", r.checks)
		}
	})

	t.Run("a failed stop names lym reed remove in the prime's log", func(t *testing.T) {
		t.Parallel()
		r := newGoneRig(t, "beam-a", tidy)
		r.shuttle.stopErr = errors.New("reed unreachable")
		r.h.perTick = func(n int) error {
			if n == 1 {
				r.removeWorktree(t)
			}
			return nil
		}
		if err := r.h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if text := r.primeLogText(t); !strings.Contains(text, "the board-tidy agent s1 of beam beam-a did not stop: reed unreachable; way forward: lym reed remove s1") {
			t.Errorf("prime log = %q; want the failed stop line", text)
		}
	})

	t.Run("no tidy in flight leaves the exit line alone", func(t *testing.T) {
		t.Parallel()
		r := newGoneRig(t, "beam-a", nil)
		r.h.perTick = func(n int) error {
			if n == 1 {
				r.removeWorktree(t)
			}
			return nil
		}
		if err := r.h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if text := r.primeLogText(t); !strings.Contains(text, exitLine) || strings.Contains(text, "board-tidy") {
			t.Errorf("prime log = %q; want the exit line alone", text)
		}
		if len(r.shuttle.stopped) != 0 {
			t.Errorf("stopped = %v; want none", r.shuttle.stopped)
		}
	})

	t.Run("the tidy the last tick with the worktree present read is the one stopped", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name      string
			startTidy *TidyState
			tick1Tidy *TidyState
			want      []string
		}{
			{"GainedDuringTickOne", nil, &TidyState{StrandGUID: "s2"}, []string{"s2"}},
			{"ClearedDuringTickOne", tidy, nil, nil},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				r := newGoneRig(t, "beam-a", tt.startTidy)
				r.h.perTick = func(n int) error {
					switch n {
					case 1:
						r.setTidy(t, tt.tick1Tidy)
					case 2:
						r.removeWorktree(t)
					}
					return nil
				}
				if err := r.h.run(t); err != nil {
					t.Fatalf("Run() = %v; want nil", err)
				}
				if len(r.h.ticks) != 2 || !slices.Equal(r.shuttle.stopped, tt.want) {
					t.Errorf("ticks, stopped = %d, %v; want 2, %v", len(r.h.ticks), r.shuttle.stopped, tt.want)
				}
			})
		}
	})

	t.Run("a worktree removed after a tick ends exits at the top of the next iteration without a second tick", func(t *testing.T) {
		t.Parallel()
		r := newGoneRig(t, "beam-a", tidy)
		r.removeOnFirstSleep(t)
		if err := r.h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if len(r.h.ticks) != 1 || r.checks != 1 {
			t.Errorf("ticks, CheckBinary calls = %d, %d; want 1, 1", len(r.h.ticks), r.checks)
		}
		if len(r.shuttle.stopped) != 1 || r.shuttle.stopped[0] != "s1" {
			t.Errorf("stopped = %v; want [s1]", r.shuttle.stopped)
		}
		if _, err := os.Stat(r.root); !os.IsNotExist(err) {
			t.Errorf("Stat(worktree) = %v; want it absent, nothing recreated at its path", err)
		}
	})

	t.Run("an unreadable state record keeps the earlier tidy and the loop goes on", func(t *testing.T) {
		t.Parallel()
		r := newGoneRig(t, "beam-a", tidy)
		r.h.perTick = func(n int) error {
			if n == 1 {
				if err := os.Remove(r.h.geom.StatePath); err != nil {
					return err
				}
				return os.Mkdir(r.h.geom.StatePath, 0o755)
			}
			return nil
		}
		r.removeOnFirstSleep(t)
		if err := r.h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if len(r.h.ticks) != 1 || r.checks != 1 || len(r.shuttle.stopped) != 1 || r.shuttle.stopped[0] != "s1" {
			t.Errorf("ticks, CheckBinary calls, stopped = %d, %d, %v; want 1, 1, [s1]", len(r.h.ticks), r.checks, r.shuttle.stopped)
		}
	})

	t.Run("an undecodable state record reads as no tidy", func(t *testing.T) {
		t.Parallel()
		r := newGoneRig(t, "beam-a", tidy)
		r.h.perTick = func(n int) error {
			if n == 1 {
				return os.WriteFile(r.h.geom.StatePath, []byte("{not json"), 0o644)
			}
			return nil
		}
		r.removeOnFirstSleep(t)
		if err := r.h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if len(r.h.ticks) != 1 || r.checks != 1 || len(r.shuttle.stopped) != 0 {
			t.Errorf("ticks, CheckBinary calls, stopped = %d, %d, %v; want 1, 1, none", len(r.h.ticks), r.checks, r.shuttle.stopped)
		}
	})

	t.Run("a prime dobby whose git link is missing goes on ticking", func(t *testing.T) {
		t.Parallel()
		r := newGoneRig(t, "", tidy)
		if err := os.Remove(filepath.Join(r.root, ".git")); err != nil {
			t.Fatalf("Remove() = %v; want nil", err)
		}
		if err := r.h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if len(r.h.ticks) != 3 || len(r.shuttle.stopped) != 0 {
			t.Errorf("ticks, stopped = %d, %v; want 3, none", len(r.h.ticks), r.shuttle.stopped)
		}
	})
}
