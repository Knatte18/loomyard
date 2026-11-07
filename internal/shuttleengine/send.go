// send.go holds what a verified send does around its typing:
// the context it runs in; the wait for an idle session; the settle;
// the submit confirmation and clear of an input-box send; and the gated wait loop's send and boundary-idle checks.
// None of it ends, classifies or finalizes a run.

package shuttleengine

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// ErrSessionBusy marks a verified send whose idle wait ran out while its readiness reading still held the session:
// a turn running; a question awaiting an answer; a draft in the input box; an unmatched turn start;
// an operator active in the pane within reed's quiet window; a pane showing no input box to an engine that reads one;
// or, when the reading is unknown, a pane that is not idle.
// Nothing was typed, so the caller may retry the send later.
var ErrSessionBusy = errors.New("shuttle: the session is not idle")

// SessionBusyError is the error of a verified send held by its idle wait.
// Reason is the hold the wait's last reading named, without the pane's last lines.
// Draft is true when that hold is text in the input box.
// NoInputBox is true when that hold is a pane showing no input box to an engine that reads one;
// a picker covering the box is one such pane.
// Err is the full error, which wraps ErrSessionBusy, and is what Error returns.
type SessionBusyError struct {
	Reason     string
	Draft      bool
	NoInputBox bool
	Err        error
}

// Error returns the message of the full error.
func (e *SessionBusyError) Error() string { return e.Err.Error() }

// Unwrap returns the full error, so errors.Is(err, ErrSessionBusy) holds.
func (e *SessionBusyError) Unwrap() error { return e.Err }

const (
	// defaultSendReadyTimeoutS is the template.yaml default idle wait in seconds.
	defaultSendReadyTimeoutS = 60

	// idlePollInitial is the first interval between idle polls; the interval doubles up to idlePollMax.
	idlePollInitial = 250 * time.Millisecond
	idlePollMax     = 2 * time.Second

	// turnStartIdleOverride is how long a pane must read idle on every poll before an unmatched turn start stops holding a send.
	turnStartIdleOverride = 10 * time.Second

	// paneTailLines is how many non-blank pane lines a send failure ends with.
	paneTailLines = 15
)

// sendContext is what one verified send needs: the pane it types into; the run's record, directory and events file;
// the config; and the clock its timed waits read.
// A zero deadline means none, and a nil hold starts the idle wait's turn-start hold afresh.
// startsNoTurn marks a send whose text starts no turn, such as a slash command:
// its paste follows a clear of the box,
// and a turn start past the paste with no instruction or its text as one fails it as a command taken for a prompt.
// landingStart is the session-start source whose line confirms a send that starts no turn;
// it is empty for every other send.
// keepUnlanded marks a send that never clears a text that did not land, leaving it in the input box.
// recordless marks a send into a strand no run record names: the run state, directory and events path are empty;
// so readiness reads the pane alone, and no pending-send record is kept.
type sendContext struct {
	reed         ReedOps
	engine       Engine
	guid         string
	state        RunState
	runDir       string
	eventsPath   string
	cfg          Config
	clock        Clock
	deadline     time.Time
	hold         *turnStartHold
	startsNoTurn bool
	landingStart SessionStartSource
	keepUnlanded bool
	recordless   bool
	// paused reads whether a pause covers the target strand and returns the refusal when it does; nil reads uncovered.
	paused func() error
	// activityFailures holds the runner's operator-activity failure episodes, shared by every send and probe of one runner.
	activityFailures *logger.EpisodeSet
}

// newSendContext returns the context of a verified send into run's own pane.
func (run *Run) newSendContext() sendContext {
	return sendContext{
		reed:       run.runner.reed,
		engine:     run.engine,
		guid:       run.state.StrandGUID,
		state:      run.state,
		runDir:     run.runDir,
		eventsPath: run.state.EventsPath,
		cfg:        run.runner.cfg,
		clock:      run.clock,
		paused:     func() error { return run.runner.pausedTarget(run.state.StrandGUID, run.state.StrandName) },

		activityFailures: &run.runner.activityFailures,
	}
}

// newSendContext returns the context of a verified send into the pane of the run state names, whose directory is runDir.
// It fails with the lookup's error when the state names an engine the runner's set does not hold.
func (r *Runner) newSendContext(state RunState, runDir string) (sendContext, error) {
	engine, _, err := r.engineFor(state)
	if err != nil {
		return sendContext{}, err
	}
	return sendContext{
		reed:       r.reed,
		engine:     engine,
		guid:       state.StrandGUID,
		state:      state,
		runDir:     runDir,
		eventsPath: state.EventsPath,
		cfg:        r.cfg,
		clock:      r.clock,
		paused:     func() error { return r.pausedTarget(state.StrandGUID, state.StrandName) },

		activityFailures: &r.activityFailures,
	}, nil
}

// recordlessSendContext returns the context of a verified send into the pane of the live strand guid, which no run record names.
// name is the strand's name as reed reports it, which its coverage reading judges by.
// It keeps the runner's turn-start hold for the strand and never clears a text that did not land.
// The send goes through the engine named by provider, the strand's provider tag;
// an empty tag selects the runner's default engine.
// It fails with the lookup's error when the set does not hold that engine.
func (r *Runner) recordlessSendContext(guid, name, provider string) (sendContext, error) {
	engine, _, err := r.engineFor(RunState{Engine: provider})
	if err != nil {
		return sendContext{}, err
	}
	return sendContext{
		reed:         r.reed,
		engine:       engine,
		guid:         guid,
		cfg:          r.cfg,
		clock:        r.clock,
		hold:         r.holdFor(guid),
		keepUnlanded: true,
		recordless:   true,
		paused:       func() error { return r.pausedTarget(guid, name) },

		activityFailures: &r.activityFailures,
	}, nil
}

// sendReadyTimeout returns how long a verified send waits for an idle session.
// A non-positive value in a hand-built Config is floored to the template default.
func sendReadyTimeout(cfg Config) time.Duration {
	if cfg.SendReadyTimeoutS <= 0 {
		return defaultSendReadyTimeoutS * time.Second
	}
	return time.Duration(cfg.SendReadyTimeoutS) * time.Second
}

