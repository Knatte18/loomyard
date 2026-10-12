// parentguard.go — the guard the lock-taking landing rows share: it takes and frees the parent lock and decides whether the parent has moved since Merge-In merged it.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentlock"
)

// parentGuard holds the lock seams, the parent branch, the tip reads and the merge-record path one row takes the lock and checks the parent through.
type parentGuard struct {
	lock            ParentLockDeps
	parentBranch    string
	pushSkipped     bool
	readParentTips  func(fetch bool) (fabricengine.ParentTips, error)
	holdsWork       func(base, source string) (bool, error)
	taskHead        func() (string, error)
	mergeRecordPath string
	waitMark        func(label string, start time.Time) error
	// sleep replaces the queue wait's timer; nil waits on the real one.
	sleep func(ctx context.Context, d time.Duration) error
}

// newParentGuard builds a parentGuard from deps' told values.
func newParentGuard(deps Deps) parentGuard {
	return parentGuard{
		lock:            deps.Lock,
		parentBranch:    deps.ParentBranch,
		pushSkipped:     deps.PushSkipped,
		readParentTips:  deps.ReadParentTips,
		holdsWork:       deps.HoldsWork,
		taskHead:        deps.TaskHead,
		mergeRecordPath: deps.MergeRecordPath,
		waitMark:        deps.VerifyWaitMark,
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
	waiter := parentlock.Waiter{
		OnWait: func(holder parentlock.Record) {
			waiting = true
			holderSlug = holder.Owner.Slug
			if err := parentlock.WriteWait(g.lock.WaitPath, parentlock.NewWait(g.parentBranch, holderSlug)); err != nil {
				logger.Warn("landingshed: could not write the parent lock wait record", "cause", err)
			}
			g.markWait(label, time.Now())
		},
		Paused: g.pauseRequested,
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
		return fmt.Sprintf("halted while waiting for the parent lock on %q held by %s, as \"lyx loom pause --after %s\" asked; way forward: run \"lyx loom goto --to %s\", then \"lyx loom resume\"", g.parentBranch, holderSlug, producer, producer), nil
	case errors.Is(err, parentlock.ErrUnreadable):
		return fmt.Sprintf("the parent lock record of %q is unreadable: %v; way forward: run \"lyx batten lock release %s\" from the prime, then \"lyx loom resume\" in the task worktree", g.parentBranch, err, g.parentBranch), nil
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

// moved decides whether the parent moved since Merge-In merged it, returning the reroute reason or "" when it has not.
// A missing or unreadable merge record counts as moved, so the row reroutes to Merge-In, which writes one.
// The parent has moved when a tip differs from the merge record's merged tip and does not hold the task's work;
// the upstream tip counts only when one exists and the push is not skipped.
// A local parent ahead of or diverged from its upstream therefore reads as moved.
func (g parentGuard) moved(producer string) (string, error) {
	rec, found, err := ReadMergeRecord(g.mergeRecordPath)
	if err != nil {
		logger.Warn("landingshed: merge record unreadable, treating the parent as moved", "producer", producer, "cause", err)
	}
	if err != nil || !found {
		return fmt.Sprintf("no readable merge record from Merge-In for parent branch %q; rerouting to Merge-In", g.parentBranch), nil
	}

	tips, err := g.readParentTips(!g.pushSkipped)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", producer, g.parentBranch, err)
	}
	head, err := g.taskHead()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read the task head: %w", producer, err)
	}

	differs := func(tip string) (bool, error) {
		if tip == "" || tip == rec.MergedTip {
			return false, nil
		}
		holds, err := g.holdsWork(tip, head)
		if err != nil {
			return false, fmt.Errorf("landingshed: %s: test whether %s holds the task's work: %w", producer, tip, err)
		}
		return !holds, nil
	}
	localMoved, err := differs(tips.Local)
	if err != nil {
		return "", err
	}
	upstream := tips.Upstream
	if g.pushSkipped {
		upstream = ""
	}
	upstreamMoved, err := differs(upstream)
	if err != nil {
		return "", err
	}
	if !localMoved && !upstreamMoved {
		return "", nil
	}
	return fmt.Sprintf("parent branch %q moved since Merge-In merged %s (local %s, upstream %s); rerouting to Merge-In", g.parentBranch, rec.MergedTip, tips.Local, tips.Upstream), nil
}

// pauseRequested reads the told pause seam, which reports a `--after` naming the waiting row; a read error counts as not paused and logs one Warn.
func (g parentGuard) pauseRequested() bool {
	if g.lock.PauseRequested == nil {
		return false
	}
	paused, err := g.lock.PauseRequested()
	if err != nil {
		logger.Warn("landingshed: could not read the pause flag during the parent lock wait", "cause", err)
		return false
	}
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
