// watchloop.go — the watcher daemon's loop: single-instance per prime, polling Tick until the strand is gone, the context is cancelled, or ticks keep failing.

package orchengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

const (
	// maxConsecutiveTickErrors caps consecutive failed ticks, since most ticks spawn tmux through reed.
	// At the default two-second poll interval it gives about a minute of failures.
	maxConsecutiveTickErrors = 30

	// watchLockAttempts and watchLockRetryInterval bound how long Run waits for a watch lock that WatcherLive holds for the instant of its probe.
	watchLockAttempts      = 5
	watchLockRetryInterval = 200 * time.Millisecond //lyx:one-shot bounded lock retry of watchLockAttempts
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

// WaitWatcherGone probes the watch lock and, while the state records the watcher stopping and the lock is still held, sleeps interval and probes again, up to bound.
// It answers whether a watcher still holds the lock at the end.
// With no stopping record it probes once and returns,
// so a healthy live watcher costs no wait.
func WaitWatcherGone(p Paths, bound, interval time.Duration, sleep func(time.Duration)) (live bool, err error) {
	for waited := time.Duration(0); ; waited += interval {
		live, err = WatcherLive(p)
		if err != nil || !live {
			return live, err
		}
		st, err := LoadState(p)
		if err != nil {
			return true, err
		}
		if !st.WatcherStopping || waited >= bound {
			return true, nil
		}
		sleep(interval)
	}
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

// recordExit persists reason as the watcher's exit reason under the state lock, touching no other field.
// An empty reason marks a watcher as running and clears the stopping record with it.
func (w *Watcher) recordExit(reason string) error {
	return updateState(w.paths, func(st State) State {
		st.WatcherExit = reason
		if reason == "" {
			st.WatcherStopping = false
		}
		return st
	})
}

// recordStoppingOnCancel persists WatcherStopping as soon as ctx is cancelled,
// so the record lands before the in-flight tick finishes and before the lock is released.
// It returns once ctx is cancelled or runReturned is closed, whichever comes first.
func (w *Watcher) recordStoppingOnCancel(ctx context.Context, runReturned <-chan struct{}) {
	select {
	case <-runReturned:
		return
	case <-ctx.Done():
	}
	err := updateState(w.paths, func(st State) State {
		st.WatcherStopping = true
		return st
	})
	if err != nil {
		logger.Warn("orch: record watcher stopping failed", "error", err)
	}
}

// readMailboxAtStart reads the watcher's creel mailbox once through the handler every tick's read uses, so creel knows the program as a receiver before any reply can arrive.
// The handler declines while the watcher has not bound to the recorded strand, which leaves every message unread for the first tick to handle once the events cursor is set.
// A failed read is logged and changes nothing else.
func (w *Watcher) readMailboxAtStart() {
	err := w.creel.ReadMailbox(w.readReplies)
	switch {
	case errors.Is(err, errNotBound):
		logger.Debug("orch: creel mailbox left unread until the first tick binds the watcher")
	case err != nil:
		logger.Warn("orch: reading the creel mailbox at start failed", "error", err)
	}
}

// WatchBinary tells the watcher the function that re-execs the process onto a replaced binary, which Run calls at a restart-safe tick; a watcher never told one never calls it.
// check does not return when it execs.
func (w *Watcher) WatchBinary(check func()) {
	w.binaryCheck = check
}

// restartSafe reports whether the process can be replaced now without losing anything in flight.
// It re-reads the state from disk and is true only when the phase is idle, so no cycle step is in flight and the persisted cycle state carries over.
// It also needs the told pause record to read with no live hub entry, so no in-memory pause handling is live to be lost.
// A watcher never told a pause record reads no entry, and a state or record read error is not safe.
func (w *Watcher) restartSafe() bool {
	st, err := LoadState(w.paths)
	if err != nil || (st.Phase != PhaseIdle && st.Phase != "") {
		return false
	}
	if w.pause == nil {
		return true
	}
	record, err := pauserecord.Read(w.pause.RecordPath)
	if err != nil {
		return false
	}
	_, live := record.Covering("")
	return !live
}

// Run holds the watch lock and calls Tick every PollInterval until Tick reports done, ctx is cancelled, or the cap on consecutive tick errors is reached.
// It returns ErrWatcherRunning when another watcher holds the lock through every retry.
// It reads the creel mailbox once before its first tick; a failed read is logged and is not a tick error.
// A run of failing ticks warns at its first failure and logs the rest at Debug, then logs one Info at the next tick that succeeds.
// After a tick that succeeded and returned not done, and before the poll sleep, it calls the function WatchBinary was told when restartSafe reports true, so the check adds no wake of its own.
func (w *Watcher) Run(ctx context.Context, sleep func(time.Duration)) error {
	if err := os.MkdirAll(w.paths.Dir, 0o755); err != nil {
		return fmt.Errorf("orch: create %s: %w", w.paths.Dir, err)
	}
	l, err := acquireWatchLock(w.paths, sleep)
	if err != nil {
		return err
	}
	defer l.Release()

	if err := w.recordExit(""); err != nil {
		return err
	}
	w.readMailboxAtStart()

	// The goroutine ends before the lock is released,
	// so its record can never land after a successor watcher cleared it.
	runReturned := make(chan struct{})
	recorderExited := make(chan struct{})
	go func() {
		defer close(recorderExited)
		w.recordStoppingOnCancel(ctx, runReturned)
	}()
	defer func() {
		close(runReturned)
		<-recorderExited
	}()

	failures := 0
	var tickFailures logger.Episode
	for {
		if ctx.Err() != nil {
			return w.recordExit("stopped by signal")
		}
		done, err := w.Tick()
		switch {
		case err != nil:
			failures++
			tickFailures.Fail("orch: watcher tick failed", "error", err, "consecutive", failures)
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
			tickFailures.Recover("orch: watcher tick succeeded again")
			if w.binaryCheck != nil && w.restartSafe() {
				w.binaryCheck()
			}
		}
		sleep(w.cfg.PollInterval())
	}
}
