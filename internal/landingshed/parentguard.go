// parentguard.go — the guard Finalize takes and frees the parent lock through, and re-checks the lock with before it lands.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentlock"
)

// parentGuard holds the lock seams, the parent branch and the wait mark one row takes the lock through.
type parentGuard struct {
	lock         ParentLockDeps
	parentBranch string
	waitMark     func(label string, start time.Time) error
	// sleep replaces the queue wait's timer; nil waits on the real one.
	sleep func(ctx context.Context, d time.Duration) error
}

// newParentGuard builds a parentGuard from deps' told values.
func newParentGuard(deps Deps) parentGuard {
	return parentGuard{
		lock:         deps.Lock,
		parentBranch: deps.ParentBranch,
		waitMark:     deps.VerifyWaitMark,
	}
}

// ensure takes the parent lock for the run, waiting in the queue while another run holds it.
// While it waits it keeps the run's wait record and the driver strand's pane mark, and removes both on every way out.
//
// It returns "" once the lock is held, and a non-empty Stuck reason when the operator halted the wait or the lock record is unreadable;
// a caller returns either reason as a Stuck with Halt set.
// A cancellation and every other failure are returned as an error.
// A guard with no lock store takes no lock.
func (g parentGuard) ensure(ctx context.Context, producer string) (string, error) {
	if g.lock.Store == nil {
		return "", nil
	}
	owner, err := g.lock.Owner()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: resolve the parent lock owner: %w", producer, err)
	}

	label := "parent lock " + g.parentBranch
	var waiting bool
	var holderSlug string
	var pauseFailures logger.Episode
	waiter := parentlock.Waiter{
		OnWait: func(holder parentlock.Record) {
			waiting = true
			holderSlug = holder.Owner.Slug
			if err := parentlock.WriteWait(g.lock.WaitPath, parentlock.NewWait(g.parentBranch, holderSlug)); err != nil {
				logger.Warn("landingshed: could not write the parent lock wait record", "cause", err)
			}
			g.markWait(label, time.Now())
		},
		Paused: func() bool { return g.pauseRequested(&pauseFailures) },
		Sleep:  g.sleep,
	}
	_, err = g.lock.Store.Acquire(ctx, g.parentBranch, owner, waiter)
	if waiting {
		if removeErr := parentlock.RemoveWait(g.lock.WaitPath); removeErr != nil {
			logger.Warn("landingshed: could not remove the parent lock wait record", "cause", removeErr)
		}
		g.markWait("", time.Time{})
	}

	switch {
	case err == nil:
		return "", nil
	case errors.Is(err, parentlock.ErrPaused):
		return fmt.Sprintf("halted while waiting for the parent lock on %q held by %s, as \"lym loom pause --after %s\" asked; way forward: run \"lym loom goto --to %s\", then \"lym loom resume\"", g.parentBranch, holderSlug, producer, producer), nil
	case errors.Is(err, parentlock.ErrUnreadable):
		return fmt.Sprintf("the parent lock record of %q is unreadable: %v; way forward: run \"lym batten lock release %s\" from the prime, then \"lym loom resume\" in the task worktree", g.parentBranch, err, g.parentBranch), nil
	default:
		return "", fmt.Errorf("landingshed: %s: take the parent lock on %q: %w", producer, g.parentBranch, err)
	}
}

// release frees the run's own parent lock record, leaving a record another process owns alone.
// A failure is logged and changes no verdict.
func (g parentGuard) release(producer string) {
	if g.lock.Store == nil {
		return
	}
	owner, err := g.lock.Owner()
	if err != nil {
		logger.Warn("landingshed: could not resolve the parent lock owner to release it", "producer", producer, "cause", err)
		return
	}
	if _, err := g.lock.Store.Release(g.parentBranch, owner); err != nil {
		logger.Warn("landingshed: could not release the parent lock", "producer", producer, "branch", g.parentBranch, "cause", err)
	}
}

// stillHeld re-reads the parent lock record just before a landing and returns "" when it still names this run.
// A record that no longer names the run, or none, means something released the lock: an owner pause, another machine's ownership of the run or an operator's `lym batten lock release`.
// An unreadable record is a reason too, as at ensure.
// Both reasons open with LandingStopPrefix, so only the orch's `lym loom resume` lifts the halt they make.
// A guard with no lock store holds no lock and checks none.
// Any other failure is returned as an error.
func (g parentGuard) stillHeld(producer string) (string, error) {
	if g.lock.Store == nil {
		return "", nil
	}
	owner, err := g.lock.Owner()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: resolve the parent lock owner: %w", producer, err)
	}
	record, found, err := g.lock.Store.Read(g.parentBranch)
	switch {
	case errors.Is(err, parentlock.ErrUnreadable):
		return fmt.Sprintf("%sthe parent lock record of %q is unreadable before the landing: %v; nothing was landed; way forward: run \"lym batten lock release %s\" from the prime, then \"lym loom resume\" in the task worktree", LandingStopPrefix, g.parentBranch, err, g.parentBranch), nil
	case err != nil:
		return "", fmt.Errorf("landingshed: %s: re-read the parent lock on %q: %w", producer, g.parentBranch, err)
	case found && record.Owner.SameRun(owner):
		return "", nil
	}
	return fmt.Sprintf("%sthe parent lock on %q no longer names this run, so something released it before the landing: an owner pause, another machine's ownership of the run or an operator's `lym batten lock release`; nothing was landed; way forward: `lym loom resume` once no owner pause stands and the run is this machine's, which after an operator's release holds already", LandingStopPrefix, g.parentBranch), nil
}

// pauseRequested reads the told pause seam, which reports a `--after` naming the waiting row; a read error counts as not paused.
// The wait polls it, so failures is the episode of its read errors: a Warn at the first, Debug while it repeats and an Info at the next good read.
func (g parentGuard) pauseRequested(failures *logger.Episode) bool {
	if g.lock.PauseRequested == nil {
		return false
	}
	paused, err := g.lock.PauseRequested()
	if err != nil {
		failures.Fail("landingshed: could not read the pause flag during the parent lock wait", "cause", err)
		return false
	}
	failures.Recover("landingshed: the pause flag reads again during the parent lock wait")
	return paused
}

// markWait sets the driver strand's pane mark to label, started at start, and clears it when label is empty.
// A failure is logged: the mark is display only.
func (g parentGuard) markWait(label string, start time.Time) {
	if g.waitMark == nil {
		return
	}
	if err := g.waitMark(label, start); err != nil {
		logger.Warn("landingshed: could not set the parent lock wait mark", "label", label, "cause", err)
	}
}
