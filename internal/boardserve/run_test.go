// run_test.go drives the server's lifecycle in process on a loopback port, over a temp board and a hand-moved clock.

package boardserve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	testIdleBound = time.Minute
	// testDeadline only ends a test that would otherwise hang.
	testDeadline = time.Minute
)

// manualClock is a clock a test moves, whose timers fire only when the test sends on them.
type manualClock struct {
	mu  sync.Mutex
	now time.Time
	// read receives a value, when it has room, each time the clock is read.
	read   chan struct{}
	timers chan manualTimer
}

type manualTimer struct {
	wait time.Duration
	fire chan time.Time
}

func newManualClock() *manualClock {
	return &manualClock{now: testNow, read: make(chan struct{}, 1), timers: make(chan manualTimer, 64)}
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case c.read <- struct{}{}:
	default:
	}
	return c.now
}

func (c *manualClock) After(wait time.Duration) <-chan time.Time {
	timer := manualTimer{wait: wait, fire: make(chan time.Time, 1)}
	c.timers <- timer
	return timer.fire
}

func (c *manualClock) advance(by time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(by)
	return c.now
}

// awaitRead returns once the clock is read after the call, as the server does on every API request.
func (c *manualClock) awaitRead(t *testing.T, request func()) {
	t.Helper()
	select {
	case <-c.read:
	default:
	}
	go request()
	receive(t, c.read, "a request to reach the server")
}

func receive[T any](t *testing.T, from <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-from:
		return value
	case <-time.After(testDeadline):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

// serving is one board and the Run over it.
type serving struct {
	fixture fixture
	options Options
	clock   *manualClock
	log     *os.File
	cancel  context.CancelFunc
	done    chan error
	// timer is the watch loop's armed timer.
	timer      manualTimer
	state      State
	registered int
}

// newServing prepares a Run over a fresh board, with a watch that fails, so the timer alone reads the board.
func newServing(t *testing.T) *serving {
	t.Helper()
	f := newFixture(t, time.Minute)
	s := &serving{fixture: f, clock: newManualClock(), done: make(chan error, 1)}
	paths := Paths(t.TempDir())
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	log, err := os.OpenFile(paths.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })
	s.log = log
	s.options = Options{
		Paths:     paths,
		Board:     f.board,
		BoardPath: f.dir,
		// A short wait can run out on a loaded machine before the free lock is tried.
		LockWait:  testDeadline,
		IdleBound: testIdleBound,
		PollBound: testDeadline,
		Log:       log,
		Register:  func() error { s.registered++; return nil },
		Replaced:  func() bool { return false },
		Watch: func(string, ...string) (*fswatch.Watcher, error) {
			return nil, errors.New("no watch")
		},
		Now:   s.clock.Now,
		After: s.clock.After,
	}
	return s
}

// start runs the server and returns once its watch loop waits.
func (s *serving) start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	finished := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		receive(t, finished, "Run to return at cleanup")
	})
	go func() {
		defer close(finished)
		s.done <- Run(ctx, s.options)
	}()
	s.awaitTimer(t)
	current, found, err := ReadState(s.options.Paths)
	if err != nil || !found {
		t.Fatalf("ReadState() = found %v, %v; want the serving state", found, err)
	}
	s.state = current
}

// awaitTimer returns once the watch loop arms its next timer, so its previous wake is over and did not end the server.
func (s *serving) awaitTimer(t *testing.T) {
	t.Helper()
	select {
	case s.timer = <-s.clock.timers:
	case err := <-s.done:
		t.Fatalf("Run() ended with %v; want it serving", err)
	case <-time.After(testDeadline):
		t.Fatalf("timed out waiting for the watch loop's timer")
	}
}

// wake moves the clock on and fires the armed timer.
func (s *serving) wake(by time.Duration) {
	s.timer.fire <- s.clock.advance(by)
}

