//go:build tier0

// loop_test.go pins dobby's daemon loop over a fake clock, a fake watch and a temp state record: the lock, the owner exit, the wakeups, the sleep record and the start-or-re-exec reading.

package dobbyengine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// loopClock is a fake wall clock and awake clock: Sleep advances both, and suspend advances only the wall clock, as a machine sleep does.
type loopClock struct {
	mu        sync.Mutex
	wall      time.Time
	suspended time.Duration
	waits     []time.Duration
}

func newLoopClock() *loopClock {
	return &loopClock{wall: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}
}

func (c *loopClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wall
}

func (c *loopClock) awake() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wall.Sub(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)) - c.suspended
}

func (c *loopClock) sleep(_ context.Context, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wall = c.wall.Add(d)
	c.waits = append(c.waits, d)
}

func (c *loopClock) suspend(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wall = c.wall.Add(d)
	c.suspended += d
}

// loopLogCall is one call of the Log seam.
type loopLogCall struct {
	path, line string
	rotate     bool
}

// loopStartTicks is the start ticks the harness tells the dobby.
const loopStartTicks = "4242"

// loopHarness runs the loop and stops it on the tick named by stopAt.
type loopHarness struct {
	clock  *loopClock
	geom   Geometry
	deps   Deps
	ticks  []TickInput
	cancel context.CancelFunc
	stopAt int
	// perTick runs inside the tick seam with the 1-based tick count, and its error is the tick's.
	perTick func(n int) error

	mu      sync.Mutex
	logs    []loopLogCall
	notices []string
	locked  int
	// checkBinary is the CheckBinary fake; nil does nothing.
	checkBinary func(beforeExec func(), failed func(error))
}

// logged returns the Log calls so far.
func (h *loopHarness) logged() []loopLogCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.logs)
}

// noticed returns the notices posted so far.
func (h *loopHarness) noticed() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.notices)
}

// loggedLine returns the first logged call whose line ends with suffix.
func (h *loopHarness) loggedLine(suffix string) (loopLogCall, bool) {
	for _, call := range h.logged() {
		if strings.HasSuffix(call.line, suffix) {
			return call, true
		}
	}
	return loopLogCall{}, false
}

func newLoopHarness(t *testing.T, stopAt int) *loopHarness {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "beam")
	h := &loopHarness{
		clock: newLoopClock(),
		geom: Geometry{
			WorktreeName: "prime", LockPath: filepath.Join(dir, LockFileName), StatePath: filepath.Join(dir, StateFileName),
			LogPath: filepath.Join(dir, "dobby.log"),
		},
		stopAt: stopAt,
	}
	h.deps = Deps{
		Now:        h.clock.now,
		Awake:      h.clock.awake,
		Sleep:      h.clock.sleep,
		Machine:    func() (string, error) { return "m1", nil },
		Owner:      func() (string, error) { return "", nil },
		StartTicks: func() string { return loopStartTicks },
		CheckBinary: func(beforeExec func(), failed func(error)) {
			if h.checkBinary != nil {
				h.checkBinary(beforeExec, failed)
			}
		},
		Log: func(path, line string, rotate bool) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.logs = append(h.logs, loopLogCall{path, line, rotate})
		},
		Notice: func(_ context.Context, line string) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.notices = append(h.notices, line)
			return nil
		},
		Locked: func() error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.locked++
			return nil
		},
		ProbeInterval: 10 * time.Second,
		Runs:          func() ([]RunView, error) { return nil, nil },
		LoadConfig:    func() (Config, error) { return Config{}, nil },
		Tick: func(_ context.Context, in TickInput) (bool, error) {
			h.ticks = append(h.ticks, in)
			var err error
			if h.perTick != nil {
				err = h.perTick(len(h.ticks))
			}
			if len(h.ticks) >= h.stopAt {
				h.cancel()
			}
			return false, err
		},
	}
	return h
}

func (h *loopHarness) run(t *testing.T) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ctx, h.cancel = context.WithCancel(ctx)
	return Run(ctx, h.geom, h.deps)
}

