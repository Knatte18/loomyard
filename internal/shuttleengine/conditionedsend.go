// conditionedsend.go holds Runner.SendConditioned, the send that gives a line to a session only while it reads a named state, and the reading and wait around it.

package shuttleengine

import (
	"errors"
	"fmt"
)

// ErrSendConditionUnmet marks a conditioned send whose session did not read the condition at its one reading: nothing was typed.
var ErrSendConditionUnmet = errors.New("shuttle: the session does not read the send's condition")

// SendCondition names the session state a conditioned send's ready reading must show.
type SendCondition struct {
	State SessionStateName
	Cause string
}

// SendConditioned types text into the pane of the run identified by guid when one ready reading shows the session in the state want names, and submits it.
// turnStarted is true when a turn start past the send reads within one submit window of the landing, and false with a nil error for a send that landed with none seen.
//
// The reading takes no wait, no settle and no key, and refuses at once:
// with an error wrapping ErrRewindPickerOpen when the rewind picker is open, leaving it open;
// with an error wrapping ErrSendConditionUnmet when the session reads another state, a busy session and an unmatched turn start included;
// and with a *SessionBusyError when the pane is too short or shows no input box, when the input box holds any text, a draft or a lone stray character, and when an operator was active in the pane.
// A refused send types nothing.
//
// It never clears a stray character, never closes the picker and never clears a text that did not land, so a send that does not land fails with ErrSubmissionNotLanded and leaves its text in the box.
// It never consults a pending-send record, so a line an earlier unlanded send left pending is typed again.
func (r *Runner) SendConditioned(guid, text string, want SendCondition) (turnStarted bool, err error) {
	if r.toldErr != nil {
		return false, r.toldErr
	}
	if err := validateSendText(text); err != nil {
		return false, err
	}
	state, runDir, err := FindRun(r.cfg, r.anchorPath, guid)
	if err != nil {
		return false, fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	sc := r.newSendContext(state, runDir)
	sc.keepUnlanded = true
	if err := conditionedReading(sc, want); err != nil {
		return false, err
	}
	sentAt := eventsSize(sc.eventsPath)
	if err := typeVerified(sc, text); err != nil {
		return false, err
	}
	return awaitTurnStart(sc, sentAt), nil
}

// conditionedReading takes one reading of the strand's pane and events file and returns nil only when the session reads want and can take typed input.
// It waits on nothing and plays no key.
func conditionedReading(sc sendContext, want SendCondition) error {
	capture, err := sc.reed.CapturePane(sc.guid)
	if err != nil {
		return fmt.Errorf("shuttle: capture strand %q's pane before the conditioned send: %w", sc.guid, err)
	}
	if reader, ok := sc.engine.(RewindPickerReader); ok && reader.RewindPicker(capture) {
		return fmt.Errorf("%w: strand %q: nothing was typed or closed, so close it in the pane, then retry", ErrRewindPickerOpen, sc.guid)
	}
	state := readRunSessionState(sc.state, sc.engine, sc.clock.Now()).State
	if state.Name != want.State || state.Cause != want.Cause {
		return fmt.Errorf("%w: strand %q reads %s (%s), want %s (%s)", ErrSendConditionUnmet, sc.guid, state.Name, state.Cause, want.State, want.Cause)
	}
	if reading := judgeReadiness(state, false, readBox(sc.engine, capture)); !reading.ready {
		return &SessionBusyError{Reason: reading.reason, Draft: reading.draft != "", Err: fmt.Errorf("%w: strand %q: %s; retry once the session can take input", ErrSessionBusy, sc.guid, reading.reason)}
	}
	if hold := operatorActivityHold(sc.reed, sc.guid, sc.activityFailures); hold != "" {
		return &SessionBusyError{Reason: hold, Err: fmt.Errorf("%w: strand %q: %s; retry once the operator is idle", ErrSessionBusy, sc.guid, hold)}
	}
	return nil
}

// awaitTurnStart polls the events file for a turn start past byte offset sentAt, for at most one submit window on the context's clock, and reports whether one read.
// The polls start at idlePollInitial and double up to idlePollMax, and the window's poll count bounds them under a clock that never advances.
func awaitTurnStart(sc sendContext, sentAt int64) bool {
	window := submitConfirmTimeout(sc.cfg)
	closeAt := sc.clock.Now().Add(window)
	interval := idlePollInitial
	maxPolls := windowPollCap(window, idlePollInitial)
	for poll := 1; ; poll++ {
		if turnStartedSince(sc.engine, sc.eventsPath, sentAt) {
			return true
		}
		if windowClosed(sc.clock, closeAt) || poll >= maxPolls {
			return false
		}
		sc.clock.Sleep(min(interval, closeAt.Sub(sc.clock.Now())))
		interval = min(interval*2, idlePollMax)
	}
}
