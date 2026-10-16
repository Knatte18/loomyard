// interruptguard.go holds the guarded interrupt and the guarded rewind-picker close: every interrupt key shuttle sends into a pane passes the pane guard here.
package shuttleengine

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/Knatte18/loomyard/internal/logger"
)

// ErrNoRunningTurn marks an interrupt refused because the pane shows no running turn: there is nothing to stop.
// A caller that only needs the turn stopped treats it as success.
var ErrNoRunningTurn = errors.New("shuttle: the pane shows no running turn, so there is nothing to interrupt; way forward: \"lyx shuttle send <guid> <text>\" gives the agent a turn")

// ErrRewindPickerOpen marks an operation that found the provider's rewind picker open in the pane.
// It never wraps ErrSessionBusy, so no caller retries a picker as a busy session.
var ErrRewindPickerOpen = errors.New("shuttle: the rewind picker was open in the pane")

// guardedInterrupt plays the engine's interrupt sequence into guid's pane only when the pane shows a running turn.
// The pane must classify as a ready agent pane first, and one fresh capture decides.
// An open rewind picker is closed with one key and fails with ErrRewindPickerOpen.
// A pane with no running turn gets nothing and fails with ErrNoRunningTurn, and a capture that fails sends nothing.
// Bound: the guard only withholds a key, so a turn that ends between the capture and the key, or a capture quoting the running-turn hint, costs one key on an idle prompt, which opens nothing.
func guardedInterrupt(reed ReedOps, engine Engine, clock Clock, guid string) error {
	if err := requireReadyAgentPane(reed, engine, guid); err != nil {
		return err
	}
	guard, ok := engine.(InterruptGuard)
	if !ok {
		return fmt.Errorf("shuttle: the engine has no running-turn reading, so no interrupt key is sent to strand %q", guid)
	}
	capture, err := reed.CapturePane(guid)
	if err != nil {
		return fmt.Errorf("shuttle: capture strand %q's pane before the interrupt: %w", guid, err)
	}
	if err := closeRewindPicker(reed, engine, clock, guid, capture); err != nil {
		return err
	}
	if !guard.RunningTurn(capture) {
		return fmt.Errorf("%w (strand %q)", ErrNoRunningTurn, guid)
	}
	return playInputs(reed, guid, engine.InterruptSequence())
}

// ErrInterruptClearFailed marks an interrupt whose key was sent but whose clear of the restored prompt did not finish.
// A caller treats the interrupt as sent: the turn was stopped, and only the box may still hold the restored text.
var ErrInterruptClearFailed = errors.New("shuttle: the interrupt key was sent, but clearing the prompt it restored did not finish; treat the interrupt as sent")

// clearRestoredPrompt empties the prompt the provider restored into guid's input box after lyx's own interrupt key, and does nothing for an engine without RestoredPromptClearer, InputBoxReader and InterruptGuard.
// It waits the engine's interrupt settle on clock and reads the box.
// A box read empty gets one more settle and one more read, and a box still empty, or one the engine cannot read, returns nil.
// A box holding text is cleared only behind a guard reading taken from that same capture plus one operator-activity query:
// the provider pane reads ready with no running turn, and the operator is not active.
// A reading that fails any of these sends nothing and returns nil, leaving the text as a draft for the next send to hold on.
// The clear is the engine's RestoredPromptClearSequence, played at most once per call; the box then settles for SubmitSettle and is read once more, and text still there is logged and nothing more is sent.
// Its keys are the provider's restored-prompt clear, never an Escape, C-c or Enter.
// A capture, the activity query or the play failing is logged and returned wrapping ErrInterruptClearFailed, and a failed guard reading sends no clear key.
func clearRestoredPrompt(reed ReedOps, engine Engine, clock Clock, guid string) error {
	clearer, canClear := engine.(RestoredPromptClearer)
	reader, canRead := engine.(InputBoxReader)
	guard, canGuard := engine.(InterruptGuard)
	if !canClear || !canRead || !canGuard {
		return nil
	}
	read := func() (string, boxReading, error) {
		capture, err := boxCapture(reed, engine, guid)
		if err != nil {
			return "", boxReading{}, err
		}
		return capture, readBox(engine, capture), nil
	}
	settle := guard.InterruptSettle()
	clock.Sleep(settle)
	capture, box, err := read()
	if err != nil {
		return interruptClearFailed(guid, "capture the pane after the interrupt key", err)
	}
	if box.read && strings.TrimSpace(box.text) == "" {
		clock.Sleep(settle)
		if capture, box, err = read(); err != nil {
			return interruptClearFailed(guid, "capture the pane after the interrupt key", err)
		}
	}
	if !box.read || strings.TrimSpace(box.text) == "" {
		return nil
	}
	if engine.Startup(capture) != StartupReady || guard.RunningTurn(capture) {
		return nil
	}
	active, _, err := reed.OperatorActive(guid)
	if err != nil {
		return interruptClearFailed(guid, "read operator activity before the clear", err)
	}
	if active {
		return nil
	}
	if err := playInputs(reed, guid, clearer.RestoredPromptClearSequence(capture)); err != nil {
		return interruptClearFailed(guid, "clear the restored prompt", err)
	}
	clock.Sleep(reader.SubmitSettle())
	_, after, err := read()
	if err != nil {
		return interruptClearFailed(guid, "capture the pane after the clear", err)
	}
	if after.read && strings.TrimSpace(after.text) != "" {
		logger.Info("shuttle: the restored prompt survived its clear, sending nothing more", "strandGUID", guid)
	}
	return nil
}

