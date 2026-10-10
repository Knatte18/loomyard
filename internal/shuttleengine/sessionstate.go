// sessionstate.go declares the provider-neutral session state and the pure fold that reduces session signals and read facts to it.
// The fold reads no file and no clock; its caller reads the signals and the facts, and states the time of its reading.

package shuttleengine

import "time"

// SessionStateName names what a session is doing.
type SessionStateName string

// States a session can read as.
const (
	// SessionBusy: the agent is working, on a turn or on background work it launched.
	SessionBusy SessionStateName = "busy"
	// SessionIdleDone: the agent's turn ended and the run's output files exist.
	SessionIdleDone SessionStateName = "idle-done"
	// SessionIdleStalled: the agent stopped without finishing the run, through no question.
	SessionIdleStalled SessionStateName = "idle-stalled"
	// SessionAsking: the session waits on an answer.
	SessionAsking SessionStateName = "asking"
	// SessionDead: the session's process is gone.
	SessionDead SessionStateName = "dead"
	// SessionUnknown: the evidence does not settle a state.
	SessionUnknown SessionStateName = "unknown"
)

// Causes a state names besides a session end's own reason, which is the cause of a state a session end produced.
const (
	SessionCauseTurn             = "turn"
	SessionCauseBackground       = "background"
	SessionCauseDone             = "done"
	SessionCauseNoOutput         = "no-output"
	SessionCauseAwaitingInput    = "awaiting-input"
	SessionCauseAPIError         = "api-error"
	SessionCauseInterrupt        = "interrupt"
	SessionCauseContradiction    = "contradiction"
	SessionCauseAsk              = "ask"
	SessionCauseProcessGone      = "process-gone"
	SessionCauseLivenessUnproven = "liveness-unproven"
	SessionCauseEventsUnreadable = "events-unreadable"
	SessionCauseNoSignal         = "no-signal"
	SessionCauseNoState          = "no-state"
	sessionCauseSessionEnd       = "session-end"
)

// SessionState is one reading of what a session is doing.
type SessionState struct {
	// Name is the state.
	Name SessionStateName
	// Cause says why the session is in the state.
	Cause string
	// Detail is an API error's text; empty on every other state.
	Detail string
	// Outstanding is the background tasks of a busy state whose cause is background.
	Outstanding []BackgroundTask
	// Since is when the causing line or reading happened; zero when the causing line has no time.
	Since time.Time
}

// Liveness is what the caller proved about the session's process.
type Liveness int

// Livenesses a caller can report.
const (
	// LivenessUnproven: the caller could not tell; the zero value.
	LivenessUnproven Liveness = iota
	// LivenessAlive: the process is running.
	LivenessAlive
	// LivenessDead: the process is gone.
	LivenessDead
)

// SessionFacts is what the caller read for one fold, beyond the signals.
type SessionFacts struct {
	// Interactive is true for a run an operator answers in its pane; its unfinished turn end is an ask, not a stall.
	Interactive bool
	// OutputsExist is true when every output file of the run exists.
	OutputsExist bool
	// Liveness is the session process's proven liveness.
	Liveness Liveness
	// EventsUnreadable is true when the events file could not be read.
	EventsUnreadable bool
	// APIError is true when the transcript marks the fold's newest turn end as an API error; APIErrorText is the error's text.
	APIError     bool
	APIErrorText string
	// Interrupted is true when the transcript marks the fold's newest turn start as interrupted; InterruptAt is the marking entry's time.
	Interrupted bool
	InterruptAt time.Time
	// ReadAt is the reading's time from the caller's clock.
	ReadAt time.Time
}

