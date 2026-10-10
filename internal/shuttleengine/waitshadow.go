// waitshadow.go logs the session state beside the wait loop's own classification, in shadow mode:
// the loop never branches on it, and a failure to read it changes nothing.

package shuttleengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Classifications the wait loop gives a run, logged beside the session state.
const (
	// loopRunning: no turn end is recorded as waiting or held.
	loopRunning = "running"
	// loopWaiting: a recorded waiting turn end or a pending gate entry.
	loopWaiting = "waiting"
	// loopHeld: the held wait is on show.
	loopHeld = "held"
	// loopDone: the run finalizes done.
	loopDone = "done"
	// loopDied: the run finalizes died.
	loopDied = "died"
)

// shadowPair is one pairing of the loop's classification with a session state, the unit a disagreement is logged once for.
type shadowPair struct {
	loop  string
	name  SessionStateName
	cause string
}

// sessionShadow is what a Run keeps to log its session state while it waits.
type sessionShadow struct {
	// active is true while Wait runs; a finalize outside Wait logs nothing.
	active bool
	// cursor is the events-file offset the next signal read starts from, separate from Run.offset.
	// It starts at the run record's prompt offset and stops before an unpaired trailing stamp.
	cursor int64
	fold   SessionFold
	// sessionID is the session id of the newest signal that named one.
	sessionID string
	// turnStart and turnEnd are the newest turn start and turn end folded so far, whose markers every refresh re-reads.
	// A transcript marker can land after the tick that folded its signal.
	// Each is kept without its session id, so the fact read probes sessionID instead.
	turnStart, turnEnd *SessionSignal
	// facts holds the liveness and transcript facts of the latest refresh, reused between refreshes.
	facts     SessionFacts
	factsRead bool

	logged          SessionState
	hasLogged       bool
	disagreement    shadowPair
	hasDisagreement bool
	// unavailableLogged is true once a missing capability or an unreadable events file was logged.
	unavailableLogged bool
}

// loopClassification returns the wait loop's classification of the run: outcome when it is a terminal done or died, else what the loop currently waits on.
func (run *Run) loopClassification(outcome Outcome) string {
	switch {
	case outcome == OutcomeDone:
		return loopDone
	case outcome == OutcomeDied:
		return loopDied
	case !run.wait.heldStart.IsZero():
		return loopHeld
	case len(run.waitingTasks) > 0 || run.gatePending:
		return loopWaiting
	}
	return loopRunning
}

// loopAgreesWithState reports whether the loop classification and the session state are a pairing the fixed mapping allows:
// waiting and running read busy, held reads idle-stalled or asking, done reads idle-done, died reads dead.
// Two further pairings are expected: a pending gate entry waiting beside idle-done once every output exists,
// and a done run beside busy on background work, since a turn end counted at once leaves the session running tasks the loop no longer waits on.
func loopAgreesWithState(loop string, state SessionState, gatePending bool) bool {
	busyOnBackground := state.Name == SessionBusy && state.Cause == SessionCauseBackground
	switch loop {
	case loopRunning:
		return state.Name == SessionBusy
	case loopWaiting:
		return state.Name == SessionBusy || (gatePending && state.Name == SessionIdleDone)
	case loopHeld:
		return state.Name == SessionIdleStalled || state.Name == SessionAsking
	case loopDone:
		return state.Name == SessionIdleDone || busyOnBackground
	case loopDied:
		return state.Name == SessionDead
	}
	return false
}

// logSessionState reads the signals appended since the shadow cursor, folds them with this tick's facts and logs a changed state and a new disagreement with the loop's classification.
// outcome is the outcome Wait is about to finalize, empty on an ordinary tick;
// refreshFacts re-reads the liveness and the transcript markers of the newest turn start and turn end folded so far, which are otherwise reused until new signals arrive.
// It returns nothing Wait reads: an engine without a SessionSignalParser, an unreadable events file or a missing fact logs once at Debug and changes nothing.
func (run *Run) logSessionState(outcome Outcome, refreshFacts bool) {
	shadow := &run.shadow
	if !shadow.active {
		return
	}
	parser, ok := run.runner.engine.(SessionSignalParser)
	if !ok {
		run.logSessionUnavailable("the engine cannot parse session signals", nil)
		return
	}
	data, _, err := readEventsFrom(run.state.EventsPath, shadow.cursor)
	if err != nil {
		run.logSessionUnavailable("the events file is unreadable", err)
		return
	}

	var signals []SessionSignal
	if len(data) > 0 {
		var consumed int
		signals, consumed = parser.ParseSessionSignals(data)
		shadow.cursor += int64(consumed)
	}
	for _, signal := range signals {
		if signal.SessionID != "" {
			shadow.sessionID = signal.SessionID
		}
		signal.SessionID = ""
		switch signal.Kind {
		case SessionSignalTurnStart:
			shadow.turnStart = &signal
		case SessionSignalTurnEnd, SessionSignalAPIErrorTurnEnd:
			shadow.turnEnd = &signal
		}
	}

	now := run.clock.Now()
	if len(signals) > 0 || refreshFacts || !shadow.factsRead {
		record := run.state
		record.OutputFiles = nil
		if shadow.sessionID != "" {
			record.SessionID = shadow.sessionID
		}
		var markerSignals []SessionSignal
		for _, signal := range []*SessionSignal{shadow.turnStart, shadow.turnEnd} {
			if signal != nil {
				markerSignals = append(markerSignals, *signal)
			}
		}
		shadow.facts, shadow.factsRead = readSessionFacts(record, run.runner.engine, markerSignals, now), true
	}
	facts := shadow.facts
	facts.Interactive = run.spec.Interactive
	facts.OutputsExist = allOutputFilesExist(run.spec.OutputFiles)
	facts.ReadAt = now
	shadow.fold.Fold(signals, facts)

	state := shadow.fold.State()
	loop := run.loopClassification(outcome)
	if !shadow.hasLogged || state.Name != shadow.logged.Name || state.Cause != shadow.logged.Cause {
		shadow.logged, shadow.hasLogged = state, true
		logger.Info("shuttle: session state", "strandGUID", run.state.StrandGUID, "state", string(state.Name), "cause", state.Cause, "since", state.Since.Format(time.RFC3339), "loop", loop)
	}

	if loopAgreesWithState(loop, state, run.gatePending) {
		shadow.hasDisagreement = false
		return
	}
	pair := shadowPair{loop: loop, name: state.Name, cause: state.Cause}
	if shadow.hasDisagreement && shadow.disagreement == pair {
		return
	}
	shadow.disagreement, shadow.hasDisagreement = pair, true
	logger.Warn("shuttle: session state disagrees", "strandGUID", run.state.StrandGUID, "loop", loop, "state", string(state.Name), "cause", state.Cause)
}

// logSessionUnavailable logs at Debug, once per Wait, why the session state could not be read.
func (run *Run) logSessionUnavailable(reason string, cause error) {
	if run.shadow.unavailableLogged {
		return
	}
	run.shadow.unavailableLogged = true
	logger.Debug("shuttle: session state unavailable", "strandGUID", run.state.StrandGUID, "reason", reason, "cause", cause)
}
