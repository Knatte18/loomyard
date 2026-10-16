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
	// readinessReasonNoInputBox is the held reason of a pane whose input box an engine able to read one finds nowhere on screen, as when a menu or picker is open.
	readinessReasonNoInputBox = "no input box is on screen; a menu or picker may be open"
	// sessionCauseReleasedTurn is the cause of the idle-stalled state a turn start reads as once its hold released.
	sessionCauseReleasedTurn = "released-turn"
	// sessionCauseNoRunRecord is the cause of the unknown reading of a strand no run record names.
	sessionCauseNoRunRecord = "no-run-record"
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
	// draft is the box's text of a reading held only for a draft in the input box; empty on every other reading.
	draft string
}

// boxReading is what a pane capture says about the provider's input box.
type boxReading struct {
	// read is true when the engine reads an input box and found one.
	read bool
	// text is the box's text; meaningful only when read is true.
	text string
	// tooShort is true when the pane is too short to draw the box.
	tooShort bool
	// reader is true when the engine can read an input box at all, so read false then means the capture shows none.
	reader bool
}

// boxCapture takes the one pane capture a box reading decides on.
// An engine with StyledBoxReader gets a styled capture turned plain with its placeholder blanked, and any other engine gets the plain capture.
// An error is the capture's, unchanged.
func boxCapture(reed ReedOps, engine Engine, guid string) (string, error) {
	styledReader, ok := engine.(StyledBoxReader)
	if !ok {
		return reed.CapturePane(guid)
	}
	styled, err := reed.CapturePaneStyled(guid)
	if err != nil {
		return "", err
	}
	return styledReader.PlainBoxCapture(styled), nil
}

// readBox reads the input box out of capture.
// An engine without an InputBoxReader reads no box and is never too short.
func readBox(engine Engine, capture string) boxReading {
	reader, ok := engine.(InputBoxReader)
	if !ok {
		return boxReading{}
	}
	box := boxReading{reader: true}
	box.text, box.read = reader.InputBoxText(capture)
	if cycler, ok := engine.(SessionCycler); ok {
		box.tooShort = cycler.PaneTooShort(capture)
	}
	return box
}

// judgeReadiness turns a session state, whether a turn start is still unmatched and the box reading into a readiness.
// A finished, stalled or freshly started session, one waiting on input at a turn end and one busy only on background work are ready, provided no turn start is unmatched and the box, where one is read, is blank.
// A running turn, a question awaiting an answer and a dead process are held, as are a pane too short, a pane showing no input box to an engine that reads one, an unmatched turn start and a draft in the box.
// An unknown state is neither: the caller falls back to the pane.
func judgeReadiness(state SessionState, unmatched bool, box boxReading) readiness {
	switch state.Name {
	case SessionUnknown:
		return readiness{unknown: true, cause: state.Cause}
	case SessionIdleDone, SessionIdleStalled, SessionIdleFresh:
	case SessionAsking:
		if state.Cause != SessionCauseAwaitingInput {
			return readiness{reason: "the session waits on an answer"}
		}
	case SessionBusy:
		if state.RunningTurn() {
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
	case box.reader && !box.read:
		return readiness{reason: readinessReasonNoInputBox}
	case unmatched:
		return readiness{reason: "a turn start has no turn end"}
	case box.read && strings.TrimSpace(box.text) != "":
		draft := strings.TrimSpace(box.text)
		return readiness{reason: fmt.Sprintf("the input box holds a draft: %q", draft), draft: draft}
	}
	return readiness{ready: true}
}

// sessionReadiness reads the readiness of the strand sc names from capture, its pane just captured.
// A recordless context reads unknown with cause no-run-record, so the caller takes the pane-only reading.
// It folds the run's session state at the context's clock; an unknown state settles nothing, so the reading is unknown without a look at the box.
// Otherwise it decides through hold whether an unmatched turn start still holds, and reads the box.
// A turn start whose hold released, by a later turn end, a reported interrupt, a later session start or the pane reading idle for turnStartIdleOverride, no longer counts as a running turn;
// a turn start the engine reads in flight is not released by the idle pane.
func sessionReadiness(sc sendContext, cycler SessionCycler, hold *turnStartHold, capture string) readiness {
	if sc.recordless {
		return judgeReadiness(SessionState{Name: SessionUnknown, Cause: sessionCauseNoRunRecord}, false, boxReading{})
	}
	now := sc.clock.Now()
	state := readRunSessionState(sc.state, sc.engine, now).State
	if state.Name == SessionUnknown {
		return judgeReadiness(state, false, boxReading{})
	}

	unmatched := false
	if turnStart, restarted, found := unmatchedTurnStart(sc.engine, sc.eventsPath); found {
		paneIdle := sc.engine.Startup(capture) == StartupReady && cycler.IdleSession(capture)
		unmatched = hold.holds(sc.engine, turnStart, restarted, paneIdle, now)
		if !unmatched && state.Name == SessionBusy && state.Cause == SessionCauseTurn {
			state = SessionState{Name: SessionIdleStalled, Cause: sessionCauseReleasedTurn, Since: state.Since}
		}
	}
	return judgeReadiness(state, unmatched, readBox(sc.engine, capture))
}
