// run.go is the life of one board server, which starts by taking the single-instance lock.
// It serves and follows the board for changes.
// It exits when it is idle or its binary is replaced.

package boardserve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// The watch loop's timer starts at watchBase and doubles to watchCeiling while the board is unchanged.
// watchBase is also the least time between two wakes.
const (
	watchBase    = time.Second
	watchCeiling = 8 * time.Second
)

// tokenBytes is the size of a server's token before it is written in hex.
const tokenBytes = 16

// The reasons a server exits on its own.
const (
	exitReplaced = "replaced"
	exitIdle     = "idle"
	exitStopped  = "stopped"
)

// ErrRunning reports that another server holds the lock, so this one has nothing to do.
var ErrRunning = errors.New("a board server already runs")

// Options is what Run serves with.
type Options struct {
	Paths StatePaths
	Board *boardengine.Board
	// BoardPath is the board's data directory.
	BoardPath string
	// LockWait is how long Run waits for the lock, long enough to outlast a reader asking whether it is held.
	LockWait time.Duration
	// IdleBound is how long the server lives with no API request.
	IdleBound time.Duration
	// PollBound is how long a long poll waits for a new revision.
	PollBound time.Duration
	// Log is the writer the server logs to, appending to Paths.Log.
	Log io.Writer
	// Register records the process as the serving one; it runs only once the lock is held.
	Register func() error
	// Replaced reports whether the binary the process started from was replaced.
	Replaced func() bool
	// Watch opens a file-event watcher on a directory; when it fails, the board is read on a timer alone.
	Watch func(dir string, names ...string) (*fswatch.Watcher, error)
	Now   func() time.Time
	After func(time.Duration) <-chan time.Time
}

// Run serves the board on a loopback port until the server is idle, its binary is replaced or ctx ends,
// and it returns nil once the requests in flight have been answered.
// At most one server holds a state directory; while another does, Run returns ErrRunning and writes nothing.
func Run(ctx context.Context, opts Options) error {
	if err := os.MkdirAll(opts.Paths.Dir, 0o700); err != nil {
		return fmt.Errorf("create the board server's state directory: %w", err)
	}
	held, acquired, err := lock.AcquireWriteLockWithin(opts.Paths.Lock, opts.LockWait)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrRunning
	}
	release := sync.OnceValue(held.Release)
	defer release()

	if err := os.WriteFile(opts.Paths.Log, nil, 0o600); err != nil {
		return fmt.Errorf("truncate the server log: %w", err)
	}
	fmt.Fprintf(opts.Log, "lym board serve pid %d\n", os.Getpid())
	if err := os.Remove(opts.Paths.State); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove the previous server's state: %w", err)
	}
	if err := opts.Register(); err != nil {
		return fmt.Errorf("register the board server: %w", err)
	}

	listener, err := net.Listen("tcp", loopbackAddress(0))
	if err != nil {
		return fmt.Errorf("listen on loopback: %w", err)
	}
	defer listener.Close()
	secret := make([]byte, tokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("draw the server's token: %w", err)
	}
	self := State{PID: os.Getpid(), Port: listener.Addr().(*net.TCPAddr).Port, Token: hex.EncodeToString(secret)}
	handler, err := NewServer(ServerConfig{
		Board:     opts.Board,
		BoardPath: opts.BoardPath,
		Token:     self.Token,
		Port:      self.Port,
		LongPoll:  opts.PollBound,
		Now:       opts.Now,
	})
	if err != nil {
		return err
	}
	if err := state.WriteJSON(opts.Paths.State, stateLockPath(opts.Paths), self); err != nil {
		return fmt.Errorf("write the server's state: %w", err)
	}
	if err := hardenFile(opts.Paths.State); err != nil {
		return err
	}

	serving := &http.Server{Handler: handler, ErrorLog: log.New(opts.Log, "", log.LstdFlags)}
	watching, stopWatching := context.WithCancel(ctx)
	defer stopWatching()
	go func() {
		// A server that can no longer accept exits instead of holding the lock.
		if err := serving.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(opts.Log, "serve: %v\n", err)
			stopWatching()
		}
	}()
	fmt.Fprintf(opts.Log, "exiting: %s\n", watchBoard(watching, handler, opts))

	// The listener closes before the lock is released, so no new request reaches a server that no longer holds it.
	serving.SetKeepAlivesEnabled(false)
	_ = listener.Close()
	if err := release(); err != nil {
		fmt.Fprintf(opts.Log, "release the server lock: %v\n", err)
	}
	handler.beginDrain()
	// Shutdown reports the listener closed above; what matters is that it returns once the requests are answered.
	_ = serving.Shutdown(context.Background())
	return nil
}

// watchBoard tells server of each change of board.json until the server should exit, and returns why.
// It wakes on an event of board.json or on its timer, never within watchBase of the previous wake.
func watchBoard(ctx context.Context, server *Server, opts Options) string {
	started := opts.Now()
	var events <-chan fswatch.Event
	if watcher, err := opts.Watch(opts.BoardPath, boardengine.BoardStoreFile); err != nil {
		fmt.Fprintf(opts.Log, "watch the board directory: %v; reading the board on a timer\n", err)
	} else {
		defer watcher.Close()
		events = watcher.Events()
	}

	revision, _ := server.current()
	wait := watchBase
	failing := false
	var woke time.Time
	for {
		timer := opts.After(wait)
		select {
		case <-ctx.Done():
			return exitStopped
		case _, open := <-events:
			if !open {
				events = nil
			}
			if rest := watchBase - opts.Now().Sub(woke); rest > 0 {
				var running bool
				if events, running = coalesceEvents(ctx, events, opts.After(rest)); !running {
					return exitStopped
				}
			}
		case <-timer:
		}
		woke = opts.Now()

		next, err := boardengine.BoardRevision(opts.BoardPath)
		switch {
		case err != nil:
			if !failing {
				fmt.Fprintf(opts.Log, "%v\n", err)
			}
			wait = min(2*wait, watchCeiling)
		case next != revision:
			revision = next
			server.BoardChanged(next)
			wait = watchBase
		default:
			wait = min(2*wait, watchCeiling)
		}
		failing = err != nil

		if opts.Replaced() {
			return exitReplaced
		}
		last := server.lastAPIRequest()
		if last.IsZero() {
			last = started
		}
		if opts.Now().Sub(last) >= opts.IdleBound {
			return exitIdle
		}
	}
}

// coalesceEvents takes the events that arrive until floor fires, so they share the one wake that follows.
// It returns the event channel to go on with, nil once it is closed, and false when ctx ended first.
func coalesceEvents(ctx context.Context, events <-chan fswatch.Event, floor <-chan time.Time) (<-chan fswatch.Event, bool) {
	for {
		select {
		case <-ctx.Done():
			return events, false
		case _, open := <-events:
			if !open {
				events = nil
			}
		case <-floor:
			return events, true
		}
	}
}
