// startup.go implements Startup (classifying a pane's capture during the launch window) and the
// fixed key-choreography sequences — InterruptSequence, ComposeSend, and ModelSwitchSequence — that
// the run loop and long-lived callers send into a pane to interrupt a turn, resume one, or switch
// the session's active model.
// All are pure over a capture string / literal text — the classification heuristics were proven
// live against a real claude TUI (docs/research/reed-hooks-exploration.md and reedcli's
// dismissTrust).

package claudeengine

import (
	"strings"
	"unicode"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// trustDialogNeedles are whitespace-stripped, lowercased phrases identifying claude's trust-this-folder gate.
var trustDialogNeedles = []string{"trustthisfolder", "filesinthisfolder"}

// Startup classifies the pane's rendered content during launch.
// Trust gate is checked FIRST (the real dialog contains the "❯" ready marker as its selection
// caret).
// Then ready markers (the input marker "❯" or the footer hint "shortcuts") are checked; anything
// else is still booting.
func (c *Claude) Startup(capture string) shuttleengine.StartupState {
	normalized := normalizeCapture(capture)
	for _, needle := range trustDialogNeedles {
		if strings.Contains(normalized, needle) {
			return shuttleengine.StartupTrustPrompt
		}
	}
	if strings.Contains(capture, "❯") || strings.Contains(normalized, "shortcuts") {
		return shuttleengine.StartupReady
	}
	return shuttleengine.StartupPending
}

// normalizeCapture lowercases and strips whitespace from capture, the canonical form for matching phrase needles.
func normalizeCapture(capture string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, capture)
}

// InterruptSequence returns the key choreography that interrupts a claude turn: a single Escape key
// press.
func (c *Claude) InterruptSequence() []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Key: "Escape"}}
}

// trustAcceptNeedles are whitespace-stripped, lowercased phrases identifying the trust gate's
// ACCEPTING option line, as distinct from trustDialogNeedles, which identify the gate itself.
// They are deliberately narrower than trustDialogNeedles: the gate's own prose paragraph asks
// whether this is "a project you created or one you trust", so a needle broad enough to match the
// paragraph would locate the wrong line and move the caret to nowhere.
// Two spellings are carried rather than one so a reworded option ("Yes, I trust this directory")
// still matches; both are substrings of claude 2.1.263's own "Yes, I trust this folder".
var trustAcceptNeedles = []string{"trustthisfolder", "yes,itrust"}

// trustCaretMarker is the glyph claude renders beside the currently-selected option of a select
// list, the same marker Startup already reads as its ready marker.
const trustCaretMarker = "❯"

// trustSelectSettleMS is the pause after each caret-moving key press, so a burst of arrow keys is
// not coalesced into a single escape-sequence read and silently dropped — the same hazard
// ComposeSend's own leading pause exists for.
const trustSelectSettleMS = 150

// TrustDismissSequence returns the key choreography that ACCEPTS claude's trust gate as rendered in
// capture: enough Down (or Up) presses to move the caret from wherever claude put it onto the
// "Yes, I trust this folder" option, then Enter.
//
// It is capture-driven rather than a fixed single Enter, and that is not a refinement — the fixed
// form was a live-confirmed defect. Claude 2.1.263 renders the gate with the caret on the REFUSING
// option:
//
//	❯ No, exit
//	  Yes, I trust this folder
//
// so a bare Enter confirmed "No, exit" and claude quit. Every agent lyx spawned in a directory
// claude had not previously been accepted in therefore died at startup, reported only as an opaque
// pane-died outcome (crucible round opus-medium-r5, R5-2). Which option the caret starts on is the
// provider's choice and has already changed once, so this reads the caret rather than assuming it.
//
// When capture carries no caret line, or no line naming the accepting option, it returns NO inputs
// at all rather than a blind Enter: pressing whatever is selected is exactly how the old form
// refused on lyx's behalf. The caller re-probes on its next liveness tick and the startup window
// bounds the retries, so returning nothing costs a bounded wait and never a wrong keypress.
func (c *Claude) TrustDismissSequence(capture string) []shuttleengine.PaneInput {
	lines := strings.Split(capture, "\n")

	// Last occurrence, not first: the gate is drawn at the BOTTOM of the pane, and everything
	// above it is scrollback that may carry both a stale caret and stale option text.
	caretLine, acceptLine := -1, -1
	for i, line := range lines {
		if strings.Contains(line, trustCaretMarker) {
			caretLine = i
		}
		normalized := normalizeCapture(line)
		for _, needle := range trustAcceptNeedles {
			if strings.Contains(normalized, needle) {
				acceptLine = i
				break
			}
		}
	}
	if caretLine == -1 || acceptLine == -1 {
		return nil
	}

	steps := acceptLine - caretLine
	key := "Down"
	if steps < 0 {
		key = "Up"
		steps = -steps
	}

	inputs := make([]shuttleengine.PaneInput, 0, steps+1)
	for i := 0; i < steps; i++ {
		inputs = append(inputs, shuttleengine.PaneInput{Key: key, SettleMS: trustSelectSettleMS})
	}
	return append(inputs, shuttleengine.PaneInput{Key: "Enter"})
}

// composeSendSettleMS is the pause after ComposeSend's leading Escape before its text step.
// Without this gap, the Escape and text bytes can coalesce into an escape-sequence read and be discarded.
const composeSendSettleMS = 300

// ComposeSend returns the key choreography that submits text as claude's next turn.
// Escape is sent first to clear leaked auto-suggest, with a settle pause before text is typed and
// submitted.
func (c *Claude) ComposeSend(text string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{
		{Key: "Escape", SettleMS: composeSendSettleMS},
		{Text: text, Submit: true},
	}
}

// ModelSwitchSequence returns the key choreography that switches a live claude session's model: the
// `/model <name>` slash command.
// Unlike ComposeSend, it sends NO leading Escape (injected mid-tool-call, Escape there interrupts
// the tool and aborts the turn).
func (c *Claude) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{
		{Text: "/model " + model, Submit: true},
	}
}
