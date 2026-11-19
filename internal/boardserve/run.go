// run.go is the board server's lifecycle.
// It covers the single-instance lock, the listener and state file, and the watch loop.
// The watch loop carries board changes to waiting pages.
// It also holds the exits that end a server nobody stops.

package boardserve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/lock"
)

// What a production server runs with; the serve verb passes them in Options.
const (
	// DefaultLockWait is how long Run waits for a lock a starting or draining server holds before it gives up.
	DefaultLockWait = 5 * time.Second
	// DefaultIdleBound is how long a server serves with no authorized API request before it exits.
	DefaultIdleBound = 30 * time.Minute
	// DefaultPollBound is how long a long poll waits for a change before it answers "unchanged".
	DefaultPollBound = 25 * time.Second
)

// The watch loop's timer starts at watchBaseInterval and doubles to watchCeilingInterval while the board is unchanged.
const (
	watchBaseInterval    = time.Second
	watchCeilingInterval = 8 * time.Second
)

// watchEventFloor is the shortest gap between a read of board.json and the next one a file event causes.
// The event source coalesces nothing, and one board write raises several events.
const watchEventFloor = time.Second

// readHeaderTimeout bounds how long a connection may take to send its request header.
const readHeaderTimeout = 10 * time.Second

// boardFileName is the file the watch loop watches in the board directory.
const boardFileName = "board.json"

// The reasons watchBoard ends.
const (
	exitReplaced = "replaced"
	exitIdle     = "idle"
	exitStopped  = "stopped"
)

// ErrRunning is what Run returns when another board server already holds the hub's lock.
var ErrRunning = errors.New("another board server serves this hub")

// WatchFunc starts watching the named files of dir.
// It returns the channel of their change events and the function that stops the watch.
type WatchFunc func(dir string, names ...string) (events <-chan fswatch.Event, stop func() error, err error)

// WatchDirectory is the WatchFunc over the file-event source: it watches dir for changes to the named files.
func WatchDirectory(dir string, names ...string) (<-chan fswatch.Event, func() error, error) {
	watcher, err := fswatch.Watch(dir, names...)
	if err != nil {
		return nil, nil, err
	}
	return watcher.Events(), watcher.Close, nil
}

// Options holds what Run is built from.
type Options struct {
	// Paths is where the server keeps its files.
	Paths StatePaths
	// Board is the board every read and write goes through.
	Board *boardengine.Board
	// BoardPath is the board directory.
	BoardPath string
	// LockWait is how long Run waits for the server lock before it returns ErrRunning.
	LockWait time.Duration
	// IdleBound is how long the server serves with no authorized API request before it exits.
	IdleBound time.Duration
	// PollBound is how long a long poll waits for a change.
	PollBound time.Duration
	// Log receives the server's log lines; the first names the server's pid.
	Log io.Writer
	// Register records the server process once it holds the lock.
	Register func() error
	// Replaced reports whether the server's binary was replaced on disk.
	Replaced func() bool
	// Watch watches the board directory; nil, or a failing Watch, leaves the timer alone to read the board.
	Watch WatchFunc
	// Now is the clock the idle exit and the API activity stamp read.
	Now func() time.Time
	// After returns a channel that delivers once the duration has passed.
	After func(time.Duration) <-chan time.Time
}

