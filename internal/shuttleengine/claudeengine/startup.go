// startup.go implements Startup, which classifies a pane's capture during launch.
// It also holds the fixed key sequences, InterruptSequence and ComposeSend, that interrupt a turn or resume one.
// Every function here is pure over a capture string or literal text.

package claudeengine

import (
	"strings"
	"unicode"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// startupGateNeedles are whitespace-stripped, lowercased phrases that identify a one-time claude gate.
// Two such gates exist, and both must be recognized here.
// An unrecognized gate draws its selection caret with the same "❯" that Startup reads as the ready marker.
// It would then classify as `StartupReady`, and the run would park on the dialog until the master timeout.
// Recognized as a gate, the dialog is sent its dismissal keys instead of parking the run.
// The list is built from gateAcceptNeedles and filesInThisFolderNeedle,
// so a wording added to gateAcceptNeedles is recognized here too.
//
// The bypass gate is matched by its accepting option's label, not its banner text.
// A running claude session also renders "bypass permissions" in its footer,
// so a banner needle would classify every healthy pane as a gate.
var startupGateNeedles = append([]string{filesInThisFolderNeedle}, gateAcceptNeedles...)

// filesInThisFolderNeedle is the trust gate's explanatory prose, "the files in this folder".
// It never appears on an accepting-option line, so it marks the surrounding text, not one of the two options.
const filesInThisFolderNeedle = "filesinthisfolder"

// Startup classifies the pane's rendered content during launch as StartupTrustPrompt, StartupReady or StartupPending.
// Gates are checked first, because a gate's own selection caret draws the "❯" that the ready check reads.
// Ready markers, the input "❯" or the footer hint "shortcuts", give StartupReady; otherwise the pane is booting.
//
// A needle alone does not make a gate, since its phrases are ordinary English that an agent's transcript can contain.
// A gate needs positive evidence: a needle plus an accepting-option line or the gate footer.
// A real gate is a two-option select list: its accepting option sits next to the last caret line,
// and its footer is drawn a few lines below.
// A healthy pane's only caret is its input box at the bottom, far from any prose above it.
// A capture with no caret still classifies as a gate: nothing is pressed for it, and the startup window bounds the wait.
//
// A gate whose accepting option matches no needle and that draws no footer falls through to the ready check.
// It then parks until the run timeout instead of failing at the startup deadline.
// Prose that opens with an option label on the line directly adjacent to the input-box caret still misclassifies.
// claude draws a blank line between the transcript and its input box, so that window is one rendering quirk wide.
//
// The heuristics were verified against a live claude TUI:
// docs/research/reed/reed-hooks-exploration.md and reedcli's `dismissTrust`.
func (c *Claude) Startup(capture string) shuttleengine.StartupState {
	normalized := normalizeCapture(capture)
	if containsAnyNeedle(normalized, startupGateNeedles) && gateIsRendered(capture) {
		return shuttleengine.StartupTrustPrompt
	}
	if strings.Contains(capture, gateCaretMarker) || strings.Contains(normalized, "shortcuts") {
		return shuttleengine.StartupReady
	}
	return shuttleengine.StartupPending
}

// gateAcceptAdjacencyLines bounds how far, in lines either way, the accepting option may sit from the last caret.
// A real gate's caret sits on one of two adjacent option lines, at a distance of 0 or 1.
// A healthy pane's input-box caret sits at the bottom, at least a blank line below any transcript prose.
const gateAcceptAdjacencyLines = 1

// gateFooterAdjacencyLines bounds how far below the last caret the gate footer may sit and still count.
// A gate draws its footer three lines below the caret, after the other option and a blank line.
// Prose such as "Enter to confirm" sits above a healthy pane's input-box caret, never below it;
// requiring the footer below the caret keeps that prose inert.
const gateFooterAdjacencyLines = 4

// acceptLineIsGateOption reports whether acceptLine, an accepting-option line index, is a rendered gate's option.
// It holds when the line is present and either within gateAcceptAdjacencyLines of caretLine,
// or the capture has no caret, where caretLine is -1.
// Startup and the `TrustDismissSequence` walk share this rule, so they cannot disagree about what a gate's option is.
func acceptLineIsGateOption(caretLine, acceptLine int) bool {
	if acceptLine == -1 {
		return false
	}
	if caretLine == -1 {
		return true
	}
	distance := acceptLine - caretLine
	if distance < 0 {
		distance = -distance
	}
	return distance <= gateAcceptAdjacencyLines
}

// gateIsRendered reports whether capture shows a one-time gate dialog, not prose that merely names a needle.
// The evidence is an accepting option near the last caret, or the footer within gateFooterAdjacencyLines below it.
// A rendered footer never sits above its own dialog's caret, so the footer must be strictly below.
// With no caret in the capture, either piece of evidence stands alone.
func gateIsRendered(capture string) bool {
	caretLine, acceptLine := locateGateLines(capture)
	if acceptLineIsGateOption(caretLine, acceptLine) {
		return true
	}
	footerLine := locateGateFooterLine(capture)
	if footerLine == -1 {
		return false
	}
	if caretLine == -1 {
		return true
	}
	distance := footerLine - caretLine
	return distance > 0 && distance <= gateFooterAdjacencyLines
}

// locateGateFooterLine returns the index of the last line that carries the gate footer, or -1 when there is none.
// The last occurrence is taken because a gate is drawn at the bottom of the pane, above the scrollback.
func locateGateFooterLine(capture string) int {
	footerLine := -1
	for i, line := range strings.Split(capture, "\n") {
		if strings.Contains(normalizeCapture(line), gateFooterNeedle) {
			footerLine = i
		}
	}
	return footerLine
}

// gateOptionDecoration is the set of runes claude may draw before an option's label on a select list line.
// It holds the selection caret, the list numbering and its punctuation, and the whitespace between them.
// Stripping them lets an option line be recognized by what it begins with.
const gateOptionDecoration = " \t❯>-*.)([]0123456789"

// isGateAcceptOptionLine reports whether line is a gate's accepting option, not prose that contains the same words.
//
// An option line begins with its label once the caret and list numbering are stripped, and prose does not,
// so the needle must be a prefix of the normalized line.
// A needle found anywhere in the line would also match the agent's own transcript.
//
// A match is not gate evidence on its own, since prose can open with an option label after list decoration.
// A caller also needs the caret to sit beside the matched line.
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
// normalized must already be lowercased with whitespace removed, the form every needle is written in.
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

// InterruptSequence returns the key choreography that interrupts a claude turn: a single Escape key press.
func (c *Claude) InterruptSequence() []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Key: escapeKey}}
}

