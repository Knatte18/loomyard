// create.go implements NewWorktreeCreate, the producer that creates the task worktree pair.
// It also defines HoldPrimeLock, the hold a create closure takes on the hub's prime lock for the steps that need it.

package battenshed

import (
	"context"
	"errors"
	"sync"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// PreconditionError is the failure a create closure returns for a precondition no tackler can settle.
// Examples are a dirty source worktree and a task branch left from an earlier pair.
// Worktree-Create turns it into a precondition halt,
// so the run hands back to its parent with the way forward in the halt's reason.
type PreconditionError struct {
	// Unmet states the precondition that does not hold, on one line.
	Unmet string
	// WayForward is the command that settles it.
	WayForward string
	// Err is the cause, returned by Unwrap.
	Err error
}

// Error returns the unmet text followed by the way forward.
func (e *PreconditionError) Error() string {
	return e.Unmet + "; way forward: " + e.WayForward
}

// Unwrap returns Err, so errors.Is and errors.As reach the cause.
func (e *PreconditionError) Unwrap() error {
	return e.Err
}

// HoldPrimeLock takes the hub's prime lock for a create closure, waiting out a contended lock, and returns its release.
// The closure calls it around the steps that write the hub's worktree registry or read the prime's tree.
// The origin probes start before the hold and overlap its wait, but Add waits for them under the hold,
// so the hold covers the probe time left after the pre-flight reads.
// The steps under the hold also fetch when they adopt a live pair.
// Those steps fetch to fast-forward a behind branch of the pair and to replace an archived leftover on origin.
// Calling the release more than once releases the lock once;
// a hold the closure never releases is released when the row returns.
// An error is returned as the closure's own error:
// the row reads it as Stuck when the wait ran out and as a hard error when the lock itself failed.
type HoldPrimeLock func() (release func(), err error)

// PrimeLockBusyError is the error a HoldPrimeLock returns when its wait for a contended prime lock ran out.
// Worktree-Create reads it as Stuck, with Reason as the stuck reason.
type PrimeLockBusyError struct {
	// Reason names the lock path and the wait.
	Reason string
}

// Error returns the stuck reason, which names the lock path and the wait.
func (e *PrimeLockBusyError) Error() string {
	return e.Reason
}

// primeLockFailure is the error a HoldPrimeLock returns when acquiring the lock itself fails.
// It also covers a context that ends during the wait.
// Worktree-Create returns it as a hard error: the lock mechanism failing is no producer verdict.
type primeLockFailure struct {
	err error
}

func (e *primeLockFailure) Error() string {
	return e.err.Error()
}

func (e *primeLockFailure) Unwrap() error {
	return e.err
}

// worktreeCreateProducer creates the task worktree pair, handing its create closure the hold on the hub's prime lock.
type worktreeCreateProducer struct {
	name           string
	slug           string
	createWorktree func(context.Context, HoldPrimeLock) error
	primeLock      PrimeLock
	scratchDir     string
}

var _ shedengine.ShedProducer = (*worktreeCreateProducer)(nil)

// NewWorktreeCreate returns a shedengine.ShedProducer that creates the task worktree pair for slug.
// It hands createWorktree a HoldPrimeLock over primeLock.
//
// name is told rather than hardcoded because a second producer list may name this row independently.
// slug is used only as a log field and in stuck-reason text: it is never compared, parsed, or used for control flow.
func NewWorktreeCreate(name, slug string, createWorktree func(context.Context, HoldPrimeLock) error, primeLock PrimeLock, scratchDir string) shedengine.ShedProducer {
	return &worktreeCreateProducer{
		name:           name,
		slug:           slug,
		createWorktree: createWorktree,
		primeLock:      primeLock,
		scratchDir:     scratchDir,
	}
}

// Call implements shedengine.ShedProducer.
// It calls createWorktree with a hold on the prime lock.
// On every exit path, including every Stuck one, it releases each hold the closure took and did not release.
// The closure takes the lock only for the steps that need it.
// Two rows creating different pairs therefore overlap everywhere else.
//
// A hold waits for a contended lock, polling it at a fixed interval until a fixed bound runs out.
// A context cancelled at any point returns the cancelled-during-run error.
// An Acquire error at any attempt is a returned hard error:
// the lock mechanism itself failed, which is not a producer verdict.
// The bound spent with the lock still held is Stuck, with a reason naming the lock path and the wait.
// It never names a holder identity: the lock file carries no record of one, so it cannot report one.
// Any other createWorktree error is Stuck with that error's own text passed through verbatim:
// fabric's own refusals already name their own remedies, and reworking that text would only drop information.
// A PreconditionError from createWorktree is instead a Halt Stuck:
// its reason starts with shedengine.ReasonPreconditionUnmet.
// That halt blocks the run at this row and spends no bounce budget.
func (p *worktreeCreateProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	var holds []func()
	defer func() {
		for _, release := range holds {
			release()
		}
	}()
	hold := func() (func(), error) {
		release, reason, err := acquirePrimeLock(ctx, p.name, p.slug, p.primeLock)
		if err != nil {
			return nil, &primeLockFailure{err: err}
		}
		if release == nil {
			return nil, &PrimeLockBusyError{Reason: reason}
		}
		once := p.releaseOnce(release)
		holds = append(holds, once)
		return once, nil
	}

	if err := p.createWorktree(ctx, hold); err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		var failure *primeLockFailure
		if errors.As(err, &failure) {
			return "", shedengine.OutputPointer{}, failure.err
		}
		var busy *PrimeLockBusyError
		if errors.As(err, &busy) {
			reportStuck(p.name, busy.Reason, p.scratchDir, "slug", p.slug)
			return shedengine.Stuck, shedengine.OutputPointer{Reason: busy.Reason}, nil
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

// releaseOnce wraps release so it runs once however often it is called, warning when it fails.
func (p *worktreeCreateProducer) releaseOnce(release func() error) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			if err := release(); err != nil {
				logger.Warn("battenshed: release prime lock failed", "producer", p.name, "slug", p.slug, "path", p.primeLock.Path, "error", err)
			}
		})
	}
}
