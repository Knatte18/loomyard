// innerrun_parentlock.go implements InnerRun's two parent lock duties: keeping or releasing the lock of the child's run on every probe check, and resuming a child awaiting at the gate row whose parent moved.

package battenshed

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// lockWatch is the parent lock step's bookkeeping across the checks of one Call.
type lockWatch struct {
	// awaitingSince is the awake reading at which a handed record's child began awaiting without a decision, valid while awaiting is true.
	awaitingSince time.Duration
	awaiting      bool
	// idle counts how long the owner's agents have waited, and idleOwner is the owner it counts for.
	idle      *parentlock.IdleWatch
	idleOwner parentlock.Owner
}

// lockStep acts on the lock record naming the child's run, once per probe check; it does nothing without deps.ParentLock.
// A record handed to this batten (its owner pid is SelfPID) is kept while the child awaits at the gate row and is released once that awaiting has lasted IdleBound with no decision to resume on.
// It is kept while the child runs, so the resumed loop takes it back, and released when the child is in any other state.
// A record owned by another process is released once its owner's agents have all waited for IdleBound with none busy; the watch restarts whenever the owner changes, and a failed agent read releases nothing.
// Every release logs one Warn, and every failure of a seam is a Warn that changes nothing.
func (p *innerRunProducer) lockStep(ctx context.Context, w *childWait) {
	lock := p.deps.ParentLock
	if lock == nil {
		return
	}
	record, found, err := lock.Read()
	if err != nil {
		logger.Warn("battenshed: parent lock record unreadable; leaving it alone", "producer", p.name, "slug", p.slug, "error", err)
		return
	}
	if !found {
		w.lock = lockWatch{}
		return
	}
	if record.Owner.PID == lock.SelfPID {
		p.handedLockStep(w, record)
		return
	}
	p.loopLockStep(w, record)
}

// handedLockStep keeps or releases a record handed to this batten, by the child's state.
func (p *innerRunProducer) handedLockStep(w *childWait, record parentlock.Record) {
	lock := p.deps.ParentLock
	w.lock.idle = nil
	status := w.status
	switch {
	case status.State == shedengine.StateAwaiting && status.CurrentProducer == lock.GateRow:
		pending, err := p.resumePending(w)
		if err != nil {
			logger.Warn("battenshed: decision unreadable for the parent lock step; keeping the handed lock", "producer", p.name, "slug", p.slug, "error", err)
			return
		}
		if pending {
			w.lock.awaiting = false
			return
		}
		awake := p.deps.Awake()
		if !w.lock.awaiting {
			w.lock.awaiting, w.lock.awaitingSince = true, awake
			return
		}
		if awake-w.lock.awaitingSince >= lock.IdleBound {
			p.releaseLock(w, record, fmt.Sprintf("the child awaited at %s for %s with no decision", lock.GateRow, lock.IdleBound))
		}
	case status.State == shedengine.StateRunning:
		w.lock.awaiting = false
	default:
		p.releaseLock(w, record, fmt.Sprintf("the child is %s and holds no resume to take the lock back", status.State))
	}
}

// loopLockStep releases a record owned by a process other than this batten once the idle rule holds for its owner.
func (p *innerRunProducer) loopLockStep(w *childWait, record parentlock.Record) {
	lock := p.deps.ParentLock
	w.lock.awaiting = false
	if w.lock.idle == nil || w.lock.idleOwner != record.Owner {
		w.lock.idle = &parentlock.IdleWatch{Bound: lock.IdleBound}
		w.lock.idleOwner = record.Owner
	}
	states, err := lock.Agents()
	if err != nil {
		logger.Warn("battenshed: the child's agent states are unreadable; keeping its parent lock", "producer", p.name, "slug", p.slug, "error", err)
	}
	if w.lock.idle.Observe(states, err == nil, p.deps.Awake()) {
		p.releaseLock(w, record, fmt.Sprintf("every agent of the lock owner waited for %s with none busy", lock.IdleBound))
	}
}

// releaseLock releases record through deps.ParentLock and logs one Warn when it did.
func (p *innerRunProducer) releaseLock(w *childWait, record parentlock.Record, why string) {
	released, err := p.deps.ParentLock.Release(record)
	if err != nil {
		logger.Warn("battenshed: could not release the child's parent lock", "producer", p.name, "slug", p.slug, "branch", record.Branch, "error", err)
		return
	}
	w.lock = lockWatch{}
	if released {
		logger.Warn("battenshed: released the child's parent lock", "producer", p.name, "slug", p.slug, "branch", record.Branch, "reason", why)
	}
}

// resumePending reports whether the awaiting child has a decision this run is about to resume on or has resumed on without the child's driver stepping since.
// A decision already acted on whose resume the child has answered by awaiting again is not pending, nor is a missing one.
func (p *innerRunProducer) resumePending(w *childWait) (bool, error) {
	decision, found, err := p.deps.ReadDecision()
	if err != nil || !found {
		return false, err
	}
	acted, err := os.ReadFile(decisionActedFile(p.scratchDir, p.name))
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read decision-acted marker: %w", err)
	}
	identity, actedLen, hasLen := parseDecisionActed(string(acted))
	if identity != decisionIdentity(decision) {
		return true, nil
	}
	return hasLen && len(w.status.History) == actedLen, nil
}

// parentMovedUnderGate reports whether the child awaits at the gate row with no decision pending and its parent has moved.
// deps.ParentMoved is read at most once per probe and the answer reused on the checks between; a read error is warned about and reads as not moved.
// It stays true once the stale-base resume landed, until the child leaves awaiting.
func (p *innerRunProducer) parentMovedUnderGate(w *childWait) bool {
	lock := p.deps.ParentLock
	if lock == nil || w.status.State != shedengine.StateAwaiting || w.status.CurrentProducer != lock.GateRow {
		return false
	}
	if w.staleResumed {
		return true
	}
	if !w.baseStale {
		return w.baseMoved
	}
	w.baseStale = false
	w.baseMoved = false
	pending, err := p.resumePending(w)
	if err != nil {
		logger.Warn("battenshed: decision unreadable for the stale-base check; not resuming", "producer", p.name, "slug", p.slug, "error", err)
		return false
	}
	if pending {
		return false
	}
	moved, err := lock.ParentMoved()
	if err != nil {
		logger.Warn("battenshed: could not tell whether the parent moved; not resuming", "producer", p.name, "slug", p.slug, "error", err)
		return false
	}
	w.baseMoved = moved
	return moved
}

// staleBaseResume resumes a child awaiting at the gate row whose parent moved, so the gate's next call reroutes it to Merge-In under the lock.
// It resumes as it does after a decision: a refusal as not parked or as a busy pane is retried at most once per noticeProbe, and the awaiting notice is suppressed.
// It only resumes: whether to reroute is the gate's decision.
func (p *innerRunProducer) staleBaseResume(ctx context.Context, w *childWait) *waitEnd {
	if w.staleResumed || !p.parentMovedUnderGate(w) {
		return nil
	}
	resumed, end := p.resumeAwaitingChild(ctx, w, "the parent's move")
	if end != nil || !resumed {
		return end
	}
	w.staleResumed = true
	p.report(w, fmt.Sprintf("the parent moved while the child awaited at %s: the child was resumed to merge it in; watching it", p.deps.ParentLock.GateRow))
	return nil
}
