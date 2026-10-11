// send.go holds what a verified send does around its typing: the context it runs in, the wait for an idle session, the settle, submit confirmation and clear of an input-box send, and the gated wait loop's send and boundary-idle checks.
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

// ErrSessionBusy marks a verified send that found the session still held by its readiness reading when its idle wait ran out: a turn running, a question awaiting an answer, a draft in the input box, an unmatched turn start, an operator active in the pane within reed's quiet window, a pane showing no input box to an engine that reads one or, when the reading is unknown, a pane that is not idle.
// Nothing was typed, so the caller may retry the send later.
var ErrSessionBusy = errors.New("shuttle: the session is not idle")

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

// sendContext is what one verified send needs: the pane it types into, the run's record, directory and events file, the config and the clock its timed waits read.
// A zero deadline means none, and a nil hold starts the idle wait's turn-start hold afresh.
// startsNoTurn marks a send whose text starts no turn, such as a slash command, so no turn start reads as its landing.
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
}

// newSendContext returns the context of a verified send into run's own pane.
func (run *Run) newSendContext() sendContext {
	return sendContext{
		reed:       run.runner.reed,
		engine:     run.runner.engine,
		guid:       run.state.StrandGUID,
		state:      run.state,
		runDir:     run.runDir,
		eventsPath: run.state.EventsPath,
		cfg:        run.runner.cfg,
		clock:      run.clock,
	}
}

// newSendContext returns the context of a verified send into the pane of the run state names, whose directory is runDir.
func (r *Runner) newSendContext(state RunState, runDir string) sendContext {
	return sendContext{
		reed:       r.reed,
		engine:     r.engine,
		guid:       state.StrandGUID,
		state:      state,
		runDir:     runDir,
		eventsPath: state.EventsPath,
		cfg:        r.cfg,
		clock:      r.clock,
	}
}

// sendReadyTimeout returns how long a verified send waits for an idle session.
// A non-positive value in a hand-built Config is floored to the template default.
func sendReadyTimeout(cfg Config) time.Duration {
	if cfg.SendReadyTimeoutS <= 0 {
		return defaultSendReadyTimeoutS * time.Second
	}
	return time.Duration(cfg.SendReadyTimeoutS) * time.Second
}

// windowPollCap returns how many polls paced at least minInterval apart fit in window, plus slack for a first poll at once and a last sleep cut short.
// Every poll of a send runs a real tmux process through reed, so this attempt-count bound sits beside the window and also ends the loop under a clock that never advances.
// A non-positive minInterval is paced as idlePollInitial.
func windowPollCap(window, minInterval time.Duration) int {
	if minInterval <= 0 {
		minInterval = idlePollInitial
	}
	return int(window/minInterval) + 2
}

