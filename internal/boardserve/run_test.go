// run_test.go tests the board server's lifecycle in process.
// It covers the single-instance lock, the state and launcher files, the watch loop and the exits.
// The tests run over a temp board with an injected clock.

package boardserve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"slices"
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

// testDeadline bounds every wait for something a test expects to happen; it is reached only when a test fails.
const testDeadline = 10 * time.Second

// fakeClock is the injected clock: After registers a waiter that advance fires once its deadline passes.
type fakeClock struct {
	mu         sync.Mutex
	cond       *sync.Cond
	now        time.Time
	waiters    []fakeWaiter
	registered int
	nowCalls   int
	// durations are the durations of the After calls, in call order.
	durations []time.Duration
}

// fakeWaiter is one After call waiting for its deadline.
type fakeWaiter struct {
	deadline time.Time
	channel  chan time.Time
}

func newFakeClock() *fakeClock {
	clock := &fakeClock{now: testClock}
	clock.cond = sync.NewCond(&clock.mu)
	return clock
}

// Now returns the fake time and counts the call, which is how a test sees the server stamp an API request.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nowCalls++
	c.cond.Broadcast()
	return c.now
}

// After registers a waiter that fires once advance moves the clock to d from now.
func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	waiter := fakeWaiter{deadline: c.now.Add(d), channel: make(chan time.Time, 1)}
	c.waiters = append(c.waiters, waiter)
	c.registered++
	c.durations = append(c.durations, d)
	c.cond.Broadcast()
	return waiter.channel
}

// afterDurations returns the durations of the After calls the clock has seen, in call order.
func (c *fakeClock) afterDurations() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.durations)
}

// advance moves the clock forward by d and fires every waiter whose deadline has passed.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	remaining := c.waiters[:0]
	for _, waiter := range c.waiters {
		if waiter.deadline.After(c.now) {
			remaining = append(remaining, waiter)
			continue
		}
		waiter.channel <- c.now
	}
	c.waiters = remaining
}

// registrations returns how many After calls the clock has seen.
func (c *fakeClock) registrations() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.registered
}

// nowReads returns how many Now calls the clock has seen.
func (c *fakeClock) nowReads() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nowCalls
}

// waitFor blocks until condition holds under the clock's lock, failing the test at the deadline.
func (c *fakeClock) waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	expired := false
	timer := time.AfterFunc(testDeadline, func() {
		c.mu.Lock()
		expired = true
		c.mu.Unlock()
		c.cond.Broadcast()
	})
	defer timer.Stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	for !condition() {
		if expired {
			t.Fatalf("timed out waiting for %s", what)
		}
		c.cond.Wait()
	}
}

// waitRegistrations blocks until the clock has seen n After calls.
func (c *fakeClock) waitRegistrations(t *testing.T, n int) {
	t.Helper()
	c.waitFor(t, fmt.Sprintf("%d timers", n), func() bool { return c.registered >= n })
}

// waitNowReads blocks until the clock has seen n Now calls.
func (c *fakeClock) waitNowReads(t *testing.T, n int) {
	t.Helper()
	c.waitFor(t, fmt.Sprintf("%d clock reads", n), func() bool { return c.nowCalls >= n })
}

// runFixture is one Run's options over a temp hub scratch directory and a seeded temp board.
type runFixture struct {
	opts          Options
	clock         *fakeClock
	board         *boardengine.Board
	boardPath     string
	replaced      atomic.Bool
	registerCalls atomic.Int32
	// replacedCalls counts the Replaced calls, which the watch loop makes once per wake that reads the board.
	replacedCalls atomic.Int32
}