func (s *serving) ended(t *testing.T) {
	t.Helper()
	if err := receive(t, s.done, "Run to end"); err != nil {
		t.Errorf("Run() = %v; want nil", err)
	}
	s.wantLive(t, false)
}

func (s *serving) wantLive(t *testing.T, want bool) {
	t.Helper()
	if live, err := Live(s.options.Paths); err != nil || live != want {
		t.Errorf("Live() = %v, %v; want %v", live, err, want)
	}
}

// send sends one authorized request to the server; it is safe off the test's goroutine.
func (s *serving) send(method, target, body string) (reply, int, error) {
	request, err := http.NewRequest(method, strings.TrimSuffix(PageAddress(s.state.Port), routePage)+target, strings.NewReader(body))
	if err != nil {
		return reply{}, 0, err
	}
	request.Header.Set("Authorization", "Bearer "+s.state.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: testDeadline, Transport: &http.Transport{DisableKeepAlives: true}}).Do(request)
	if err != nil {
		return reply{}, 0, err
	}
	defer response.Body.Close()
	var decoded reply
	err = json.NewDecoder(response.Body).Decode(&decoded)
	return decoded, response.StatusCode, err
}

type answered struct {
	reply  reply
	status int
	err    error
}

// sendLater sends a request in the background, and returns once the server has taken it.
func (s *serving) sendLater(t *testing.T, method, target, body string) <-chan answered {
	t.Helper()
	out := make(chan answered, 1)
	s.clock.awaitRead(t, func() {
		got, status, err := s.send(method, target, body)
		out <- answered{got, status, err}
	})
	return out
}

// revision is the revision the server answers for the board.
func (s *serving) revision(t *testing.T) string {
	t.Helper()
	got, status, err := s.send(http.MethodGet, routeBoard, "")
	if err != nil || status != http.StatusOK {
		t.Fatalf("GET /api/board = %d, %v; want 200", status, err)
	}
	return got.Revision
}

// change writes the board and returns its new revision.
func (s *serving) change(t *testing.T) string {
	t.Helper()
	if _, err := s.fixture.board.UpsertTask(map[string]any{"slug": "released", "title": "Moved on"}); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}
	revision, err := boardengine.BoardRevision(s.fixture.dir)
	if err != nil {
		t.Fatalf("BoardRevision: %v", err)
	}
	return revision
}

func TestRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("LiveOverAnAbsentDirectoryCreatesNothing", func(t *testing.T) {
		t.Parallel()
		paths := Paths(t.TempDir())
		live, err := Live(paths)
		if _, statErr := os.Stat(paths.Dir); live || err != nil || !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("Live() = %v, %v, directory stat %v; want false, nil and no directory", live, err, statErr)
		}
	})

	t.Run("HeldLockIsErrRunningAndWritesNothing", func(t *testing.T) {
		t.Parallel()
		s := newServing(t)
		held, err := lock.AcquireWriteLock(s.options.Paths.Lock)
		if err != nil {
			t.Fatalf("hold the lock: %v", err)
		}
		t.Cleanup(func() { _ = held.Release() })
		var log bytes.Buffer
		s.options.Log = &log
		s.options.LockWait = time.Millisecond
		if _, err := s.log.WriteString("the holder's log\n"); err != nil {
			t.Fatalf("seed log: %v", err)
		}

		if err := Run(context.Background(), s.options); !errors.Is(err, ErrRunning) {
			t.Errorf("Run() = %v; want ErrRunning", err)
		}
		kept, err := os.ReadFile(s.options.Paths.Log)
		if err != nil {
			t.Fatalf("read log: %v", err)
		}
		_, found, stateErr := ReadState(s.options.Paths)
		if found || stateErr != nil || log.Len() != 0 || string(kept) != "the holder's log\n" || s.registered != 0 {
			t.Errorf("after ErrRunning: state found %v (%v), wrote %q, log file %q, registered %d; want nothing written",
				found, stateErr, log.String(), kept, s.registered)
		}
	})

	t.Run("ServesOverALeftoverState", func(t *testing.T) {
		t.Parallel()
		s := newServing(t)
		paths := s.options.Paths
		if err := os.WriteFile(paths.State, []byte(`{"pid":7,"port":8,"token":"old","later":true}`), 0o600); err != nil {
			t.Fatalf("seed state: %v", err)
		}
		if _, err := s.log.WriteString("a leftover log\n"); err != nil {
			t.Fatalf("seed log: %v", err)
		}
		if got, found, err := ReadState(paths); err != nil || !found || got != (State{PID: 7, Port: 8, Token: "old"}) {
			t.Fatalf("ReadState() = %+v, %v, %v; want the leftover record, its unknown key ignored", got, found, err)
		}

		s.start(t)
		s.wantLive(t, true)
		if s.state.PID != os.Getpid() || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(s.state.Token) {
			t.Errorf("state = %+v; want this process's pid and a 32-hex-character token", s.state)
		}
		if err := Probe(s.state); err != nil {
			t.Errorf("Probe() = %v; want nil", err)
		}
		if err := Probe(State{Port: s.state.Port, Token: "old"}); err == nil {
			t.Errorf("Probe() with another token = nil; want an error")
		}
		log, err := os.ReadFile(paths.Log)
		if err != nil {
			t.Fatalf("read log: %v", err)
		}
		if first, _, _ := strings.Cut(string(log), "\n"); first != fmt.Sprintf("lym board serve pid %d", os.Getpid()) {
			t.Errorf("log's first line = %q; want this process's pid", first)
		}
		if s.registered != 1 {
			t.Errorf("Register ran %d times; want once", s.registered)
		}

		launcher, err := WriteLauncher(paths, s.state.Port, s.state.Token)
		if err != nil || launcher != paths.Launcher {
			t.Fatalf("WriteLauncher() = %q, %v; want %q", launcher, err, paths.Launcher)
		}
		page, err := os.ReadFile(launcher)
		if err != nil {
			t.Fatalf("read launcher: %v", err)
		}
		address := "http://127.0.0.1:" + strconv.Itoa(s.state.Port) + "/"
		if PageAddress(s.state.Port) != address || !strings.Contains(string(page), `location.replace("`+address+"#"+s.state.Token+`")`) ||
			strings.Count(string(page), s.state.Token) != 1 {
			t.Errorf("launcher = %q; want it to replace its location with %s and the token after #", page, address)
		}
		if runtime.GOOS != "windows" {
			for _, path := range []string{paths.State, launcher} {
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Errorf("stat %s = %v, %v; want mode 0600", path, info.Mode().Perm(), err)
				}
			}
		}

		s.cancel()
		s.ended(t)
	})

	t.Run("TimerBacksOffWhileTheBoardIsUnchanged", func(t *testing.T) {
		t.Parallel()
		s := newServing(t)
		s.start(t)
		current := s.revision(t)
		var waits []time.Duration
		for range 5 {
			waits = append(waits, s.timer.wait)
			s.wake(s.timer.wait)
			s.awaitTimer(t)
		}

		changed := s.change(t)
		poll := s.sendLater(t, http.MethodGet, routeBoard+"?after="+current, "")
		s.wake(s.timer.wait)
		s.awaitTimer(t)
		got := receive(t, poll, "the long poll")
		if got.err != nil || got.reply.Unchanged || got.reply.Revision != changed {
			t.Errorf("long poll = %+v; want the changed revision %s", got, changed)
		}
		waits = append(waits, s.timer.wait)
		want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, time.Second}
		if fmt.Sprint(waits) != fmt.Sprint(want) {
			t.Errorf("timer waits = %v; want %v", waits, want)
		}
	})

	t.Run("WatchEventWakesAWaitingRequest", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			// since is how long after the previous wake the event comes.
			since time.Duration
			// floor is the wait the loop holds the event back for, zero for none.
			floor time.Duration
		}{
			{"AfterTheFloor", 2 * time.Second, 0},
			{"WithinTheFloor", 0, time.Second},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				s := newServing(t)
				// The watched directory stands in for the board's, so the test alone raises events.
				watched := t.TempDir()
				s.options.Watch = func(_ string, names ...string) (*fswatch.Watcher, error) {
					return fswatch.Watch(watched, names...)
				}
				s.start(t)
				current := s.revision(t)
				s.clock.advance(tt.since)
				changed := s.change(t)
				poll := s.sendLater(t, http.MethodGet, routeBoard+"?after="+current, "")
				if err := os.WriteFile(filepath.Join(watched, boardengine.BoardStoreFile), nil, 0o600); err != nil {
					t.Fatalf("raise an event: %v", err)
				}
				if tt.floor != 0 {
					s.awaitTimer(t)
					if s.timer.wait != tt.floor {
						t.Errorf("held-back wait = %v; want %v", s.timer.wait, tt.floor)
					}
					s.wake(s.timer.wait)
				}
				got := receive(t, poll, "the long poll")
				if got.err != nil || got.reply.Unchanged || got.reply.Revision != changed {
					t.Errorf("long poll = %+v; want the changed revision %s", got, changed)
				}
			})
		}
	})

	t.Run("IdleExit", func(t *testing.T) {
		t.Parallel()
		s := newServing(t)
		s.start(t)
		s.wake(testIdleBound)
		s.ended(t)
	})

	t.Run("ARequestPutsTheIdleExitOff", func(t *testing.T) {
		t.Parallel()
		s := newServing(t)
		s.start(t)
		s.wake(testIdleBound - time.Second)
		s.awaitTimer(t)
		s.revision(t)
		s.wake(time.Second)
		s.awaitTimer(t)
		s.wake(testIdleBound - time.Second)
		s.ended(t)
	})

	t.Run("ReplacedBinaryExit", func(t *testing.T) {
		t.Parallel()
		s := newServing(t)
		s.options.Replaced = func() bool { return true }
		s.start(t)
		s.wake(time.Second)
		s.ended(t)
	})

	t.Run("ExitAnswersWaitersAndFinishesTheWriteInFlight", func(t *testing.T) {
		t.Parallel()
		s := newServing(t)
		s.start(t)
		current := s.revision(t)
		edit := encoded(t, writeRequest{Op: opEdit, Slug: "released", Revision: s.fixture.revisionOf(t, "released"), Fields: map[string]any{"title": "In flight"}})
		lockPath, err := fabricengine.BoltLockPath(s.fixture.dir, fabricengine.BoardWriteLockFile)
		if err != nil {
			t.Fatalf("BoltLockPath: %v", err)
		}
		held, err := lock.AcquireWriteLock(lockPath)
		if err != nil {
			t.Fatalf("hold the board's write lock: %v", err)
		}
		release := sync.OnceFunc(func() { _ = held.Release() })
		t.Cleanup(release)
		poll := s.sendLater(t, http.MethodGet, routeBoard+"?after="+current, "")
		write := s.sendLater(t, http.MethodPost, routeWrite, edit)

		s.wake(testIdleBound)
		if got := receive(t, poll, "the long poll"); got.err != nil || !got.reply.Unchanged {
			t.Errorf("long poll at exit = %+v; want unchanged", got)
		}
		// The waiter is answered after the lock is released, with the write still blocked.
		s.wantLive(t, false)
		select {
		case err := <-s.done:
			t.Fatalf("Run() ended with %v while a write was in flight", err)
		default:
		}
		release()

		if got := receive(t, write, "the write"); got.err != nil || got.status != http.StatusOK || !got.reply.OK {
			t.Errorf("write in flight = %+v; want 200", got)
		}
		s.ended(t)
		if !bytes.Contains(s.fixture.boardJSON(t), []byte("In flight")) {
			t.Errorf("board.json does not hold the write in flight")
		}
	})
}