// awaitIdleSession returns nil once the strand's session is idle in fact, and fails with an error wrapping ErrSessionBusy when it stays busy past the send-ready window, sc's deadline or the window's poll count.
// An engine without the SessionCycler idle reading keeps requireReadyAgentPane alone, with no wait.
// Otherwise every poll takes the session's readiness reading, which is built on the hook-derived session state;
// when that state is unknown the poll falls back to the pane, which must classify ready and idle, and for an engine that parses session signals no turn start may be left unmatched by a later turn end, unless the pane has read idle for turnStartIdleOverride or the engine reports that turn interrupted.
func awaitIdleSession(sc sendContext) error {
	cycler, ok := sc.engine.(SessionCycler)
	if !ok {
		if err := requireReadyAgentPane(sc.reed, sc.engine, sc.guid); err != nil {
			return err
		}
		if hold := operatorActivityHold(sc.reed, sc.guid); hold != "" {
			return fmt.Errorf("%w: strand %q: %s; retry once the operator is idle", ErrSessionBusy, sc.guid, hold)
		}
		return nil
	}
	if err := requireLiveStrand(sc.reed, sc.guid); err != nil {
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
	for poll := 1; ; poll++ {
		busy := busyReading(sc, cycler, hold)
		if busy == "" {
			return nil
		}
		now := sc.clock.Now()
		if !now.Before(limit) || poll >= maxPolls {
			return fmt.Errorf("%w: strand %q: %s; retry once the session is idle. The pane's last lines:\n%s", ErrSessionBusy, sc.guid, busy, paneTail(sc.reed, sc.guid))
		}
		sc.clock.Sleep(min(interval, limit.Sub(now)))
		interval = min(interval*2, idlePollMax)
	}
}

// busyReading polls the pane and the events file once and returns what keeps the session from being idle, or "" when it is idle.
// The readiness reading decides: a ready reading returns "" and a held one its reason.
// An unknown reading falls back to the pane, logging its cause at Debug.
// Operator activity is asked last, and only once the rest of the reading passed, so each poll is one fresh reading of readiness, box and activity.
func busyReading(sc sendContext, cycler SessionCycler, hold *turnStartHold) string {
	capture, err := sc.reed.CapturePane(sc.guid)
	if err != nil {
		return fmt.Sprintf("the pane could not be captured: %v", err)
	}
	reading := sessionReadiness(sc, cycler, hold, capture)
	switch {
	case reading.ready:
		return operatorActivityHold(sc.reed, sc.guid)
	case !reading.unknown:
		return reading.reason
	}
	logger.Debug("shuttle: send readiness unknown, reading the pane", "strandGUID", sc.guid, "cause", reading.cause)

	ready := sc.engine.Startup(capture) == StartupReady
	paneIdle := ready && cycler.IdleSession(capture)

	var unmatched string
	if turnStart, found := unmatchedTurnStart(sc.engine, sc.eventsPath); found && hold.holds(sc.engine, turnStart, paneIdle, sc.clock.Now()) {
		unmatched = fmt.Sprintf("a turn start at %s has no turn end", turnStart.At.Format(time.RFC3339))
	}
	switch {
	case !ready:
		return "the pane shows no input-ready provider"
	case !paneIdle:
		return "the pane is not idle"
	case unmatched != "":
		return unmatched
	}
	return operatorActivityHold(sc.reed, sc.guid)
}

// operatorActivityHold asks reed once whether an attached client was active in the strand's pane within the quiet window, and returns the reason a write is held for it, or "" when none.
// A query error is logged at Warn and holds nothing, so a tmux that cannot list its clients stalls no run.
func operatorActivityHold(reed ReedOps, guid string) string {
	active, quiet, err := reed.OperatorActive(guid)
	if err != nil {
		logger.Warn("shuttle: operator activity unreadable, not holding the write", "strandGUID", guid, "error", err)
		return ""
	}
	if !active {
		return ""
	}
	return fmt.Sprintf("the operator typed in this pane within the last %d s", int(quiet.Seconds()))
}

// unmatchedTurnStart returns the newest turn_start in the events file that no later turn_end or api_error_turn_end follows.
// An engine without SessionSignalParser and an unreadable file read as none.
func unmatchedTurnStart(engine Engine, eventsPath string) (SessionSignal, bool) {
	parser, ok := engine.(SessionSignalParser)
	if !ok {
		return SessionSignal{}, false
	}
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		return SessionSignal{}, false
	}
	signals, _ := parser.ParseSessionSignals(data)
	var pending SessionSignal
	found := false
	for _, signal := range signals {
		switch signal.Kind {
		case SessionSignalTurnStart:
			pending, found = signal, true
		case SessionSignalTurnEnd, SessionSignalAPIErrorTurnEnd:
			found = false
		}
	}
	return pending, found
}

// turnStartHold decides, poll by poll, whether an unmatched turn start still holds a send against a pane that reads idle.
// The zero value holds nothing yet.
type turnStartHold struct {
	signal    SessionSignal
	tracking  bool
	idleSince time.Time
	released  bool
}

// holds reports whether turnStart, the unmatched turn start, still holds the send at now.
// It releases once the engine's SessionProber reports that turn interrupted,
// or once paneIdle has been true on every poll since turnStartIdleOverride ago, logging one Warn naming the signal.
// A poll with a busy pane resets the idle timer.
// A turn start once released stays released for as long as it is the one asked about.
func (h *turnStartHold) holds(engine Engine, turnStart SessionSignal, paneIdle bool, now time.Time) bool {
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
	var lines []string
	for _, line := range strings.Split(capture, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimRight(line, " \t\r"))
		}
	}
	if len(lines) > paneTailLines {
		lines = lines[len(lines)-paneTailLines:]
	}
	return strings.Join(lines, "\n")
}

// ErrSubmissionNotLanded marks a verified send whose text was never seen to be submitted: the submit window closed before typing began, the typed text did not appear, its input box did not settle, or the box still held it when the window closed.
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

// ErrSentTextInDraft marks a pasted send whose text was found in the input box beside other text: the operator's draft and the send have merged.
// It wraps ErrSessionBusy, so a caller that retries a busy session retries this too.
// Nothing was submitted, cleared or edited.
var ErrSentTextInDraft = fmt.Errorf("%w: the sent text sits in the operator's draft", ErrSessionBusy)

