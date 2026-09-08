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

// startupGateNeedles are whitespace-stripped, lowercased phrases identifying a one-time claude gate
// that stands between the launch and a usable TUI.
//
// There are two such gates, not one, and both must be listed here rather than only the first.
// "trustthisfolder"/"filesinthisfolder" identify the trust-this-folder gate. "yes,iaccept"
// identifies the Bypass Permissions acceptance modal, which claude raises on every
// --dangerously-skip-permissions launch in a fresh environment — which is every launch lyx makes.
// Missing the second one is not a missed nicety: the modal draws its own selection caret with the
// same "❯" glyph Startup reads as its ready marker, so an unrecognized gate is classified
// StartupReady, the startup deadline stops applying, and the run parks on the dialog for the whole
// master timeout before reporting "timed out" rather than dying fast (crucible round
// opus-medium-r5, R5-7).
//
// The bypass gate is keyed on its accepting option's own label rather than on its banner text
// ("Bypass Permissions mode"), because a running claude session renders "bypass permissions" in its
// own footer: a banner needle would classify every healthy pane as a gate and never reach ready.
var startupGateNeedles = []string{"trustthisfolder", "filesinthisfolder", "yes,iaccept"}

// Startup classifies the pane's rendered content during launch.
// Every one-time gate is checked FIRST (each real dialog contains the "❯" ready marker as its own
// selection caret, so a gate reached after the ready check would be indistinguishable from a
// booted TUI).
// Then ready markers (the input marker "❯" or the footer hint "shortcuts") are checked; anything
// else is still booting.
//
// A gate needle ALONE is not a gate, and treating it as one was a live-path defect. The needles are
// ordinary English — "the files in this folder", "trust this folder", "Yes, I accept" — and they are
// matched against the whole capture, which on a running pane is the agent's own transcript. A
// healthy, ready pane whose agent had written any of those phrases was therefore classified
// StartupTrustPrompt, with three consequences: Wait played TrustDismissSequence's arrow keys and
// Enter INTO the live agent's pane; *started was never set, so the run was classified OutcomeDied at
// the startup deadline while its agent was working; and requireReadyAgentPane refused every Send and
// Interrupt for as long as the phrase stayed on screen (crucible round opus-medium-r6, R6-1).
//
// So a gate is classified only on POSITIVE evidence that one is rendered: a needle PLUS either an
// accepting-option line — the same line TrustDismissSequence must find to act, located by the same
// helper so the two can never disagree about what an accepting option is — or claude's own gate
// footer. Prose carries neither.
//
// The residual, stated rather than papered over: a future gate whose accepting option matches none of
// gateAcceptNeedles AND which also drops the footer would fall through to the ready check. That gate
// is already undismissable today (TrustDismissSequence returns nothing for it), so what changes is
// only how it fails — parking until the run timeout instead of dying at the startup deadline. That is
// the deliberate trade for no longer killing healthy runs over ordinary agent prose.
func (c *Claude) Startup(capture string) shuttleengine.StartupState {
	normalized := normalizeCapture(capture)
	if containsAnyNeedle(normalized, startupGateNeedles) && gateIsRendered(capture, normalized) {
		return shuttleengine.StartupTrustPrompt
	}
	if strings.Contains(capture, gateCaretMarker) || strings.Contains(normalized, "shortcuts") {
		return shuttleengine.StartupReady
	}
	return shuttleengine.StartupPending
}

// gateIsRendered reports whether capture carries positive evidence of a one-time gate DIALOG, as
// opposed to a mere mention of one of the phrases startupGateNeedles matches.
// normalized is capture's whitespace-stripped, lowercased form, passed in rather than recomputed
// because the caller already has it.
func gateIsRendered(capture, normalized string) bool {
	if _, acceptLine := locateGateLines(capture); acceptLine != -1 {
		return true
	}
	return strings.Contains(normalized, gateFooterNeedle)
}

// gateOptionDecoration is the set of runes claude may draw BEFORE an option's own label on a select
// list line: the selection caret, list numbering and its punctuation, and the whitespace between them.
// Stripping them is what lets an option line be recognized by what it BEGINS with.
const gateOptionDecoration = " \t❯>-*.)([]0123456789"

// isGateAcceptOptionLine reports whether line is a gate's ACCEPTING OPTION line, as opposed to prose
// that merely contains the same words.
//
// The distinction is the whole point. A gate needle is ordinary English, and an agent's own
// transcript is what the capture holds once a run is under way — "You asked whether to trust this
// folder", "the accepting option reads yes, i accept". Matching those as accepting options let
// Startup call a healthy pane a gate and let TrustDismissSequence walk the caret onto a line of the
// agent's own output and press Enter (crucible round opus-medium-r6, R6-1).
//
// An option line BEGINS with its label once the caret and any list numbering are stripped, and prose
// does not, so the needle must be a PREFIX of the stripped, normalized line rather than merely
// present in it. The residual is prose that opens with an option label verbatim ("Yes, I accept, but
// first…"); it needs a gate needle elsewhere in the same capture to matter at all, and it is far
// narrower than matching anywhere on any line.
func isGateAcceptOptionLine(line string) bool {
	label := normalizeCapture(strings.TrimLeft(line, gateOptionDecoration))
	for _, needle := range gateAcceptNeedles {
		if strings.HasPrefix(label, needle) {
			return true
		}
	}
	return false
}