// windowPollCap returns how many polls paced at least minInterval apart fit in window;
// it adds slack for a first poll at once and a last sleep cut short.
// Every poll of a send runs a real tmux process through reed, so this attempt-count bound sits beside the window;
// it also ends the loop under a clock that never advances.
// A non-positive minInterval is paced as idlePollInitial.
func windowPollCap(window, minInterval time.Duration) int {
	if minInterval <= 0 {
		minInterval = idlePollInitial
	}
	return int(window/minInterval) + 2
}

// awaitIdleSession returns nil once the strand's session is idle in fact.
// It fails with an error wrapping ErrSessionBusy when the session stays busy past its limit:
// the send-ready window, sc's deadline, or the window's poll count.
// An engine without the SessionCycler idle reading keeps requireReadyAgentPane alone, with no wait.
// Otherwise every poll takes the session's readiness reading, which is built on the hook-derived session state;
// when that state is unknown, the poll falls back to the pane, which must classify ready and idle;
// for an engine that parses session signals, no turn start may be left unmatched by a later turn end;
// unless the pane has read idle for turnStartIdleOverride or the engine reports that turn interrupted.
// An engine that reads the unmatched turn in flight keeps the send waiting past turnStartIdleOverride;
// the wait ends at a turn end, a reported interrupt, a later session start, or the send-ready window.
// A held reading over a picker the engine knows how to dismiss gets the picker's keys at most once per call.
func awaitIdleSession(sc sendContext) error {
	cycler, ok := sc.engine.(SessionCycler)
	if !ok {
		if err := requireReadyAgentPane(sc.reed, sc.engine, sc.guid); err != nil {
			return err
		}
		if err := closePickerBeforeSend(sc, true); err != nil {
			return err
		}
		if hold := operatorActivityHold(sc.reed, sc.guid, sc.activityFailures); hold != "" {
			return &SessionBusyError{Reason: hold, Err: fmt.Errorf("%w: strand %q: %s; retry once the operator is idle", ErrSessionBusy, sc.guid, hold)}
		}
		return nil
	}
	if err := requireLiveStrand(sc.reed, sc.guid); err != nil {
		return err
	}
	if err := closePickerBeforeSend(sc, false); err != nil {
		return err
	}

	limit := sc.clock.Now().Add(sendReadyTimeout(sc.cfg))
	if !sc.deadline.IsZero() && sc.deadline.Before(limit) {
		limit = sc.deadline
	}
	hold := sc.hold
	if hold == nil {
		hold = &turnStartHold{}
	}
	interval := idlePollInitial
	maxPolls := windowPollCap(sendReadyTimeout(sc.cfg), idlePollInitial)
	stray := &strayClear{}
	picker := &pickerDismissal{}
	for poll := 1; ; poll++ {
		busy := busyReading(sc, cycler, hold, stray, picker)
		if busy.reason == "" {
			return nil
		}
		now := sc.clock.Now()
		if !now.Before(limit) || poll >= maxPolls {
			return &SessionBusyError{Reason: busy.reason, Draft: busy.draft, NoInputBox: busy.noInputBox || busy.reason == readinessReasonNoInputBox, Err: fmt.Errorf("%w: strand %q: %s; retry once the session is idle. The pane's last lines:\n%s", ErrSessionBusy, sc.guid, busy.reason, paneTail(sc.reed, sc.guid))}
		}
		sc.clock.Sleep(min(interval, limit.Sub(now)))
		interval = min(interval*2, idlePollMax)
	}
}

// busyHold is one idle-wait reading's answer.
// reason is what keeps the session from being idle, and it is empty when the session is idle;
// draft is true when that hold is text in the input box.
// noInputBox is true when that hold is a pane showing no input box;
// a dismissal whose key could not be sent counts as one.
type busyHold struct {
	reason     string
	draft      bool
	noInputBox bool
}

// busyReading polls the pane and the events file once and returns what keeps the session from being idle.
// It returns an empty reason when the session is idle.
// The readiness reading decides: a ready reading returns "" and a held one its reason.
// An unknown reading falls back to the pane, logging its cause at Debug.
// Operator activity is asked last, only once the rest of the reading passed.
// So each poll is one fresh reading of readiness, box and activity.
//
// A lone stray-character candidate in the box is cleared once per stray, behind a full guard reading.
// It is cleared when the session-state reading holds only for that draft, or when the pane reads ready but not idle.
// Once stray records a clear, every reading of the attempt is a post-clear reading,
// and a box still holding exactly the recorded character reads as empty.
// A reading that is not ready over a picker the engine can dismiss gets one dismissal try per send attempt.
// Every pane-reading hold whose box reads non-empty quotes the box.
func busyReading(sc sendContext, cycler SessionCycler, hold *turnStartHold, stray *strayClear, picker *pickerDismissal) busyHold {
	capture, err := boxCapture(sc.reed, sc.engine, sc.guid)
	if err != nil {
		return busyHold{reason: fmt.Sprintf("the pane could not be captured: %v", err)}
	}
	reading := sessionReadiness(sc, cycler, hold, capture)
	if reading.draft != "" {
		character, isCandidate := strayCandidate(sc.engine, capture)
		switch {
		case isCandidate && character == reading.draft && stray.cleared && character == stray.character:
			reading = readiness{ready: true}
		case isCandidate && character == reading.draft && !stray.cleared:
			if busy, handled := clearStrayCharacter(sc, cycler, hold, stray, picker, character); handled {
				return busy
			}
		}
	}
	if !reading.ready && !picker.tried {
		if busy, handled := dismissPicker(sc, cycler, hold, stray, picker, capture); handled {
			return busy
		}
	}
	switch {
	case reading.ready:
		return busyHold{reason: operatorActivityHold(sc.reed, sc.guid, sc.activityFailures)}
	case !reading.unknown:
		return busyHold{reason: reading.reason, draft: reading.draft != ""}
	}
	logger.Debug("shuttle: send readiness unknown, reading the pane", "strandGUID", sc.guid, "cause", reading.cause)

	ready := sc.engine.Startup(capture) == StartupReady
	paneIdle := ready && cycler.IdleSession(capture)
	var character string
	isCandidate := false
	if ready && !paneIdle {
		character, isCandidate = strayCandidate(sc.engine, capture)
	}
	residue := isCandidate && stray.cleared && character == stray.character
	if residue {
		paneIdle = true
	}

	var unmatched string
	if turnStart, restarted, found := unmatchedTurnStart(sc.engine, sc.eventsPath); found && hold.holds(sc.engine, turnStart, restarted, paneIdle, sc.clock.Now()) {
		unmatched = fmt.Sprintf("a turn start at %s has no turn end", turnStart.At.Format(time.RFC3339))
	}
	if isCandidate && !stray.cleared && unmatched == "" {
		if busy, handled := clearStrayCharacter(sc, cycler, hold, stray, picker, character); handled {
			return busy
		}
	}
	switch {
	case !ready:
		return withBoxQuote("the pane shows no input-ready provider", sc.engine, capture)
	case !paneIdle:
		return withBoxQuote("the pane is not idle", sc.engine, capture)
	case unmatched != "":
		return withBoxQuote(unmatched, sc.engine, capture)
	}
	return busyHold{reason: operatorActivityHold(sc.reed, sc.guid, sc.activityFailures)}
}