// pasteVerified writes text into the pane as one paste through reed and confirms it was submitted.
// The paste carries its trailing Enter when the provider's Enter submits a paste; otherwise the box is settled first, so the guarded Enter lands on a redrawn box.
// The send's window opens before the paste and is cut to the send's deadline.
func pasteVerified(sc sendContext, paster TextPaster, reader InputBoxReader, text string) error {
	closeAt := sc.clock.Now().Add(submitConfirmTimeout(sc.cfg))
	if !sc.deadline.IsZero() && sc.deadline.Before(closeAt) {
		closeAt = sc.deadline
	}
	if windowClosed(sc.clock, closeAt) {
		return withPaneTail(fmt.Errorf("%w: the submit window, cut to the send's deadline, had closed before the paste; nothing was pasted", ErrSubmissionNotLanded), "", sc)
	}
	sentAt := eventsSize(sc.eventsPath)
	if err := sc.reed.PasteSubmit(sc.guid, text, paster.PasteSubmits()); err != nil {
		return fmt.Errorf("shuttle: paste text: %w", err)
	}
	err := settleAndConfirmPasted(sc, paster, reader, text, sentAt, closeAt)
	if errors.Is(err, ErrSentTextInDraft) || errors.Is(err, ErrSubmissionNotLanded) {
		if saveErr := savePendingSend(sc.runDir, pendingSend{Text: text, EventsOffset: sentAt}); saveErr != nil {
			logger.Warn("shuttle: pending send not recorded, a retry may type the text twice", "strandGUID", sc.guid, "error", saveErr)
		}
	}
	return err
}

// settleAndConfirmPasted runs what follows a paste: the box settles first when the paste carries no Enter, then confirmPasted reads the box until the send lands or the window closes.
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
// An empty choreography, a provider's no-command answer, returns nil before the guard, with no idle wait, no activity query and no paste.
// An engine that takes pastes sends the choreography's text as a verified send, so a slash command is pasted once and confirmed by the box reading empty.
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

// retryLanded reports whether the pending-send record of an earlier failed send of this same text shows the text went out, so the retry types nothing.
// The record is cleared whatever it says.
// It reads as landed when the events file holds a turn start past the recorded offset, which is the text going out with the draft or with an Enter the operator pressed.
// A text that starts no turn never reads as landed, since a turn start past the offset is some other turn.
// A record for another text, an unreadable record and one that cannot be cleared read as not landed, the last two logged at Warn, so a broken file never wedges a strand.
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

// confirmPasted reads the input box after a paste, first reader.SubmitSettle() later and then at a doubling interval up to confirmBackoffCap, cut to end at closeAt.
// A turn start past sentAt, or an empty box, means the paste landed.
// A box holding exactly the sent text, or the provider's placeholder for it, means the Enter did not take, and the guarded Enter is sent and the box read again.
// No readable box sends nothing and is read again.
// The sent text beside other text fails with ErrSentTextInDraft, and any other text fails with ErrSessionBusy quoting the box; neither sends a key.
// A window that ends on an exact match or on no readable box fails with ErrSubmissionNotLanded.
func confirmPasted(sc sendContext, paster TextPaster, reader InputBoxReader, text string, sentAt int64, closeAt time.Time) error {
	normalized := normalizePaneText(text)
	settle := reader.SubmitSettle()
	interval := settle
	maxReads := windowPollCap(submitConfirmTimeout(sc.cfg), idlePollInitial)
	for read := 1; ; read++ {
		wait := min(interval, closeAt.Sub(sc.clock.Now()))
		sc.clock.Sleep(max(wait, settle))
		if turnStartedSince(sc.engine, sc.eventsPath, sentAt) {
			return nil
		}
		if boxText, readable := readInputBox(sc, reader); readable {
			box := normalizePaneText(boxText)
			switch {
			case box == "":
				return nil
			case box == normalized || paster.OwnPastePlaceholder(boxText, text):
				if _, err := guardedEnter(sc); err != nil {
					return err
				}
			case strings.Contains(box, normalized):
				return withPaneTail(fmt.Errorf("%w: the input box holds %q", ErrSentTextInDraft, strings.TrimSpace(boxText)), "", sc)
			default:
				return withPaneTail(fmt.Errorf("%w: the input box holds text that is not this send's: %q", ErrSessionBusy, strings.TrimSpace(boxText)), "", sc)
			}
		}
		if windowClosed(sc.clock, closeAt) || read >= maxReads {
			reason := fmt.Errorf("%w: the pasted text is still pending, or its box unreadable, after %d read(s) within the %s submit window", ErrSubmissionNotLanded, read, submitConfirmTimeout(sc.cfg))
			return failUnlanded(sc, reader, normalized, reason)
		}
		interval = min(max(2*interval, idlePollInitial), confirmBackoffCap)
	}
}

