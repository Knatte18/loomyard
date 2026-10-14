// watch.go implements the daemon's sequence: wait for the trigger, pause the hub, confirm the pause, hibernate the machine, detect the wake and lift the daemon's own pause entry.

package hibernateengine

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

const (
	// pauseGrace is the grace the hub pause gives busy sessions before the Esc.
	pauseGrace = 10 * time.Second
	// confirmInterval is the wait between two confirmation readings.
	confirmInterval = 5 * time.Second
	// confirmRereads is how many re-reads follow the first confirmation reading, so confirmation gives up after ten minutes.
	confirmRereads = 120
	// wakeWindow is how much awake time after the hibernate call may pass without a wake before the request fails.
	wakeWindow = 5 * time.Minute
	// wakePoll is how often the wake detection reads the clocks.
	wakePoll = time.Second
	// triggerPollBase is the first wait between two trigger readings; it doubles to triggerPollCeiling while the reading is unchanged.
	triggerPollBase = time.Second
	// triggerPollCeiling is the longest wait between two trigger readings.
	triggerPollCeiling = 30 * time.Second
)

// runGoneError means the batten status an after trigger waits for no longer exists.
type runGoneError struct{ slug string }

func (e runGoneError) Error() string {
	return fmt.Sprintf("the batten status of %s is gone", e.slug)
}

// Watch runs the daemon's sequence for the request whose daemon is the running process, and returns the request as it ended.
// It acts only for a non-terminal request carrying the process's own pid and start time; with no request, a terminal one or another daemon's it returns at once and writes nothing.
// A request cancelled meanwhile ends the sequence quietly, since the cancel lifts the owned pause entry.
// The daemon sends no key and no text of its own: every interrupt and resume line is tender's.
func Watch(deps Deps) (Request, error) {
	if err := refuseCaller(deps.Caller); err != nil {
		return Request{}, err
	}
	if err := refusePlatform(deps.GOOS); err != nil {
		return Request{}, err
	}
	var r Request
	var owns bool
	err := updateRequest(deps.RequestPath, func(cur Request, found bool) (Request, error) {
		if found && ours(deps, cur) && !cur.Terminal() {
			r, owns = cur, true
		}
		return Request{}, errUnchanged
	})
	if err != nil || !owns {
		return Request{}, err
	}

	r, ok, err := waitForTrigger(deps, r)
	if err != nil || !ok {
		return r, err
	}
	if r, ok, err = advance(deps, PhasePausing, ""); err != nil || !ok {
		return r, err
	}

	// kept holds the entry the pause added even when recording it in the request fails, so the daemon can still lift it.
	var kept *tenderengine.EntryRef
	report, err := deps.Pause(tenderengine.PauseRequest{Grace: pauseGrace, Added: func(entry pauserecord.Entry) error {
		ref := tenderengine.EntryRef{Scope: entry.Scope, Caller: entry.Caller, PausedAt: entry.PausedAt}
		kept = &ref
		return recordOwned(deps, ref)
	}})
	if err != nil {
		return fail(deps, r, kept, err.Error())
	}
	if report.ResumedDuringGrace {
		return finish(deps, r, PhaseCancelled, "the pause was lifted before the hibernate")
	}

	if r, ok, err = advance(deps, PhaseConfirming, ""); err != nil || !ok {
		return r, err
	}
	switch outcome, reason := awaitConfirmation(deps, report); outcome {
	case confirmationStopped:
		return r, nil
	case confirmationLifted:
		return finish(deps, r, PhaseCancelled, "the pause was lifted before the hibernate")
	case confirmationTimedOut:
		return fail(deps, r, kept, reason)
	}

	// The status read comes first so a cancel that lands during it is seen by the advance that follows.
	status, err := deps.TenderStatus()
	if err != nil {
		return fail(deps, r, kept, fmt.Sprintf("read the hub before the hibernate: %v", err))
	}
	if !hubPaused(status) {
		return finish(deps, r, PhaseCancelled, "the pause was lifted before the hibernate")
	}
	if r, ok, err = advance(deps, PhaseHibernating, ""); err != nil || !ok {
		return r, err
	}
	woke, err := awaitWake(deps)
	if err != nil {
		return fail(deps, r, kept, err.Error())
	}
	if !woke {
		return fail(deps, r, kept, "no wake was seen within five minutes")
	}

	if r, ok, err = advance(deps, PhaseResuming, ""); err != nil || !ok {
		return r, err
	}
	lifted, err := liftOwned(deps, r)
	if err != nil {
		return fail(deps, r, nil, err.Error())
	}
	r, err = finish(deps, r, PhaseDone, "")
	if err != nil {
		return r, err
	}
	return r, deps.Notify("[lyx hibernate] The machine woke from hibernation; " + wakeOutcome(lifted))
}

