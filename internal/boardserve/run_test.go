// run_test.go drives one server's life in process, on a loopback port over a temp board and an injected clock.

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

const (
	testIdleBound = 30 * time.Minute
	// deadline bounds a wait that a passing test never reaches.
	deadline = time.Minute
)

// fakeClock is the server's clock: time moves only by advance, and the watch loop's timer fires only by tick.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
	// timer is unbuffered, so a send returns only once the watch loop took it.
	timer chan time.Time
	// armed is closed when the watch loop first sets its timer, with the server then up.
	armed     chan struct{}
	armedOnce sync.Once
	// read gets a value, when it has room, for every reading of the time.
	read chan struct{}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case c.read <- struct{}{}:
	default:
	}
	return c.now
}

func (c *fakeClock) advance(by time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(by)
}

func (c *fakeClock) After(time.Duration) <-chan time.Time {
	c.armedOnce.Do(func() { close(c.armed) })
	return c.timer
}

// lifecycle is one server's options over its own temp directories, and what its run leaves behind.
type lifecycle struct {
	opts       Options
	clock      *fakeClock
	log        *bytes.Buffer
	replaced   atomic.Bool
	registered atomic.Int32
	cancel     context.CancelFunc
	// done is closed when Run returned, and result then holds what it returned.
	done   chan struct{}
	result error
}

func newLifecycle(t *testing.T) *lifecycle {
	t.Helper()
	boardDir := t.TempDir()
	l := &lifecycle{
		clock: &fakeClock{
			now:   time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC),
			timer: make(chan time.Time),
			armed: make(chan struct{}),
			read:  make(chan struct{}, 1),
		},
		log:  &bytes.Buffer{},
		done: make(chan struct{}),
	}
	l.opts = Options{
		Paths:     Paths(t.TempDir()),
		Board:     boardengine.New(boardengine.Config{Path: boardDir, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true, Clock: l.clock.Now, Types: []boardengine.Label{{Name: "bug"}}}),
		BoardPath: boardDir,
		LockWait:  10 * time.Millisecond,
		IdleBound: testIdleBound,
		PollBound: time.Hour,
		Log:       l.log,
		Register:  func() error { l.registered.Add(1); return nil },
		Replaced:  l.replaced.Load,
		Watch:     fswatch.Watch,
		Now:       l.clock.Now,
		After:     l.clock.After,
	}
	return l
}

// start runs the server and waits until it serves.
func (l *lifecycle) start(t *testing.T) State {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	go func() {
		l.result = Run(ctx, l.opts)
		close(l.done)
	}()
	t.Cleanup(func() {
		cancel()
		<-l.done
	})
	select {
	case <-l.clock.armed:
	case <-l.done:
		t.Fatalf("Run ended before serving: %v", l.result)
	case <-time.After(deadline):
		t.Fatal("the server never served")
	}
	st, found, err := ReadState(l.opts.Paths)
	must(t, err)
	if !found {
		t.Fatal("a serving server recorded no state")
	}
	return st
}

// tick fires the watch loop's timer, and reports false when the server ended instead of taking it.
func (l *lifecycle) tick(t *testing.T) bool {
	t.Helper()
	select {
	case l.clock.timer <- l.clock.Now():
		return true
	case <-l.done:
		return false
	case <-time.After(deadline):
		t.Fatal("the watch loop took no tick and the server did not end")
		return false
	}
}

func (l *lifecycle) awaitExit(t *testing.T) {
	t.Helper()
	select {
	case <-l.done:
		must(t, l.result)
	case <-time.After(deadline):
		t.Fatal("Run did not return")
	}
}

func (l *lifecycle) wantLive(t *testing.T, want bool) {
	t.Helper()
	got, err := Live(l.opts.Paths)
	must(t, err)
	if got != want {
		t.Errorf("Live() = %v; want %v", got, want)
	}
}

// exchange is the status and answer of one request to a serving server.
type exchange struct {
	status int
	answer answer
	err    error
}

// send makes one request to the server of st and delivers its outcome.
func send(st State, method, target, body string) <-chan exchange {
	outcome := make(chan exchange, 1)
	go func() {
		got := exchange{}
		defer func() { outcome <- got }()
		request, err := http.NewRequest(method, fmt.Sprintf("http://%s%s", loopbackAddress(st.Port), target), strings.NewReader(body))
		if err != nil {
			got.err = err
			return
		}
		request.Header.Set("Authorization", "Bearer "+st.Token)
		request.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: deadline, Transport: &http.Transport{DisableKeepAlives: true}}
		response, err := client.Do(request)
		if err != nil {
			got.err = err
			return
		}
		defer response.Body.Close()
		got.status = response.StatusCode
		got.err = json.NewDecoder(response.Body).Decode(&got.answer)
	}()
	return outcome
}