// interruptClearFailed logs a failure after the interrupt key was sent and returns it wrapping ErrInterruptClearFailed.
func interruptClearFailed(guid, step string, err error) error {
	logger.Warn("shuttle: the interrupt key was sent but the restored-prompt clear failed", "strandGUID", guid, "step", step, "error", err)
	return fmt.Errorf("%w (strand %q: %s: %w)", ErrInterruptClearFailed, guid, step, err)
}

// refuseInterruptKey fails when inputs hold a key of the engine's interrupt sequence or a text holding a control character.
// Keys compare case-insensitively with tmux's "C-[" read as "Escape", since tmux delivers both as the ESC byte;
// a text is typed literally, so an ESC byte in it reaches the pane as the Escape key.
func refuseInterruptKey(engine Engine, inputs []PaneInput) error {
	interruptKeys := map[string]bool{}
	for _, input := range engine.InterruptSequence() {
		if input.Key != "" {
			interruptKeys[canonicalKeyName(input.Key)] = true
		}
	}
	for index, input := range inputs {
		if input.Key != "" && interruptKeys[canonicalKeyName(input.Key)] {
			return fmt.Errorf("shuttle: Inject: the inputs hold the interrupt key %q, which Inject never sends; way forward: \"lyx shuttle interrupt <guid>\" sends it behind the pane guard", input.Key)
		}
		if control := strings.IndexFunc(input.Text, unicode.IsControl); control >= 0 {
			return fmt.Errorf("shuttle: Inject: input %d's text holds the control character %q, which Inject never types; way forward: send a key as a Key input, and the interrupt key through \"lyx shuttle interrupt <guid>\"", index, []rune(input.Text[control:])[0])
		}
	}
	return nil
}

// canonicalKeyName lowercases a tmux key name and reads "C-[" as "escape".
func canonicalKeyName(key string) string {
	key = strings.ToLower(key)
	if key == "c-[" {
		return "escape"
	}
	return key
}

// closePickerBeforeSend gives a verified send's pane one capture and closes an open rewind picker in it, so nothing is typed into the picker.
// An engine that cannot read the picker pays no capture.
// A failed capture fails the send when failClosed is set, since nothing after it reads the pane again;
// otherwise the close is skipped and the idle wait's polls read the pane.
func closePickerBeforeSend(sc sendContext, failClosed bool) error {
	if _, ok := sc.engine.(RewindPickerReader); !ok {
		return nil
	}
	capture, err := sc.reed.CapturePane(sc.guid)
	if err != nil {
		if failClosed {
			return fmt.Errorf("shuttle: capture strand %q's pane before the send: %w", sc.guid, err)
		}
		return nil
	}
	return closeRewindPicker(sc.reed, sc.engine, sc.clock, sc.guid, capture)
}

// closeRewindPicker closes the provider's rewind picker when capture shows it, and otherwise does nothing.
// It sends exactly one key, the first key of the engine's interrupt sequence, waits the engine's interrupt settle and captures the pane once more.
// It never selects an entry and never sends a second key, and it fails with ErrRewindPickerOpen either way:
// the key closed the picker and nothing was typed or interrupted, or the picker or another dialog is still on screen.
func closeRewindPicker(reed ReedOps, engine Engine, clock Clock, guid, capture string) error {
	reader, ok := engine.(RewindPickerReader)
	if !ok || !reader.RewindPicker(capture) {
		return nil
	}
	var closeKey []PaneInput
	for _, input := range engine.InterruptSequence() {
		if input.Key != "" {
			closeKey = []PaneInput{input}
			break
		}
	}
	if err := playInputs(reed, guid, closeKey); err != nil {
		return fmt.Errorf("shuttle: close the rewind picker in strand %q: %w", guid, err)
	}
	if guard, ok := engine.(InterruptGuard); ok {
		clock.Sleep(guard.InterruptSettle())
	}
	after, err := reed.CapturePane(guid)
	var box boxReading
	if err == nil {
		box = readBox(engine, after)
	}
	if err != nil || (box.reader && !box.read) {
		return fmt.Errorf("%w: strand %q: one interrupt key was sent, but the picker or another dialog is still on screen; nothing more was sent, so close it in the pane, then retry", ErrRewindPickerOpen, guid)
	}
	return fmt.Errorf("%w: strand %q: one interrupt key closed it; nothing was typed or interrupted, so read the pane before giving the session a turn", ErrRewindPickerOpen, guid)
}