// strayClear is the stray-character state of one send attempt's idle wait: the zero value has cleared nothing.
// After a clear, cleared is true and character is the one that was found, which a box still holding reads as residue.
type strayClear struct {
	cleared   bool
	character string
}

// strayCandidate returns the stray-character candidate the engine reads in capture;
// ok is false for an engine without StrayInputReader and for a capture with none.
func strayCandidate(engine Engine, capture string) (character string, ok bool) {
	reader, canRead := engine.(StrayInputReader)
	if !canRead {
		return "", false
	}
	return reader.StrayInput(capture)
}

// clearStrayCharacter handles a stray-character candidate found before any clear of this attempt.
// handled is false for an engine that cannot clear the box and read it back;
// the caller then holds on the reading it has.
// An active operator holds the send, quoting the box.
// Otherwise the clear plays right after this reading, with no wait or other key between:
// stray records the character, the box settles for `SubmitSettle`, and a second full reading is taken.
// That post-clear reading is the write's guard reading; busy is what it returns.
// The active-operator hold is a draft hold; a failed clear is not; the post-clear reading carries its own kind.
func clearStrayCharacter(sc sendContext, cycler SessionCycler, hold *turnStartHold, stray *strayClear, picker *pickerDismissal, character string) (busy busyHold, handled bool) {
	clearer, canClear := sc.engine.(InputBoxClearer)
	reader, canRead := sc.engine.(InputBoxReader)
	if !canClear || !canRead {
		return busyHold{}, false
	}
	if activity := operatorActivityHold(sc.reed, sc.guid, sc.activityFailures); activity != "" {
		return busyHold{reason: fmt.Sprintf("%s; the input box holds %q", activity, character), draft: true}, true
	}
	if err := playInputs(sc.reed, sc.guid, clearer.ClearInputSequence()); err != nil {
		return busyHold{reason: fmt.Sprintf("clearing the stray character %q failed: %v", character, err)}, true
	}
	stray.cleared, stray.character = true, character
	sc.clock.Sleep(reader.SubmitSettle())
	return busyReading(sc, cycler, hold, stray, picker), true
}

// pickerDismissal is the dismissible-picker state of one send attempt's idle wait: the zero value has tried no dismissal.
type pickerDismissal struct {
	tried bool
}

// dismissPicker closes a picker the engine knows how to dismiss when capture shows one,
// so the send can go on to the input box behind it.
// handled is false for an engine without DismissiblePickerReader and for a capture that shows no picker.
// It is also false when an InterruptGuard reads a running turn, since the picker's keys may interrupt it.
// The caller then holds on the reading it has.
// Otherwise an active operator holds the send without a key: the operator may be answering the picker,
// and a later poll tries again.
// Otherwise the picker's keys play once and the try is recorded on picker.
// The pane then settles for the engine's interrupt settle where it has one.
// A second full reading is taken, and busy is what it returns.
// A key that cannot be sent holds the send as a pane with no input box, since the picker still covers it.
// The keys keep the provider's current setting, so the dismissal answers nothing on the session's behalf.
func dismissPicker(sc sendContext, cycler SessionCycler, hold *turnStartHold, stray *strayClear, picker *pickerDismissal, capture string) (busy busyHold, handled bool) {
	reader, ok := sc.engine.(DismissiblePickerReader)
	if !ok {
		return busyHold{}, false
	}
	name, keys, shown := reader.DismissiblePicker(capture)
	if !shown {
		return busyHold{}, false
	}
	guard, guarded := sc.engine.(InterruptGuard)
	if guarded && guard.RunningTurn(capture) {
		return busyHold{}, false
	}
	if activity := operatorActivityHold(sc.reed, sc.guid, sc.activityFailures); activity != "" {
		return busyHold{reason: fmt.Sprintf("%s; the provider's %s picker is open", activity, name)}, true
	}
	picker.tried = true
	if err := playInputs(sc.reed, sc.guid, keys); err != nil {
		return busyHold{reason: fmt.Sprintf("dismissing the provider's %s picker failed: %v", name, err), noInputBox: true}, true
	}
	logger.Info("shuttle: dismissed the provider's picker before a send", "strandGUID", sc.guid, "picker", name)
	if guarded {
		sc.clock.Sleep(guard.InterruptSettle())
	}
	return busyReading(sc, cycler, hold, stray, picker), true
}

// withBoxQuote returns reason followed by the input box's text when the engine reads a non-empty box in capture;
// otherwise it returns the bare reason, and only the quoted hold is a draft hold.
func withBoxQuote(reason string, engine Engine, capture string) busyHold {
	box := readBox(engine, capture)
	if text := strings.TrimSpace(box.text); box.read && text != "" {
		return busyHold{reason: fmt.Sprintf("%s; the input box holds %q", reason, text), draft: true}
	}
	return busyHold{reason: reason}
}

