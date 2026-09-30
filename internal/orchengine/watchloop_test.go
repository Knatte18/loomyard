package orchengine

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

// flakySession fails StrandAlive per a script, then defers to the embedded fake.
type flakySession struct {
	*fakeSession
	errs  []bool // true means the tick errors; consumed one per StrandAlive call.
	calls int
}

func (f *flakySession) StrandAlive(g string) (bool, error) {
	f.calls++
	if len(f.errs) > 0 {
		fail := f.errs[0]
		f.errs = f.errs[1:]
		if fail {
			return false, errBoom
		}
		return true, nil
	}
	return f.fakeSession.StrandAlive(g)
}

func noSleep(time.Duration) {}

func holdWatchLock(t *testing.T, p Paths) *lock.FileLock {
	t.Helper()
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	l, ok, err := lock.TryAcquireWriteLock(p.WatchLockPath)
	if err != nil || !ok {
		t.Fatalf("hold watch lock: ok=%v err=%v", ok, err)
	}
	return l
}

func TestRun_ErrWatcherRunningWhileLockHeld(t *testing.T) {
	e := newWatchEnv(t)
	l := holdWatchLock(t, e.paths)
	defer l.Release()

	sleeps := 0
	err := e.w.Run(context.Background(), func(time.Duration) { sleeps++ })
	if !errors.Is(err, ErrWatcherRunning) {
		t.Fatalf("Run = %v, want ErrWatcherRunning", err)
	}
	if sleeps != watchLockAttempts-1 {
		t.Errorf("retry sleeps = %d, want %d", sleeps, watchLockAttempts-1)
	}
}

func TestRun_TakesLockReleasedBetweenRetries(t *testing.T) {
	e := newWatchEnv(t)
	e.s.alive = false
	l := holdWatchLock(t, e.paths)

	released := false
	err := e.w.Run(context.Background(), func(time.Duration) {
		if !released {
			released = true
			l.Release()
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !released {
		t.Error("Run never retried")
	}
}

func TestWatcherLive(t *testing.T) {
	p := testPaths(t)
	live, err := WatcherLive(p)
	if err != nil || live {
		t.Fatalf("no watcher: live=%v err=%v", live, err)
	}
	l := holdWatchLock(t, p)
	live, err = WatcherLive(p)
	if err != nil || !live {
		t.Fatalf("held: live=%v err=%v", live, err)
	}
	l.Release()
	live, err = WatcherLive(p)
	if err != nil || live {
		t.Fatalf("released: live=%v err=%v", live, err)
	}
}

func TestRun_ReturnsWhenStrandGone(t *testing.T) {
	e := newWatchEnv(t)
	e.s.alive = false
	st, _ := LoadState(e.paths)
	st.WatcherExit = "stale"
	if err := SaveState(e.paths, st); err != nil {
		t.Fatal(err)
	}

	if err := e.w.Run(context.Background(), noSleep); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := LoadState(e.paths)
	if got.WatcherExit != "strand gone" {
		t.Errorf("WatcherExit = %q, want %q", got.WatcherExit, "strand gone")
	}
	if live, _ := WatcherLive(e.paths); live {
		t.Error("watch lock still held after Run returned")
	}
}

func TestRun_CancelledContextRecordsSignal(t *testing.T) {
	e := newWatchEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	sleeps := 0
	err := e.w.Run(ctx, func(time.Duration) {
		sleeps++
		if sleeps == 3 {
			cancel()
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := LoadState(e.paths)
	if got.WatcherExit != "stopped by signal" {
		t.Errorf("WatcherExit = %q, want stopped by signal", got.WatcherExit)
	}
}

func TestRun_StopsAtErrorCap(t *testing.T) {
	e := newWatchEnv(t)
	fs := &flakySession{fakeSession: e.s, errs: make([]bool, 0)}
	for i := 0; i < maxConsecutiveTickErrors+10; i++ {
		fs.errs = append(fs.errs, true)
	}
	w := NewWatcher(fs, e.cfg, e.paths, e.stDir, e.clock)

	err := w.Run(context.Background(), noSleep)
	if !errors.Is(err, errBoom) {
		t.Fatalf("Run = %v, want errBoom", err)
	}
	if fs.calls != maxConsecutiveTickErrors {
		t.Errorf("ticks = %d, want exactly %d", fs.calls, maxConsecutiveTickErrors)
	}
	got, _ := LoadState(e.paths)
	if got.WatcherExit == "" {
		t.Error("WatcherExit not recorded")
	}
}

func TestRun_SuccessfulTickResetsErrorCount(t *testing.T) {
	e := newWatchEnv(t)
	fs := &flakySession{fakeSession: e.s}
	for i := 0; i < maxConsecutiveTickErrors-1; i++ {
		fs.errs = append(fs.errs, true)
	}
	fs.errs = append(fs.errs, false)
	for i := 0; i < maxConsecutiveTickErrors+5; i++ {
		fs.errs = append(fs.errs, true)
	}
	w := NewWatcher(fs, e.cfg, e.paths, e.stDir, e.clock)

	if err := w.Run(context.Background(), noSleep); !errors.Is(err, errBoom) {
		t.Fatalf("Run = %v, want errBoom", err)
	}
	want := (maxConsecutiveTickErrors - 1) + 1 + maxConsecutiveTickErrors
	if fs.calls != want {
		t.Errorf("ticks = %d, want %d", fs.calls, want)
	}
}
