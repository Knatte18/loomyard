// catchupwait.go is the dobby's use of the catch-up from origin:
// when it owes one, the wait it records while one stands unfinished, and the sync of its runs' bolt-hold entries.

package dobbyengine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/runfsm"
)

// The reasons a catch-up waits, as a CatchUpWait records them.
const (
	// CatchUpWaitOrigin means origin did not answer.
	CatchUpWaitOrigin = "origin"
	// CatchUpWaitRefused means this machine's registration was refused.
	CatchUpWaitRefused = "refused"
	// CatchUpWaitFailed means any other failure: a Bolt pull skipped, or a run's push or hand-over failed.
	CatchUpWaitFailed = "failed"
)

// CatchUpWait is the catch-up from origin a dobby waits on, as its state record keeps it.
// The record carries none while no catch-up is owed or the last one succeeded.
type CatchUpWait struct {
	// Reason is why the catch-up waits: CatchUpWaitOrigin, CatchUpWaitRefused or CatchUpWaitFailed.
	Reason string `json:"reason"`
	// Detail is the failure's text, folded onto one line.
	Detail string `json:"detail,omitempty"`
	// At is when the failure was read.
	At time.Time `json:"at"`
	// Since is when the catch-up first failed with this reason in a row, so how long it has waited.
	Since time.Time `json:"since,omitzero"`
}

// waitingSince returns when the wait began, At for a record written before Since was kept.
func (w CatchUpWait) waitingSince() time.Time {
	if w.Since.IsZero() {
		return w.At
	}
	return w.Since
}

// oweCatchUp marks the catch-up from origin owed.
// A dobby whose Deps.CatchUp has no seams owes none.
func (d *daemon) oweCatchUp() {
	if d.deps.CatchUp.PullBolt != nil {
		d.catchUpOwed = true
	}
}

// catchUp runs the owed catch-up from origin when its next try is due, and reports whether it stands owed after the call.
// Success clears the wait, resets the delay and owes every listed run a full pass.
// It also drops the per-run memory of the runs the catch-up touched.
// Failure keeps the catch-up owed and schedules the next try one delay later, doubling from a second to idleCeiling.
// It records the wait in the state record, and a refusal or any other failure posts one notice per episode.
func (d *daemon) catchUp(ctx context.Context, now time.Time) bool {
	if !d.catchUpOwed {
		return false
	}
	if now.Before(d.catchUpNext) {
		return true
	}
	report, err := CatchUp(d.deps.CatchUp)
	if err != nil {
		d.catchUpFailed(ctx, now, err)
		return true
	}
	d.catchUpSucceeded(now, report)
	return false
}

// catchUpFailed records the failed catch-up as the wait of its class and schedules the next try.
func (d *daemon) catchUpFailed(ctx context.Context, now time.Time, err error) {
	reason := catchUpWaitReason(err)
	detail := oneLine(err.Error())
	d.catchUpDelay = nextCatchUpDelay(d.catchUpDelay)
	d.catchUpNext = now.Add(d.catchUpDelay)
	if updateErr := UpdateState(d.geom.StatePath, func(s *State) error {
		since := now
		if s.CatchUp != nil && s.CatchUp.Reason == reason {
			since = s.CatchUp.waitingSince()
		}
		s.CatchUp = &CatchUpWait{Reason: reason, Detail: detail, At: now, Since: since}
		return nil
	}); updateErr != nil {
		logger.Warn("dobbyengine: the catch-up wait could not be recorded", "worktree", d.geom.WorktreeName, "error", updateErr)
	}
	if d.catchUpReason != reason {
		d.catchUpReason = reason
		d.logLine(now, "catch-up "+reason+": "+detail)
	}
	if reason == CatchUpWaitOrigin || d.catchUpNoticed[reason] {
		return
	}
	if d.catchUpNoticed == nil {
		d.catchUpNoticed = map[string]bool{}
	}
	d.catchUpNoticed[reason] = true
	if reason == CatchUpWaitRefused {
		d.post(ctx, fmt.Sprintf("%s: this machine's registration is refused, so the dobby schedules nothing: %v; a refusal that names an environment variable also needs lym beam stop, then lym beam start, since the dobby keeps the environment it started with", d.geom.WorktreeName, err))
		return
	}
	d.post(ctx, fmt.Sprintf("%s: the dobby's catch-up from origin failed, so it schedules nothing: %v; way forward: fix the cause the detail names, and the dobby retries on its own", d.geom.WorktreeName, err))
}