// newRunFixture returns a fixture whose log is the file at its Paths().Log, which a leftover can fill first.
func newRunFixture(t *testing.T) *runFixture {
	t.Helper()
	board, boardPath := newTestBoard(t)
	paths := Paths(t.TempDir())
	if err := os.MkdirAll(paths.Dir, 0o755); err != nil {
		t.Fatalf("create the state directory: %v", err)
	}
	logFile, err := os.OpenFile(paths.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open the log: %v", err)
	}
	t.Cleanup(func() { logFile.Close() })

	fixture := &runFixture{clock: newFakeClock(), board: board, boardPath: boardPath}
	fixture.opts = Options{
		Paths:     paths,
		Board:     board,
		BoardPath: boardPath,
		LockWait:  50 * time.Millisecond,
		IdleBound: 10 * time.Minute,
		PollBound: time.Minute,
		Log:       logFile,
		Register: func() error {
			fixture.registerCalls.Add(1)
			return nil
		},
		Replaced: func() bool {
			fixture.replacedCalls.Add(1)
			return fixture.replaced.Load()
		},
		Now:   fixture.clock.Now,
		After: fixture.clock.After,
	}
	return fixture
}

// runningServer is a Run in a goroutine; err is valid once done is closed.
type runningServer struct {
	done chan struct{}
	err  error
}

// start runs the fixture's Run in the background and ends it when the test does.
func (f *runFixture) start(t *testing.T) *runningServer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	running := &runningServer{done: make(chan struct{})}
	go func() {
		running.err = Run(ctx, f.opts)
		close(running.done)
	}()
	t.Cleanup(func() {
		cancel()
		<-running.done
	})
	return running
}

// exited reports whether Run has returned.
func (r *runningServer) exited() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// waitForExit fails the test unless Run returns nil before the deadline.
func (r *runningServer) waitForExit(t *testing.T) {
	t.Helper()
	select {
	case <-r.done:
		if r.err != nil {
			t.Fatalf("Run = %v; want nil", r.err)
		}
	case <-time.After(testDeadline):
		t.Fatalf("Run did not return")
	}
}

// waitUntil polls condition every few milliseconds until it holds, failing the test at the deadline.
func waitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(testDeadline)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// waitForServer returns the state of the server that answers a probe on the fixture's paths.
func waitForServer(t *testing.T, paths StatePaths) State {
	t.Helper()
	var st State
	waitUntil(t, "the board server to answer", func() bool {
		read, found, err := ReadState(paths)
		if err != nil || !found {
			return false
		}
		st = read
		return Probe(st, time.Second) == nil
	})
	return st
}

// httpResult is the outcome of a request a test sends from a goroutine.
type httpResult struct {
	status int
	body   []byte
	err    error
}

// send issues one authorized request to the server st describes and returns the channel its outcome arrives on.
func send(st State, method, path string, body []byte) <-chan httpResult {
	results := make(chan httpResult, 1)
	go func() {
		request, err := http.NewRequest(method, PageAddress(st.Port)+strings.TrimPrefix(path, "/"), bytes.NewReader(body))
		if err != nil {
			results <- httpResult{err: err}
			return
		}
		request.Header.Set("Authorization", "Bearer "+st.Token)
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: testDeadline}
		response, err := client.Do(request)
		if err != nil {
			results <- httpResult{err: err}
			return
		}
		defer response.Body.Close()
		payload, err := io.ReadAll(response.Body)
		results <- httpResult{status: response.StatusCode, body: payload, err: err}
	}()
	return results
}

// receive returns the outcome of a send, decoded as the JSON the server answers with.
func receive(t *testing.T, results <-chan httpResult) (int, answer) {
	t.Helper()
	select {
	case result := <-results:
		if result.err != nil {
			t.Fatalf("request: %v", result.err)
		}
		var decoded answer
		if err := json.Unmarshal(result.body, &decoded); err != nil {
			t.Fatalf("decode %q: %v", result.body, err)
		}
		return result.status, decoded
	case <-time.After(testDeadline):
		t.Fatalf("no answer to the request")
		return 0, answer{}
	}
}