func TestRun_LockOwnerAndWakeups(t *testing.T) {
	t.Parallel()

	t.Run("a held lock on a fresh start returns ErrDobbyRunning, logs the refusal unrotated and posts nothing", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		holdLoopLock(t, h)

		if err := h.run(t); !errors.Is(err, ErrDobbyRunning) {
			t.Errorf("Run() = %v; want ErrDobbyRunning", err)
		}
		if _, err := os.Stat(h.geom.StatePath); !os.IsNotExist(err) {
			t.Errorf("Stat(state) = %v; want it absent", err)
		}
		if len(h.ticks) != 0 || h.locked != 0 || len(h.noticed()) != 0 {
			t.Errorf("ticks, Locked calls, notices = %d, %d, %v; want 0, 0, none", len(h.ticks), h.locked, h.noticed())
		}
		call, ok := h.loggedLine("dobby: start refused: another dobby holds the lock")
		if !ok || call.rotate || call.path != h.geom.LogPath {
			t.Errorf("refusal log = %+v, %v; want the line on the log path with rotate false", call, ok)
		}
	})

	t.Run("an owner read naming another machine exits at start and logs the adopt way forward", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 5)
		h.deps.Owner = func() (string, error) { return "m2", nil }
		if err := h.run(t); err != nil {
			t.Errorf("Run() = %v; want nil", err)
		}
		if len(h.ticks) != 0 {
			t.Errorf("ticks = %d; want 0", len(h.ticks))
		}
		if _, ok := h.loggedLine("stop: another machine owns this beam (m2); lym adopt root takes it over here"); !ok {
			t.Errorf("log = %+v; want the owner stop line naming lym adopt root", h.logged())
		}
	})

	t.Run("a beam dobby's owner stop line names its slug", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 5)
		h.geom.Slug, h.geom.WorktreeRoot = "beam-a", t.TempDir()
		if err := os.WriteFile(filepath.Join(h.geom.WorktreeRoot, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}
		h.deps.Owner = func() (string, error) { return "m2", nil }
		if err := h.run(t); err != nil {
			t.Errorf("Run() = %v; want nil", err)
		}
		if _, ok := h.loggedLine("lym adopt beam-a takes it over here"); !ok {
			t.Errorf("log = %+v; want the owner stop line naming lym adopt beam-a", h.logged())
		}
	})

	t.Run("an owner that moves to another machine on a probe tick exits then", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 5)
		reads := 0
		h.deps.Owner = func() (string, error) {
			reads++
			if reads == 1 {
				return "m1", nil
			}
			return "m2", nil
		}
		if err := h.run(t); err != nil {
			t.Errorf("Run() = %v; want nil", err)
		}
		if len(h.ticks) != 1 || reads != 2 {
			t.Errorf("ticks, owner reads = %d, %d; want 1, 2", len(h.ticks), reads)
		}
	})

	t.Run("a burst of five events in one second runs one tick", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 2)
		events := make(chan struct{}, 5)
		for range 5 {
			events <- struct{}{}
		}
		h.deps.Watch = func() (<-chan struct{}, func(), error) { return events, func() {}, nil }
		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if len(h.ticks) != 2 || len(events) != 0 {
			t.Errorf("ticks, undrained events = %d, %d; want 2, 0", len(h.ticks), len(events))
		}
		if len(h.clock.waits) != 1 || h.clock.waits[0] != time.Second {
			t.Errorf("waits = %v; want the one second the coalescing holds", h.clock.waits)
		}
	})

	t.Run("a sleep longer than a minute between ticks is recorded in the state and owes every run a full pass", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 3)
		h.perTick = func(n int) error {
			if n == 1 {
				h.clock.suspend(5 * time.Minute)
			}
			return nil
		}
		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		record, found, err := ReadState(h.geom.StatePath)
		if err != nil || !found || len(record.Sleeps) != 1 {
			t.Fatalf("ReadState() = %+v, %v, %v; want one recorded sleep", record, found, err)
		}
		if got := record.Sleeps[0].End.Sub(record.Sleeps[0].Start); got < 4*time.Minute || got > 5*time.Minute {
			t.Errorf("recorded sleep = %v; want about five minutes", got)
		}
		if owes := []bool{h.ticks[0].OwesAll, h.ticks[1].OwesAll, h.ticks[2].OwesAll}; !owes[0] || !owes[1] || owes[2] {
			t.Errorf("OwesAll over three ticks = %v; want true (start), true (the sleep), false", owes)
		}
	})

	t.Run("a watch that fails to open still ticks on the timer", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 3)
		h.deps.Watch = func() (<-chan struct{}, func(), error) { return nil, nil, errors.New("no inotify") }
		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if len(h.ticks) != 3 || h.clock.waits[0] != h.deps.ProbeInterval {
			t.Errorf("ticks, first wait = %d, %v; want 3, the probe interval", len(h.ticks), h.clock.waits[0])
		}
		if call, ok := h.loggedLine("stop: the context ended"); !ok || !call.rotate {
			t.Errorf("stop log = %+v, %v; want the cancel line with rotate true", call, ok)
		}
	})

	t.Run("a state record holding the dobby's own pid reads as a re-exec and another pid as a start", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name    string
			pid     int
			wantOwe bool
		}{
			{"OwnPid", os.Getpid(), false},
			{"OtherPid", os.Getpid() + 1, true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newLoopHarness(t, 1)
				if err := UpdateState(h.geom.StatePath, func(s *State) error { s.PID, s.StartTicks = tt.pid, loopStartTicks; return nil }); err != nil {
					t.Fatalf("UpdateState() = %v; want nil", err)
				}
				if err := h.run(t); err != nil {
					t.Fatalf("Run() = %v; want nil", err)
				}
				if h.ticks[0].OwesAll != tt.wantOwe {
					t.Errorf("first tick OwesAll = %v; want %v", h.ticks[0].OwesAll, tt.wantOwe)
				}
			})
		}
	})

	t.Run("a record made undecodable between two ticks is rewritten fresh and owes every run a full pass", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 3)
		h.perTick = func(n int) error {
			if n == 2 {
				return os.WriteFile(h.geom.StatePath, []byte("{not json"), 0o644)
			}
			return nil
		}
		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		record, found, err := ReadState(h.geom.StatePath)
		if err != nil || !found || record.PID != os.Getpid() || record.StartTicks != loopStartTicks {
			t.Errorf("ReadState() = %+v, %v, %v; want the pid and start ticks recorded again", record, found, err)
		}
		if h.ticks[1].OwesAll || !h.ticks[2].OwesAll {
			t.Errorf("OwesAll on ticks two and three = %v, %v; want false, true", h.ticks[1].OwesAll, h.ticks[2].OwesAll)
		}
	})

	t.Run("a fresh start with a free lock registers through Locked and logs the start", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if _, ok := h.loggedLine(fmt.Sprintf("start fresh pid %d", os.Getpid())); !ok || h.locked != 1 {
			t.Errorf("log, Locked calls = %+v, %d; want the fresh start line and one call", h.logged(), h.locked)
		}
	})

	t.Run("a re-exec whose lock stays held past the wait logs the refusal and posts one notice", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		h.deps.ReexecLockWait = 400 * time.Millisecond
		writeLoopRecord(t, h, os.Getpid(), loopStartTicks)
		holdLoopLock(t, h)

		if err := h.run(t); !errors.Is(err, ErrDobbyRunning) {
			t.Fatalf("Run() = %v; want ErrDobbyRunning", err)
		}
		if call, ok := h.loggedLine("dobby: start refused: another dobby holds the lock"); !ok || call.rotate {
			t.Errorf("refusal log = %+v, %v; want the line with rotate false", call, ok)
		}
		if notices := h.noticed(); len(notices) != 1 || !strings.Contains(notices[0], "lym beam start") {
			t.Errorf("notices = %v; want one naming lym beam start", notices)
		}
		if h.locked != 0 {
			t.Errorf("Locked calls = %d; want 0", h.locked)
		}
	})

	t.Run("a re-exec whose record another dobby rewrites during the wait posts nothing", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		h.deps.ReexecLockWait = time.Second
		writeLoopRecord(t, h, os.Getpid(), loopStartTicks)
		holdLoopLock(t, h)
		rewritten := make(chan error, 1)
		go func() {
			<-time.After(500 * time.Millisecond)
			rewritten <- UpdateState(h.geom.StatePath, func(s *State) error { s.PID = os.Getpid() + 1; return nil })
		}()

		if err := h.run(t); !errors.Is(err, ErrDobbyRunning) {
			t.Fatalf("Run() = %v; want ErrDobbyRunning", err)
		}
		if err := <-rewritten; err != nil {
			t.Fatalf("UpdateState() = %v; want nil", err)
		}
		if notices := h.noticed(); len(notices) != 0 {
			t.Errorf("notices = %v; want none", notices)
		}
	})

	t.Run("a re-exec takes a lock released after the probe grace and owes nothing", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		h.deps.ReexecLockWait = 3 * time.Second
		writeLoopRecord(t, h, os.Getpid(), loopStartTicks)
		wait := releaseLoopLockAfter(t, h, lock.ProbeGrace+time.Second)

		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		wait()
		if len(h.ticks) != 1 || h.ticks[0].OwesAll {
			t.Errorf("ticks = %+v; want one tick owing no full pass", h.ticks)
		}
		if _, ok := h.loggedLine(fmt.Sprintf("start re-exec pid %d", os.Getpid())); !ok {
			t.Errorf("log = %+v; want the re-exec start line", h.logged())
		}
	})

	t.Run("a record with this pid and other ticks is a fresh start refused after the probe grace without a notice", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		h.deps.ReexecLockWait = 3 * time.Second
		writeLoopRecord(t, h, os.Getpid(), "other")
		holdLoopLock(t, h)

		started := time.Now()
		if err := h.run(t); !errors.Is(err, ErrDobbyRunning) {
			t.Fatalf("Run() = %v; want ErrDobbyRunning", err)
		}
		if waited := time.Since(started); waited > 2*time.Second {
			t.Errorf("Run() waited %v; want only the probe grace", waited)
		}
		if notices := h.noticed(); len(notices) != 0 {
			t.Errorf("notices = %v; want none", notices)
		}
	})

	t.Run("a record with this pid and other ticks under a free lock owes a full pass and is left holding this process's ticks", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		writeLoopRecord(t, h, os.Getpid(), "other")
		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		record, _, err := ReadState(h.geom.StatePath)
		if err != nil || record.StartTicks != loopStartTicks || !h.ticks[0].OwesAll {
			t.Errorf("record, first tick = %+v, %+v; want this process's ticks and a full pass owed (err %v)", record, h.ticks[0], err)
		}
		if _, ok := h.loggedLine(fmt.Sprintf("start fresh pid %d", os.Getpid())); !ok {
			t.Errorf("log = %+v; want the fresh start line", h.logged())
		}
	})

	t.Run("a record with this pid and no ticks reads as a re-exec, owes a full pass and gains the ticks", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		h.deps.ReexecLockWait = 3 * time.Second
		writeLoopRecord(t, h, os.Getpid(), "")
		wait := releaseLoopLockAfter(t, h, lock.ProbeGrace+time.Second)

		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		wait()
		record, _, err := ReadState(h.geom.StatePath)
		if err != nil || record.StartTicks != loopStartTicks || !h.ticks[0].OwesAll {
			t.Errorf("record, first tick = %+v, %+v; want this process's ticks and a full pass owed (err %v)", record, h.ticks[0], err)
		}
	})

	t.Run("a record with this pid and no ticks under a held lock posts the refused re-exec notice", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		h.deps.ReexecLockWait = 400 * time.Millisecond
		writeLoopRecord(t, h, os.Getpid(), "")
		holdLoopLock(t, h)

		if err := h.run(t); !errors.Is(err, ErrDobbyRunning) {
			t.Fatalf("Run() = %v; want ErrDobbyRunning", err)
		}
		if notices := h.noticed(); len(notices) != 1 {
			t.Errorf("notices = %v; want one", notices)
		}
	})

	t.Run("a failed re-exec writes its line and one notice, and the loop goes on", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 3)
		calls := 0
		h.checkBinary = func(beforeExec func(), failed func(error)) {
			calls++
			if calls == 1 {
				beforeExec()
				failed(errors.New("exec refused"))
			}
		}
		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if _, ok := h.loggedLine("re-exec onto the changed lym binary"); !ok {
			t.Errorf("log = %+v; want the before-exec line", h.logged())
		}
		if _, ok := h.loggedLine("re-exec failed: exec refused; the dobby keeps running"); !ok {
			t.Errorf("log = %+v; want the failed line", h.logged())
		}
		if notices := h.noticed(); len(notices) != 1 || !strings.Contains(notices[0], "exec refused") || len(h.ticks) != 3 {
			t.Errorf("notices, ticks = %v, %d; want one naming the error, and 3 ticks", notices, len(h.ticks))
		}
	})

	t.Run("a Locked error returns the error, logs the stop and posts a notice only after a re-exec", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name        string
			ticks       string
			wantNotices int
		}{
			{"AfterReexec", loopStartTicks, 1},
			{"AfterFreshStart", "other", 0},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newLoopHarness(t, 1)
				writeLoopRecord(t, h, os.Getpid(), tt.ticks)
				boom := errors.New("registry unwritable")
				h.deps.Locked = func() error { return boom }
				if err := h.run(t); !errors.Is(err, boom) {
					t.Fatalf("Run() = %v; want the Locked error", err)
				}
				call, ok := h.loggedLine("dobby: stopped on error: registry unwritable; way forward: fix the cause, then run lym beam start")
				if !ok || !call.rotate {
					t.Errorf("stop log = %+v, %v; want the line with rotate true, the lock being held", call, ok)
				}
				if notices := h.noticed(); len(notices) != tt.wantNotices {
					t.Errorf("notices = %v; want %d", notices, tt.wantNotices)
				}
			})
		}
	})

	t.Run("a state record that cannot be read fails the re-exec test, logs the stop unrotated and posts nothing", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		if err := os.MkdirAll(h.geom.StatePath, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		if err := h.run(t); err == nil {
			t.Fatalf("Run() = nil; want the read error")
		}
		call, ok := h.loggedLine("; way forward: fix the cause, then run lym beam start")
		if !ok || call.rotate || !strings.Contains(call.line, "dobby: stopped on error: ") {
			t.Errorf("stop log = %+v, %v; want the stop line with rotate false", call, ok)
		}
		if notices := h.noticed(); len(notices) != 0 {
			t.Errorf("notices = %v; want none", notices)
		}
	})
}