// gateAcceptNeedles are the whitespace-stripped, lowercased phrases that identify a gate's accepting option line.
// They are narrower than the gate's own needles.
// A broader needle would match the trust gate's prose paragraph, and it would walk the caret to nowhere.
// The list covers both gates' accepting options, such as "Yes, I trust this folder" and "Yes, I accept".
// It also covers the older trust-gate wording "Yes, proceed", which is recognized as a gate.
// A gate whose accepting option matches none of these is classified but never dismissed.
// The pane then dies at the startup deadline.
var gateAcceptNeedles = []string{"trustthisfolder", "yes,itrust", "yes,iaccept", "yes,proceed"}

// gateCaretMarker is the glyph claude draws beside the selected option of a select list.
// The startup check also reads this glyph as its ready marker.
const gateCaretMarker = "❯"

// ReadyFooterFixture is the footer text a live claude TUI draws when ready, in the form the readiness check matches.
// A caller outside this package that needs a realistic ready-marker fixture should use this constant,
// so a rewording of claude's ready marker is fixed in one place.
// One such caller is a stubbed provider binary standing in for claude in a black-box test.
const ReadyFooterFixture = "? for shortcuts"

// IdleInputBoxFixture is a live claude TUI's idle bottom rows:
// an empty input box between two horizontal rules, with the ready-marker footer under it.
// It reads as ready at startup and as an idle session.
// A stubbed provider that prints it after each line it reads takes verified sends one after another.
const IdleInputBoxFixture = "────────────────────────────────\n" +
	gateCaretMarker + " \n" +
	"────────────────────────────────\n" +
	"  " + ReadyFooterFixture

// gateFooterNeedle is the whitespace-stripped, lowercased prefix of the footer claude draws under every one-time gate:
// "Enter to confirm · Esc to cancel".
// It is a second signal that a gate is on screen, beside the accepting option line.
// It lets a gate whose accepting option has been reworded fail fast at the startup deadline;
// without it, that gate would park until the run timeout.
// The accepting option line in turn keeps a gate that drops the footer recognized.
// A running claude session with --dangerously-skip-permissions raises no confirmation prompts of its own,
// so this phrase does not appear on a healthy pane.
const gateFooterNeedle = "entertoconfirm"

// locateGateLines returns two indices into the lines of capture, each -1 when absent:
// the last line carrying the selection caret, and the last line naming a gate's accepting option.
//
// The last occurrence wins, because the gate is drawn at the bottom of the pane.
// Everything above it is scrollback that may carry a stale caret or stale option text.
//
// Callers share this one locator, so a capture classified as a gate is one the dismissal can act on.
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

// gateSelectSettleMS is the pause after each caret-moving key press.
// It stops a burst of arrow keys from being coalesced into one escape-sequence read and silently dropped.
const gateSelectSettleMS = 150

// TrustDismissSequence returns the key choreography that accepts the one-time claude gate shown in capture:
// Down or Up presses that move the caret onto the accepting option, then Enter.
// Both gates are the same two-option select list, so one mechanism covers both.
//
// The caret is read from capture rather than assumed:
// the provider chooses which option the caret starts on, and that option can be the refusing one.
// A bare Enter would then confirm the refusing option, and claude would quit.
//
// It returns no inputs when capture has no caret line or no accepting option line.
// It also returns no inputs when the caret and the accepting option are not adjacent.
// A real gate's caret sits on one of two adjacent option lines.
// A longer walk means the caret is a healthy pane's own input-box marker and the option is transcript prose.
//
// Returning nothing is safer than a blind Enter, which presses whatever is selected.
// The caller re-probes on its next liveness tick,
// and the startup window bounds the retries, so an empty result costs a bounded wait.
func (c *Claude) TrustDismissSequence(capture string) []shuttleengine.PaneInput {
	caretLine, acceptLine := locateGateLines(capture)
	if caretLine == -1 || !acceptLineIsGateOption(caretLine, acceptLine) {
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

// ComposeSend returns the choreography that submits text as claude's next turn: the text step and the Enter.
// It opens with no Escape: prompt suggestion is off in every shuttle-launched session,
// so there is no leaked suggestion to clear, and an Escape could complete a double-Esc or interrupt a turn.
func (c *Claude) ComposeSend(text string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{
		{Text: text},
		{Key: "Enter"},
	}
}

// TypeSequence returns the choreography that types text as claude's next turn without submitting it.
// It is the text step alone: the caller sends the Enter itself once the input box has settled.
func (c *Claude) TypeSequence(text string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Text: text}}
}
