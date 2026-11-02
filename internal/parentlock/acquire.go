// acquire.go — the unbounded queue wait for a parent lock: attempts until granted, polling with backoff and waking early on a change to the lock record.

package parentlock

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// acquireWakeFloor is the first poll delay, the delay the wait returns to when the holder changes, and the least time between two wakes.
	acquireWakeFloor = time.Second
	// acquireWakeCeiling is the longest poll delay while the holder is unchanged.
	acquireWakeCeiling = 30 * time.Second
)

// ErrPaused is returned by Acquire when the waiter's Paused seam reports a pause.
var ErrPaused = errors.New("parent lock wait paused")

// Waiter carries what a caller tells Acquire; each field is optional.
type Waiter struct {
	// OnWait is called once when the wait starts and again whenever the holder changes, so the caller can record the wait.
	OnWait func(holder Record)
	// Paused is read before every sleep; true ends the wait with ErrPaused.
	Paused func() bool
	// Sleep replaces the timer wait; it returns the context's error when ctx ends first.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now replaces the clock the least time between two wakes is measured on.
	Now func() time.Time
	// Front queues the caller ahead of every ticket not itself at the front, for a run taking back a lock it freed to work outside it.
	Front bool
	// Watch replaces fswatch.Watch, which opens the watch on the lock record.
	Watch func(dir string, names ...string) (*fswatch.Watcher, error)
}

func (w Waiter) sleep(ctx context.Context, d time.Duration) error {
	if w.Sleep != nil {
		return w.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (w Waiter) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

// pause sleeps d, or until changes delivers a change to the lock record.
// A change still waits out the rest of acquireWakeFloor since wokeAt, so no wake follows another by less than the floor.
// A nil changes sleeps d.
// changesClosed reports that changes is closed, which the caller then replaces with nil.
// A closed channel would end every later pause at once and lose the backoff.
func (w Waiter) pause(ctx context.Context, d time.Duration, wokeAt time.Time, changes <-chan fswatch.Event) (changesClosed bool, err error) {
	if changes == nil {
		return false, w.sleep(ctx, d)
	}
	sleepCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	slept := make(chan error, 1)
	go func() { slept <- w.sleep(sleepCtx, d) }()
	select {
	case err := <-slept:
		return channelClosed(changes), err
	case _, open := <-changes:
		if !open {
			return true, <-slept
		}
		cancel()
		if err := <-slept; err == nil {
			return false, nil
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if rest := acquireWakeFloor - w.now().Sub(wokeAt); rest > 0 {
			return false, w.sleep(ctx, rest)
		}
		return false, nil
	}
}

// channelClosed reports whether changes is closed, without waiting for an event.
func channelClosed(changes <-chan fswatch.Event) bool {
	select {
	case _, open := <-changes:
		return !open
	default:
		return false
	}
}

// Acquire attempts branch's lock for owner until it is granted, waiting in the queue with no time limit, and returns the granted record.
// It polls from one second, doubling to thirty seconds while the holder is unchanged and returning to one second when it changes.
// A change to the lock record, such as its holder's release, ends the poll delay early, still a second after the last attempt began;
// a watch that cannot open, or that closes mid-wait, leaves the wait on its poll alone.
// A cancelled context, a pause and an attempt error each return with owner's ticket removed.
// It only waits and takes: it never frees another owner's live record.
func (s *Store) Acquire(ctx context.Context, branch string, owner Owner, w Waiter) (Record, error) {
	var lastHolder Owner
	var delay time.Duration
	var changes <-chan fswatch.Event
	for first := true; ; first = false {
		wokeAt := w.now()
		if err := ctx.Err(); err != nil {
			return Record{}, s.abandonTicket(branch, owner, err)
		}
		attempt, err := s.tryAcquire(branch, owner, w.Front)
		if err != nil {
			return Record{}, s.abandonTicket(branch, owner, err)
		}
		if attempt.Granted {
			record, _, err := s.Read(branch)
			return record, err
		}
		if first {
			watch := w.Watch
			if watch == nil {
				watch = fswatch.Watch
			}
			watcher, err := watch(s.Dir, s.recordName(branch))
			if err != nil {
				logger.Warn("parentlock: could not watch the lock record, polling only", "branch", branch, "cause", err)
			} else {
				defer watcher.Close()
				changes = watcher.Events()
			}
		}
		if first || attempt.Holder.Owner != lastHolder {
			lastHolder = attempt.Holder.Owner
			delay = acquireWakeFloor
			if w.OnWait != nil {
				w.OnWait(attempt.Holder)
			}
		} else {
			delay = min(delay*2, acquireWakeCeiling)
		}
		if w.Paused != nil && w.Paused() {
			return Record{}, s.abandonTicket(branch, owner, ErrPaused)
		}
		changesClosed, err := w.pause(ctx, delay, wokeAt, changes)
		if changesClosed {
			changes = nil
		}
		if err != nil {
			return Record{}, s.abandonTicket(branch, owner, err)
		}
	}
}

// abandonTicket removes owner's ticket and returns cause, joined with any removal failure.
func (s *Store) abandonTicket(branch string, owner Owner, cause error) error {
	if err := os.Remove(s.ticketPath(branch, owner)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.Join(cause, fmt.Errorf("remove parent lock ticket: %w", err))
	}
	return cause
}