// guardedEnter sends Enter into the strand's pane unless an operator was active there within the quiet window, asked immediately before the key.
// sent is false for a held Enter, which is no error; the caller reads the box again.
func guardedEnter(sc sendContext) (sent bool, err error) {
	if operatorActivityHold(sc.reed, sc.guid) != "" {
		return false, nil
	}
	if err := sc.reed.SendKey(sc.guid, "Enter"); err != nil {
		return false, err
	}
	return true, nil
}

// settleAndConfirm runs the box path of a delivered send: wait for the input box to settle, send the Enter and confirm the submission.
// A send that did not land is handed to failUnlanded, which returns nil when the submission landed late.
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
// otherwise the error carries clearUnlandedText's note and ends with the pane's last lines.
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

// awaitSettledBox reads the input box every Config.SubmitSettleMS until two consecutive reads agree, so no Enter lands inside a typing burst.
// A collapsed paste placeholder is content like any other, and two reads that both show no readable box agree.
// A box that has not settled when closeAt arrives, or within the window's read count, fails with ErrSubmissionNotLanded, and so does one whose agreeing read comes at or after closeAt, so the first Enter never goes out after the window.
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

// confirmSubmitted sends the first Enter and then reads the input box at an interval that starts at reader.SubmitSettle() and doubles up to confirmBackoffCap.
// An interval that would end past closeAt is cut to end at it, but a read never comes sooner than SubmitSettle() after its Enter.
// A read taken while the window is open that shows the box still holding the sent text sends one more Enter;
// a read at or after closeAt, or after the window's Enter count, sends none and ends the loop, so every Enter is followed by exactly one read.
// The submission is confirmed when the box no longer holds the sent text, or when the engine's session signals show a turn start past offset sentAt.
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
// For an engine with both the idle reading and InputBoxClearer, a box that holds exactly this send's own text is cleared and read again;
// a box holding anything else gets no key.
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

// readInputBox returns the engine's reading of the strand's input box; ok is false when the capture failed or shows no readable box.
func readInputBox(sc sendContext, reader InputBoxReader) (text string, ok bool) {
	capture, err := sc.reed.CapturePane(sc.guid)
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
// An engine without SessionSignalParser and an unreadable file read as no.
func turnStartedSince(engine Engine, eventsPath string, offset int64) bool {
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
		if signal.Kind == SessionSignalTurnStart {
			return true
		}
	}
	return false
}

// sendWithin is the gated wait loop's send: Run.Send's validation and sendVerified with the send's deadline set to the run's,
// so the idle wait and the submit window both end by the run deadline.
// The idle wait shares the loop's turn-start hold, so a turn start the loop already released does not hold the send again.
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
// An engine lacking the SessionCycler idle reading or the session signal parser answers true at once.
// Otherwise a turn start left unmatched by a later turn end means the writer began a new turn after the boundary:
// the boundary and the unsent re-prompt mark are cleared, the cleared state is recorded for startHoldReleased, and the answer is false.
func (run *Run) boundaryIdle() bool {
	engine := run.runner.engine
	if _, ok := engine.(SessionCycler); !ok {
		return true
	}
	turnStart, found := unmatchedTurnStart(engine, run.state.EventsPath)
	if !found {
		return true
	}
	run.gateAtBoundary = false
	run.unsentReprompt = false
	run.startCleared = true
	run.startHold = turnStartHold{}
	logger.Info("shuttle: gate: a turn started after the boundary, holding the gate send", "strandGUID", run.state.StrandGUID, "turnStartAt", turnStart.At)
	return false
}

// startHoldReleased reports whether a boundary cleared by an unmatched turn start is restored on this tick.
// It reads the pane for the idle reading that times the hold, and restores the boundary once turnStartHold releases the turn start.
// A turn start that a later turn end has since matched ends the cleared state without restoring the boundary.
// That turn end is either a gated Done arrival or a held turn end with nothing to evaluate.
func (run *Run) startHoldReleased() bool {
	engine := run.runner.engine
	cycler := engine.(SessionCycler)
	turnStart, found := unmatchedTurnStart(engine, run.state.EventsPath)
	if !found {
		run.startCleared = false
		return false
	}
	capture, err := run.runner.reed.CapturePane(run.state.StrandGUID)
	paneIdle := err == nil && engine.Startup(capture) == StartupReady && cycler.IdleSession(capture)
	if run.startHold.holds(engine, turnStart, paneIdle, run.clock.Now()) {
		return false
	}
	run.startCleared = false
	run.gateAtBoundary = true
	return true
}