func TestReadsAsReexec(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		record *State
		want   bool
	}{
		{"OwnPidOwnTicks", &State{PID: 7, StartTicks: "t1"}, true},
		{"OwnPidNoTicks", &State{PID: 7}, true},
		{"OwnPidOtherTicks", &State{PID: 7, StartTicks: "t2"}, false},
		{"OtherPid", &State{PID: 8, StartTicks: "t1"}, false},
		{"NoRecord", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), StateFileName)
			if tt.record != nil {
				if err := UpdateState(path, func(s *State) error { *s = *tt.record; return nil }); err != nil {
					t.Fatalf("UpdateState() = %v; want nil", err)
				}
			}
			got, err := ReadsAsReexec(path, 7, "t1")
			if err != nil || got != tt.want {
				t.Errorf("ReadsAsReexec() = %v, %v; want %v, nil", got, err, tt.want)
			}
		})
	}

	t.Run("MissingDirectoryReadsFalseAndStaysAbsent", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "gone")
		got, err := ReadsAsReexec(filepath.Join(dir, StateFileName), 7, "t1")
		if err != nil || got {
			t.Errorf("ReadsAsReexec() = %v, %v; want false, nil", got, err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("Stat(dir) = %v; want it still absent", err)
		}
	})
}

// holdLoopLock takes the harness's dobby lock until the test ends.
func holdLoopLock(t *testing.T, h *loopHarness) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(h.geom.LockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v; want nil", err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(h.geom.LockPath)
	if err != nil || !acquired {
		t.Fatalf("TryAcquireWriteLock() = %v, %v; want it acquired", acquired, err)
	}
	t.Cleanup(func() { _ = held.Release() })
}

