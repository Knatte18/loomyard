// startup.go implements Startup (classifying a pane's capture during the launch window) and the fixed key-choreography sequences — InterruptSequence, ComposeSend, and ModelSwitchSequence — that the run loop and long-lived callers send into a pane to interrupt a turn, resume one, or switch the session's active model.
// All are pure over a capture string / literal text — the classification heuristics were proven live against a real claude TUI (docs/research/reed-hooks-exploration.md and reedcli's dismissTrust).

package claudeengine

import (
	"strings"
	"unicode"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// trustDialogNeedles are whitespace-stripped, lowercased phrases identifying claude's trust-this-folder gate.
var trustDialogNeedles = []string{"trustthisfolder", "filesinthisfolder"}

// Startup classifies the pane's rendered content during launch.
// Trust gate is checked FIRST (the real dialog contains the "❯" ready marker as its selection caret).
// Then ready markers (the input marker "❯" or the footer hint "shortcuts") are checked; anything else is still booting.
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

// InterruptSequence returns the key choreography that interrupts a claude turn: a single Escape key press.
func (c *Claude) InterruptSequence() []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Key: "Escape"}}
}

// TrustDismissSequence returns the key choreography that dismisses the trust gate: a single Enter key press.
func (c *Claude) TrustDismissSequence() []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Key: "Enter"}}
}

// composeSendSettleMS is the pause after ComposeSend's leading Escape before its text step.
// Without this gap, the Escape and text bytes can coalesce into an escape-sequence read and be discarded.
const composeSendSettleMS = 300

// ComposeSend returns the key choreography that submits text as claude's next turn.
// Escape is sent first to clear leaked auto-suggest, with a settle pause before text is typed and submitted.
func (c *Claude) ComposeSend(text string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{
		{Key: "Escape", SettleMS: composeSendSettleMS},
		{Text: text, Submit: true},
	}
}

// ModelSwitchSequence returns the key choreography that switches a live claude session's model: the `/model <name>` slash command.
// Unlike ComposeSend, it sends NO leading Escape (injected mid-tool-call, Escape there interrupts the tool and aborts the turn).
func (c *Claude) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{
		{Text: "/model " + model, Submit: true},
	}
}
