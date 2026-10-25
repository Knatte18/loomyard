//go:build tier0

// loop_test.go pins dobby's daemon loop over a fake clock, a fake watch and a temp state record: the lock, the owner exit, the wakeups, the sleep record and the start-or-re-exec reading.

package dobbyengine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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
}

func newLoopHarness(t *testing.T, stopAt int) *loopHarness {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "beam")
	h := &loopHarness{
		clock:  newLoopClock(),
		geom:   Geometry{WorktreeName: "prime", LockPath: filepath.Join(dir, LockFileName), StatePath: filepath.Join(dir, StateFileName)},
		stopAt: stopAt,
	}
	h.deps = Deps{
		Now:           h.clock.now,
		Awake:         h.clock.awake,
		Sleep:         h.clock.sleep,
		Machine:       func() (string, error) { return "m1", nil },
		Owner:         func() (string, error) { return "", nil },
		CheckBinary:   func() {},
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

	t.Run("a held lock returns ErrDobbyRunning and writes no state", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 1)
		if err := os.MkdirAll(filepath.Dir(h.geom.LockPath), 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		held, acquired, err := lock.TryAcquireWriteLock(h.geom.LockPath)
		if err != nil || !acquired {
			t.Fatalf("TryAcquireWriteLock() = %v, %v; want it acquired", acquired, err)
		}
		defer func() { _ = held.Release() }()

		if err := h.run(t); !errors.Is(err, ErrDobbyRunning) {
			t.Errorf("Run() = %v; want ErrDobbyRunning", err)
		}
		if _, err := os.Stat(h.geom.StatePath); !os.IsNotExist(err) {
			t.Errorf("Stat(state) = %v; want it absent", err)
		}
		if len(h.ticks) != 0 {
			t.Errorf("ticks = %d; want 0", len(h.ticks))
		}
	})

	t.Run("an owner read naming another machine exits at start", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 5)
		h.deps.Owner = func() (string, error) { return "m2", nil }
		if err := h.run(t); err != nil {
			t.Errorf("Run() = %v; want nil", err)
		}
		if len(h.ticks) != 0 {
			t.Errorf("ticks = %d; want 0", len(h.ticks))
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
				if err := UpdateState(h.geom.StatePath, func(s *State) error { s.PID = tt.pid; return nil }); err != nil {
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
		if err != nil || !found || record.PID != os.Getpid() {
			t.Errorf("ReadState() = %+v, %v, %v; want the pid recorded again", record, found, err)
		}
		if h.ticks[1].OwesAll || !h.ticks[2].OwesAll {
			t.Errorf("OwesAll on ticks two and three = %v, %v; want false, true", h.ticks[1].OwesAll, h.ticks[2].OwesAll)
		}
	})
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