// releaseLoopLockAfter takes the harness's dobby lock and releases it after d on its own goroutine.
// The returned function waits for that release, so the test leaves no goroutine behind.
func releaseLoopLockAfter(t *testing.T, h *loopHarness, d time.Duration) (wait func()) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(h.geom.LockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v; want nil", err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(h.geom.LockPath)
	if err != nil || !acquired {
		t.Fatalf("TryAcquireWriteLock() = %v, %v; want it acquired", acquired, err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-time.After(d)
		_ = held.Release()
	}()
	return func() { <-done }
}

// writeLoopRecord writes a state record holding pid and startTicks at the harness's state path.
func writeLoopRecord(t *testing.T, h *loopHarness, pid int, startTicks string) {
	t.Helper()
	if err := UpdateState(h.geom.StatePath, func(s *State) error { s.PID, s.StartTicks = pid, startTicks; return nil }); err != nil {
		t.Fatalf("UpdateState() = %v; want nil", err)
	}
}

// TestRun_TickFailuresLogOncePerEpisode captures the process-global logger output, so it does not run in parallel.
func TestRun_TickFailuresLogOncePerEpisode(t *testing.T) {
	var captured bytes.Buffer
	logger.SetVerbosity(1)
	logger.SetOutput(&captured)
	t.Cleanup(func() {
		logger.SetVerbosity(0)
		logger.SetOutput(logger.DefaultOutput())
	})

	h := newLoopHarness(t, 5)
	h.perTick = func(n int) error {
		if n <= 3 {
			return errors.New("board unreadable")
		}
		return nil
	}
	if err := h.run(t); err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}
	out := captured.String()
	for _, line := range []string{"a tick failed", "ticks succeed again"} {
		if got := strings.Count(out, line); got != 1 {
			t.Errorf("log lines %q = %d; want 1\n%s", line, got, out)
		}
	}
	for prefix, want := range map[string]int{"tick failing: board unreadable": 1, "ticks succeed again": 1} {
		if got := h.countLogged(prefix); got != want {
			t.Errorf("dobby log lines starting %q = %d; want %d\n%v", prefix, got, want, h.logged())
		}
	}

	owner := newLoopHarness(t, 5)
	reads := 0
	owner.deps.Owner = func() (string, error) {
		reads++
		if reads <= 2 {
			return "", errors.New("board unreadable")
		}
		return "", nil
	}
	if err := owner.run(t); err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}
	for prefix, want := range map[string]int{"owner unreadable: board unreadable": 1, "owner reads again": 1} {
		if got := owner.countLogged(prefix); got != want {
			t.Errorf("dobby log lines starting %q = %d; want %d\n%v", prefix, got, want, owner.logged())
		}
	}
}

