// watchloop.go — the watcher daemon's loop: single-instance per prime, polling Tick until the
// strand is gone, the context is cancelled, or ticks keep failing.

package orchengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// maxConsecutiveTickErrors caps consecutive failed ticks, since most ticks spawn tmux through reed.
	// At the default two-second poll interval it gives about a minute of failures.
	maxConsecutiveTickErrors = 30

	// watchLockAttempts and watchLockRetryInterval bound how long Run waits for a watch lock that
	// WatcherLive holds for the instant of its probe.
	watchLockAttempts      = 5
	watchLockRetryInterval = 200 * time.Millisecond
)

// ErrWatcherRunning reports that another watcher already holds the watch lock.
var ErrWatcherRunning = errors.New("orch: a watcher is already running")

// WatcherLive reports whether a watcher holds the watch lock.
// It probes by taking the lock and releasing it at once.
func WatcherLive(p Paths) (bool, error) {
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return false, fmt.Errorf("orch: create %s: %w", p.Dir, err)
	}
	l, acquired, err := lock.TryAcquireWriteLock(p.WatchLockPath)
	if err != nil {
		return false, err
	}
	if !acquired {
		return true, nil
	}
	if err := l.Release(); err != nil {
		return false, fmt.Errorf("orch: release watch lock: %w", err)
	}
	return false, nil
}

// acquireWatchLock takes the watch lock, retrying a bounded number of times.
func acquireWatchLock(p Paths, sleep func(time.Duration)) (*lock.FileLock, error) {
	for attempt := 0; attempt < watchLockAttempts; attempt++ {
		if attempt > 0 {
			sleep(watchLockRetryInterval)
		}
		l, acquired, err := lock.TryAcquireWriteLock(p.WatchLockPath)
		if err != nil {
			return nil, err
		}
		if acquired {
			return l, nil
		}
	}
	return nil, ErrWatcherRunning
}

// recordExit persists why the watcher is exiting.
func (w *Watcher) recordExit(reason string) error {
	st, err := LoadState(w.paths)
	if err != nil {
		return err
	}
	st.WatcherExit = reason
	return SaveState(w.paths, st)
}

// Run holds the watch lock and calls Tick every PollInterval until Tick reports done, ctx is
// cancelled, or the cap on consecutive tick errors is reached.
// It returns ErrWatcherRunning when another watcher holds the lock through every retry.
func (w *Watcher) Run(ctx context.Context, sleep func(time.Duration)) error {
	if err := os.MkdirAll(w.paths.Dir, 0o755); err != nil {
		return fmt.Errorf("orch: create %s: %w", w.paths.Dir, err)
	}
	l, err := acquireWatchLock(w.paths, sleep)
	if err != nil {
		return err
	}
	defer l.Release()

	st, err := LoadState(w.paths)
	if err != nil {
		return err
	}
	st.WatcherExit = ""
	if err := SaveState(w.paths, st); err != nil {
		return err
	}

	failures := 0
	for {
		if ctx.Err() != nil {
			return w.recordExit("stopped by signal")
		}
		done, err := w.Tick()
		switch {
		case err != nil:
			failures++
			logger.Warn("orch: watcher tick failed", "error", err, "consecutive", failures)
			if failures >= maxConsecutiveTickErrors {
				if recErr := w.recordExit(fmt.Sprintf("tick failed %d times in a row: %v", failures, err)); recErr != nil {
					logger.Warn("orch: record watcher exit failed", "error", recErr)
				}
				return err
			}
		case done:
			return nil
		default:
			failures = 0
		}
		sleep(w.cfg.PollInterval())
	}
}