// wakeOutcome says what became of the request's pause entry, as the tail of the wake notice.
func wakeOutcome(lifted *tenderengine.ResumeReport) string {
	switch {
	case lifted == nil:
		return "it owned no pause, so the hub pause that stood before the request still stands."
	case len(lifted.Resumed) == 0:
		return "its pause had already been lifted."
	}
	return "its pause was lifted."
}

// ours reports whether r is the request of the running daemon.
func ours(deps Deps, r Request) bool {
	return r.DaemonPID == deps.SelfPID && r.DaemonStart == deps.SelfStart
}

// stillOurs reports whether the request file still holds a non-terminal request of the running daemon.
// A file that cannot be read is taken as still ours, since the daemon cannot tell otherwise.
func stillOurs(deps Deps) bool {
	r, found, err := readRequest(deps.RequestPath)
	if err != nil {
		logger.Debug("hibernate: the request could not be read, taking it as still this daemon's", "error", err)
		return true
	}
	return found && ours(deps, r) && !r.Terminal()
}

// recordOwned writes ref as the request's owned entry, whatever the phase.
func recordOwned(deps Deps, ref tenderengine.EntryRef) error {
	return updateRequest(deps.RequestPath, func(cur Request, found bool) (Request, error) {
		if !found {
			return Request{}, errUnchanged
		}
		cur.Owned = &ref
		return cur, nil
	})
}

// advance moves the request to phase with reason under the lock.
// It reports false, writing nothing, when the request is no longer this daemon's or has gone terminal, as after a cancel.
func advance(deps Deps, phase Phase, reason string) (Request, bool, error) {
	var next Request
	moved := false
	err := updateRequest(deps.RequestPath, func(cur Request, found bool) (Request, error) {
		if !found || !ours(deps, cur) || cur.Terminal() {
			return Request{}, errUnchanged
		}
		cur.Phase, cur.Reason = phase, reason
		next, moved = cur, true
		return cur, nil
	})
	if err != nil || !moved {
		return Request{}, false, err
	}
	logger.Info("hibernate: phase change", "phase", phase, "reason", reason)
	return next, true, nil
}

// finish writes a terminal phase and its reason the same way advance does, and returns the request as written, or r when nothing was written.
func finish(deps Deps, r Request, phase Phase, reason string) (Request, error) {
	var written Request
	moved := false
	err := updateRequest(deps.RequestPath, func(cur Request, found bool) (Request, error) {
		if !found || !ours(deps, cur) || cur.Terminal() {
			return Request{}, errUnchanged
		}
		cur.Phase, cur.Reason = phase, reason
		written, moved = cur, true
		return cur, nil
	})
	if err != nil {
		return r, err
	}
	if !moved {
		return r, nil
	}
	logger.Info("hibernate: request ended", "phase", phase, "reason", reason)
	return written, nil
}

