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
	"unicode"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// trustDialogNeedles are the whitespace-stripped, lowercased phrases that
// identify claude's one-time "do you trust this folder?" gate in a pane
// capture: "I trust this folder" (its confirm option, current TUI) and
// "files in this folder" (the older question wording). Matching whole
// phrases — never loose word co-occurrence — is what lets the trust check
// run BEFORE the ready markers without masking a genuinely ready pane whose
// agent text merely mentions trusting a folder.
var trustDialogNeedles = []string{"trustthisfolder", "filesinthisfolder"}

// Startup classifies capture, the pane's currently rendered content, during
// the window between launch and claude becoming ready for input.
//
// The trust gate is checked FIRST, because the REAL trust dialog contains
// the "❯" ready marker itself — the selection caret on its
// "❯ 1. Yes, I trust this folder" option (proven live against claude
// 2.1.200) — so a ready-first ordering classifies the dialog as ready and
// the Enter dismissal never fires, hanging every run in a not-yet-trusted
// directory until its full timeout. The trust match is deliberately tight:
// whole phrase needles over a whitespace-stripped, lowercased capture (the
// TUI's rendering can drop spaces entirely, an observed capture quirk), so
// an agent's own on-screen text that merely mentions trusting a folder
// (e.g. "trust that the folder layout is correct") cannot match and mask a
// pane that is in fact already ready.
//
// Absent a trust match, the ready markers apply: the TUI's own input marker
// "❯" or the ASCII footer hint "shortcuts" (from "? for shortcuts" — kept
// as a fallback for renderings that corrupt "❯"; note the bypass-permissions
// footer shows no "shortcuts" text at all, so "❯" must stay a ready marker).
// Anything else is still booting. Known limitation: a shell prompt styled
// with "❯" (starship/oh-my-posh profiles — mux panes load the operator's
// pwsh profile) also satisfies the ready marker, which degrades the
// fast-fail for a claude that exits at launch into waiting out the full run
// timeout; environment-dependent and accepted for v1.
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

// normalizeCapture lowercases capture and strips every whitespace rune —
// the canonical form Startup matches its phrase needles against. The claude
// TUI's pane rendering can drop spaces entirely (an observed capture quirk:
// "Yes,Itrustthisfolder"), so any space-sensitive match would be unreliable
// in exactly the captures that matter.
func normalizeCapture(capture string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, capture)
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
