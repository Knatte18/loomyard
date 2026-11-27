// run_test.go drives the board server's lifecycle in process, on a loopback port over a temp board.
// The clock, the replacement check and the file watch are injected.

package boardserve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
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

// fakeClock is the server's clock: its timers fire only when advance passes them.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers map[chan time.Time]time.Time
	// armed receives the duration of every timer as it is armed.
	armed chan time.Duration
	// read, when set, is closed by the next Now.
	read chan struct{}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.read != nil {
		close(c.read)
		c.read = nil
	}
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	timer := make(chan time.Time, 1)
	c.timers[timer] = c.now.Add(d)
	c.mu.Unlock()
	c.armed <- d
	return timer
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for timer, due := range c.timers {
		if !due.After(c.now) {
			timer <- c.now
			delete(c.timers, timer)
		}
	}
}

// nextRead returns a channel closed by the next reading of the clock.
// While the watch loop waits on its timer, that reading is an arriving /api request's.
func (c *fakeClock) nextRead() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	read := make(chan struct{})
	c.read = read
	return read
}

// lifecycle is one Run's options over a seeded board, with the handles that drive it.
type lifecycle struct {
	t        *testing.T
	board    fixture
	opts     Options
	clock    *fakeClock
	events   chan fswatch.Event
	watched  chan []string
	replaced atomic.Bool
	registry atomic.Int32
	done     chan struct{}
	err      error
}

func newLifecycle(t *testing.T) *lifecycle {
	t.Helper()
	l := &lifecycle{
		t:     t,
		board: newFixture(t, time.Minute),
		clock: &fakeClock{
			now:    time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC),
			timers: map[chan time.Time]time.Time{},
			armed:  make(chan time.Duration, 64),
		},
		events:  make(chan fswatch.Event),
		watched: make(chan []string, 1),
		done:    make(chan struct{}),
	}
	paths := Paths(t.TempDir())
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(paths.Log, []byte("a former server's log\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	log, err := os.OpenFile(paths.Log, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })
	l.opts = Options{
		Paths:     paths,
		Board:     l.board.board,
		BoardPath: l.board.dir,
		// A free lock is taken at once; a short wait could run out before the first attempt on a loaded machine.
		LockWait:  time.Minute,
		IdleBound: 30 * time.Minute,
		PollBound: time.Minute,
		Log:       log,
		Register:  func() error { l.registry.Add(1); return nil },
		Replaced:  l.replaced.Load,
		Watch: func(dir string, names ...string) (<-chan fswatch.Event, func() error, error) {
			l.watched <- append([]string{dir}, names...)
			return l.events, func() error { return nil }, nil
		},
		Now:   l.clock.Now,
		After: l.clock.After,
	}
	return l
}

// start runs the server and returns its state once it serves and its watch loop waits.
func (l *lifecycle) start() State {
	l.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer close(l.done)
		l.err = Run(ctx, l.opts)
	}()
	l.t.Cleanup(func() {
		cancel()
		<-l.done
	})
	select {
	case <-l.clock.armed:
	case <-l.done:
		l.t.Fatalf("Run ended at its start: %v", l.err)
	}
	server, found, err := ReadState(l.opts.Paths)
	if err != nil || !found {
		l.t.Fatalf("ReadState = %v, %v", found, err)
	}
	return server
}

// ended waits for Run to return and asserts it returned nil with the lock free.
func (l *lifecycle) ended() {
	l.t.Helper()
	<-l.done
	if l.err != nil {
		l.t.Errorf("Run = %v; want nil", l.err)
	}
	l.wantLive(false)
}

func (l *lifecycle) wantLive(want bool) {
	l.t.Helper()
	if live, err := Live(l.opts.Paths); err != nil || live != want {
		l.t.Errorf("Live = %v, %v; want %v", live, err, want)
	}
}

type answered struct {
	code  int
	reply reply
	err   error
}