// operatorActivityHold asks reed once whether an attached client was active in the pane within the quiet window.
// It returns the reason a write is held for it, or "" when none.
// A query error holds nothing, so a tmux that cannot list its clients stalls no run.
// Through failures, keyed by the strand, it logs one Warn per episode and an Info when the query reads again.
// When failures is nil, each error logs a Warn.
func operatorActivityHold(reed ReedOps, guid string, failures *logger.EpisodeSet) string {
	active, quiet, err := reed.OperatorActive(guid)
	if err != nil {
		if failures == nil {
			logger.Warn("shuttle: operator activity unreadable, not holding the write", "strandGUID", guid, "error", err)
		} else {
			failures.Fail(guid, "shuttle: operator activity unreadable, not holding the write", "strandGUID", guid, "error", err)
		}
		return ""
	}
	if failures != nil {
		failures.Recover(guid, "shuttle: operator activity readable again", "strandGUID", guid)
	}
	if !active {
		return ""
	}
	return fmt.Sprintf("the operator typed in this pane within the last %d s", int(quiet.Seconds()))
}

// unmatchedTurnStart returns the newest turn_start in the events file that no later turn_end or api_error_turn_end follows.
// restarted is true when a session start of any source lies after that turn start:
// the session's context began anew, so the turn cannot still be running.
// An engine without SessionSignalParser and an unreadable file read as none.
func unmatchedTurnStart(engine Engine, eventsPath string) (turnStart SessionSignal, restarted, found bool) {
	parser, ok := engine.(SessionSignalParser)
	if !ok {
		return SessionSignal{}, false, false
	}
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		return SessionSignal{}, false, false
	}
	signals, _ := parser.ParseSessionSignals(data)
	for _, signal := range signals {
		switch signal.Kind {
		case SessionSignalTurnStart:
			turnStart, restarted, found = signal, false, true
		case SessionSignalSessionStart:
			if found {
				restarted = true
			}
		case SessionSignalTurnEnd, SessionSignalAPIErrorTurnEnd:
			found = false
		}
	}
	return turnStart, restarted, found
}

// turnStartHold decides, poll by poll, whether an unmatched turn start still holds a send while the pane reads idle.
// The zero value holds nothing yet.
type turnStartHold struct {
	signal    SessionSignal
	tracking  bool
	idleSince time.Time
	released  bool
}

// holds reports whether turnStart, the unmatched turn start, still holds the send at now.
// It releases once the engine reports that the turn was interrupted.
// It also releases once paneIdle has been true on every poll for at least turnStartIdleOverride;
// that release logs one Warn naming the signal.
// While the engine reads the turn in flight and no session start followed it, the hold stays regardless of paneIdle.
// A session start after the turn start means the turn is gone.
// A poll with a busy pane, or one that reads the turn in flight, resets the idle timer.
// A turn start once released stays released for as long as it is the one asked about.
func (h *turnStartHold) holds(engine Engine, turnStart SessionSignal, restarted, paneIdle bool, now time.Time) bool {
	if !h.tracking || !sameSignal(h.signal, turnStart) {
		h.signal, h.tracking, h.idleSince, h.released = turnStart, true, time.Time{}, false
	}
	if h.released {
		return false
	}
	if prober, ok := engine.(SessionProber); ok {
		if _, interrupted := prober.TurnStartInterrupt(turnStart); interrupted {
			h.released = true
			return false
		}
	}
	if reader, ok := engine.(TurnProgressReader); ok && !restarted && reader.TurnStartProgress(turnStart) == TurnInFlight {
		h.idleSince = time.Time{}
		return true
	}
	if !paneIdle {
		h.idleSince = time.Time{}
		return true
	}
	if h.idleSince.IsZero() {
		h.idleSince = now
	}
	if now.Sub(h.idleSince) >= turnStartIdleOverride {
		logger.Warn("shuttle: send released an unmatched turn start held against an idle pane", "turnStartAt", turnStart.At, "sessionID", turnStart.SessionID, "idleFor", now.Sub(h.idleSince))
		h.released = true
		return false
	}
	return true
}

// sameSignal reports whether a and b are the one hook line.
func sameSignal(a, b SessionSignal) bool {
	return a.Kind == b.Kind && a.At.Equal(b.At) && a.SessionID == b.SessionID && string(a.Raw) == string(b.Raw)
}

// paneTail returns the last paneTailLines non-blank lines of the strand's pane, or a sentence saying the capture failed.
func paneTail(reed ReedOps, guid string) string {
	capture, err := reed.CapturePane(guid)
	if err != nil {
		return fmt.Sprintf("(the final pane capture failed: %v)", err)
	}
	return captureTail(capture)
}

// captureTail returns the last paneTailLines non-blank lines of capture, each right-trimmed and joined by newlines.
func captureTail(capture string) string {
	lines := nonBlankLines(capture)
	if len(lines) > paneTailLines {
		lines = lines[len(lines)-paneTailLines:]
	}
	return strings.Join(lines, "\n")
}

// nonBlankLines returns the lines of capture that hold more than whitespace, each right-trimmed, in order.
func nonBlankLines(capture string) []string {
	var lines []string
	for _, line := range strings.Split(capture, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimRight(line, " \t\r"))
		}
	}
	return lines
}

// ErrSubmissionNotLanded marks a verified send whose text was never seen to be submitted.
// The submit window closed before typing began, the typed text did not appear, or the input box did not settle;
// or the box still held the text when the window closed.
// The send clears its own typed text from the box where the engine can, so the caller may retry.
var ErrSubmissionNotLanded = errors.New("shuttle: Send: the send did not land")

const (
	// defaultSubmitConfirmTimeoutS is the template.yaml default submit window in seconds.
	defaultSubmitConfirmTimeoutS = 30

	// confirmBackoffCap is the longest interval between two input-box reads after an Enter.
	confirmBackoffCap = 5 * time.Second
)

// submitConfirmTimeout returns the window a send has to land once typing begins.
// A non-positive value in a hand-built Config is floored to the template default.
func submitConfirmTimeout(cfg Config) time.Duration {
	if cfg.SubmitConfirmTimeoutS <= 0 {
		return defaultSubmitConfirmTimeoutS * time.Second
	}
	return time.Duration(cfg.SubmitConfirmTimeoutS) * time.Second
}

// windowClosed reports whether closeAt, the end of the submit window, has been reached; a zero closeAt is no window.
func windowClosed(clock Clock, closeAt time.Time) bool {
	return !closeAt.IsZero() && !clock.Now().Before(closeAt)
}

