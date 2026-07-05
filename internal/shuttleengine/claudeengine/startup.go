// startup.go implements Startup (classifying a pane's capture during the
// launch window) and the two fixed key-choreography sequences,
// InterruptSequence and ComposeSend, that the run loop sends into a pane to
// interrupt or resume a turn. All three are pure over a capture string /
// literal text — the classification heuristics were proven live against a
// real claude TUI (docs/research/mux-hooks-exploration.md and muxcli's
// dismissTrust).

package claudeengine

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Startup classifies capture, the pane's currently rendered content, during
// the window between launch and claude becoming ready for input. It checks
// the ready markers FIRST: the TUI's own input marker "❯" or the ASCII
// status hint "shortcuts" (from its "? for shortcuts" footer — robust
// across a non-ASCII-space rendering quirk that can corrupt "❯") means
// claude has reached its ready-for-input state, and that takes priority over
// the trust-prompt heuristic below. Readiness is checked first (not the
// trust prompt) because the trust match is a loose, case-insensitive
// substring test ("trust" and "folder" both present) that could in
// principle also match unrelated pane content (e.g. an agent's own message
// echoed onto the screen); checking readiness first means such a false
// trust-prompt match can never mask a pane that has, in fact, already
// become ready. Absent a ready marker, claude showing a one-time "do you
// trust this folder?" gate (the same substring heuristic muxcli's
// dismissTrust proved live) must be dismissed before any ready marker can
// appear. Anything else is still booting.
func (c *Claude) Startup(capture string) shuttleengine.StartupState {
	lower := strings.ToLower(capture)
	if strings.Contains(capture, "❯") || strings.Contains(lower, "shortcuts") {
		return shuttleengine.StartupReady
	}
	if strings.Contains(lower, "trust") && strings.Contains(lower, "folder") {
		return shuttleengine.StartupTrustPrompt
	}
	return shuttleengine.StartupPending
}

// InterruptSequence returns the key choreography that interrupts an
// in-progress claude turn: a single Escape key press.
func (c *Claude) InterruptSequence() []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Key: "Escape"}}
}

// ComposeSend returns the key choreography that submits text as claude's
// next turn. Escape is sent first to clear any leaked auto-suggest
// remaining in the input line (an empirical rule from the mux research)
// before text is typed and submitted — reuse turns are single-line, so no
// further choreography is needed.
func (c *Claude) ComposeSend(text string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{
		{Key: "Escape"},
		{Text: text, Submit: true},
	}
}