// send makes one request of the serving server and delivers its answer.
// It returns once the server has read its clock for the request, so the request is in flight.
func (l *lifecycle) send(server State, method, target, body string) <-chan answered {
	l.t.Helper()
	request, err := http.NewRequest(method, strings.TrimSuffix(PageAddress(server.Port), "/")+target, strings.NewReader(body))
	if err != nil {
		l.t.Fatalf("request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+server.Token)
	request.Header.Set("Content-Type", "application/json")
	arrived := l.clock.nextRead()
	answer := make(chan answered, 1)
	go func() {
		client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
		response, err := client.Do(request)
		if err != nil {
			answer <- answered{err: err}
			return
		}
		defer response.Body.Close()
		got := answered{code: response.StatusCode}
		data, err := io.ReadAll(response.Body)
		if err == nil {
			err = json.Unmarshal(data, &got.reply)
		}
		got.err = err
		answer <- got
	}()
	<-arrived
	return answer
}

func (l *lifecycle) receive(answer <-chan answered) reply {
	l.t.Helper()
	got := <-answer
	if got.err != nil || got.code != http.StatusOK {
		l.t.Fatalf("answer = %d, %v", got.code, got.err)
	}
	return got.reply
}

func (l *lifecycle) retitle(title string) {
	l.t.Helper()
	if _, err := l.board.board.UpsertTask(map[string]any{"slug": "open", "title": title}); err != nil {
		l.t.Fatalf("retitle: %v", err)
	}
}

func TestRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("NoStateDirectoryIsNoServer", func(t *testing.T) {
		t.Parallel()
		paths := Paths(t.TempDir())
		if live, err := Live(paths); err != nil || live {
			t.Errorf("Live = %v, %v; want false", live, err)
		}
		if _, err := os.Stat(paths.Dir); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Live created %s: %v", paths.Dir, err)
		}
	})

	t.Run("AHeldLockServesNothing", func(t *testing.T) {
		t.Parallel()
		l := newLifecycle(t)
		held, err := lock.AcquireWriteLock(l.opts.Paths.Lock)
		if err != nil {
			t.Fatalf("lock: %v", err)
		}
		defer held.Release()
		var logged strings.Builder
		l.opts.Log = &logged
		l.opts.LockWait = 10 * time.Millisecond
		if err := Run(context.Background(), l.opts); !errors.Is(err, ErrRunning) {
			t.Errorf("Run = %v; want ErrRunning", err)
		}
		if _, found, err := ReadState(l.opts.Paths); err != nil || found || logged.Len() != 0 || l.registry.Load() != 0 {
			t.Errorf("state found %v (%v), log %q, registrations %d; want none", found, err, logged.String(), l.registry.Load())
		}
	})

	t.Run("StartReplacesTheLeftoversAndServes", func(t *testing.T) {
		t.Parallel()
		l := newLifecycle(t)
		leftover := `{"pid":7,"port":8,"token":"old","newer":"key"}`
		if err := os.WriteFile(l.opts.Paths.State, []byte(leftover), 0o600); err != nil {
			t.Fatalf("write state: %v", err)
		}
		if got, found, err := ReadState(l.opts.Paths); err != nil || !found || got != (State{PID: 7, Port: 8, Token: "old"}) {
			t.Fatalf("ReadState = %+v, %v, %v", got, found, err)
		}

		server := l.start()
		l.wantLive(true)
		if server.PID != os.Getpid() || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(server.Token) {
			t.Errorf("state = %+v; want this process and a 32-hex-character token", server)
		}
		if err := Probe(server); err != nil {
			t.Errorf("Probe = %v", err)
		}
		if err := Probe(State{Port: server.Port, Token: "wrong"}); err == nil {
			t.Errorf("Probe with a wrong token = nil; want an error")
		}
		written, err := os.ReadFile(l.opts.Paths.State)
		if err != nil || strings.Contains(string(written), "newer") {
			t.Errorf("state file = %s, %v; want nothing of the former file", written, err)
		}
		logged, err := os.ReadFile(l.opts.Paths.Log)
		if first, _, _ := strings.Cut(string(logged), "\n"); err != nil || first != "lym board serve pid "+strconv.Itoa(os.Getpid()) {
			t.Errorf("log = %q, %v; want it to open with the pid", logged, err)
		}
		if got := l.registry.Load(); got != 1 {
			t.Errorf("registrations = %d; want 1", got)
		}
		if got, want := <-l.watched, []string{l.opts.BoardPath, "board.json"}; !slices.Equal(got, want) {
			t.Errorf("watched %q; want %q", got, want)
		}

		launcher, err := WriteLauncher(l.opts.Paths, server.Port, server.Token)
		if err != nil {
			t.Fatalf("WriteLauncher: %v", err)
		}
		page, err := os.ReadFile(launcher)
		want := fmt.Sprintf("http://127.0.0.1:%d/#%s", server.Port, server.Token)
		if err != nil || !strings.Contains(string(page), `location.replace("`+want+`")`) || strings.Count(string(page), server.Token) != 1 {
			t.Errorf("launcher = %s, %v; want it to go to %s", page, err, want)
		}
		if runtime.GOOS != "windows" {
			for _, path := range []string{l.opts.Paths.State, launcher} {
				if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
					t.Errorf("mode of %s = %v, %v; want 0600", path, info.Mode(), err)
				}
			}
		}
	})

	t.Run("IdleExitCountsFromTheLatestAPIRequest", func(t *testing.T) {
		t.Parallel()
		unused := newLifecycle(t)
		unused.start()
		unused.clock.advance(30 * time.Minute)
		unused.ended()

		used := newLifecycle(t)
		server := used.start()
		used.clock.advance(20 * time.Minute)
		<-used.clock.armed
		used.receive(used.send(server, http.MethodGet, routeBoard, ""))
		used.clock.advance(10 * time.Minute)
		select {
		case <-used.clock.armed:
		case <-used.done:
			t.Fatalf("Run ended before the bound passed from the latest request")
		}
		used.clock.advance(20 * time.Minute)
		used.ended()
	})

	t.Run("AReplacedBinaryEndsTheServerAndAnswersTheLongPoll", func(t *testing.T) {
		t.Parallel()
		l := newLifecycle(t)
		server := l.start()
		revision := l.receive(l.send(server, http.MethodGet, routeBoard, "")).Revision
		poll := l.send(server, http.MethodGet, routeBoard+"?after="+revision, "")
		l.replaced.Store(true)
		l.clock.advance(time.Second)
		if got := l.receive(poll); !got.Unchanged {
			t.Errorf("a poll across the exit = %+v; want unchanged", got)
		}
		l.ended()
	})

	t.Run("AWriteInFlightCompletes", func(t *testing.T) {
		t.Parallel()
		l := newLifecycle(t)
		server := l.start()
		path, err := fabricengine.BoltLockPath(l.board.dir, fabricengine.BoardWriteLockFile)
		if err != nil {
			t.Fatalf("lock path: %v", err)
		}
		held, err := lock.AcquireWriteLock(path)
		if err != nil {
			t.Fatalf("lock the board: %v", err)
		}
		release := sync.OnceValue(held.Release)
		defer release()

		write := l.send(server, http.MethodPost, routeWrite, `{"op":"create","slug":"late","fields":{"kind":"task","title":"Late","labels":["bug"]}}`)
		l.clock.advance(30 * time.Minute)
		deadline := time.Now().Add(time.Minute)
		for {
			live, err := Live(l.opts.Paths)
			if err != nil {
				t.Fatalf("Live: %v", err)
			}
			if !live {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("the exiting server kept its lock")
			}
			time.Sleep(10 * time.Millisecond)
		}
		select {
		case <-l.done:
			t.Fatalf("Run returned with a write in flight")
		default:
		}

		if err := release(); err != nil {
			t.Fatalf("release the board: %v", err)
		}
		l.receive(write)
		l.ended()
		if !strings.Contains(l.board.store(), `"late"`) {
			t.Errorf("board.json does not hold the write")
		}
	})

	t.Run("AnEventOnTheStoreWakesTheLongPoll", func(t *testing.T) {
		t.Parallel()
		l := newLifecycle(t)
		server := l.start()
		event := fswatch.Event{Name: boardengine.EntryStoreFile, Op: "rename"}
		current := func() string { return l.receive(l.send(server, http.MethodGet, routeBoard, "")).Revision }

		first := current()
		poll := l.send(server, http.MethodGet, routeBoard+"?after="+first, "")
		l.retitle("Once")
		l.events <- event
		second := l.receive(poll).Revision
		if second == first {
			t.Errorf("a poll across an event = %q; want a new revision", second)
		}

		// An event within a second of that wake waits for the second to pass.
		<-l.clock.armed
		l.retitle("Twice")
		l.events <- event
		<-l.clock.armed
		if got := current(); got != second {
			t.Errorf("revision within a second of a wake = %q; want still %q", got, second)
		}
		poll = l.send(server, http.MethodGet, routeBoard+"?after="+second, "")
		l.clock.advance(time.Second)
		if got := l.receive(poll).Revision; got == second {
			t.Errorf("a poll past the second = %q; want a new revision", got)
		}
	})

	t.Run("WithNoWatchTheTimerBacksOffAndWakes", func(t *testing.T) {
		t.Parallel()
		l := newLifecycle(t)
		l.opts.Watch = func(string, ...string) (<-chan fswatch.Event, func() error, error) {
			return nil, nil, errors.New("no watch available")
		}
		server := l.start()
		step := func() time.Duration {
			l.clock.advance(8 * time.Second)
			return <-l.clock.armed
		}
		var waits []time.Duration
		for range 4 {
			waits = append(waits, step())
		}

		revision := l.receive(l.send(server, http.MethodGet, routeBoard, "")).Revision
		poll := l.send(server, http.MethodGet, routeBoard+"?after="+revision, "")
		l.retitle("Changed")
		waits = append(waits, step())
		if got := l.receive(poll); got.Unchanged || got.Revision == revision {
			t.Errorf("a poll across the timer = %+v; want a new revision", got)
		}
		want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, time.Second}
		if !slices.Equal(waits, want) {
			t.Errorf("timer waits = %v; want %v", waits, want)
		}
	})
}