// sendAndAwaitHandler is send, returning once the server's handler has the request.
// The caller makes sure nothing else reads the clock meanwhile.
func (l *lifecycle) sendAndAwaitHandler(t *testing.T, st State, method, target, body string) <-chan exchange {
	t.Helper()
	select {
	case <-l.clock.read:
	default:
	}
	outcome := send(st, method, target, body)
	select {
	case <-l.clock.read:
	case <-time.After(deadline):
		t.Fatal("the request never reached the server")
	}
	return outcome
}

func awaitExchange(t *testing.T, outcome <-chan exchange) exchange {
	t.Helper()
	got := <-outcome
	must(t, got.err)
	return got
}

// pollTarget is the long poll that waits for a change of the board's current revision.
func (l *lifecycle) pollTarget(t *testing.T) string {
	t.Helper()
	revision, err := boardengine.BoardRevision(l.opts.BoardPath)
	must(t, err)
	return routeBoard + "?after=" + revision
}

func TestRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("NoStateDirectory", testRunNoStateDirectory)
	t.Run("SecondServer", testRunSecondServer)
	t.Run("StartState", testRunStartState)
	t.Run("IdleExit", testRunIdleExit)
	t.Run("RequestDefersIdleExit", testRunRequestDefersIdleExit)
	t.Run("ReplacedExit", testRunReplacedExit)
	t.Run("DrainFinishesWrites", testRunDrainFinishesWrites)
	t.Run("EventWakesThePoll", testRunEventWakesThePoll)
	t.Run("TimerAloneWakesThePoll", testRunTimerAloneWakesThePoll)
}

func testRunNoStateDirectory(t *testing.T) {
	t.Parallel()
	l := newLifecycle(t)
	l.wantLive(t, false)
	if _, err := os.Stat(l.opts.Paths.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Live() left a state directory: %v", err)
	}
}

func testRunSecondServer(t *testing.T) {
	t.Parallel()
	l := newLifecycle(t)
	must(t, os.MkdirAll(l.opts.Paths.Dir, 0o700))
	must(t, os.WriteFile(l.opts.Paths.Log, []byte("the holder's log\n"), 0o600))
	held, err := lock.AcquireWriteLock(l.opts.Paths.Lock)
	must(t, err)
	defer func() { must(t, held.Release()) }()

	if err := Run(context.Background(), l.opts); !errors.Is(err, ErrRunning) {
		t.Errorf("Run() = %v; want ErrRunning", err)
	}
	if _, found, err := ReadState(l.opts.Paths); found || err != nil {
		t.Errorf("ReadState() = found %v, %v; want no state", found, err)
	}
	log, err := os.ReadFile(l.opts.Paths.Log)
	must(t, err)
	if string(log) != "the holder's log\n" || l.log.Len() != 0 || l.registered.Load() != 0 {
		t.Errorf("a refused start wrote log %q and %q, and registered %d times", log, l.log, l.registered.Load())
	}
}

func testRunStartState(t *testing.T) {
	t.Parallel()
	l := newLifecycle(t)
	must(t, os.MkdirAll(l.opts.Paths.Dir, 0o700))
	must(t, os.WriteFile(l.opts.Paths.State, []byte(`{"pid":7,"port":8,"token":"old","later":true}`), 0o600))
	leftover, found, err := ReadState(l.opts.Paths)
	must(t, err)
	if want := (State{PID: 7, Port: 8, Token: "old"}); !found || leftover != want {
		t.Errorf("ReadState() = %+v, %v; want %+v", leftover, found, want)
	}
	must(t, os.WriteFile(l.opts.Paths.Log, []byte("an earlier server's log, longer than the first line\n"), 0o600))
	logFile, err := os.OpenFile(l.opts.Paths.Log, os.O_WRONLY|os.O_APPEND, 0o600)
	must(t, err)
	t.Cleanup(func() { _ = logFile.Close() })
	l.opts.Log = logFile

	st := l.start(t)
	l.wantLive(t, true)
	if st.PID != os.Getpid() || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(st.Token) {
		t.Errorf("state = %+v; want this process and a 32-hex token", st)
	}
	if err := Probe(st); err != nil {
		t.Errorf("Probe(%+v) = %v", st, err)
	}
	log, err := os.ReadFile(l.opts.Paths.Log)
	must(t, err)
	if first, _, _ := strings.Cut(string(log), "\n"); first != fmt.Sprintf("lym board serve pid %d", os.Getpid()) {
		t.Errorf("first log line = %q", first)
	}
	if got := l.registered.Load(); got != 1 {
		t.Errorf("registered %d times; want 1", got)
	}

	launcher, err := WriteLauncher(l.opts.Paths, st)
	must(t, err)
	page, err := os.ReadFile(launcher)
	must(t, err)
	address := fmt.Sprintf("http://127.0.0.1:%d/", st.Port)
	if want := `location.replace("` + address + "#" + st.Token + `")`; !strings.Contains(string(page), want) || PageAddress(st.Port) != address {
		t.Errorf("launcher = %q, address %q; want %q and %q", page, PageAddress(st.Port), want, address)
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{l.opts.Paths.State, launcher} {
			info, err := os.Stat(path)
			must(t, err)
			if got := info.Mode().Perm(); got != 0o600 {
				t.Errorf("mode of %s = %v; want 0600", filepath.Base(path), got)
			}
		}
	}

	l.cancel()
	l.awaitExit(t)
	l.wantLive(t, false)
}

