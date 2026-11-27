// run_test.go drives the board server's whole life in process, on a loopback port over a temporary board.
// The clock, watcher and binary check are injected.

package boardserve

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const testIdleBound = 30 * time.Minute

// stepClock is a clock that moves only when advanced.
type stepClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []chan time.Time
	// armed receives once per timer the server asks for, and read once per reading of the clock.
	armed chan struct{}
	read  chan struct{}
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case c.read <- struct{}{}:
	default:
	}
	return c.now
}

func (c *stepClock) After(time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	timer := make(chan time.Time, 1)
	c.timers = append(c.timers, timer)
	c.armed <- struct{}{}
	return timer
}

// advance moves the clock by d and fires every timer asked for so far.
func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for _, timer := range c.timers {
		timer <- c.now
	}
	c.timers = nil
}

func drain(signals chan struct{}) {
	for {
		select {
		case <-signals:
		default:
			return
		}
	}
}

// runningServer is one Run over a seeded board, with the seams a test steers it through.
type runningServer struct {
	t        *testing.T
	opts     Options
	board    *boardengine.Board
	clock    *stepClock
	replaced atomic.Bool
	register atomic.Int32
	done     chan error
}

// newRunningServer builds the options of a server that has not started yet.
// Its watcher fails, so only the clock wakes the server unless a test sets another one.
func newRunningServer(t *testing.T) *runningServer {
	t.Helper()
	board, dir := seededBoard(t)
	s := &runningServer{
		t:     t,
		board: board,
		clock: &stepClock{now: testNow, armed: make(chan struct{}, 1000), read: make(chan struct{}, 1000)},
		done:  make(chan error, 1),
	}
	paths := Paths(t.TempDir())
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.OpenFile(paths.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logFile.Close() })
	s.opts = Options{
		Paths:     paths,
		Board:     board,
		BoardPath: dir,
		LockWait:  10 * time.Millisecond,
		IdleBound: testIdleBound,
		PollBound: time.Minute,
		Log:       logFile,
		Register:  func() error { s.register.Add(1); return nil },
		Replaced:  s.replaced.Load,
		Watch: func(string, ...string) (*fswatch.Watcher, error) {
			return nil, errors.New("no watch available")
		},
		Now:   s.clock.Now,
		After: s.clock.After,
	}
	return s
}

// start runs the server and returns its state once it serves.
func (s *runningServer) start() State {
	s.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		s.done <- Run(ctx, s.opts)
	}()
	s.t.Cleanup(func() {
		cancel()
		<-finished
	})
	s.awaitWake()
	server, found, err := ReadState(s.opts.Paths)
	if err != nil || !found {
		s.t.Fatalf("ReadState = %v, %v; want the serving state", found, err)
	}
	return server
}

// awaitWake waits until the watch loop sleeps again.
func (s *runningServer) awaitWake() {
	s.t.Helper()
	select {
	case <-s.clock.armed:
	case err := <-s.done:
		s.t.Fatalf("Run returned %v; want it serving", err)
	}
}

// step moves the clock and waits for the server to have judged the new time and gone back to sleep.
func (s *runningServer) step(d time.Duration) {
	s.t.Helper()
	drain(s.clock.armed)
	s.clock.advance(d)
	s.awaitWake()
}

func (s *runningServer) awaitExit() {
	s.t.Helper()
	select {
	case err := <-s.done:
		if err != nil {
			s.t.Fatalf("Run = %v; want nil", err)
		}
	case <-time.After(time.Minute):
		s.t.Fatal("Run did not return")
	}
}

func (s *runningServer) wantLive(want bool) {
	s.t.Helper()
	live, err := Live(s.opts.Paths)
	if err != nil || live != want {
		s.t.Errorf("Live = %v, %v; want %v", live, err, want)
	}
}

// httpAnswer is one response of the serving server.
type httpAnswer struct {
	status int
	body   string
	err    error
}

// send sends one API request and returns, once the server has taken it up, the channel of its answer.
func (s *runningServer) send(server State, method, path, body string) <-chan httpAnswer {
	s.t.Helper()
	request, err := http.NewRequest(method, "http://"+loopbackAddress(server.Port)+path, strings.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+server.Token)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	answered := make(chan httpAnswer, 1)
	drain(s.clock.read)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			answered <- httpAnswer{err: err}
			return
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		answered <- httpAnswer{status: response.StatusCode, body: string(data), err: err}
	}()
	// The server reads the clock as each API request arrives.
	<-s.clock.read
	return answered
}

