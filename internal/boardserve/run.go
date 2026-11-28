// run.go holds the server's lifecycle: the single-instance lock, serving, the watch loop and the exit.

package boardserve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// tokenBytes is the length of the token before its hex encoding.
const tokenBytes = 16

// The watch loop's timer starts at the floor and doubles to the ceiling while the board is unchanged.
const (
	watchFloor   = time.Second
	watchCeiling = 8 * time.Second
)

// The reasons the watch loop ends the server for.
const (
	exitReplaced = "replaced"
	exitIdle     = "idle"
)

// Options is what Run serves and the seams it runs on.
type Options struct {
	Paths StatePaths
	Board *boardengine.Board
	// BoardPath is the board directory, which the server watches and serves files from.
	BoardPath string
	// LockWait bounds the wait for the single-instance lock,
	// so a reader that takes the lock for an instant never turns a server away.
	LockWait time.Duration
	// IdleBound is how long the server lives with no API request.
	IdleBound time.Duration
	// PollBound bounds how long a board request waits for a change.
	PollBound time.Duration
	// Log is the writer the server logs to, which appends to the file at Paths.Log.
	Log io.Writer
	// Register records the serving process, once it holds the lock.
	Register func() error
	// Replaced reports whether the binary on disk is no longer the one that runs.
	Replaced func() bool
	// Watch opens a file watcher; when it fails, the timer alone finds a change.
	Watch func(dir string, names ...string) (*fswatch.Watcher, error)
	Now   func() time.Time
	After func(time.Duration) <-chan time.Time
}

// ErrRunning reports that another server holds the lock, so this one served nothing.
var ErrRunning = errors.New("boardserve: a board server already runs")

// Run serves the board on a loopback port, as the one server of its state directory.
// It returns nil once the binary is replaced, no API request has come within the idle bound, or ctx ends.
// Before it returns nil, it waits for the requests in flight to finish.
// It returns ErrRunning, having written nothing, when another server holds the lock.
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
	release := sync.OnceFunc(func() {
		if err := held.Release(); err != nil {
			logger.Warn("boardserve: the server's lock did not release", "error", err)
		}
	})
	defer release()

	if err := os.Truncate(opts.Paths.Log, 0); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("truncate the board server's log: %w", err)
	}
	if _, err := fmt.Fprintf(opts.Log, "lym board serve pid %d\n", os.Getpid()); err != nil {
		return fmt.Errorf("write the board server's log: %w", err)
	}
	if err := os.Remove(opts.Paths.State); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove the previous server's state: %w", err)
	}
	if err := opts.Register(); err != nil {
		return fmt.Errorf("register the board server: %w", err)
	}

	listener, err := net.Listen("tcp", loopbackHost+":0")
	if err != nil {
		return fmt.Errorf("bind the board server: %w", err)
	}
	defer listener.Close()
	secret := make([]byte, tokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("draw the board server's token: %w", err)
	}
	current := State{PID: os.Getpid(), Port: listener.Addr().(*net.TCPAddr).Port, Token: hex.EncodeToString(secret)}
	server, err := NewServer(ServerConfig{
		Board:     opts.Board,
		BoardPath: opts.BoardPath,
		Token:     current.Token,
		Port:      current.Port,
		LongPoll:  opts.PollBound,
		Now:       opts.Now,
	})
	if err != nil {
		return err
	}
	if err := writeState(opts.Paths, current); err != nil {
		return err
	}

	httpServer := &http.Server{Handler: server}
	go func() {
		if err := httpServer.Serve(listener); !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			logger.Warn("boardserve: the server stopped accepting", "error", err)
		}
	}()

	if reason := watchBoard(ctx, opts, server); reason != "" {
		logger.Info("boardserve: the server exits", "reason", reason)
	}

	// New requests are turned away before the lock is free, so a state that answers is always the lock holder's.
	// The requests in flight are waited for after it, so a write finishes while a fresh server may already start.
	httpServer.SetKeepAlivesEnabled(false)
	if err := listener.Close(); err != nil {
		logger.Warn("boardserve: the listener did not close", "error", err)
	}
	release()
	server.beginDrain()
	// The only error left to report is the second close of the listener.
	_ = httpServer.Shutdown(context.Background())
	return nil
}

// watchBoard tells server of every change to the board until the server is to exit, and returns why.
// It returns "" when ctx ends.
// It wakes on a file event or its timer, never twice within the floor on an event.
func watchBoard(ctx context.Context, opts Options, server *Server) string {
	var events <-chan fswatch.Event
	watcher, err := opts.Watch(opts.BoardPath, boardengine.BoardStoreFile)
	if err != nil {
		logger.Info("boardserve: the board is not watched, so the timer alone reads it", "error", err)
	} else {
		defer watcher.Close()
		events = watcher.Events()
	}

	// known is read after the watch is open, so no change falls between the two.
	known, unread := "", false
	read := func() (changed bool) {
		revision, err := boardengine.BoardRevision(opts.BoardPath)
		if err != nil {
			if !unread {
				logger.Warn("boardserve: the board's revision could not be read", "error", err)
			}
			unread = true
			return false
		}
		unread = false
		if revision == known {
			return false
		}
		known = revision
		server.BoardChanged(revision)
		return true
	}
	read()

	interval := watchFloor
	woke := opts.Now()
	timer := opts.After(interval)
	// deferred fires for an event that came within the floor of the previous wake.
	var deferred <-chan time.Time
	for {
		timed := false
		select {
		case <-ctx.Done():
			return ""
		case <-timer:
			timed = true
		case <-deferred:
		case _, open := <-events:
			if !open {
				events = nil
				continue
			}
			if deferred != nil {
				continue
			}
			if wait := watchFloor - opts.Now().Sub(woke); wait > 0 {
				deferred = opts.After(wait)
				continue
			}
		}
		deferred = nil
		woke = opts.Now()

		switch {
		case read():
			interval = watchFloor
		case timed:
			interval = min(2*interval, watchCeiling)
		}
		if opts.Replaced() {
			return exitReplaced
		}
		if opts.Now().Sub(server.lastAPIRequest()) >= opts.IdleBound {
			return exitIdle
		}
		timer = opts.After(interval)
	}
}
