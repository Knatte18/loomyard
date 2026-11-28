// run.go is the board server's life: the single-instance lock, serving, the watch loop and the exit.

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
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/lock"
)

// The watch loop's timer starts at watchBaseWait and doubles to watchMaxWait while the board is unchanged.
const (
	watchBaseWait = time.Second
	watchMaxWait  = 8 * time.Second
)

// readHeaderTimeout bounds how long a connection may take to send its request headers.
const readHeaderTimeout = 10 * time.Second

// tokenBytes is the length of a server's token before hex encoding.
const tokenBytes = 16

// The reasons the watch loop ends a server.
const (
	exitReplaced = "replaced"
	exitIdle     = "idle"
)

// Options is what one server runs with.
type Options struct {
	// Paths is where the server keeps its lock, state and log.
	Paths StatePaths
	// Board is the board served, and BoardPath its directory.
	Board     *boardengine.Board
	BoardPath string
	// LockWait is how long a start waits for the server's lock.
	LockWait time.Duration
	// IdleBound is how long the server lives with no request for board data.
	IdleBound time.Duration
	// PollBound is the longest a long poll waits for a change.
	PollBound time.Duration
	// Log is where the server logs to.
	Log io.Writer
	// Register records the serving process, once it holds the lock.
	Register func() error
	// Replaced reports whether the running binary was replaced, which ends the server.
	Replaced func() bool
	// Watch opens the file events of the board directory; when it fails the timer alone notices changes.
	Watch func(dir string, names ...string) (*fswatch.Watcher, error)
	// Now and After are the server's clock.
	Now   func() time.Time
	After func(time.Duration) <-chan time.Time
}

// ErrRunning reports that another process holds the server's lock.
var ErrRunning = errors.New("the board server is already running")

// Run serves the board on a loopback port until ctx ends, the binary is replaced or the server has been idle.
// At most one process serves per state directory: a second returns ErrRunning, having written nothing.
// On exit it takes no new request, releases the lock and returns once the requests in flight have ended.
func Run(ctx context.Context, opts Options) error {
	if err := os.MkdirAll(opts.Paths.Dir, 0o700); err != nil {
		return fmt.Errorf("create the board server's state directory: %w", err)
	}
	// The lock is waited for, so that a caller reading liveness at this instant does not turn the server away.
	held, acquired, err := lock.AcquireWriteLockWithin(opts.Paths.Lock, opts.LockWait)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrRunning
	}
	released := false
	release := func() error {
		if released {
			return nil
		}
		released = true
		return held.Release()
	}
	defer func() { _ = release() }()

	if err := os.WriteFile(opts.Paths.Log, nil, 0o600); err != nil {
		return fmt.Errorf("truncate the board server's log: %w", err)
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
		return fmt.Errorf("listen on the loopback address: %w", err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port

	secret := make([]byte, tokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("draw the server's token: %w", err)
	}
	token := hex.EncodeToString(secret)
	server, err := NewServer(ServerConfig{
		Board: opts.Board, BoardPath: opts.BoardPath, Token: token, Port: port, PollBound: opts.PollBound, Now: opts.Now,
	})
	if err != nil {
		return err
	}
	if err := writeState(opts.Paths, State{PID: os.Getpid(), Port: port, Token: token}); err != nil {
		return fmt.Errorf("write the board server's state: %w", err)
	}

	httpServer := &http.Server{Handler: server, ReadHeaderTimeout: readHeaderTimeout, ErrorLog: log.New(opts.Log, "", log.LstdFlags)}
	watching, stopWatching := context.WithCancel(ctx)
	defer stopWatching()
	served := make(chan error, 1)
	go func() {
		served <- httpServer.Serve(listener)
		stopWatching()
	}()

	reason := watchBoard(watching, opts, server)

	// A server that stopped serving by itself has failed; one that still serves is closed here.
	var failed error
	select {
	case err := <-served:
		failed = fmt.Errorf("serve the board: %w", err)
	default:
		httpServer.SetKeepAlivesEnabled(false)
		_ = listener.Close()
		<-served
	}
	if reason != "" {
		fmt.Fprintf(opts.Log, "exiting: %s\n", reason)
	}
	// The lock goes before the wait on the requests in flight, so a successor can start while they end.
	releaseErr := release()
	server.beginDrain()
	// The listener is closed already, so the error Shutdown reports for it says nothing.
	_ = httpServer.Shutdown(context.Background())
	return errors.Join(failed, releaseErr)
}

// watchBoard tells server of every change to board.json, until the server should end or ctx ends.
// It returns why the server should end, "" when ctx ended.
func watchBoard(ctx context.Context, opts Options, server *Server) string {
	var events <-chan fswatch.Event
	watcher, err := opts.Watch(opts.BoardPath, boardengine.FabricReplay{}.File())
	if err != nil {
		fmt.Fprintf(opts.Log, "watching the board by timer alone: %v\n", err)
	} else {
		defer func() { _ = watcher.Close() }()
		events = watcher.Events()
	}

	server.mu.Lock()
	known := server.revision
	server.mu.Unlock()
	start := opts.Now()
	wait := watchBaseWait
	timer := opts.After(wait)
	for {
		fired := false
		select {
		case <-ctx.Done():
			return ""
		case _, open := <-events:
			if !open {
				events = nil
				continue
			}
		case <-timer:
			fired = true
		}

		revision, err := boardengine.BoardRevision(opts.BoardPath)
		switch {
		case err != nil:
			fmt.Fprintf(opts.Log, "%v\n", err)
		case revision != known:
			known = revision
			server.BoardChanged(revision)
			wait = watchBaseWait
			timer = opts.After(wait)
			fired = false
		}
		if fired {
			wait = min(2*wait, watchMaxWait)
			timer = opts.After(wait)
		}

		if opts.Replaced() {
			return exitReplaced
		}
		last := server.lastAPIRequest()
		if last.IsZero() {
			last = start
		}
		if opts.Now().Sub(last) >= opts.IdleBound {
			return exitIdle
		}
	}
}