// fail ends the request failed with reason, lifts the entry it owns and queues one notice.
// When the request records no owned entry the entry kept from the pause is lifted instead.
// The lift and the notice run even when the failed write itself fails, and the write's error is returned.
func fail(deps Deps, r Request, kept *tenderengine.EntryRef, reason string) (Request, error) {
	r, writeErr := finish(deps, r, PhaseFailed, reason)
	target := r.Owned
	if current, found, err := readRequest(deps.RequestPath); err == nil && found && current.Owned != nil {
		target = current.Owned
	}
	if target == nil {
		target = kept
	}
	var liftErr error
	outcome := "it owned no pause to lift."
	if target != nil {
		if _, liftErr = deps.Lift(*target); liftErr == nil {
			outcome = "its pause was lifted."
		} else {
			outcome = "its pause could not be lifted."
		}
	}
	notifyErr := deps.Notify(fmt.Sprintf("[lyx hibernate] The hibernate request failed: %s; %s", reason, outcome))
	return r, errors.Join(writeErr, liftErr, notifyErr)
}

// waitForTrigger reads the trigger until it fires and reports true.
// It reports false when the request ended meanwhile: failed because the batten status an after trigger waits for is gone, or no longer this daemon's, as after a cancel.
// The wait has no deadline, since the operator cancels a request whose run halted.
func waitForTrigger(deps Deps, r Request) (Request, bool, error) {
	delay := triggerPollBase
	lastReason := r.Reason
	for {
		if !stillOurs(deps) {
			return r, false, nil
		}
		fire, reason, err := triggerReading(deps, r)
		var gone runGoneError
		switch {
		case errors.As(err, &gone):
			ended, finishErr := finish(deps, r, PhaseFailed, gone.Error())
			return ended, false, finishErr
		case err != nil:
			logger.Debug("hibernate: the trigger reading failed", "error", err)
			reason = lastReason
		case fire:
			return r, true, nil
		}
		if reason != lastReason {
			next, moved, err := advance(deps, PhaseWaiting, reason)
			if err != nil || !moved {
				return r, false, err
			}
			r, lastReason, delay = next, reason, triggerPollBase
		} else {
			delay = min(delay*2, triggerPollCeiling)
		}
		logger.Debug("hibernate: the trigger has not fired", "reason", reason)
		deps.Sleep(delay)
	}
}

// triggerReading reads the request's trigger once: whether it fires, and otherwise why it still waits.
// An after trigger whose status has gone fails with a runGoneError.
func triggerReading(deps Deps, r Request) (fire bool, reason string, err error) {
	switch r.Trigger {
	case TriggerAfter:
		status, found, err := deps.RunStatus(r.Slug)
		switch {
		case err != nil:
			return false, "", err
		case !found:
			return false, "", runGoneError{slug: r.Slug}
		case landFinished(status):
			return true, "", nil
		case status.State == shedengine.StateRunning:
			return false, fmt.Sprintf("waiting for %s to finish Land, now at %s", r.Slug, status.Producer), nil
		}
		return false, fmt.Sprintf("the batten run of %s halted at %s (%s); run \"lyx hibernate cancel\" to withdraw the request", r.Slug, status.Producer, status.State), nil
	case TriggerAfterAll:
		slugs, err := deps.RunSlugs()
		if err != nil {
			return false, "", err
		}
		var inFlight []string
		for _, slug := range slugs {
			status, found, err := deps.RunStatus(slug)
			if err != nil {
				return false, "", err
			}
			if found && status.State == shedengine.StateRunning {
				inFlight = append(inFlight, slug)
			}
		}
		if len(inFlight) == 0 {
			return true, "", nil
		}
		return false, "waiting for the batten runs in flight: " + strings.Join(inFlight, ", "), nil
	}
	return true, "", nil
}

// landFinished reports whether a run is past its Land row: done, or at the one row after Land in any state.
func landFinished(s RunStatus) bool {
	return s.State == shedengine.StateDone || s.Producer == battenrecipe.NameWorktreeTeardown
}

// confirmation is how the wait for a confirmed pause ended.
type confirmation int

const (
	confirmationReached confirmation = iota
	// confirmationLifted: no live hub entry stands, the operator's resume having lifted the pause.
	confirmationLifted
	// confirmationStopped: the request is no longer this daemon's.
	confirmationStopped
	confirmationTimedOut
)