// ErrSentTextInDraft marks a pasted send whose text was found in the input box beside other text:
// the operator's draft and the send have merged.
// It wraps ErrSessionBusy, so a caller that retries a busy session retries this too.
// Nothing was submitted, cleared or edited.
var ErrSentTextInDraft = fmt.Errorf("%w: the sent text sits in the operator's draft", ErrSessionBusy)

// ErrForeignBoxText marks a pasted send whose input box, read after the paste, holds text that is not the send's:
// the operator's draft stands in the box.
// It wraps ErrSessionBusy, so a caller that retries a busy session retries this too.
// Nothing was submitted, cleared or edited.
var ErrForeignBoxText = fmt.Errorf("%w: the input box holds text that is not this send's", ErrSessionBusy)

// ErrCommandTakenAsPrompt marks a pasted command, a send that starts no turn, whose paste started a turn.
// That turn start has no instruction, or the command's text as one.
// The provider took the text as a prompt, so the command never ran.
// A provider does this to a slash command behind leading whitespace.
// The agent got the text as an instruction and nothing else was typed;
// the caller may send the command again once the session is idle.
var ErrCommandTakenAsPrompt = errors.New("shuttle: the command reached the agent as a prompt and did not run")

// IsDraftHold reports whether err is a send held by text in the input box:
// a *SessionBusyError whose hold is a draft, or an error wrapping ErrSentTextInDraft or ErrForeignBoxText.
func IsDraftHold(err error) bool {
	var busy *SessionBusyError
	if errors.As(err, &busy) && busy.Draft {
		return true
	}
	return errors.Is(err, ErrSentTextInDraft) || errors.Is(err, ErrForeignBoxText)
}

// pasteVerified writes text into the pane as one paste through reed and confirms it was submitted.
// The paste carries its trailing Enter when the provider's Enter submits a paste;
// otherwise the box is settled first, so the guarded Enter lands on a redrawn box.
// The send's window opens before the paste and is cut to the send's deadline.
// A send that starts no turn clears the box right before its paste.
// A send into a strand with no run record keeps no pending-send record, since it has no run directory.
func pasteVerified(sc sendContext, paster TextPaster, reader InputBoxReader, text string) error {
	closeAt := sc.clock.Now().Add(submitConfirmTimeout(sc.cfg))
	if !sc.deadline.IsZero() && sc.deadline.Before(closeAt) {
		closeAt = sc.deadline
	}
	if windowClosed(sc.clock, closeAt) {
		return withPaneTail(fmt.Errorf("%w: the submit window, cut to the send's deadline, had closed before the paste; nothing was pasted", ErrSubmissionNotLanded), "", sc)
	}
	if sc.startsNoTurn {
		if err := clearBoxBeforeCommand(sc, reader); err != nil {
			return err
		}
	}
	sentAt := eventsSize(sc.eventsPath)
	if err := sc.reed.PasteSubmit(sc.guid, text, paster.PasteSubmits()); err != nil {
		return fmt.Errorf("shuttle: paste text: %w", err)
	}
	err := settleAndConfirmPasted(sc, paster, reader, text, sentAt, closeAt)
	if !sc.recordless && (errors.Is(err, ErrSentTextInDraft) || errors.Is(err, ErrSubmissionNotLanded)) {
		if saveErr := savePendingSend(sc.runDir, pendingSend{Text: text, EventsOffset: sentAt}); saveErr != nil {
			logger.Warn("shuttle: pending send not recorded, a retry may type the text twice", "strandGUID", sc.guid, "error", saveErr)
		}
	}
	return err
}

// clearBoxBeforeCommand plays the engine's input-box clear, for an engine that has one, right before a command's paste.
// The idle reading takes a box holding only whitespace as empty,
// and a provider takes a command pasted behind that whitespace as a prompt.
// The clear goes only into a box that reads blank or unreadable.
// No operator may have been active in the pane within the quiet window.
// Text typed since the idle reading holds the send with a *SessionBusyError, and nothing is cleared or pasted.
// A failed key fails the send before anything is pasted.
func clearBoxBeforeCommand(sc sendContext, reader InputBoxReader) error {
	clearer, ok := sc.engine.(InputBoxClearer)
	if !ok {
		return nil
	}
	if boxText, readable := readInputBox(sc, reader); readable && normalizePaneText(boxText) != "" {
		reason := fmt.Sprintf("the input box holds %q", strings.TrimSpace(boxText))
		busy := fmt.Errorf("%w: strand %q: %s; nothing was cleared or pasted", ErrSessionBusy, sc.guid, reason)
		return &SessionBusyError{Reason: reason, Draft: true, Err: withPaneTail(busy, "", sc)}
	}
	if hold := operatorActivityHold(sc.reed, sc.guid, sc.activityFailures); hold != "" {
		return &SessionBusyError{Reason: hold, Err: fmt.Errorf("%w: strand %q: %s; nothing was cleared or pasted; retry once the operator is idle", ErrSessionBusy, sc.guid, hold)}
	}
	if err := playInputs(sc.reed, sc.guid, clearer.ClearInputSequence()); err != nil {
		return fmt.Errorf("shuttle: clear the input box before the command; nothing was pasted: %w", err)
	}
	return nil
}

// settleAndConfirmPasted runs what follows a paste.
// The box settles first when the paste carries no Enter;
// then confirmPasted reads the box until the send lands or the window closes.
func settleAndConfirmPasted(sc sendContext, paster TextPaster, reader InputBoxReader, text string, sentAt int64, closeAt time.Time) error {
	if !paster.PasteSubmits() {
		if err := awaitSettledBox(sc, reader, closeAt); err != nil {
			return failUnlanded(sc, reader, normalizePaneText(text), err)
		}
	}
	return confirmPasted(sc, paster, reader, text, sentAt, closeAt)
}

// sequenceText joins the Text of a choreography's steps, ignoring its keys and pauses.
func sequenceText(inputs []PaneInput) string {
	var text strings.Builder
	for _, in := range inputs {
		text.WriteString(in.Text)
	}
	return text.String()
}

