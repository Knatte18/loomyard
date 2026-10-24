// innerrun_parentlock.go implements InnerRun's parent lock duty: releasing the lock of the child's run on every probe check once its owner has idled.

package battenshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentlock"
)

// lockWatch is the parent lock step's bookkeeping across the checks of one Call.
type lockWatch struct {
	// idle counts how long the owner's agents have waited, and idleOwner is the owner it counts for.
	idle      *parentlock.IdleWatch
	idleOwner parentlock.Owner
}

// lockStep acts on the lock record naming the child's run, once per probe check; it does nothing without deps.ParentLock.
// A record is released once its owner's agents have all waited for IdleBound with none busy; the watch restarts whenever the owner changes, and a failed agent read releases nothing.
// Every release logs one Info, and a failure of a seam warns once per episode and changes nothing.
// While a pause covers the slug it releases nothing, except the record of a pause held for an owner.
func (p *innerRunProducer) lockStep(ctx context.Context, w *childWait) {
	lock := p.deps.ParentLock
	if lock == nil {
		return
	}
	// A covered holder keeps its record, and the idle watch restarts after the resume so paused time never counts as waiting.
	// A pause held for an owner releases the record instead, unless a Finalize past its parent-side merge keeps it.
	if w.pause.Covered {
		if w.pause.OwnerHeld && !p.keepsLockForFinalize(w.status) {
			p.releaseChildLock(w, "an owner pause holds the run")
		}
		w.lock = lockWatch{}
		return
	}
	record, found, err := lock.Read()
	if err != nil {
		p.seamFailed(failLockRecordUnreadable, err)
		return
	}
	p.seamRecovered(failLockRecordUnreadable)
	if !found {
		w.lock = lockWatch{}
		return
	}
	p.loopLockStep(w, record)
}

// failKeepsLockRead is the failure message, and episode key, of the read that tells whether a landing holds the lock.
const failKeepsLockRead = "battenshed: could not tell whether a landing holds the child's parent lock; keeping it"

// loopLockStep releases a record once the idle rule holds for its owner.
// It releases nothing while deps.ParentLock.KeepsLock answers true, and restarts the idle watch; a failed read keeps the lock and warns once per episode.
func (p *innerRunProducer) loopLockStep(w *childWait, record parentlock.Record) {
	lock := p.deps.ParentLock
	if lock.KeepsLock != nil {
		keeps, err := lock.KeepsLock()
		if err != nil {
			p.seamFailed(failKeepsLockRead, err)
			w.lock.idle = nil
			return
		}
		p.seamRecovered(failKeepsLockRead)
		if keeps {
			w.lock.idle = nil
			return
		}
	}
	if w.lock.idle == nil || w.lock.idleOwner != record.Owner {
		w.lock.idle = &parentlock.IdleWatch{Bound: lock.IdleBound}
		w.lock.idleOwner = record.Owner
	}
	states, err := lock.Agents()
	if err != nil {
		p.seamFailed(failAgentStatesUnreadable, err)
	} else {
		p.seamRecovered(failAgentStatesUnreadable)
	}
	if w.lock.idle.Observe(states, err == nil, p.deps.Awake()) {
		p.releaseLock(w, record, fmt.Sprintf("every agent of the lock owner waited for %s with none busy", lock.IdleBound))
	}
}

// releaseLock releases record through deps.ParentLock and logs one Info when it did.
func (p *innerRunProducer) releaseLock(w *childWait, record parentlock.Record, why string) {
	released, err := p.deps.ParentLock.Release(record)
	if err != nil {
		p.seamFailed(failLockRelease, err, "branch", record.Branch)
		return
	}
	p.seamRecovered(failLockRelease)
	w.lock = lockWatch{}
	if released {
		logger.Info("battenshed: released the child's parent lock", "producer", p.name, "slug", p.slug, "branch", record.Branch, "reason", why)
	}
}