// SessionFold reduces session signals and facts to a session state, remembering what it needs to settle the next fold.
// The zero value is an empty fold; a files-only reader folds every signal past the prompt offset once, and the wait loop keeps one fold and calls Fold at every poll, with an empty slice when nothing new arrived.
//
// Signals are ordered by file order only: At is read for a state's Since and never compared.
// The output files are consulted before the output-derived states, no-output and an interactive run's asking.
// Dead and unknown outrank idle-done, so a finished run can read dead.
// The first consumer that acts on dead must apply PATTERN-completion-signal's output check before acting.
type SessionFold struct {
	newest    SessionSignal
	hasNewest bool
	// sessionStartAt is the time of the newest session-start signal; zero when none arrived or it had no time.
	sessionStartAt time.Time

	// derived is the state the signals alone give, before the facts' liveness and readability take precedence.
	derived    SessionState
	hasDerived bool
	// derivedFromTurnEnd is true while derived is the judgment of the newest turn end.
	derivedFromTurnEnd bool
	turnEndAt          time.Time

	current    SessionState
	hasCurrent bool
	history    []SessionState
}

// State returns the current state; the zero value before the first Fold.
func (f *SessionFold) State() SessionState {
	return f.current
}

// SessionStartAt returns the time of the newest session-start signal folded; zero when none arrived.
func (f *SessionFold) SessionStartAt() time.Time {
	return f.sessionStartAt
}

// History returns every state the fold has passed through, in order, with one entry per change of state or cause.
func (f *SessionFold) History() []SessionState {
	return append([]SessionState(nil), f.history...)
}

// Fold folds signals, in file order, into the state, then settles the current state against facts.
// Every turn end of the call is judged with the call's OutputsExist;
// a call with no new turn end re-judges only the newest turn end, and only when it reads no-output or an interactive run's asking, so output files that appeared since make it idle-done.
func (f *SessionFold) Fold(signals []SessionSignal, facts SessionFacts) {
	newTurnEnd := false
	for _, signal := range signals {
		f.applySignal(signal, facts)
		if signal.Kind == SessionSignalTurnEnd || signal.Kind == SessionSignalAPIErrorTurnEnd {
			newTurnEnd = true
		}
		if f.hasDerived {
			f.recordHistory(f.derived)
		}
	}
	if !newTurnEnd {
		f.rejudgeNewestTurnEnd(facts)
	}
	f.applyTranscriptMarkers(facts)
	f.settle(facts)
}

// applySignal moves the signal-derived state by one signal.
func (f *SessionFold) applySignal(signal SessionSignal, facts SessionFacts) {
	switch signal.Kind {
	case SessionSignalSessionStart:
		f.sessionStartAt = signal.At
		return
	case SessionSignalTurnStart:
		if !(f.hasDerived && f.derived.Name == SessionBusy && f.derived.Cause == SessionCauseTurn) {
			f.setDerived(SessionState{Name: SessionBusy, Cause: SessionCauseTurn, Since: signal.At}, false)
		}
	case SessionSignalTurnEnd:
		f.turnEndAt = signal.At
		f.setDerived(judgeTurnEnd(signal, facts), true)
	case SessionSignalAPIErrorTurnEnd:
		f.turnEndAt = signal.At
		f.setDerived(SessionState{Name: SessionIdleStalled, Cause: SessionCauseAPIError, Detail: signal.ErrorText, Since: signal.At}, true)
	case SessionSignalAsk:
		f.setDerived(SessionState{Name: SessionAsking, Cause: SessionCauseAsk, Since: signal.At}, false)
	case SessionSignalIdleNotice:
		if f.hasNewest && f.newest.Kind == SessionSignalTurnStart {
			f.setDerived(SessionState{Name: SessionUnknown, Cause: SessionCauseContradiction, Since: signal.At}, false)
		}
	case SessionSignalSessionEnd:
		cause := signal.Reason
		if cause == "" {
			cause = sessionCauseSessionEnd
		}
		name := SessionUnknown
		if signal.EndsProcess {
			name = SessionDead
		}
		f.setDerived(SessionState{Name: name, Cause: cause, Since: signal.At}, false)
	}
	f.newest, f.hasNewest = signal, true
}

// setDerived replaces the signal-derived state.
func (f *SessionFold) setDerived(state SessionState, fromTurnEnd bool) {
	f.derived, f.hasDerived, f.derivedFromTurnEnd = state, true, fromTurnEnd
}