// sendSequence sends a choreography into sc's pane as one guarded write.
// An empty choreography, a provider's no-command answer, returns nil before the guard:
// no idle wait, no activity query and no paste.
// An engine that takes pastes sends the choreography's text as a verified send that starts no turn,
// so a slash command is pasted once into a cleared box and confirmed by the box reading empty.
// Any other engine waits for an idle session and then plays its own keys.
// A held write returns an error wrapping ErrSessionBusy and writes nothing.
func sendSequence(sc sendContext, inputs []PaneInput) error {
	if len(inputs) == 0 {
		return nil
	}
	if _, pastes := sc.engine.(TextPaster); pastes {
		if _, readsBox := sc.engine.(InputBoxReader); readsBox {
			if _, cycles := sc.engine.(SessionCycler); cycles {
				sc.startsNoTurn = true
				return sendVerified(sc, sequenceText(inputs))
			}
		}
	}
	if err := awaitIdleSession(sc); err != nil {
		return err
	}
	return playInputs(sc.reed, sc.guid, inputs)
}

// retryLanded reports whether the pending-send record of an earlier failed send shows that its text went out,
// so the retry types nothing.
// The record is cleared whatever it says.
// It reads as landed when the events file holds a turn start past the recorded offset;
// that turn start is the text going out with the draft or with an Enter the operator pressed.
// A text that starts no turn never reads as landed, since a turn start past the offset is some other turn.
// A record for another text, an unreadable record and one that cannot be cleared read as not landed;
// the last two are logged at Warn, so a broken file never wedges a strand.
func retryLanded(sc sendContext, text string) bool {
	pending, found, err := loadPendingSend(sc.runDir)
	if err != nil {
		logger.Warn("shuttle: pending send unreadable, sending as usual", "strandGUID", sc.guid, "error", err)
		if clearErr := clearPendingSend(sc.runDir); clearErr != nil {
			logger.Warn("shuttle: pending send not cleared", "strandGUID", sc.guid, "error", clearErr)
		}
		return false
	}
	if !found {
		return false
	}
	if err := clearPendingSend(sc.runDir); err != nil {
		logger.Warn("shuttle: pending send not cleared, sending as usual", "strandGUID", sc.guid, "error", err)
		return false
	}
	return pending.Text == text && !sc.startsNoTurn && turnStartedSince(sc.engine, sc.eventsPath, pending.EventsOffset)
}

// confirmPasted returns nil once the paste is seen to land, and an error when it is not.
// The input box is read first after reader.SubmitSettle(), then at a doubling interval capped at confirmBackoffCap.
// The window closeAt cuts the last interval short.
// A turn start past sentAt means the paste landed.
// For a send that starts no turn, only a turn start whose instruction is empty or the pasted text counts:
// a provider records no instruction for a slash command it took as a prompt,
// and the send fails with ErrCommandTakenAsPrompt.
// A turn start with another instruction, such as an operator's prompt, is no signal for such a send;
// the checks below decide it.
// When landingStart is set and the engine parses session signals, a session start of that source past sentAt does too.
// For such a send, an empty box does not end the reads: the Enter is taken as sent, and later reads send no key.
// The reads go on until a landing shows, or the window or read cap runs out; then the send returns nil.
// For a send without that session start, an empty box means the paste landed.
// A box holding exactly the sent text, or the provider's placeholder for it, means the Enter did not take:
// an Enter is sent, unless an operator is active, and the box is read again.
// A box with the sent text beside other text fails with ErrSentTextInDraft,
// and any other text fails with ErrForeignBoxText quoting the box; neither sends a key.
// An unreadable box sends nothing and is read again.
// A window that closes before a landing or an empty box fails with ErrSubmissionNotLanded.
func confirmPasted(sc sendContext, paster TextPaster, reader InputBoxReader, text string, sentAt int64, closeAt time.Time) error {
	normalized := normalizePaneText(text)
	settle := reader.SubmitSettle()
	interval := settle
	maxReads := windowPollCap(submitConfirmTimeout(sc.cfg), idlePollInitial)
	submitted := false
	for read := 1; ; read++ {
		wait := min(interval, closeAt.Sub(sc.clock.Now()))
		sc.clock.Sleep(max(wait, settle))
		if sc.startsNoTurn {
			if commandTurnStartedSince(sc.engine, sc.eventsPath, sentAt, text) {
				return withPaneTail(fmt.Errorf("%w: a turn started after the paste of %q", ErrCommandTakenAsPrompt, text), "", sc)
			}
		} else if turnStartedSince(sc.engine, sc.eventsPath, sentAt) {
			return nil
		}
		_, parsesSignals := sc.engine.(SessionSignalParser)
		awaitsLanding := sc.landingStart != "" && parsesSignals
		if awaitsLanding && sessionStartedSince(sc.engine, sc.eventsPath, sentAt, sc.landingStart) {
			return nil
		}
		if boxText, readable := readInputBox(sc, reader); readable && !submitted {
			box := normalizePaneText(boxText)
			switch {
			case box == "" && !awaitsLanding:
				return nil
			case box == "":
				submitted = true
			case box == normalized || paster.OwnPastePlaceholder(boxText, text):
				if _, err := guardedEnter(sc); err != nil {
					return err
				}
			case strings.Contains(box, normalized):
				return withPaneTail(fmt.Errorf("%w: the input box holds %q", ErrSentTextInDraft, strings.TrimSpace(boxText)), "", sc)
			default:
				return withPaneTail(fmt.Errorf("%w: %q", ErrForeignBoxText, strings.TrimSpace(boxText)), "", sc)
			}
		}
		if windowClosed(sc.clock, closeAt) || read >= maxReads {
			if submitted {
				return nil
			}
			reason := fmt.Errorf("%w: the pasted text is still pending, or its box unreadable, after %d read(s) within the %s submit window", ErrSubmissionNotLanded, read, submitConfirmTimeout(sc.cfg))
			return failUnlanded(sc, reader, normalized, reason)
		}
		interval = min(max(2*interval, idlePollInitial), confirmBackoffCap)
	}
}

