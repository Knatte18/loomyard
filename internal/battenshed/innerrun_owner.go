// innerrun_owner.go implements InnerRun's owner check: after a pause hold, or while an owner stands, it asks whether another machine owns the run now,
// and a run found owned elsewhere is paused, its parent lock released and the orch noticed, unless a Finalize past its parent-side merge still holds the lock.

package battenshed

import (
	"context"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// finalizeRow is the child recipe row that lands the task's work on its parent.
const finalizeRow = "Finalize"

// The failure messages of the owner seams, each the key of its episode.
const (
	failOwnerCheck        = "battenshed: owner check failed; retrying at the next check"
	failOwnerPause        = "battenshed: could not pause the run owned elsewhere; retrying at the next probe"
	failOwnerFinalizeRead = "battenshed: could not tell whether the parent holds the task's work; keeping the parent lock"
)

// OwnerVerdict is what an owner check reads: the machine that owns the run when it is not this one, or that origin could not be reached to tell.
type OwnerVerdict struct {
	// Owner names the machine that owns the run; set when Elsewhere is true.
	Owner string
	// AdoptSubject is the beam or `root` that `lym adopt` takes to move the run; set when Elsewhere is true, and empty when the verdict brought none.
	AdoptSubject string
	// Elsewhere is true when another machine owns the run.
	Elsewhere bool
	// Unreachable is true when origin could not be reached, so the verdict is neither owned here nor elsewhere.
	Unreachable bool
}

// ChildOwnedElsewhereError is the error an InnerRunDeps.Spawn closure returns when the child's bootstrap refused because another machine owns the run.
// InnerRun reads it as an owned-elsewhere verdict and never as a failed spawn.
type ChildOwnedElsewhereError struct {
	// Owner names the machine the refusal named.
	Owner string
}

// Error implements error.
func (e *ChildOwnedElsewhereError) Error() string {
	return fmt.Sprintf("battenshed: the run is owned by machine %s", e.Owner)
}

// spawnOwnedElsewhere returns the owner a spawn error names and true when the error carries a ChildOwnedElsewhereError.
func spawnOwnedElsewhere(err error) (string, bool) {
	var owned *ChildOwnedElsewhereError
	if errors.As(err, &owned) {
		return owned.Owner, true
	}
	return "", false
}

// ownerHoldReason is the wait reason of a run owned by another machine.
func (p *innerRunProducer) ownerHoldReason() string {
	return fmt.Sprintf("held: the run is owned by machine %s; take it over with \"lym adopt %s\"", p.ownedBy, p.adoptSubject())
}

// adoptSubject returns what `lym adopt` takes to move the run: the subject the owner verdict brought, the slug when it brought none.
func (p *innerRunProducer) adoptSubject() string {
	if p.ownedSubject == "" {
		return p.slug
	}
	return p.ownedSubject
}

// foldOwnerTrigger folds this check's pause coverage into the owner check's trigger.
// The check becomes due in two cases.
// The previous check was covered and this one is not.
// Or this is a probe check while an owner stands that no pause covers.
// A machine wake reaches the owner verdict through the full pass the scheduling dobby owes, not through a trigger here.
func (p *innerRunProducer) foldOwnerTrigger(w *childWait) {
	released := p.ownerCovered && !w.pause.Covered
	retry := p.ownedBy != "" && !w.pause.Covered && w.probeDue
	p.ownerCovered = w.pause.Covered
	if p.deps.OwnerCheck != nil && (released || retry) {
		p.ownerDue = true
	}
}

// ownerStep runs the owner check when it is due.
// An unreachable verdict and a check error keep it due for the next check and hold nothing.
// An owned-here verdict clears the owner the producer stands on, and an owned-elsewhere verdict acts on it.
func (p *innerRunProducer) ownerStep(ctx context.Context, w *childWait) {
	if !p.ownerDue || p.deps.OwnerCheck == nil {
		return
	}
	verdict, err := p.deps.OwnerCheck(ctx)
	if err != nil {
		if ctx.Err() == nil {
			p.seamFailed(failOwnerCheck, err)
		}
		return
	}
	p.seamRecovered(failOwnerCheck)
	switch {
	case verdict.Unreachable:
	case verdict.Elsewhere:
		p.ownerDue = false
		p.ownedElsewhere(ctx, w, verdict.Owner, verdict.AdoptSubject)
	default:
		p.ownerDue = false
		p.ownedBy, p.ownedSubject, p.ownerNoticed = "", "", false
	}
}

// ownedElsewhere records owner as the run's owner and subject as what `lym adopt` takes to move it, and releases the parent lock record of the child's run, unless a Finalize past its parent-side merge keeps it.
// It then pauses the slug under the owner caller and sends one notice per episode naming the owner and the adopt command, which names the slug when subject is empty.
// A failed pause warns once per episode and is retried at the next probe while the owner stands.
func (p *innerRunProducer) ownedElsewhere(ctx context.Context, w *childWait, owner, subject string) {
	p.ownedBy, p.ownedSubject = owner, subject
	if !p.keepsLockForFinalize(w.status) {
		p.releaseChildLock(w, fmt.Sprintf("the run is owned by machine %s", owner))
	}
	if err := p.deps.PauseOwned(ctx, owner); err != nil {
		if ctx.Err() == nil {
			p.seamFailed(failOwnerPause, err, "owner", owner)
		}
	} else {
		p.seamRecovered(failOwnerPause)
	}
	p.noticeOwnedElsewhere(ctx, owner)
	p.reportState(w, p.ownerHoldReason())
}

// noticeOwnedElsewhere sends the one notice of the owner episode, and does nothing once it is posted or when no Notify was wired.
func (p *innerRunProducer) noticeOwnedElsewhere(ctx context.Context, owner string) {
	if !p.notices || p.ownerNoticed {
		return
	}
	if p.episode.deliveries == nil {
		p.episode.deliveries = map[string]*noticeDelivery{}
	}
	condition := "owned by machine " + owner
	line := fmt.Sprintf("%s %s: run paused; %s; take it over with \"lym adopt %s\"", noticePrefix, p.slug, condition, p.adoptSubject())
	if p.deliver(ctx, "owner "+owner, condition, line) {
		p.ownerNoticed = true
	}
}

// releaseChildLock releases the parent lock record naming the child's run, if there is one.
func (p *innerRunProducer) releaseChildLock(w *childWait, why string) {
	lock := p.deps.ParentLock
	if lock == nil {
		return
	}
	record, found, err := lock.Read()
	if err != nil {
		p.seamFailed(failLockRecordUnreadable, err)
		return
	}
	p.seamRecovered(failLockRecordUnreadable)
	if found {
		p.releaseLock(w, record, why)
	}
}

// keepsLockForFinalize reports whether status is a Finalize already past its parent-side merge, which keeps its parent lock and finishes its landing.
// That is a Finalize running, or failed on a transient cause with no re-step recorded for that stop, while the parent holds the task's work.
// A failed read of either seam holds the lock until the next check, and any other state at Finalize does not hold it.
func (p *innerRunProducer) keepsLockForFinalize(status shedengine.Status) bool {
	holds, err := FinalizePastMerge(status, p.deps.Restepped, p.deps.ParentHoldsTaskWork)
	if err != nil {
		p.seamFailed(failOwnerFinalizeRead, err)
		return true
	}
	if status.CurrentProducer == finalizeRow {
		p.seamRecovered(failOwnerFinalizeRead)
	}
	return holds
}

// FinalizePastMerge reports whether status is a Finalize already past its parent-side merge, which finishes its landing.
// That is a Finalize running, or failed on a transient cause with no re-step recorded for that stop, while the parent holds the task's work.
// restepped reads the re-step record of the stop status names and parentHolds reads whether the parent holds the task's work; the error of either is returned.
func FinalizePastMerge(status shedengine.Status, restepped func(shedengine.Status) (bool, error), parentHolds func() (bool, error)) (bool, error) {
	if status.CurrentProducer != finalizeRow {
		return false, nil
	}
	switch {
	case status.State == shedengine.StateRunning:
	case status.State == shedengine.StateFailed && status.Transient != "":
		stepped, err := restepped(status)
		if err != nil {
			return false, err
		}
		if stepped {
			return false, nil
		}
	default:
		return false, nil
	}
	return parentHolds()
}