// judgeTurnEnd returns the state a Stop turn end reads as under facts.
func judgeTurnEnd(signal SessionSignal, facts SessionFacts) SessionState {
	state := SessionState{Since: signal.At}
	switch {
	case len(signal.Outstanding) > 0:
		state.Name, state.Cause = SessionBusy, SessionCauseBackground
		state.Outstanding = append([]BackgroundTask(nil), signal.Outstanding...)
	case facts.OutputsExist:
		state.Name, state.Cause = SessionIdleDone, SessionCauseDone
	case facts.Interactive:
		state.Name, state.Cause = SessionAsking, SessionCauseAwaitingInput
	default:
		state.Name, state.Cause = SessionIdleStalled, SessionCauseNoOutput
	}
	return state
}

// rejudgeNewestTurnEnd turns a no-output or awaiting-input reading of the newest turn end into idle-done once the output files exist.
func (f *SessionFold) rejudgeNewestTurnEnd(facts SessionFacts) {
	if !f.hasDerived || !f.derivedFromTurnEnd || !facts.OutputsExist {
		return
	}
	noOutput := f.derived.Name == SessionIdleStalled && f.derived.Cause == SessionCauseNoOutput
	awaitingInput := f.derived.Name == SessionAsking && f.derived.Cause == SessionCauseAwaitingInput
	if !noOutput && !awaitingInput {
		return
	}
	f.setDerived(SessionState{Name: SessionIdleDone, Cause: SessionCauseDone, Since: facts.ReadAt}, true)
	f.recordHistory(f.derived)
}

// applyTranscriptMarkers applies the transcript's API-error marker to the newest turn end and its interrupt marker to the newest turn start.
func (f *SessionFold) applyTranscriptMarkers(facts SessionFacts) {
	if !f.hasDerived {
		return
	}
	if facts.APIError && f.derivedFromTurnEnd && f.derived.Cause != SessionCauseAPIError {
		f.setDerived(SessionState{Name: SessionIdleStalled, Cause: SessionCauseAPIError, Detail: facts.APIErrorText, Since: f.turnEndAt}, true)
		f.recordHistory(f.derived)
	}
	if facts.Interrupted && f.derived.Name == SessionBusy && f.derived.Cause == SessionCauseTurn {
		f.setDerived(SessionState{Name: SessionIdleStalled, Cause: SessionCauseInterrupt, Since: facts.InterruptAt}, false)
		f.recordHistory(f.derived)
	}
}

// settle sets the current state: the first of a process-ending session end, a dead process, an unproven liveness, an unreadable events file and an empty signal history that applies, else the signal-derived state.
// A state that keeps the current state's name and cause keeps its Since.
func (f *SessionFold) settle(facts SessionFacts) {
	var settled SessionState
	switch {
	case f.hasNewest && f.newest.Kind == SessionSignalSessionEnd && f.newest.EndsProcess:
		settled = f.derived
	case facts.Liveness == LivenessDead:
		settled = SessionState{Name: SessionDead, Cause: SessionCauseProcessGone, Since: facts.ReadAt}
	case facts.Liveness == LivenessUnproven:
		settled = SessionState{Name: SessionUnknown, Cause: SessionCauseLivenessUnproven, Since: facts.ReadAt}
	case facts.EventsUnreadable:
		settled = SessionState{Name: SessionUnknown, Cause: SessionCauseEventsUnreadable, Since: facts.ReadAt}
	case !f.hasNewest:
		settled = SessionState{Name: SessionUnknown, Cause: SessionCauseNoSignal, Since: facts.ReadAt}
	case !f.hasDerived:
		settled = SessionState{Name: SessionUnknown, Cause: SessionCauseNoState, Since: facts.ReadAt}
	default:
		settled = f.derived
	}
	if f.hasCurrent && settled.Name == f.current.Name && settled.Cause == f.current.Cause {
		settled.Since = f.current.Since
	}
	f.current, f.hasCurrent = settled, true
	f.recordHistory(settled)
}

// recordHistory appends state unless the last entry already has its name and cause.
func (f *SessionFold) recordHistory(state SessionState) {
	if n := len(f.history); n > 0 && f.history[n-1].Name == state.Name && f.history[n-1].Cause == state.Cause {
		return
	}
	state.Outstanding = append([]BackgroundTask(nil), state.Outstanding...)
	f.history = append(f.history, state)
}