// countLogged counts the logged lines whose text, after the timestamp, starts with prefix.
func (h *loopHarness) countLogged(prefix string) int {
	count := 0
	for _, call := range h.logged() {
		if _, text, _ := strings.Cut(call.line, " "); strings.HasPrefix(text, prefix) {
			count++
		}
	}
	return count
}

func TestRun_Heartbeat(t *testing.T) {
	t.Parallel()
	listedRun := RunView{Slug: "a", State: runStateRunning, Producer: producerRunShed}

	// ticksAcrossTen runs three ticks whose clock is nine minutes and ten seconds past the start at the second and just past ten minutes at the third.
	ticksAcrossTen := func(t *testing.T, h *loopHarness, atThird func()) (beforeThird, total int) {
		t.Helper()
		h.perTick = func(n int) error {
			switch n {
			case 1:
				h.clock.suspend(9 * time.Minute)
			case 2:
				beforeThird = h.countLogged("heartbeat:")
				h.clock.suspend(time.Minute)
				if atThird != nil {
					atThird()
				}
			}
			return nil
		}
		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		return beforeThird, h.countLogged("heartbeat:")
	}

	t.Run("the start writes a heartbeat naming each run, and a tick at ten minutes writes the next, not one at nine", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 3)
		h.deps.Runs = func() ([]RunView, error) { return []RunView{listedRun}, nil }
		h.deps.SpawnPass = func(string, bool, bool) (<-chan PassExit, error) { return make(chan PassExit), nil }
		before, after := ticksAcrossTen(t, h, nil)
		if before != 1 || after != 2 {
			t.Errorf("heartbeats before the third tick, after it = %d, %d; want 1, 2", before, after)
		}
		if h.countLogged("heartbeat: a running Run-Shed") != 2 {
			t.Errorf("log = %+v; want both heartbeats to name the run's state and producer", h.logged())
		}
	})

	t.Run("a re-exec with an unchanged run logs no change line on its first tick", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		h.deps.Runs = func() ([]RunView, error) { return []RunView{listedRun}, nil }
		h.deps.SpawnPass = func(string, bool, bool) (<-chan PassExit, error) { return make(chan PassExit), nil }
		writeLoopRecord(t, h, os.Getpid(), loopStartTicks)
		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if got := h.countLogged("run a:"); got != 0 {
			t.Errorf("change lines = %d; want 0\n%+v", got, h.logged())
		}
	})

	t.Run("a re-exec whose start heartbeat cannot read the runs logs no change line on its first tick", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		reads := 0
		h.deps.Runs = func() ([]RunView, error) {
			if reads++; reads == 1 {
				return nil, errors.New("status unreadable")
			}
			return []RunView{listedRun}, nil
		}
		h.deps.SpawnPass = func(string, bool, bool) (<-chan PassExit, error) { return make(chan PassExit), nil }
		writeLoopRecord(t, h, os.Getpid(), loopStartTicks)
		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if got := h.countLogged("heartbeat: runs unreadable"); got != 1 {
			t.Fatalf("unreadable heartbeats = %d; want 1\n%+v", got, h.logged())
		}
		if got := h.countLogged("run a:"); got != 0 {
			t.Errorf("change lines = %d; want 0\n%+v", got, h.logged())
		}
	})

	t.Run("no runs listed says so", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}
		if got := h.countLogged("heartbeat: no runs"); got != 1 {
			t.Errorf("no-runs heartbeats = %d; want 1\n%+v", got, h.logged())
		}
	})

	t.Run("a standing anchor change keeps no heartbeat from the ten-minute tick", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 3)
		record := filepath.Join(t.TempDir(), "move.json")
		if err := os.WriteFile(record, []byte("{}"), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}
		h.geom.MoveRecords = []string{record}
		if _, after := ticksAcrossTen(t, h, nil); after != 2 {
			t.Errorf("heartbeats = %d; want 2", after)
		}
	})

	t.Run("a run listing that fails from the ten-minute tick on writes the unreadable heartbeat", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 3)
		failing, failedReads := false, 0
		h.deps.Runs = func() ([]RunView, error) {
			if !failing {
				return nil, nil
			}
			// The failing tick never reaches the Tick seam that stops the loop, so the heartbeat's own read, the second, ends it.
			if failedReads++; failedReads == 2 {
				h.cancel()
			}
			return nil, errors.New("status unreadable")
		}
		ticksAcrossTen(t, h, func() { failing = true })
		if got := h.countLogged("heartbeat: runs unreadable: status unreadable"); got != 1 {
			t.Errorf("unreadable heartbeats = %d; want 1\n%+v", got, h.logged())
		}
	})
}

func TestNextWait(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		current  time.Duration
		inFlight bool
		event    bool
		want     time.Duration
	}{
		{"IdleDoubles", 10 * time.Second, false, false, 20 * time.Second},
		{"IdleStopsAtTheCeiling", 40 * time.Second, false, false, time.Minute},
		{"IdleStaysAtTheCeiling", time.Minute, false, false, time.Minute},
		{"AnEventResetsToTheProbeInterval", 40 * time.Second, false, true, 0},
		{"InFlightKeepsTheProbeInterval", 40 * time.Second, true, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := nextWait(tt.current, tt.inFlight, tt.event); got != tt.want {
				t.Errorf("nextWait(%v, %v, %v) = %v; want %v", tt.current, tt.inFlight, tt.event, got, tt.want)
			}
		})
	}
}