func (s *runningServer) revision() string {
	s.t.Helper()
	revision, err := boardengine.BoardRevision(s.opts.BoardPath)
	if err != nil {
		s.t.Fatal(err)
	}
	return revision
}

// moveBoard changes the board past the server's back, as a sync pull does, and returns the new revision.
func (s *runningServer) moveBoard() string {
	s.t.Helper()
	if _, err := s.board.UpsertTask(map[string]any{"slug": "drafted", "title": "Moved"}); err != nil {
		s.t.Fatal(err)
	}
	return s.revision()
}

func wantAnswer(t *testing.T, answered <-chan httpAnswer, fragments ...string) {
	t.Helper()
	got := <-answered
	if got.err != nil || got.status != http.StatusOK {
		t.Fatalf("answer = %d %q, %v; want 200", got.status, got.body, got.err)
	}
	for _, fragment := range fragments {
		if !strings.Contains(got.body, fragment) {
			t.Errorf("answer %q lacks %q", got.body, fragment)
		}
	}
}

// TestRun pins the server's single instance, its files, its exits and its watch loop.
func TestRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("NoStateDirectory", func(t *testing.T) {
		t.Parallel()
		paths := Paths(t.TempDir())
		if live, err := Live(paths); live || err != nil {
			t.Errorf("Live = %v, %v; want no server", live, err)
		}
		if _, err := os.Stat(paths.Dir); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Live created the state directory: %v", err)
		}
	})

	t.Run("LockHeld", func(t *testing.T) {
		t.Parallel()
		s := newRunningServer(t)
		held, err := lock.AcquireWriteLock(s.opts.Paths.Lock)
		if err != nil {
			t.Fatal(err)
		}
		defer held.Release()

		if err := Run(context.Background(), s.opts); !errors.Is(err, ErrRunning) {
			t.Fatalf("Run = %v; want ErrRunning", err)
		}
		if _, found, err := ReadState(s.opts.Paths); found || err != nil {
			t.Errorf("ReadState = %v, %v; want no state", found, err)
		}
		if data, err := os.ReadFile(s.opts.Paths.Log); err != nil || len(data) != 0 {
			t.Errorf("log = %q, %v; want it empty", data, err)
		}
	})

	t.Run("ServesAndReplacesWhatThePreviousServerLeft", func(t *testing.T) {
		t.Parallel()
		s := newRunningServer(t)
		paths := s.opts.Paths
		leftover := `{"pid":1,"port":2,"token":"old","added_later":true}`
		if err := os.WriteFile(paths.State, []byte(leftover), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, found, err := ReadState(paths); err != nil || !found || got != (State{PID: 1, Port: 2, Token: "old"}) {
			t.Fatalf("ReadState = %+v, %v, %v; want the leftover's three values", got, found, err)
		}
		if err := os.WriteFile(paths.Log, []byte("an earlier server\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		server := s.start()
		s.wantLive(true)
		if server.PID != os.Getpid() || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(server.Token) {
			t.Errorf("state = %+v; want this process and a 32-hex token", server)
		}
		if err := Probe(server); err != nil {
			t.Errorf("Probe = %v; want an answer", err)
		}
		logFile, err := os.Open(paths.Log)
		if err != nil {
			t.Fatal(err)
		}
		defer logFile.Close()
		first, err := bufio.NewReader(logFile).ReadString('\n')
		if want := fmt.Sprintf("lym board serve pid %d\n", os.Getpid()); err != nil || first != want {
			t.Errorf("first log line = %q, %v; want %q", first, err, want)
		}
		if got := s.register.Load(); got != 1 {
			t.Errorf("Register ran %d times; want once", got)
		}

		launcher, err := WriteLauncher(paths, server)
		if err != nil || launcher != paths.Launcher {
			t.Fatalf("WriteLauncher = %q, %v; want %q", launcher, err, paths.Launcher)
		}
		page, err := os.ReadFile(launcher)
		if err != nil {
			t.Fatal(err)
		}
		address := fmt.Sprintf("http://127.0.0.1:%d/#%s", server.Port, server.Token)
		if !strings.Contains(string(page), address) || strings.Count(string(page), server.Token) != 1 {
			t.Errorf("launcher %q; want it to send the browser to %q", page, address)
		}
		if runtime.GOOS != "windows" {
			for _, path := range []string{paths.State, launcher} {
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Errorf("mode of %s = %v, %v; want 0600", filepath.Base(path), info.Mode().Perm(), err)
				}
			}
		}
	})

	t.Run("IdleExitCountsFromTheLastAPIRequest", func(t *testing.T) {
		t.Parallel()
		s := newRunningServer(t)
		server := s.start()

		s.step(testIdleBound - 10*time.Minute)
		wantAnswer(t, s.send(server, http.MethodGet, routeBoard, ""))
		s.step(10 * time.Minute)
		s.wantLive(true)
		s.clock.advance(testIdleBound)
		s.awaitExit()
		s.wantLive(false)
	})

	t.Run("IdleExitWithNoRequest", func(t *testing.T) {
		t.Parallel()
		s := newRunningServer(t)
		s.start()
		s.clock.advance(testIdleBound)
		s.awaitExit()
		s.wantLive(false)
	})

	t.Run("ReplacedBinaryExit", func(t *testing.T) {
		t.Parallel()
		s := newRunningServer(t)
		s.start()
		s.replaced.Store(true)
		s.clock.advance(time.Second)
		s.awaitExit()
	})

	t.Run("ExitAnswersPollsAndCompletesTheWriteInFlight", func(t *testing.T) {
		t.Parallel()
		s := newRunningServer(t)
		server := s.start()
		revision := revisionOf(t, s.board, "drafted")
		lockPath, err := fabricengine.BoltLockPath(s.opts.BoardPath, fabricengine.BoardWriteLockFile)
		if err != nil {
			t.Fatal(err)
		}
		boardLock, err := lock.AcquireWriteLock(lockPath)
		if err != nil {
			t.Fatal(err)
		}
		released := sync.OnceValue(boardLock.Release)
		t.Cleanup(func() { _ = released() })

		poll := s.send(server, http.MethodGet, routeBoard+"?after="+s.revision(), "")
		write := s.send(server, http.MethodPost, routeWrite, writeBody(t, writeRequest{
			Op: opEdit, Slug: "drafted", Revision: revision, Fields: map[string]any{"title": "Written while draining"},
		}))
		s.clock.advance(testIdleBound)

		// The poll is answered by the drain, which follows the release of the server's lock.
		wantAnswer(t, poll, `"unchanged":true`)
		s.wantLive(false)
		select {
		case err := <-s.done:
			t.Fatalf("Run returned %v while a write was in flight", err)
		default:
		}

		if err := released(); err != nil {
			t.Fatal(err)
		}
		wantAnswer(t, write, `"ok":true`)
		s.awaitExit()
		if !strings.Contains(storeBytes(t, s.opts.BoardPath), "Written while draining") {
			t.Error("board.json does not hold the write")
		}
	})

	t.Run("WatchEventWakesAPoll", func(t *testing.T) {
		t.Parallel()
		s := newRunningServer(t)
		// The injected watcher watches a directory of the test's own, so the test says when an event arrives.
		events := t.TempDir()
		s.opts.Watch = func(string, ...string) (*fswatch.Watcher, error) { return fswatch.Watch(events) }
		server := s.start()
		poll := s.send(server, http.MethodGet, routeBoard+"?after="+s.revision(), "")

		moved := s.moveBoard()
		if err := os.WriteFile(filepath.Join(events, "signal"), nil, 0o600); err != nil {
			t.Fatal(err)
		}

		wantAnswer(t, poll, `"revision":"`+moved+`"`)
	})

	t.Run("TimerWakesAPollWithoutAWatcher", func(t *testing.T) {
		t.Parallel()
		s := newRunningServer(t)
		server := s.start()
		poll := s.send(server, http.MethodGet, routeBoard+"?after="+s.revision(), "")

		moved := s.moveBoard()
		s.step(time.Second)

		wantAnswer(t, poll, `"revision":"`+moved+`"`)
	})
}