// TestRun drives the lifecycle in process on a loopback port.
func TestRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("MissingStateDirectoryReadsAsNoServerAndStaysMissing", func(t *testing.T) {
		t.Parallel()
		paths := Paths(t.TempDir())
		live, err := Live(paths)
		if live || err != nil {
			t.Errorf("Live = %v, %v; want false, nil", live, err)
		}
		if _, err := os.Stat(paths.Dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Stat(state directory) = %v; want it absent", err)
		}
	})

	t.Run("HeldLockReturnsErrRunningAndWritesNothing", func(t *testing.T) {
		t.Parallel()
		fixture := newRunFixture(t)
		paths := fixture.opts.Paths
		if err := os.WriteFile(paths.Log, []byte("earlier server\n"), 0o600); err != nil {
			t.Fatalf("write the log: %v", err)
		}
		held, err := lock.AcquireWriteLock(paths.Lock)
		if err != nil {
			t.Fatalf("hold the server lock: %v", err)
		}
		defer held.Release()

		if err := Run(context.Background(), fixture.opts); !errors.Is(err, ErrRunning) {
			t.Errorf("Run = %v; want ErrRunning", err)
		}
		if _, found, _ := ReadState(paths); found {
			t.Errorf("Run wrote a state file")
		}
		if logged, _ := os.ReadFile(paths.Log); string(logged) != "earlier server\n" {
			t.Errorf("log = %q; want it untouched", logged)
		}
		if fixture.registerCalls.Load() != 0 {
			t.Errorf("Register called for a server that did not start")
		}
	})

	t.Run("ServesOverLeftoversAndEndsWhenTheBinaryIsReplaced", func(t *testing.T) {
		t.Parallel()
		fixture := newRunFixture(t)
		paths := fixture.opts.Paths
		leftoverState := `{"pid":1,"port":2,"token":"old","unknown":true}`
		if err := os.WriteFile(paths.State, []byte(leftoverState), 0o600); err != nil {
			t.Fatalf("write the leftover state: %v", err)
		}
		if err := os.WriteFile(paths.Log, []byte("stale line\n"), 0o600); err != nil {
			t.Fatalf("write the leftover log: %v", err)
		}
		if st, found, err := ReadState(paths); err != nil || !found || st != (State{PID: 1, Port: 2, Token: "old"}) {
			t.Fatalf("ReadState over the leftover = %+v, %v, %v; want its pid, port and token", st, found, err)
		}

		running := fixture.start(t)
		st := waitForServer(t, paths)
		if st.PID != os.Getpid() || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(st.Token) || st.Token == "old" {
			t.Errorf("state = %+v; want this process's pid and a fresh 32-hex-character token", st)
		}
		logged, _ := os.ReadFile(paths.Log)
		if first, _, _ := strings.Cut(string(logged), "\n"); first != fmt.Sprintf("lym board serve pid %d", os.Getpid()) || strings.Contains(string(logged), "stale line") {
			t.Errorf("log = %q; want it to open with this process's pid and drop the leftover", logged)
		}
		if got := fixture.registerCalls.Load(); got != 1 {
			t.Errorf("Register calls = %d; want 1", got)
		}

		launcher, err := WriteLauncher(paths, st)
		if err != nil {
			t.Fatalf("WriteLauncher: %v", err)
		}
		page, _ := os.ReadFile(launcher)
		if !strings.Contains(string(page), PageAddress(st.Port)+"#"+st.Token) || strings.Count(string(page), st.Token) != 1 || strings.Contains(string(page), "localhost") {
			t.Errorf("launcher = %q; want the 127.0.0.1 address with the token only after #", page)
		}
		if runtime.GOOS != "windows" {
			for _, path := range []string{paths.State, launcher} {
				if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
					t.Errorf("Stat(%s) = %v, %v; want mode 0600", path, info, err)
				}
			}
		}
		if live, err := Live(paths); !live || err != nil {
			t.Errorf("Live while serving = %v, %v; want true", live, err)
		}

		fixture.replaced.Store(true)
		fixture.clock.waitRegistrations(t, 1)
		fixture.clock.advance(time.Second)
		running.waitForExit(t)
		if live, err := Live(paths); live || err != nil {
			t.Errorf("Live after Run returned = %v, %v; want false", live, err)
		}
	})

	t.Run("IdleBoundIsMeasuredFromTheLastAPIRequest", func(t *testing.T) {
		t.Parallel()
		fixture := newRunFixture(t)
		running := fixture.start(t)
		st := waitForServer(t, fixture.opts.Paths)
		fixture.clock.waitRegistrations(t, 1)

		wake := func(d time.Duration) {
			before := fixture.clock.registrations()
			fixture.clock.advance(d)
			fixture.clock.waitRegistrations(t, before+1)
		}
		wake(6 * time.Minute)
		if status, _ := receive(t, send(st, http.MethodGet, routeBoard, nil)); status != http.StatusOK {
			t.Fatalf("GET %s = %d; want 200", routeBoard, status)
		}
		// Twelve minutes after the start but six after the request, so the server keeps serving.
		wake(6 * time.Minute)
		if running.exited() {
			t.Fatalf("Run exited before the idle bound passed from the last API request")
		}

		fixture.clock.advance(5 * time.Minute)
		running.waitForExit(t)
		if live, err := Live(fixture.opts.Paths); live || err != nil {
			t.Errorf("Live after the idle exit = %v, %v; want false", live, err)
		}
	})

	t.Run("WriteInFlightCompletesBeforeRunReturns", func(t *testing.T) {
		t.Parallel()
		fixture := newRunFixture(t)
		running := fixture.start(t)
		st := waitForServer(t, fixture.opts.Paths)
		fixture.clock.waitRegistrations(t, 1)

		boardLockPath, err := fabricengine.BoltLockPath(fixture.boardPath, fabricengine.BoardWriteLockFile)
		if err != nil {
			t.Fatalf("BoltLockPath: %v", err)
		}
		boardLock, err := lock.AcquireWriteLock(boardLockPath)
		if err != nil {
			t.Fatalf("hold the board's write lock: %v", err)
		}
		var releaseOnce sync.Once
		release := func() { releaseOnce.Do(func() { boardLock.Release() }) }
		defer release()

		reads := fixture.clock.nowReads()
		write := []byte(`{"op":"create","slug":"late-task","fields":{"kind":"task","title":"Late","labels":["bug"]}}`)
		results := send(st, http.MethodPost, routeWrite, write)
		// The server stamps the request with the clock before it reaches the blocked write section.
		fixture.clock.waitNowReads(t, reads+1)

		fixture.replaced.Store(true)
		fixture.clock.advance(time.Second)
		waitUntil(t, "the exiting server to release its lock", func() bool {
			live, err := Live(fixture.opts.Paths)
			return !live && err == nil
		})
		if running.exited() {
			t.Fatalf("Run returned while a write was still blocked in the board's write section")
		}

		release()
		if status, got := receive(t, results); status != http.StatusOK || !got.OK {
			t.Errorf("write answered %d %+v; want 200", status, got)
		}
		if !bytes.Contains(boardJSON(t, fixture.boardPath), []byte("late-task")) {
			t.Errorf("board.json does not hold the write that was in flight")
		}
		running.waitForExit(t)
	})

	t.Run("ExitAnswersAWaitingLongPollUnchanged", func(t *testing.T) {
		t.Parallel()
		fixture := newRunFixture(t)
		running := fixture.start(t)
		st := waitForServer(t, fixture.opts.Paths)
		fixture.clock.waitRegistrations(t, 1)
		_, current := receive(t, send(st, http.MethodGet, routeBoard, nil))

		reads := fixture.clock.nowReads()
		results := send(st, http.MethodGet, routeBoard+"?after="+current.Revision, nil)
		fixture.clock.waitNowReads(t, reads+1)

		fixture.replaced.Store(true)
		fixture.clock.advance(time.Second)
		if status, got := receive(t, results); status != http.StatusOK || !got.Unchanged {
			t.Errorf("long poll answered %d %+v; want unchanged", status, got)
		}
		running.waitForExit(t)
	})

	watchTests := []struct {
		name string
		// watch is the injected Watch and wake the step that makes the loop read board.json after the change.
		watch func(events chan fswatch.Event) WatchFunc
		wake  func(t *testing.T, fixture *runFixture, events chan fswatch.Event)
	}{
		{
			"WorkingWatchWakesAWaitingPollWithoutTheTimer",
			func(events chan fswatch.Event) WatchFunc {
				return func(string, ...string) (<-chan fswatch.Event, func() error, error) {
					return events, func() error { return nil }, nil
				}
			},
			func(t *testing.T, fixture *runFixture, events chan fswatch.Event) {
				events <- fswatch.Event{Name: boardFileName, Op: "write"}
			},
		},
		{
			"FailingWatchLeavesTheTimerToReadTheBoard",
			func(chan fswatch.Event) WatchFunc {
				return func(string, ...string) (<-chan fswatch.Event, func() error, error) {
					return nil, nil, errors.New("no file events here")
				}
			},
			func(t *testing.T, fixture *runFixture, events chan fswatch.Event) {
				fixture.clock.waitRegistrations(t, 1)
				fixture.clock.advance(time.Second)
			},
		},
	}
	for _, tt := range watchTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := newRunFixture(t)
			events := make(chan fswatch.Event, 1)
			fixture.opts.Watch = tt.watch(events)
			fixture.start(t)
			st := waitForServer(t, fixture.opts.Paths)
			_, current := receive(t, send(st, http.MethodGet, routeBoard, nil))

			reads := fixture.clock.nowReads()
			results := send(st, http.MethodGet, routeBoard+"?after="+current.Revision, nil)
			fixture.clock.waitNowReads(t, reads+1)
			if _, err := fixture.board.UpsertTaskReporting(map[string]any{"slug": "draft-task", "title": "Changed under the server"}); err != nil {
				t.Fatalf("change the board: %v", err)
			}
			tt.wake(t, fixture, events)

			wantRevision, err := boardengine.BoardRevision(fixture.boardPath)
			if err != nil {
				t.Fatalf("BoardRevision: %v", err)
			}
			status, got := receive(t, results)
			if status != http.StatusOK || got.Unchanged || got.Revision != wantRevision || got.Board == nil || got.Revision == current.Revision {
				t.Errorf("long poll answered %d %+v; want the new projection at %q", status, got, wantRevision)
			}
		})
	}

	t.Run("EventBurstReadsTheBoardOncePerFloor", func(t *testing.T) {
		t.Parallel()
		fixture := newRunFixture(t)
		events := make(chan fswatch.Event, 8)
		fixture.opts.Watch = func(string, ...string) (<-chan fswatch.Event, func() error, error) {
			return events, func() error { return nil }, nil
		}
		fixture.start(t)
		waitForServer(t, fixture.opts.Paths)
		fixture.clock.waitRegistrations(t, 1)

		// No read came before the first event, so it is read at once.
		events <- fswatch.Event{Name: boardFileName, Op: "write"}
		fixture.clock.waitRegistrations(t, 2)
		if got := fixture.replacedCalls.Load(); got != 1 {
			t.Fatalf("reads after the first event = %d; want 1", got)
		}

		for range 5 {
			events <- fswatch.Event{Name: boardFileName, Op: "write"}
		}
		// The loop now waits out the floor, registered as the third timer.
		fixture.clock.waitRegistrations(t, 3)
		if got := fixture.replacedCalls.Load(); got != 1 {
			t.Fatalf("reads inside the floor = %d; want still 1", got)
		}
		fixture.clock.advance(watchEventFloor)
		fixture.clock.waitRegistrations(t, 4)
		if got := fixture.replacedCalls.Load(); got != 2 {
			t.Errorf("reads after the floor passed = %d; want 2, one for the whole burst", got)
		}
		if queued := len(events); queued != 0 {
			t.Errorf("%d events still queued; want the burst drained", queued)
		}
	})

	t.Run("TimerDoublesFromOneSecondToEightAndResetsOnAChange", func(t *testing.T) {
		t.Parallel()
		fixture := newRunFixture(t)
		fixture.start(t)
		waitForServer(t, fixture.opts.Paths)
		fixture.clock.waitRegistrations(t, 1)

		wake := func(d time.Duration) {
			before := fixture.clock.registrations()
			fixture.clock.advance(d)
			fixture.clock.waitRegistrations(t, before+1)
		}
		wake(time.Second)
		wake(2 * time.Second)
		wake(4 * time.Second)
		wake(8 * time.Second)
		if _, err := fixture.board.UpsertTaskReporting(map[string]any{"slug": "draft-task", "title": "Changed under the server"}); err != nil {
			t.Fatalf("change the board: %v", err)
		}
		wake(8 * time.Second)

		want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, time.Second}
		if got := fixture.clock.afterDurations(); !slices.Equal(got, want) {
			t.Errorf("timer durations = %v; want %v", got, want)
		}
	})
}