// Run serves the board until it is replaced, idle or ctx ends, and returns nil when it exits cleanly.
// It returns ErrRunning, having written nothing, when another server holds the hub's lock.
// On exit the server stops taking connections and releases the lock so a new server can start.
// It returns only after the requests in flight finished.
func Run(ctx context.Context, opts Options) error {
	if err := os.MkdirAll(opts.Paths.Dir, 0o755); err != nil {
		return fmt.Errorf("create the board server state directory: %w", err)
	}
	serverLock, acquired, err := lock.AcquireWriteLockWithin(opts.Paths.Lock, opts.LockWait)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrRunning
	}
	lockHeld := true
	defer func() {
		if lockHeld {
			_ = serverLock.Release()
		}
	}()

	if err := os.Truncate(opts.Paths.Log, 0); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("truncate the board server log: %w", err)
	}
	if _, err := fmt.Fprintf(opts.Log, "lym board serve pid %d\n", os.Getpid()); err != nil {
		return fmt.Errorf("write the board server log: %w", err)
	}
	if err := os.Remove(opts.Paths.State); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the previous board server state: %w", err)
	}
	if err := opts.Register(); err != nil {
		return fmt.Errorf("register the board server: %w", err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:0", loopbackHost))
	if err != nil {
		return fmt.Errorf("listen on the loopback address: %w", err)
	}
	token, err := newToken()
	if err != nil {
		_ = listener.Close()
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	server := NewServer(ServerConfig{
		Board:         opts.Board,
		BoardPath:     opts.BoardPath,
		Token:         token,
		Port:          port,
		LongPollBound: opts.PollBound,
		Now:           opts.Now,
	})
	if err := writeState(opts.Paths, State{PID: os.Getpid(), Port: port, Token: token}); err != nil {
		_ = listener.Close()
		return err
	}

	httpServer := &http.Server{Handler: server, ReadHeaderTimeout: readHeaderTimeout}
	serveCtx, stopServing := context.WithCancel(ctx)
	defer stopServing()
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- httpServer.Serve(listener)
		stopServing()
	}()

	reason := watchBoard(serveCtx, opts, server)
	fmt.Fprintf(opts.Log, "lym board serve exits: %s\n", reason)

	httpServer.SetKeepAlivesEnabled(false)
	_ = listener.Close()
	lockHeld = false
	if err := serverLock.Release(); err != nil {
		return err
	}
	server.beginDrain()
	// Shutdown reports the listener this function already closed; what it waits for is the requests in flight.
	_ = httpServer.Shutdown(context.Background())

	if serveErr := <-serveErrors; !errors.Is(serveErr, net.ErrClosed) && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("serve the board: %w", serveErr)
	}
	return nil
}

// currentRevision returns the board revision the server last answered or was told of.
func (s *Server) currentRevision() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revision
}

// newToken draws the server's bearer token: 128 random bits as 32 hex characters.
func newToken() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("draw the board server token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// watchBoard is the one watch loop of a running server and returns the reason the server should exit.
// It wakes on a change event of board.json or on its timer.
// An event wake waits until one second has passed since the previous read, then discards the events queued meanwhile,
// so a burst of events costs one read.
// The timer starts at one second and doubles to eight while the board's revision is unchanged.
// It returns to one second on a change.
// Each wake recomputes the board's revision and tells the server of a change.
// A replay or sync that rewrote board.json thus reaches the page like a local write.
// It returns "replaced" when the binary was replaced.
// It returns "idle" when no authorized API request came within the idle bound.
func watchBoard(ctx context.Context, opts Options, server *Server) string {
	var events <-chan fswatch.Event
	if opts.Watch != nil {
		watchEvents, stop, err := opts.Watch(opts.BoardPath, boardFileName)
		if err != nil {
			fmt.Fprintf(opts.Log, "lym board serve watches board.json by timer alone: %v\n", err)
		} else {
			events = watchEvents
			defer stop()
		}
	}

	// The server's own revision, so a change between its construction and this loop's start still reaches the page.
	known := server.currentRevision()
	interval := watchBaseInterval
	// lastRead is zero until the first read, so the first event is read at once.
	var lastRead time.Time
	for {
		eventWake := false
		select {
		case <-ctx.Done():
			return exitStopped
		case _, open := <-events:
			if open {
				eventWake = true
			} else {
				events = nil
			}
		case <-opts.After(interval):
		}
		if eventWake {
			if !lastRead.IsZero() {
				if rest := watchEventFloor - opts.Now().Sub(lastRead); rest > 0 {
					select {
					case <-ctx.Done():
						return exitStopped
					case <-opts.After(rest):
					}
				}
			}
			if !drainEvents(events) {
				events = nil
			}
		}

		if opts.Replaced() {
			return exitReplaced
		}
		revision, err := boardengine.BoardRevision(opts.BoardPath)
		if err == nil && revision != known {
			known = revision
			server.BoardChanged(revision)
			interval = watchBaseInterval
		} else {
			interval = min(interval*2, watchCeilingInterval)
		}
		lastRead = opts.Now()
		if lastRead.Sub(server.lastAPIRequest()) >= opts.IdleBound {
			return exitIdle
		}
	}
}

// drainEvents discards the events already queued on events and reports whether the channel is still open.
func drainEvents(events <-chan fswatch.Event) bool {
	for {
		select {
		case _, open := <-events:
			if !open {
				return false
			}
		default:
			return true
		}
	}
}