// guardedEnter sends Enter into the strand's pane unless an operator was active there within the quiet window.
// The activity check runs immediately before the key.
// sent is false for a held Enter, which is no error; the caller reads the box again.
func guardedEnter(sc sendContext) (sent bool, err error) {
	if operatorActivityHold(sc.reed, sc.guid, sc.activityFailures) != "" {
		return false, nil
	}
	if err := sc.reed.SendKey(sc.guid, "Enter"); err != nil {
		return false, err
	}
	return true, nil
}

// settleAndConfirm runs the box path of a delivered send:
// it waits for the input box to settle, sends the Enter, and confirms the submission.
// A submission that did not land gets one look at the box: the send succeeds when that look finds the box empty.
func settleAndConfirm(sc sendContext, reader InputBoxReader, normalized string, sentAt int64, closeAt time.Time) error {
	err := awaitSettledBox(sc, reader, closeAt)
	if err == nil {
		err = confirmSubmitted(sc, reader, normalized, sentAt, closeAt)
	}
	if errors.Is(err, ErrSubmissionNotLanded) {
		return failUnlanded(sc, reader, normalized, err)
	}
	return err
}

// failUnlanded turns an ErrSubmissionNotLanded reason into the send's result.
// An empty box means the submission landed late and the send succeeds;
// otherwise the error carries the note from clearing the box and ends with the pane's last lines.
func failUnlanded(sc sendContext, reader InputBoxReader, normalized string, reason error) error {
	landed, note := clearUnlandedText(sc, reader, normalized)
	if landed {
		return nil
	}
	return withPaneTail(reason, note, sc)
}

// withPaneTail returns err extended with note, when non-empty, and the pane's last lines; it still wraps err.
func withPaneTail(err error, note string, sc sendContext) error {
	if note != "" {
		note = "; " + note
	}
	return fmt.Errorf("%w%s. The pane's last lines:\n%s", err, note, paneTail(sc.reed, sc.guid))
}

// awaitSettledBox reads the input box every Config.SubmitSettleMS until two consecutive reads agree.
// This keeps any Enter out of a typing burst.
// A collapsed paste placeholder is content like any other, and two reads that both show no readable box agree.
// A box still unsettled when closeAt arrives, or past the window's read count, fails with ErrSubmissionNotLanded.
// So does a box whose agreeing read comes at or after closeAt, so the first Enter never goes out after the window.
func awaitSettledBox(sc sendContext, reader InputBoxReader, closeAt time.Time) error {
	interval := time.Duration(sc.cfg.SubmitSettleMS) * time.Millisecond
	maxReads := windowPollCap(submitConfirmTimeout(sc.cfg), interval)
	previousText, previousOK := readInputBox(sc, reader)
	for read := 1; ; read++ {
		if windowClosed(sc.clock, closeAt) || read >= maxReads {
			return fmt.Errorf("%w: the input box was still changing after %d read(s) within the %s submit window; no Enter was sent", ErrSubmissionNotLanded, read, submitConfirmTimeout(sc.cfg))
		}
		sc.clock.Sleep(interval)
		text, ok := readInputBox(sc, reader)
		if text == previousText && ok == previousOK {
			if windowClosed(sc.clock, closeAt) {
				return fmt.Errorf("%w: the input box settled only after the %s submit window closed; no Enter was sent", ErrSubmissionNotLanded, submitConfirmTimeout(sc.cfg))
			}
			return nil
		}
		previousText, previousOK = text, ok
	}
}

// confirmSubmitted sends an Enter and then reads the input box after it.
// The first wait is reader.SubmitSettle(), and each later wait doubles up to confirmBackoffCap.
// A wait is cut to end at closeAt, but a read never comes sooner than SubmitSettle() after its Enter.
// A read that finds the box still holding the sent text sends another Enter while the window is open;
// a read at or after closeAt, or after the window's Enter count, sends no Enter,
// and the loop ends with ErrSubmissionNotLanded.
// Every Enter is followed by exactly one read.
// The submission is confirmed when the box no longer holds the sent text.
// It is also confirmed when the engine's session signals show a turn start past sentAt.
// normalized is the whole sent text normalized by normalizePaneText.
func confirmSubmitted(sc sendContext, reader InputBoxReader, normalized string, sentAt int64, closeAt time.Time) error {
	settle := reader.SubmitSettle()
	interval := settle
	maxEnters := windowPollCap(submitConfirmTimeout(sc.cfg), idlePollInitial)
	for enter := 1; ; enter++ {
		if err := sc.reed.SendKey(sc.guid, "Enter"); err != nil {
			return err
		}
		wait := min(interval, closeAt.Sub(sc.clock.Now()))
		sc.clock.Sleep(max(wait, settle))
		if turnStartedSince(sc.engine, sc.eventsPath, sentAt) || !inputBoxHoldsSentText(sc.reed, reader, sc.guid, normalized) {
			return nil
		}
		if windowClosed(sc.clock, closeAt) || enter >= maxEnters {
			return fmt.Errorf("%w: the sent text is still pending in the input box after %d Enter(s) within the %s submit window", ErrSubmissionNotLanded, enter, submitConfirmTimeout(sc.cfg))
		}
		interval = min(max(2*interval, idlePollInitial), confirmBackoffCap)
	}
}

// clearUnlandedText looks at the input box once after a send that did not land.
// An empty box means the submission landed late, so landed is true.
// For an engine with both the idle reading and InputBoxClearer, a box holding exactly this send's own text is cleared,
// and the box is read again.
// A box holding anything else gets no key.
// A send that keeps its unlanded text leaves the box as it is.
// note says what was found or done, and names a box the clear did not empty.
func clearUnlandedText(sc sendContext, reader InputBoxReader, normalized string) (landed bool, note string) {
	boxText, ok := readInputBox(sc, reader)
	if !ok {
		return false, ""
	}
	box := normalizePaneText(boxText)
	if box == "" {
		return true, ""
	}
	if sc.keepUnlanded {
		return false, "the unlanded text was left in the input box"
	}
	_, idleReading := sc.engine.(SessionCycler)
	clearer, canClear := sc.engine.(InputBoxClearer)
	if !idleReading || !canClear {
		return false, ""
	}
	if box != normalized {
		return false, "the input box holds text that is not this send's, left in place"
	}
	if err := playInputs(sc.reed, sc.guid, clearer.ClearInputSequence()); err != nil {
		return false, fmt.Sprintf("clearing the unlanded text failed: %v", err)
	}
	if after, afterOK := readInputBox(sc, reader); !afterOK || normalizePaneText(after) != "" {
		return false, fmt.Sprintf("the input box still holds %q after the clear", after)
	}
	return false, "the unlanded text was cleared from the input box"
}

