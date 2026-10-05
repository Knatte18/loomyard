// session.go completes Claude's shuttleengine.SessionCycler: the idle-session probe and the clear-session and compact-session choreographies.
// All are pure over a capture string or literal text, like startup.go's classifiers,
// and all Claude TUI shape knowledge stays in this package, per the Shuttle Provider-Seam Invariant.
package claudeengine

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.SessionCycler = (*Claude)(nil)

// runningTurnNeedle is Claude's running-turn hint in normalizeCapture form.
const runningTurnNeedle = "esctointerrupt"

// boxRuleChars are the characters a horizontal-rule line of the input box may carry besides whitespace: the rule glyph itself, box corners and side bars.
const boxRuleChars = "─╭╮╰╯│"

// boxInteriorChars are stripped from a line between the rules before judging it blank: the caret and side bars, on top of whitespace.
const boxInteriorChars = gateCaretMarker + "│>"

// boxMinRows is the fewest pane rows Claude's input box needs: the top rule, the caret line, the bottom rule and the footer under it.
const boxMinRows = 4

// PaneTooShort reports whether capture shows a pane too short to draw the input box: no caret line anywhere, and fewer rows than boxMinRows.
// A pane of normal height that is busy or showing a dialog is never too short, whatever it draws.
func (c *Claude) PaneTooShort(capture string) bool {
	if strings.Contains(capture, gateCaretMarker) {
		return false
	}
	rows := strings.Split(strings.TrimSuffix(capture, "\n"), "\n")
	return len(rows) < boxMinRows
}

// IdleSession reports whether capture shows Claude idle: no turn in progress, the input box on screen, and the box empty.
//
// The box is located from the last line carrying gateCaretMarker, walking up and down to the first horizontal-rule line on each side.
// A permission prompt or other select dialog draws its caret on an option line with no rule below it, so it is not idle;
// a draft leaves non-blank text between the rules.
//
// Two fail-closed residuals, stated rather than papered over.
// A capture carries no styling, so any non-empty box classifies as not idle and the watcher skips the poll and retries;
// the greyed prompt suggestion no longer appears in a shuttle-launched session, since every settings file switches it off.
// And a transcript line quoting "esc to interrupt" anywhere in the capture classifies as not idle, because the running-turn hint is matched over the whole capture.
func (c *Claude) IdleSession(capture string) bool {
	if strings.Contains(normalizeCapture(capture), runningTurnNeedle) {
		return false
	}
	lines := strings.Split(capture, "\n")
	caret := -1
	for i, line := range lines {
		if strings.Contains(line, gateCaretMarker) {
			caret = i
		}
	}
	if caret == -1 {
		return false
	}
	top := -1
	for i := caret - 1; i >= 0; i-- {
		if isTopBoxRule(lines[i]) {
			top = i
			break
		}
	}
	bottom := -1
	for i := caret + 1; i < len(lines); i++ {
		if isBoxRule(lines[i]) {
			bottom = i
			break
		}
	}
	if top == -1 || bottom == -1 {
		return false
	}
	for _, line := range lines[top+1 : bottom] {
		if !isBlankBoxInterior(line) {
			return false
		}
	}
	return true
}

// isTopBoxRule reports whether line is the input box's top rule: a plain rule per isBoxRule, or a labelled one.
// A named session labels its top rule at the right (`──── tst:orch ─`), so a line that starts with three rule glyphs and ends with one counts as a top rule whatever its label says.
// Only the top rule carries a label, so the bottom rule is matched by isBoxRule alone,
// and a draft line shaped like a labelled rule below the caret never closes the box.
func isTopBoxRule(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "───") && strings.HasSuffix(trimmed, "─") {
		return true
	}
	return isBoxRule(line)
}

// isBoxRule reports whether line is a horizontal rule of the input box: non-blank, made only of rule glyphs, corners, side bars and whitespace, with at least one rule glyph.
func isBoxRule(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "─") {
		return false
	}
	for _, r := range trimmed {
		if r != ' ' && !strings.ContainsRune(boxRuleChars, r) {
			return false
		}
	}
	return true
}

// isBlankBoxInterior reports whether line holds nothing but the caret, side bars and whitespace.
func isBlankBoxInterior(line string) bool {
	for _, r := range line {
		if strings.ContainsRune(boxInteriorChars, r) {
			continue
		}
		if strings.TrimSpace(string(r)) != "" {
			return false
		}
	}
	return true
}

// ClearSessionSequence returns /clear typed and submitted, with no leading Escape.
// The caller has just proved the input box empty, so there is nothing to clear,
// and an Escape landing right after another Escape opens Claude's rewind menu instead.
func (c *Claude) ClearSessionSequence() []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Text: "/clear", Submit: true}}
}

// CompactSessionSequence returns /compact typed and submitted, followed by focus when it is non-empty, with no leading Escape for the same reason as ClearSessionSequence.
func (c *Claude) CompactSessionSequence(focus string) []shuttleengine.PaneInput {
	text := "/compact"
	if focus != "" {
		text += " " + focus
	}
	return []shuttleengine.PaneInput{{Text: text, Submit: true}}
}