// catchUpSucceeded clears the wait after a catch-up that succeeded and owes every run a full pass.
// The per-run memory of each run the catch-up touched is dropped, so the run's next pass is a full pass.
func (d *daemon) catchUpSucceeded(now time.Time, report CatchUpReport) {
	d.catchUpOwed, d.catchUpDelay, d.catchUpNext, d.catchUpNoticed = false, 0, time.Time{}, nil
	d.catchUpSuccesses++
	if d.catchUpReason != "" {
		d.catchUpReason = ""
		d.logLine(now, "catch-up succeeds again")
	}
	if err := UpdateState(d.geom.StatePath, func(s *State) error {
		s.CatchUp = nil
		return nil
	}); err != nil {
		logger.Warn("dobbyengine: the catch-up wait could not be cleared", "worktree", d.geom.WorktreeName, "error", err)
	}
	d.owesAll = true
	for _, slugs := range [][]string{report.Pushed, report.HandedOver, report.OwnedHere} {
		for _, slug := range slugs {
			delete(d.childSeen, slug)
			delete(d.liftPending, slug)
			delete(d.wasCovered, slug)
		}
	}
}

// catchUpWaitReason classifies a catch-up's error: an unreachable origin, a registration refusal, or any other failure.
func catchUpWaitReason(err error) string {
	var registration *boardengine.RegistrationError
	switch {
	case errors.Is(err, ErrOriginUnreachable):
		return CatchUpWaitOrigin
	case errors.As(err, &registration):
		return CatchUpWaitRefused
	default:
		return CatchUpWaitFailed
	}
}

// nextCatchUpDelay returns the delay after a failed try that followed previous: a second, then doubling to idleCeiling.
func nextCatchUpDelay(previous time.Duration) time.Duration {
	if previous == 0 {
		return wakeFloor
	}
	return min(previous*2, idleCeiling)
}

// oweHandOverCatchUp owes the catch-up for each listed run whose verdict hands it over while its pair is still here.
// A run another machine took while its pass was in flight here is handed over that way, and its processes end.
// A run that owed the catch-up through this trigger owes none from it again while it still lists as handed over,
// so a pair directory the hand-over left standing owes the catch-up once.
// The memory forgets a run once it stops answering hand over or is no longer listed.
func (d *daemon) oweHandOverCatchUp(runs []RunView) {
	if d.handedOver == nil {
		d.handedOver = map[string]bool{}
	}
	answering := make(map[string]bool, len(runs))
	for _, run := range runs {
		if run.Verdict.Answer != runfsm.AnswerHandOver {
			continue
		}
		answering[run.Slug] = true
		if run.PairPresent && !d.handedOver[run.Slug] {
			d.handedOver[run.Slug] = true
			d.oweCatchUp()
		}
	}
	for slug := range d.handedOver {
		if !answering[slug] {
			delete(d.handedOver, slug)
		}
	}
}

// syncBoltHold hands Deps.SyncBoltHold the slugs this dobby's catch-up covers and the listed runs the hold holds.
// The scope is every slug the catch-up's Runs gives.
// So a run the tick did not list still has its entry dropped once the hold clears.
// The held list is the listed runs whose verdict reason is held:
// a run whose push is owed reads another reason and gets no entry, so its push passes the gate.
// A failed sync is logged once per episode and the tick goes on.
func (d *daemon) syncBoltHold(ctx context.Context) {
	if d.deps.SyncBoltHold == nil {
		return
	}
	scope, err := d.boltHoldScope()
	if err == nil {
		var held []string
		for _, run := range d.listedRuns {
			if run.Verdict.Reason == runfsm.ReasonHeld {
				held = append(held, run.Slug)
			}
		}
		err = d.deps.SyncBoltHold(ctx, scope, held)
	}
	now := d.deps.Now()
	if err != nil {
		d.syncFailures.Fail("dobbyengine: the bolt-hold sync failed; the tick goes on", "worktree", d.geom.WorktreeName, "error", err)
		if !d.syncFailing {
			d.syncFailing = true
			d.logLine(now, "bolt hold sync failing: "+oneLine(err.Error()))
		}
		return
	}
	d.syncFailures.Recover("dobbyengine: the bolt-hold sync succeeds again", "worktree", d.geom.WorktreeName)
	if d.syncFailing {
		d.syncFailing = false
		d.logLine(now, "bolt hold sync succeeds again")
	}
}

// boltHoldScope returns the slugs the catch-up covers, or the listed runs when it names none.
func (d *daemon) boltHoldScope() ([]string, error) {
	if d.deps.CatchUp.Runs != nil {
		return d.deps.CatchUp.Runs()
	}
	scope := make([]string, 0, len(d.listedRuns))
	for _, run := range d.listedRuns {
		scope = append(scope, run.Slug)
	}
	return scope, nil
}
