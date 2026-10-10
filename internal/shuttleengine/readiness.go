// readiness.go builds the readiness reading: one answer, from the hook-derived session state, to whether a session can take typed input now.
// Runner.SessionIdle and the verified send's idle wait both gate on it, and fall back to the pane alone when the state is unknown.

package shuttleengine

import (
	"fmt"
	"strings"
)

// Reasons a readiness reading is held that callers tell apart or pin.
const (
	// readinessReasonPaneTooShort is the held reason of a pane too short to draw the input box.
	readinessReasonPaneTooShort = "the pane is too short to show the input box"
	// sessionCauseReleasedTurn is the cause of the idle-stalled state a turn start reads as once its hold released.
	sessionCauseReleasedTurn = "released-turn"
)

// readiness is one reading of whether a session can take typed input.
// ready and unknown are exclusive: a reading that is neither is held, and reason says why.
type readiness struct {
	ready bool
	// reason says why a held reading is held; empty on the others.
	reason string
	// unknown is true when the session state settles nothing, so the caller falls back to the pane.
	unknown bool
	// cause is the unknown state's cause; empty on the others.
	cause string
}

// boxReading is what a pane capture says about the provider's input box.
type boxReading struct {
	// read is true when the engine reads an input box and found one.
	read bool
	// text is the box's text; meaningful only when read is true.
	text string
	// tooShort is true when the pane is too short to draw the box.
	tooShort bool
}

// readBox reads the input box out of capture.
// An engine without an InputBoxReader reads no box and is never too short.
func readBox(engine Engine, capture string) boxReading {
	reader, ok := engine.(InputBoxReader)
	if !ok {
		return boxReading{}
	}
	var box boxReading
	box.text, box.read = reader.InputBoxText(capture)
	if cycler, ok := engine.(SessionCycler); ok {
		box.tooShort = cycler.PaneTooShort(capture)
	}
	return box
}

// judgeReadiness turns a session state, whether a turn start is still unmatched and the box reading into a readiness.
// A finished or stalled session, one waiting on input at a turn end and one busy only on background work are ready, provided no turn start is unmatched and the box, where one is read, is blank.
// A running turn, a question awaiting an answer and a dead process are held, as are a pane too short, an unmatched turn start and a draft in the box.
// An unknown state is neither: the caller falls back to the pane.
func judgeReadiness(state SessionState, unmatched bool, box boxReading) readiness {
	switch state.Name {
	case SessionUnknown:
		return readiness{unknown: true, cause: state.Cause}
	case SessionIdleDone, SessionIdleStalled:
	case SessionAsking:
		if state.Cause != SessionCauseAwaitingInput {
			return readiness{reason: "the session waits on an answer"}
		}
	case SessionBusy:
		if state.Cause != SessionCauseBackground {
			return readiness{reason: "a turn is running"}
		}
	case SessionDead:
		return readiness{reason: "the session's process is gone"}
	default:
		return readiness{reason: fmt.Sprintf("the session reads %s", state.Name)}
	}
	switch {
	case box.tooShort:
		return readiness{reason: readinessReasonPaneTooShort}
	case unmatched:
		return readiness{reason: "a turn start has no turn end"}
	case box.read && strings.TrimSpace(box.text) != "":
		return readiness{reason: fmt.Sprintf("the input box holds a draft: %q", strings.TrimSpace(box.text))}
	}
	return readiness{ready: true}
}

// sessionReadiness reads the readiness of the strand sc names from capture, its pane just captured.
// It folds the run's session state at the context's clock; an unknown state settles nothing, so the reading is unknown without a look at the box.
// Otherwise it decides through hold whether an unmatched turn start still holds, and reads the box.
// A turn start whose hold released, by a later turn end, a reported interrupt or the pane reading idle for turnStartIdleOverride, no longer counts as a running turn.
func sessionReadiness(sc sendContext, cycler SessionCycler, hold *turnStartHold, capture string) readiness {
	now := sc.clock.Now()
	state := readRunSessionState(sc.state, sc.engine, now).State
	if state.Name == SessionUnknown {
		return judgeReadiness(state, false, boxReading{})
	}

	unmatched := false
	if turnStart, found := unmatchedTurnStart(sc.engine, sc.eventsPath); found {
		paneIdle := sc.engine.Startup(capture) == StartupReady && cycler.IdleSession(capture)
		unmatched = hold.holds(sc.engine, turnStart, paneIdle, now)
		if !unmatched && state.Name == SessionBusy && state.Cause == SessionCauseTurn {
			state = SessionState{Name: SessionIdleStalled, Cause: sessionCauseReleasedTurn, Since: state.Since}
		}
	}
	return judgeReadiness(state, unmatched, readBox(sc.engine, capture))
}
