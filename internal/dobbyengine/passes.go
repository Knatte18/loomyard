// passes.go decides which batten passes dobby runs for the runs of its worktree, spawns them, and reads how each one exited.

package dobbyengine

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// runStateRunning is the batten status state of a run that has not halted, paused, failed or finished.
	runStateRunning = "running"
	// producerRunShed is the batten producer that waits on the child run, the only row a continue pass may re-enter.
	producerRunShed = "Run-Shed"
	// failingPassNoticeAfter is how many failures in a row with one text post the notice.
	failingPassNoticeAfter = 3
)

// RunView is what dobby reads of one batten run it drives.
type RunView struct {
	// Slug is the run's slug.
	Slug string
	// State is batten's status state and Producer its current producer.
	State    string
	Producer string
	// StatusModified is the modification time of batten's own status file and ChildStatusModified that of the child's status file.
	StatusModified      time.Time
	ChildStatusModified time.Time
	// CancelMarker reports whether the slug's cancel marker exists.
	CancelMarker bool
	// DriverLockHeld reports whether the run's driver lock is held, which a pass in flight does.
	DriverLockHeld bool
	// OriginOwed reports whether the run's origin-owed marker stands.
	OriginOwed bool
	// Covered reports whether a pause entry covers the hub or the slug.
	Covered bool
}

// PassExit is how a `lyx batten run` pass ended: its exit code and its last error text.
type PassExit struct {
	Code  int
	Error string
}

// passKind is the pass planPass decides on.
type passKind int

const (
	// passNone is no pass.
	passNone passKind = iota
	// passContinue is a continue pass, which re-enters the run at Run-Shed.
	passContinue
	// passFull is a full pass.
	passFull
	// passFullNoReopen is a full pass that leaves the origin reconcile owed.
	passFullNoReopen
)

// passTrigger is what woke the planner for one run.
type passTrigger struct {
	// ProbeTick is a probe tick, once per probe interval.
	ProbeTick bool
	// ChildWrite is a write of the child's status file.
	ChildWrite bool
	// Start is the dobby's start, or a machine sleep.
	Start bool
	// Lift is the lift of a pause entry that covered the run.
	Lift bool
	// InFlight reports a pass of the run this dobby spawned that has not exited.
	InFlight bool
}

// planPass decides, for one run and trigger, none, a continue pass or a full pass.
// There is none unless batten's state is running, none while the cancel marker exists or a pass is in flight, and a held driver lock reads as in flight.
// A write of batten's own status file triggers only when it is later than the run's last pass exit, so a pass's own writes never trigger the next.
// While a pause covers the run, only a continue pass runs, and the full pass at the lift; an owed full pass waits for the lift.
// A continue pass needs the run at Run-Shed with no full pass owed and no lift to answer, and every other pass is full, with no reopen while the origin-owed marker stands.
// A run recorded owned elsewhere gets a full pass at the lift of its covering pause, and while no pause covers it a full pass on each probe tick; no other trigger gives it one.
func planPass(run RunView, rs RunState, trigger passTrigger, covered bool) passKind {
	if run.State != runStateRunning || run.CancelMarker || trigger.InFlight || run.DriverLockHeld {
		return passNone
	}
	full := passFull
	if run.OriginOwed {
		full = passFullNoReopen
	}
	if rs.OwnedElsewhere {
		if trigger.Lift || (trigger.ProbeTick && !covered) {
			return full
		}
		return passNone
	}
	battenWrite := run.StatusModified.After(rs.LastPassExit)
	if !(trigger.ProbeTick || trigger.ChildWrite || trigger.Start || trigger.Lift || battenWrite) {
		return passNone
	}
	if covered {
		switch {
		case trigger.Lift:
			return full
		case rs.FullPassOwed || run.Producer != producerRunShed:
			return passNone
		}
		return passContinue
	}
	if rs.FullPassOwed || run.Producer != producerRunShed || run.OriginOwed || trigger.Lift {
		return full
	}
	return passContinue
}

// flight is a pass dobby spawned that has not exited.
type flight struct {
	exit <-chan PassExit
	full bool
	// first marks the first pass of a task dobby started from the board, which has no run yet, and fingerprint is the board entry it read.
	first       bool
	fingerprint string
}

// drivePasses reads exited passes, owes full passes, and spawns the pass each listed run's trigger calls for.
// It leaves d.busy set while any run is in flight or could be given a pass on the next probe tick.
func (d *daemon) drivePasses(ctx context.Context, now time.Time) error {
	d.tickCtx = ctx
	d.collectExits(now)
	runs, err := d.deps.Runs()
	if err != nil {
		return fmt.Errorf("list the runs: %w", err)
	}
	d.runs = runs
	if err := d.recordOwed(runs); err != nil {
		return err
	}
	current, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}

	d.busy = false
	for _, run := range runs {
		rs := current.Runs[run.Slug]
		if d.probeDue && rs.OwnedElsewhere && !rs.OwnerPauseFailed {
			d.recheckOwnerPause(ctx, run.Slug)
		}
		trigger := d.triggerFor(run)
		kind := planPass(run, rs, trigger, run.Covered)
		if !trigger.InFlight && (run.State != runStateRunning || run.CancelMarker) {
			// No pass answers the lift while the run cannot be given one, and a later pass must not inherit it.
			delete(d.liftPending, run.Slug)
		}
		switch {
		case trigger.InFlight:
			d.busy = true
		case kind != passNone:
			d.busy = true
			d.spawn(run.Slug, kind, now)
		case planPass(run, rs, passTrigger{ProbeTick: true}, run.Covered) != passNone:
			d.busy = true
		}
	}
	listed := make(map[string]bool, len(runs))
	for _, run := range runs {
		listed[run.Slug] = true
	}
	for slug := range d.liftPending {
		if !listed[slug] {
			delete(d.liftPending, slug)
		}
	}
	return nil
}

