// create.go implements NewWorktreeCreate, the producer that creates the task worktree under the
// hub's prime lock.

package battenshed

import (
	"context"
	"errors"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// PreconditionError is the failure a create closure returns for a precondition no tackler can settle, such as a dirty source worktree or a task branch left from an earlier pair.
// Worktree-Create turns it into a precondition halt, so the run hands back to its parent with the way forward in the halt's reason.
type PreconditionError struct {
	// Unmet is what is unmet, on one line.
	Unmet string
	// WayForward is the command that settles it.
	WayForward string
	// Err is the cause, returned by Unwrap.
	Err error
}

// Error reads the unmet text followed by its way forward.
func (e *PreconditionError) Error() string {
	return e.Unmet + "; way forward: " + e.WayForward
}

// Unwrap returns the cause.
func (e *PreconditionError) Unwrap() error {
	return e.Err
}

// worktreeCreateProducer creates the task worktree pair, holding the hub's prime lock for the
// duration of the create call.
type worktreeCreateProducer struct {
	name           string
	slug           string
	createWorktree func(context.Context) error
	primeLock      PrimeLock
	scratchDir     string
}

var _ shedengine.ShedProducer = (*worktreeCreateProducer)(nil)

// NewWorktreeCreate returns a shedengine.ShedProducer that acquires primeLock, then calls
// createWorktree to create the task worktree pair for slug.
//
// name is told rather than hardcoded because a second producer list may name this row
// independently. slug is used only as a log field and in stuck-reason text -- never compared,
// parsed, or used for control flow.
func NewWorktreeCreate(name, slug string, createWorktree func(context.Context) error, primeLock PrimeLock, scratchDir string) shedengine.ShedProducer {
	return &worktreeCreateProducer{
		name:           name,
		slug:           slug,
		createWorktree: createWorktree,
		primeLock:      primeLock,
		scratchDir:     scratchDir,
	}
}

// Call implements shedengine.ShedProducer. It acquires the prime lock, calls createWorktree while
// holding it, and releases it on every exit path -- including every Stuck one -- before returning.
//
// A contended lock is waited for, polling every primeLockPollInterval for up to primeLockWaitBound (see acquirePrimeLock), so overlapping batten runs take turns.
// An Acquire error at any attempt is a returned hard error: the lock mechanism itself failed, which is not a producer verdict.
// A context cancelled during the wait returns the cancelled-during-run error.
// The bound spent with the lock still held is Stuck, with a reason naming primeLock.Path and the wait -- never a holder identity, which the lock file carries no record of and so cannot report.
// A createWorktree error is Stuck with that error's own text passed through verbatim and unreworded:
// fabric's own refusals already name their own remedies,
// and reworking that text would only drop information.
// A createWorktree error that is a PreconditionError is instead a Halt Stuck whose reason starts with shedengine.ReasonPreconditionUnmet.
// That halt blocks the run at this row and spends no bounce budget.
func (p *worktreeCreateProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	release, reason, err := acquirePrimeLock(ctx, p.name, p.slug, p.primeLock)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if release == nil {
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	}
	defer func() {
		if rerr := release(); rerr != nil {
			logger.Warn("battenshed: release prime lock failed", "producer", p.name, "slug", p.slug, "path", p.primeLock.Path, "error", rerr)
		}
	}()

	if err := p.createWorktree(ctx); err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		var precondition *PreconditionError
		if errors.As(err, &precondition) {
			reason := shedengine.ReasonPreconditionUnmet + ": " + err.Error()
			reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
			return shedengine.Stuck, shedengine.OutputPointer{Halt: true, Reason: reason}, nil
		}
		reportStuck(p.name, err.Error(), p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: err.Error()}, nil
	}

	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