// containsAnyNeedle reports whether normalized contains any of needles.
// normalized must already be in normalizeCapture's form, which is the form every needle is written in.
func containsAnyNeedle(normalized string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(normalized, needle) {
			return true
		}
	}
	return false
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

// gateAcceptNeedles are whitespace-stripped, lowercased phrases identifying a gate's ACCEPTING
// option LINE, as distinct from startupGateNeedles, which identify the gate itself.
// They are deliberately narrower than a gate needle would need to be: the trust gate's own prose
// paragraph asks whether this is "a project you created or one you trust", so a needle broad enough
// to match the paragraph would locate the wrong line and walk the caret to nowhere.
// The set covers both gates and tolerates a rewording of either — "Yes, I trust this folder" and
// "Yes, I accept" as claude 2.1.263 spells them today, plus "Yes, proceed", the older trust-gate
// wording Startup's own fixture set has always treated as a recognized gate. Missing that third
// spelling was not a missing nicety either: the gate was classified StartupTrustPrompt and then
// never dismissed, because no line matched, so every agent spawned against a claude build using it
// pressed nothing at all and died at the startup deadline as an opaque "died" (crucible round
// opus-medium-r6, R6-2).
var gateAcceptNeedles = []string{"trustthisfolder", "yes,itrust", "yes,iaccept", "yes,proceed"}

// gateCaretMarker is the glyph claude renders beside the currently-selected option of a select
// list, the same marker Startup already reads as its ready marker.
const gateCaretMarker = "❯"

// gateFooterNeedle is the whitespace-stripped, lowercased prefix of the footer claude draws under
// every one-time gate ("Enter to confirm · Esc to cancel"), and the second of the two pieces of
// positive evidence Startup accepts that a gate is actually on screen.
// It is deliberately a SECOND signal rather than the only one: it keeps a gate whose accepting option
// has been reworded out of gateAcceptNeedles failing FAST, at the startup deadline, instead of
// parking until the run timeout — while the accepting-option line keeps a gate that drops the footer
// recognized. A running claude session with --dangerously-skip-permissions raises no confirmation
// prompts of its own, so this phrase does not appear on a healthy pane.
const gateFooterNeedle = "entertoconfirm"

// locateGateLines returns the indices, within capture's own lines, of the LAST line carrying the
// selection caret and the LAST line naming a gate's accepting option, each -1 when absent.
//
// Last occurrence, not first: the gate is drawn at the BOTTOM of the pane, and everything above it is
// scrollback that may carry both a stale caret and stale option text.
//
// It is shared by Startup and TrustDismissSequence deliberately. Before this, Startup matched gate
// phrases across the WHOLE whitespace-stripped capture while TrustDismissSequence matched accepting
// options PER LINE, so the two could disagree — a capture Startup called a gate and the dismissal
// could not act on pressed nothing and burned the startup window. One locator means the classifier
// never names a gate the dismissal cannot walk (crucible round opus-medium-r6, R6-1).
func locateGateLines(capture string) (caretLine, acceptLine int) {
	caretLine, acceptLine = -1, -1
	for i, line := range strings.Split(capture, "\n") {
		if strings.Contains(line, gateCaretMarker) {
			caretLine = i
		}
		if isGateAcceptOptionLine(line) {
			acceptLine = i
		}
	}
	return caretLine, acceptLine
}

// gateSelectSettleMS is the pause after each caret-moving key press, so a burst of arrow keys is
// not coalesced into a single escape-sequence read and silently dropped — the same hazard
// ComposeSend's own leading pause exists for.
const gateSelectSettleMS = 150

// TrustDismissSequence returns the key choreography that ACCEPTS whichever one-time claude gate is
// rendered in capture: enough Down (or Up) presses to move the caret from wherever claude put it
// onto the accepting option ("Yes, I trust this folder", "Yes, I accept"), then Enter.
// One mechanism covers both gates because both are the same two-option select list; nothing here
// needs to know which of them it is looking at.
//
// It is capture-driven rather than a fixed single Enter, and that is not a refinement — the fixed
// form was a live-confirmed defect. Claude 2.1.263 renders both gates with the caret on the
// REFUSING option:
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
	caretLine, acceptLine := locateGateLines(capture)
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
		inputs = append(inputs, shuttleengine.PaneInput{Key: key, SettleMS: gateSelectSettleMS})
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