// recordOwed owes every listed run a full pass while the dobby owes all, and drops the records of runs no longer listed.
func (d *daemon) recordOwed(runs []RunView) error {
	return UpdateState(d.geom.StatePath, func(s *State) error {
		listed := make(map[string]bool, len(runs))
		for _, run := range runs {
			listed[run.Slug] = true
			if d.owesAll {
				rs := s.Runs[run.Slug]
				rs.FullPassOwed = true
				if s.Runs == nil {
					s.Runs = map[string]RunState{}
				}
				s.Runs[run.Slug] = rs
			}
		}
		for slug := range s.Runs {
			if !listed[slug] {
				delete(s.Runs, slug)
			}
		}
		return nil
	})
}

// triggerFor reads what woke the planner for run and updates the memory the next tick compares against.
// A child write that arrives while a pass is in flight is kept for the tick after the pass exits.
func (d *daemon) triggerFor(run RunView) passTrigger {
	_, inFlight := d.flights[run.Slug]
	trigger := passTrigger{ProbeTick: d.probeDue, Start: d.owesAll, InFlight: inFlight}
	if !inFlight {
		seen, known := d.childSeen[run.Slug]
		trigger.ChildWrite = known && run.ChildStatusModified.After(seen)
		d.childSeen[run.Slug] = run.ChildStatusModified
	}
	if d.wasCovered[run.Slug] && !run.Covered {
		d.liftPending[run.Slug] = true
	}
	d.wasCovered[run.Slug] = run.Covered
	trigger.Lift = d.liftPending[run.Slug]
	return trigger
}

// spawn starts the pass kind names for slug.
// A spawn that fails is a failed pass with the spawn error as its text.
func (d *daemon) spawn(slug string, kind passKind, now time.Time) {
	full := kind != passContinue
	exit, err := d.deps.SpawnPass(slug, !full, kind == passFullNoReopen)
	delete(d.liftPending, slug)
	if err != nil {
		d.passExited(slug, full, 1, err.Error(), now)
		return
	}
	d.flights[slug] = flight{exit: exit, full: full}
}

// collectExits reads the exit of every pass that has ended, without waiting for one that has not.
func (d *daemon) collectExits(now time.Time) {
	for slug, f := range d.flights {
		select {
		case exit, ok := <-f.exit:
			delete(d.flights, slug)
			if !ok {
				exit = PassExit{Code: 1, Error: "the pass ended without reporting an exit"}
			}
			if f.first {
				d.firstPassExited(slug, f, exit, now)
				continue
			}
			d.passExited(slug, f.full, exit.Code, exit.Error, now)
		default:
		}
	}
}

// passExited reads how a pass of slug exited.
// Exit 0 clears the run's failing-pass count and noticed texts, and its owed full pass when the pass was full and no origin-owed marker stands.
// A refused continue pass owes a full pass, and a covered pass counts as no failure.
// The owned-elsewhere exit is never a failing pass.
// Any other exit counts a failure by its error text, and the third in a row with one text posts one notice to the nearest live orch.
func (d *daemon) passExited(slug string, full bool, code int, errText string, at time.Time) {
	originOwed := false
	if code == 0 && full {
		originOwed = d.originOwed(slug)
	}
	noticeDue := false
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		rs := s.Runs[slug]
		rs.LastPassExit = at
		switch code {
		case 0:
			rs.FailingPassError, rs.FailingPassCount, rs.Noticed = "", 0, nil
			if full && !originOwed {
				rs.FullPassOwed = false
			}
			if full {
				rs.OwnedElsewhere, rs.OwnerNoticeSent, rs.OwnerPauseFailed = false, false, false
			}
		case PassExitContinueRefused:
			rs.FullPassOwed = true
		case PassExitCovered, PassExitOwnedElsewhere:
		default:
			if errText == rs.FailingPassError {
				rs.FailingPassCount++
			} else {
				rs.FailingPassError, rs.FailingPassCount = errText, 1
			}
			noticeDue = rs.FailingPassCount >= failingPassNoticeAfter && !slices.Contains(rs.Noticed, errText)
		}
		if s.Runs == nil {
			s.Runs = map[string]RunState{}
		}
		s.Runs[slug] = rs
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the pass exit could not be recorded", "slug", slug, "error", err)
		return
	}
	if noticeDue {
		d.noticeFailingPass(slug, errText)
	}
	if code == PassExitOwnedElsewhere {
		d.ownedElsewhere(d.tickCtx, slug, at)
	}
}

// originOwed reads whether slug's origin-owed marker stands now, the pass having just exited.
// A run no longer listed has none.
func (d *daemon) originOwed(slug string) bool {
	runs, err := d.deps.Runs()
	if err != nil {
		return true
	}
	for _, run := range runs {
		if run.Slug == slug {
			return run.OriginOwed
		}
	}
	return false
}

// noticeFailingPass posts the failing-pass notice for slug and records the text as noticed once it went out.
func (d *daemon) noticeFailingPass(slug, errText string) {
	line := fmt.Sprintf("%s: batten pass keeps failing: %s; way forward: fix the cause the text names, then run lyx batten run %s; lyx batten status %s shows the run", slug, errText, slug, slug)
	if err := d.deps.Notice(d.tickCtx, line); err != nil {
		logger.Warn("dobbyengine: the failing-pass notice did not go out", "slug", slug, "error", err)
		return
	}
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		rs := s.Runs[slug]
		rs.Noticed = append(rs.Noticed, errText)
		s.Runs[slug] = rs
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the noticed failing-pass text could not be recorded", "slug", slug, "error", err)
	}
}
