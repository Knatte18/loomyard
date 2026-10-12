// acquire.go — the unbounded queue wait for a parent lock: attempts until granted, polling with backoff.

package parentlock

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

const (
	// acquireWakeFloor is the first poll delay and the delay the wait returns to when the holder changes.
	acquireWakeFloor = time.Second
	// acquireWakeCeiling is the longest poll delay while the holder is unchanged.
	acquireWakeCeiling = 30 * time.Second
)

// ErrPaused is returned by Acquire when the waiter's Paused seam reports a pause.
var ErrPaused = errors.New("parent lock wait paused")

// Waiter carries the seams a caller tells Acquire; each is optional.
type Waiter struct {
	// OnWait is called once when the wait starts and again whenever the holder changes, so the caller can record the wait.
	OnWait func(holder Record)
	// Paused is read before every sleep; true ends the wait with ErrPaused.
	Paused func() bool
	// Sleep replaces the timer wait; it returns the context's error when ctx ends first.
	Sleep func(ctx context.Context, d time.Duration) error
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

// Acquire attempts branch's lock for owner until it is granted, waiting in the queue with no time limit, and returns the granted record.
// It polls from one second, doubling to thirty seconds while the holder is unchanged and returning to one second when it changes.
// A cancelled context, a pause and an attempt error each return with owner's ticket removed.
// It only waits and takes: it never frees another owner's live record.
func (s *Store) Acquire(ctx context.Context, branch string, owner Owner, w Waiter) (Record, error) {
	var lastHolder Owner
	var delay time.Duration
	for first := true; ; first = false {
		if err := ctx.Err(); err != nil {
			return Record{}, s.abandonTicket(branch, owner, err)
		}
		attempt, err := s.TryAcquire(branch, owner)
		if err != nil {
			return Record{}, s.abandonTicket(branch, owner, err)
		}
		if attempt.Granted {
			record, _, err := s.Read(branch)
			return record, err
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
		if err := w.sleep(ctx, delay); err != nil {
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
