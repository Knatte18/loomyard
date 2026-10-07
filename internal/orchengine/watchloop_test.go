package orchengine

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

func TestRun_NoticeQueueAtExit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		clearStrand   bool
		wantNotices   int
		wantDirAbsent bool
	}{
		{"dropped when no strand is recorded", true, 0, true},
		{"kept when a strand is recorded", false, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.s.alive = false
			e.queue("notice")
			if c.clearStrand {
				e.setState(func(st *State) { st.Strand = "" })
			}

			if err := e.w.Run(context.Background(), noSleep); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := e.noticeLines(); len(got) != c.wantNotices {
				t.Errorf("queue = %v, want %d notices", got, c.wantNotices)
			}
			if c.wantDirAbsent {
				if _, err := os.Stat(e.paths.NoticesDir); !os.IsNotExist(err) {
					t.Errorf("notices dir stat = %v, want absent", err)
				}
			}
		})
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

func TestRun_RecordsStoppingBeforeTheInFlightTickFinishes(t *testing.T) {
	t.Parallel()

	e := newWatchEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	// A turn end read after the signal makes the in-flight tick save the state it loaded before it.
	e.s.events = []shuttleengine.Event{{Kind: shuttleengine.EventStop, Message: "turn end"}}
	inTick := make(chan struct{})
	finishTick := make(chan struct{})
	blocked := false
	e.s.onAlive = func() {
		if blocked {
			return
		}
		blocked = true
		close(inTick)
		<-finishTick
	}

	runDone := make(chan error, 1)
	go func() { runDone <- e.w.Run(ctx, noSleep) }()
	<-inTick
	cancel()

	deadline := time.Now().Add(10 * time.Second)
	for {
		st, err := LoadState(e.paths)
		if err != nil {
			t.Fatal(err)
		}
		if st.WatcherStopping {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("WatcherStopping not recorded while the tick was blocked")
		}
		time.Sleep(time.Millisecond)
	}
	if live, err := WatcherLive(e.paths); err != nil || !live {
		t.Errorf("WatcherLive while the tick is blocked = %v, %v; want the lock still held", live, err)
	}

	close(finishTick)
	if err := <-runDone; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if st, _ := LoadState(e.paths); !st.WatcherStopping {
		t.Error("WatcherStopping cleared by the exiting watcher or its in-flight tick's save; want it kept until the next Run")
	}

	stoppingAtNextRun := true
	e.s.onAlive = func() {
		st, _ := LoadState(e.paths)
		stoppingAtNextRun = st.WatcherStopping
	}
	e.s.alive = false
	if err := e.w.Run(context.Background(), noSleep); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if stoppingAtNextRun {
		t.Error("a new Run did not clear WatcherStopping at start")
	}
}

func TestRun_StopsAtErrorCap(t *testing.T) {
	e := newWatchEnv(t)
	fs := &flakySession{fakeSession: e.s, errs: make([]bool, 0)}
	for i := 0; i < maxConsecutiveTickErrors+10; i++ {
		fs.errs = append(fs.errs, true)
	}
	w := NewWatcher(fs, e.cfg, e.paths, e.stDir, nil, e.clock)

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
	w := NewWatcher(fs, e.cfg, e.paths, e.stDir, nil, e.clock)

	if err := w.Run(context.Background(), noSleep); !errors.Is(err, errBoom) {
		t.Fatalf("Run = %v, want errBoom", err)
	}
	want := (maxConsecutiveTickErrors - 1) + 1 + maxConsecutiveTickErrors
	if fs.calls != want {
		t.Errorf("ticks = %d, want %d", fs.calls, want)
	}
}
