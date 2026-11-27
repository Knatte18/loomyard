// run.go is the board server's lifecycle: the single-instance start, the serving, the watch loop and the exit.

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
)

// The watch loop's waits: its timer starts at the floor and doubles to the ceiling while the board is unchanged,
// and no two of its wakes by a file event are closer than the floor.
const (
	watchFloor   = time.Second
	watchCeiling = 8 * time.Second
)

// tokenBytes is the size of the server's random token.
const tokenBytes = 16

// The reasons the watch loop ends the server for.
const (
	exitReplaced = "replaced"
	exitIdle     = "idle"
)

// Options is what Run serves with.
// Every field is required.
type Options struct {
	Paths StatePaths
	Board *boardengine.Board
	// BoardPath is the board directory the served files and the watched entry store sit in.
	BoardPath string
	// LockWait is how long Run waits for the single-instance lock.
	LockWait time.Duration
	// IdleBound is how long the server lives with no /api request.
	IdleBound time.Duration
	// PollBound is how long a long poll waits for a change.
	PollBound time.Duration
	// Log is the writer the server logs to, the one whose file is Paths.Log.
	Log io.Writer
	// Register records the server's process once it holds the lock.
	Register func() error
	// Replaced reports whether the binary the server runs has been replaced.
	Replaced func() bool
	// Watch opens a file-event source on the entries names of dir, with the function that closes it.
	Watch func(dir string, names ...string) (<-chan fswatch.Event, func() error, error)
	Now   func() time.Time
	After func(time.Duration) <-chan time.Time
}

// ErrRunning reports that another server holds the hub's lock, so Run served nothing.
var ErrRunning = errors.New("boardserve: a board server already holds the hub's lock")

// Run serves the board on a loopback port.
// It stops serving when the idle bound passes, its binary is replaced or ctx ends.
// It then returns nil once the requests in flight are done.
// At most one server of a hub serves at a time.
// While another holds the lock, Run returns ErrRunning and writes nothing to the log or the state.
func Run(ctx context.Context, opts Options) error {
	if err := os.MkdirAll(opts.Paths.Dir, 0o700); err != nil {
		return fmt.Errorf("create the board server's directory: %w", err)
	}
	held, acquired, err := lock.AcquireWriteLockWithin(opts.Paths.Lock, opts.LockWait)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrRunning
	}
	release := sync.OnceValue(held.Release)
	defer func() { _ = release() }()

	// The log of the server that holds the lock opens with its pid, so an operator can end it when it hangs.
	if err := os.Truncate(opts.Paths.Log, 0); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("truncate the board server log: %w", err)
	}
	fmt.Fprintf(opts.Log, "lym board serve pid %d\n", os.Getpid())
	// A reader takes the state for the lock holder's, so a former server's state goes before anything slow.
	if err := os.Remove(opts.Paths.State); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove the former board server state: %w", err)
	}
	if err := opts.Register(); err != nil {
		return fmt.Errorf("register the board server: %w", err)
	}

	listener, err := net.Listen("tcp", loopbackHost+":0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()
	secret := make([]byte, tokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("draw the server token: %w", err)
	}
	self := State{PID: os.Getpid(), Port: listener.Addr().(*net.TCPAddr).Port, Token: hex.EncodeToString(secret)}
	handler, err := NewServer(ServerConfig{
		Board:     opts.Board,
		BoardPath: opts.BoardPath,
		Token:     self.Token,
		Port:      self.Port,
		PollBound: opts.PollBound,
		Now:       opts.Now,
	})
	if err != nil {
		return err
	}
	if err := writeState(opts.Paths, self); err != nil {
		return fmt.Errorf("write the board server state: %w", err)
	}
	server := &http.Server{Handler: handler, ErrorLog: log.New(opts.Log, "", log.LstdFlags)}
	// Serve returns only once the listener is closed, which the exit below does.
	go func() { _ = server.Serve(listener) }()

	if reason := watchBoard(ctx, opts, handler); reason != "" {
		fmt.Fprintf(opts.Log, "lym board serve exits: %s\n", reason)
	}

	// New connections stop before the lock is free, so a state that answers is always the lock holder's.
	server.SetKeepAlivesEnabled(false)
	_ = listener.Close()
	if err := release(); err != nil {
		fmt.Fprintf(opts.Log, "release the board server lock: %v\n", err)
	}
	handler.beginDrain()
	// Shutdown waits out the requests in flight; its error is that of the listener closed above.
	_ = server.Shutdown(context.Background())
	return nil
}

// watchBoard tells server of every change of the board's revision until the server should exit.
// It returns exitReplaced or exitIdle, or "" when ctx ends.
// It wakes on an event on the entry store, or on a timer that backs off while the board is unchanged.
func watchBoard(ctx context.Context, opts Options, server *Server) string {
	events, closeWatch, err := opts.Watch(opts.BoardPath, boardengine.EntryStoreFile)
	if err != nil {
		fmt.Fprintf(opts.Log, "watch the board directory, reading it on the timer alone: %v\n", err)
		events = nil
	} else {
		defer func() { _ = closeWatch() }()
	}

	wait := watchFloor
	timer := opts.After(wait)
	var lastWake time.Time
	var held <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return ""
		case _, open := <-events:
			if !open {
				events = nil
				continue
			}
			// A burst of events wakes the loop once, the floor after its last wake.
			if since := opts.Now().Sub(lastWake); since < watchFloor {
				if held == nil {
					held = opts.After(watchFloor - since)
				}
				continue
			}
		case <-held:
		case <-timer:
		}
		held = nil
		lastWake = opts.Now()

		changed := false
		revision, err := boardengine.BoardRevision(opts.BoardPath)
		if err != nil {
			fmt.Fprintln(opts.Log, err)
		} else if known, _ := server.currentRevision(); revision != known {
			server.BoardChanged(revision)
			changed = true
		}
		if opts.Replaced() {
			return exitReplaced
		}
		if !opts.Now().Before(server.lastAPIRequest().Add(opts.IdleBound)) {
			return exitIdle
		}

		wait = min(2*wait, watchCeiling)
		if changed {
			wait = watchFloor
		}
		timer = opts.After(wait)
	}
}