func testRunIdleExit(t *testing.T) {
	t.Parallel()
	l := newLifecycle(t)
	l.start(t)
	l.clock.advance(testIdleBound - time.Second)
	if !l.tick(t) || !l.tick(t) {
		t.Fatalf("the server ended before the idle bound: %v", l.result)
	}
	l.clock.advance(time.Second)
	l.tick(t)
	l.awaitExit(t)
	l.wantLive(t, false)
}

func testRunRequestDefersIdleExit(t *testing.T) {
	t.Parallel()
	l := newLifecycle(t)
	st := l.start(t)
	l.clock.advance(20 * time.Minute)
	awaitExchange(t, send(st, http.MethodGet, routeBoard, ""))
	l.clock.advance(10 * time.Minute)
	if !l.tick(t) || !l.tick(t) {
		t.Fatalf("the server ended at the bound from its start, despite a later request: %v", l.result)
	}
	l.clock.advance(20 * time.Minute)
	l.tick(t)
	l.awaitExit(t)
}

func testRunReplacedExit(t *testing.T) {
	t.Parallel()
	l := newLifecycle(t)
	l.start(t)
	l.replaced.Store(true)
	l.tick(t)
	l.awaitExit(t)
}

func testRunDrainFinishesWrites(t *testing.T) {
	t.Parallel()
	l := newLifecycle(t)
	// A board write would raise a file event, whose wake reads the clock under the request's wait.
	l.opts.Watch = func(string, ...string) (*fswatch.Watcher, error) { return nil, errors.New("no events") }
	st := l.start(t)
	lockPath, err := fabricengine.BoltLockPath(l.opts.BoardPath, fabricengine.BoardWriteLockFile)
	must(t, err)
	held, err := lock.AcquireWriteLock(lockPath)
	must(t, err)
	release := sync.OnceFunc(func() { must(t, held.Release()) })
	defer release()

	poll := l.sendAndAwaitHandler(t, st, http.MethodGet, l.pollTarget(t), "")
	write := l.sendAndAwaitHandler(t, st, http.MethodPost, routeWrite, `{"op":"create","slug":"fresh","fields":{"kind":"task","title":"Fresh","labels":["bug"]}}`)
	l.clock.advance(testIdleBound)
	l.tick(t)

	// The poll answers only once the drain began, which follows the release of the server's lock.
	if got := awaitExchange(t, poll); !got.answer.Unchanged {
		t.Errorf("waiting poll = %d %+v; want unchanged", got.status, got.answer)
	}
	l.wantLive(t, false)
	select {
	case <-l.done:
		t.Fatalf("Run returned with a write in flight: %v", l.result)
	case got := <-write:
		t.Fatalf("the write ended while the board was locked: %+v", got)
	default:
	}

	release()
	if got := awaitExchange(t, write); got.status != http.StatusOK || !got.answer.OK {
		t.Errorf("write = %d %q; want 200", got.status, got.answer.Error)
	}
	l.awaitExit(t)
	board, err := os.ReadFile(filepath.Join(l.opts.BoardPath, "board.json"))
	must(t, err)
	if !strings.Contains(string(board), `"fresh"`) {
		t.Errorf("board.json = %s; want the written entry", board)
	}
}

// wantWake changes the board under a waiting poll, runs wake and expects the poll to answer the new revision.
func (l *lifecycle) wantWake(t *testing.T, st State, wake func()) {
	t.Helper()
	poll := send(st, http.MethodGet, l.pollTarget(t), "")
	must(t, upsert(l.opts.Board, map[string]any{"slug": "fresh", "kind": "task", "title": "Fresh", "labels": []any{"bug"}}))
	want, err := boardengine.BoardRevision(l.opts.BoardPath)
	must(t, err)
	wake()
	if got := awaitExchange(t, poll); got.answer.Unchanged || got.answer.Revision != want {
		t.Errorf("poll = %+v; want the board at revision %q", got.answer, want)
	}
}

func testRunEventWakesThePoll(t *testing.T) {
	t.Parallel()
	l := newLifecycle(t)
	st := l.start(t)
	// The write raises the event; the timer is never fired.
	l.wantWake(t, st, func() {})
}

func testRunTimerAloneWakesThePoll(t *testing.T) {
	t.Parallel()
	l := newLifecycle(t)
	l.opts.Watch = func(string, ...string) (*fswatch.Watcher, error) { return nil, errors.New("no events") }
	st := l.start(t)
	l.wantWake(t, st, func() { l.tick(t) })
}