// readInputBox returns the engine's reading of the strand's input box.
// ok is false when the capture failed or shows no readable box.
func readInputBox(sc sendContext, reader InputBoxReader) (text string, ok bool) {
	capture, err := boxCapture(sc.reed, sc.engine, sc.guid)
	if err != nil {
		return "", false
	}
	return reader.InputBoxText(capture)
}

// eventsSize returns the byte size of the events file, or 0 when it cannot be read.
func eventsSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// turnStartedSince reports whether the events file holds a turn_start past byte offset.
// An engine that cannot parse session signals, or an unreadable events file, reports false.
func turnStartedSince(engine Engine, eventsPath string, offset int64) bool {
	return turnStartMatchedSince(engine, eventsPath, offset, func(SessionSignal) bool { return true })
}

// promptTurnStartedSince reports whether the events file holds a turn start past byte offset.
// Its instruction is promptLine, trimmed; a turn the operator or another send started never counts as the prompt's.
// An engine that cannot parse session signals, or an unreadable events file, reports false.
func promptTurnStartedSince(engine Engine, eventsPath string, offset int64, promptLine string) bool {
	instruction := strings.TrimSpace(promptLine)
	return turnStartMatchedSince(engine, eventsPath, offset, func(signal SessionSignal) bool { return signal.Instruction == instruction })
}

// commandTurnStartedSince reports whether the events file holds a turn start past byte offset.
// Its instruction must be empty or command, trimmed.
// A provider records no instruction for a slash command it took as a prompt, and a turn an operator typed carries one.
// An engine that cannot parse session signals, or an unreadable events file, reports false.
func commandTurnStartedSince(engine Engine, eventsPath string, offset int64, command string) bool {
	instruction := strings.TrimSpace(command)
	return turnStartMatchedSince(engine, eventsPath, offset, func(signal SessionSignal) bool {
		return signal.Instruction == "" || signal.Instruction == instruction
	})
}

// turnStartMatchedSince reports whether the events file holds a turn start past byte offset that match accepts.
// An engine that cannot parse session signals, or an unreadable events file, reports false.
func turnStartMatchedSince(engine Engine, eventsPath string, offset int64, match func(SessionSignal) bool) bool {
	parser, ok := engine.(SessionSignalParser)
	if !ok {
		return false
	}
	data, err := os.ReadFile(eventsPath)
	if err != nil || offset > int64(len(data)) {
		return false
	}
	signals, _ := parser.ParseSessionSignals(data[offset:])
	for _, signal := range signals {
		if signal.Kind == SessionSignalTurnStart && match(signal) {
			return true
		}
	}
	return false
}

// sessionStartedSince reports whether the events file holds a session-start signal of source past byte offset.
// An engine that cannot parse session signals, or an unreadable events file, reports false.
func sessionStartedSince(engine Engine, eventsPath string, offset int64, source SessionStartSource) bool {
	parser, ok := engine.(SessionSignalParser)
	if !ok {
		return false
	}
	data, err := os.ReadFile(eventsPath)
	if err != nil || offset > int64(len(data)) {
		return false
	}
	signals, _ := parser.ParseSessionSignals(data[offset:])
	for _, signal := range signals {
		if signal.Kind == SessionSignalSessionStart && signal.Source == source {
			return true
		}
	}
	return false
}

// sendWithin sends text for the gated wait loop, validating it and holding sendMu for the send.
// The send's deadline is the run's, so the idle wait and the submit window both end by the run deadline.
// The idle wait shares the loop's turn-start hold,
// so a turn start the loop already released does not hold the send again.
func (run *Run) sendWithin(text string) error {
	if err := validateSendText(text); err != nil {
		return err
	}
	run.sendMu.Lock()
	defer run.sendMu.Unlock()
	sc := run.newSendContext()
	sc.deadline = run.deadline
	sc.hold = &run.startHold
	return sendVerified(sc, text)
}

// boundaryIdle confirms that the writer's turn boundary is idle in fact before the gate is evaluated for a send.
// An engine without an idle-session reading, or without session signal parsing, reports true at once.
// Otherwise a turn start that no later turn end matches means the writer began a new turn after the boundary.
// Then the boundary and the unsent re-prompt mark are cleared.
// The cleared state is recorded for startHoldReleased, and the call reports false.
func (run *Run) boundaryIdle() bool {
	engine := run.engine
	if _, ok := engine.(SessionCycler); !ok {
		return true
	}
	turnStart, _, found := unmatchedTurnStart(engine, run.state.EventsPath)
	if !found {
		return true
	}
	run.gateAtBoundary = false
	run.boundaryHeld = false
	run.unsentReprompt = false
	run.noBoxReprompts = 0
	run.startCleared = true
	run.startHold = turnStartHold{}
	logger.Info("shuttle: gate: a turn started after the boundary, holding the gate send", "strandGUID", run.state.StrandGUID, "turnStartAt", turnStart.At)
	return false
}

// startHoldReleased reports whether a boundary cleared by an unmatched turn start is restored on this tick.
// It reads the pane for the idle reading that times the hold.
// It restores the boundary once the hold releases the turn start.
// A turn start that a later turn end has since matched ends the cleared state without restoring the boundary.
// That turn end is either a gated Done arrival or a held turn end with nothing to evaluate.
func (run *Run) startHoldReleased() bool {
	engine := run.engine
	cycler := engine.(SessionCycler)
	turnStart, restarted, found := unmatchedTurnStart(engine, run.state.EventsPath)
	if !found {
		run.startCleared = false
		return false
	}
	capture, err := run.runner.reed.CapturePane(run.state.StrandGUID)
	paneIdle := err == nil && engine.Startup(capture) == StartupReady && cycler.IdleSession(capture)
	if run.startHold.holds(engine, turnStart, restarted, paneIdle, run.clock.Now()) {
		return false
	}
	run.startCleared = false
	run.gateAtBoundary = true
	return true
}
