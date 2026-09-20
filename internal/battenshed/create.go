// create.go implements NewWorktreeCreate, the producer that creates the task worktree under the
// hub's prime lock.

package battenshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

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
// An Acquire error is a returned hard error: the lock mechanism itself failed, which is not a
// producer verdict. Acquire reporting ok == false is Stuck, with a reason naming primeLock.Path
// and the slug -- never a holder identity, which the lock file carries no record of and so cannot
// report. A createWorktree error is Stuck with that error's own text passed through verbatim and
// unreworded: fabric's own refusals (the dirty-driving-worktree probe and the pre-existing-branch
// refusal) already name their own remedies, and reworking that text would only drop information.
func (p *worktreeCreateProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	release, ok, err := p.primeLock.Acquire()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: acquire prime lock %q: %w", p.name, p.primeLock.Path, err)
	}
	if !ok {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		reason := fmt.Sprintf("prime lock %q is already held; another batten producer is creating or tearing down a task worktree", p.primeLock.Path)
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
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
		reportStuck(p.name, err.Error(), p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	}

	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