// awaitConfirmation reads the hub until a reading confirms the pause, the first reading right after the pause and then up to confirmRereads re-reads confirmInterval apart.
// The first reading also withholds while the pause report names a strand still busy or a session whose interrupt was refused.
// A reading error is logged and counts as unconfirmed; the reason is the last withholding one when the wait times out.
func awaitConfirmation(deps Deps, report tenderengine.PauseReport) (confirmation, string) {
	withheld := ""
	for reading := 0; reading <= confirmRereads; reading++ {
		if reading > 0 {
			deps.Sleep(confirmInterval)
		}
		if !stillOurs(deps) {
			return confirmationStopped, ""
		}
		status, err := deps.TenderStatus()
		landLockHeld := false
		if err == nil {
			landLockHeld, err = deps.LandLockHeld()
		}
		if err != nil {
			logger.Debug("hibernate: a confirmation reading failed", "error", err)
			withheld = "a confirmation reading failed: " + err.Error()
			continue
		}
		ok, hubEntry, reason := confirmed(status, landLockHeld)
		switch {
		case !hubEntry:
			return confirmationLifted, ""
		case ok && reading == 0 && len(report.StillBusy) > 0:
			ok, reason = false, "the pause left "+strings.Join(report.StillBusy, ", ")+" busy"
		case ok && reading == 0 && len(report.Refused) > 0:
			ok, reason = false, "the pause was refused by "+report.Refused[0].Strand
		}
		if ok {
			return confirmationReached, ""
		}
		logger.Debug("hibernate: the pause is not confirmed", "reason", reason)
		withheld = reason
	}
	return confirmationTimedOut, withheld
}

// hubPaused reports whether status shows a live hub entry.
func hubPaused(status tenderengine.StatusReport) bool {
	return slices.ContainsFunc(status.Live, func(entry pauserecord.Entry) bool { return entry.Scope == pauserecord.ScopeHub })
}

// confirmed reports whether one reading confirms the pause: a live hub entry stands, every session is idle, asking or dead, no network operation is in flight and the land lock is free.
// hubEntry is false when no live hub entry stands, and reason says what withholds a reading that is not ok.
// The parent locks in status.Holds are not read, since a paused run keeps its parent lock by design.
func confirmed(status tenderengine.StatusReport, landLockHeld bool) (ok bool, hubEntry bool, reason string) {
	if !hubPaused(status) {
		return false, false, "no hub pause is live"
	}
	for _, session := range status.Sessions {
		switch shuttleengine.SessionStateName(session.State) {
		case shuttleengine.SessionIdleDone, shuttleengine.SessionIdleStalled, shuttleengine.SessionIdleFresh, shuttleengine.SessionAsking, shuttleengine.SessionDead:
		default:
			return false, true, fmt.Sprintf("%s reads %s", session.Strand, session.State)
		}
	}
	if len(status.NetOps) > 0 {
		return false, true, fmt.Sprintf("%d network operation(s) in flight", len(status.NetOps))
	}
	if landLockHeld {
		return false, true, "the land lock is held"
	}
	return true, true, ""
}

// awaitWake observes the clocks right before the hibernate call and then every wakePoll, and reports whether a sleep ending after the call is seen within wakeWindow of awake time.
// A Hibernate error is returned.
func awaitWake(deps Deps) (woke bool, err error) {
	var watch pauserecord.SleepWatch
	watch.Observe(deps.Now(), deps.Awake())
	calledAt := deps.Now()
	if err := deps.Hibernate(); err != nil {
		return false, fmt.Errorf("hibernate: the machine could not be hibernated: %w", err)
	}
	logger.Info("hibernate: the hibernate job is queued")
	start := deps.Awake()
	for deps.Awake()-start < wakeWindow {
		deps.Sleep(wakePoll)
		watch.Observe(deps.Now(), deps.Awake())
		for _, sleep := range watch.Sleeps() {
			if sleep.End.After(calledAt) {
				return true, nil
			}
		}
	}
	return false, nil
}
