// pane.go reads Codex's TUI from a plain pane capture and holds the key choreography sent into it.
// The readings come from the pane table of docs/research/codex/codex-spike-2026-10-24/report.md: the `›` composer, the `Working (… esc to interrupt)` busy line, the folder-trust prompt and the collapsed-paste placeholder.
// Every reading is pure over a capture string, and all Codex TUI shape knowledge stays in this package, per PATTERN-shuttle-provider-seam.

package codexengine

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var (
	_ shuttleengine.SessionCycler  = (*Codex)(nil)
	_ shuttleengine.InputBoxReader = (*Codex)(nil)
	_ shuttleengine.TextPaster     = (*Codex)(nil)
	_ shuttleengine.InterruptGuard = (*Codex)(nil)
)

// composerMarker opens the composer line, and the lines of the transcript that echo a submitted prompt.
const composerMarker = "›"

// composerPlaceholder is the text an empty composer shows.
const composerPlaceholder = "Ask Codex to do anything"

// Needles in normalizeCapture form.
const (
	// busyNeedle and busyLineNeedle together mark the line a running turn shows.
	busyNeedle     = "esctointerrupt"
	busyLineNeedle = "working("
	// trustPromptNeedle is the folder-trust prompt's question.
	trustPromptNeedle = "trustthisfolder?"
	// trustAcceptNeedle is the trust prompt's accepting option.
	trustAcceptNeedle = "trustandcontinue"
	// trustFooterNeedle is the footer the trust prompt draws under its options.
	trustFooterNeedle = "entercontinue·escquit"
	// hookReviewNeedle is the hook-review prompt the launch's bypass flag keeps away.
	hookReviewNeedle = "hooksneedreview"
)

// trustOptionAdjacencyLines bounds how far the accepting option may sit from the last composer-marker line and still count as the prompt's own option: the prompt's caret sits on one of its two adjacent option lines.
const trustOptionAdjacencyLines = 1

// optionDecoration is what Codex draws before an option's label: the caret, the numbering and the space between.
const optionDecoration = " \t›0123456789."

// interruptSettle is how long after Escape the pane is left alone before it is read again.
const interruptSettle = 2 * time.Second

// composeSubmitSettleMS is the pause between typed text and its Enter: Codex reads a fast burst of characters as a paste and drops an Enter inside it.
const composeSubmitSettleMS = 1000

// optionMoveSettleMS is the pause after each caret-moving key, so a burst of arrow keys is not read as one escape sequence and dropped.
const optionMoveSettleMS = 150

// paneMinRows is the fewest rows that draw the composer with the status line under it.
const paneMinRows = 3

// pastePlaceholderShape matches the composer reading of a collapsed paste, its character count captured.
var pastePlaceholderShape = regexp.MustCompile(`^\[Pasted Content (\d+) chars\]$`)

// normalizeCapture lowercases capture and strips its whitespace, the form every needle is written in.
func normalizeCapture(capture string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, capture)
}

// isComposerLine reports whether line opens with the composer marker.
func isComposerLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), composerMarker)
}

// isTrustAcceptLine reports whether line is the trust prompt's accepting option: its label, once the caret and numbering are stripped, begins with the option's words.
func isTrustAcceptLine(line string) bool {
	return strings.HasPrefix(normalizeCapture(strings.TrimLeft(line, optionDecoration)), trustAcceptNeedle)
}

// locateTrustLines returns the indices of the last composer-marker line and the last accepting-option line in capture, each -1 when absent.
func locateTrustLines(capture string) (caretLine, acceptLine int) {
	caretLine, acceptLine = -1, -1
	for i, line := range strings.Split(capture, "\n") {
		if isComposerLine(line) {
			caretLine = i
		}
		if isTrustAcceptLine(line) {
			acceptLine = i
		}
	}
	return caretLine, acceptLine
}

// trustOptionAdjacent reports whether the accepting option sits on or next to the caret line, as a rendered prompt draws it.
func trustOptionAdjacent(caretLine, acceptLine int) bool {
	if caretLine == -1 || acceptLine == -1 {
		return false
	}
	distance := acceptLine - caretLine
	if distance < 0 {
		distance = -distance
	}
	return distance <= trustOptionAdjacencyLines
}

// trustPromptShown reports whether capture shows the folder-trust prompt: its question, and either its accepting option next to the caret or its footer.
// A transcript that only quotes the question carries neither, so it is not the prompt.
func trustPromptShown(capture string) bool {
	normalized := normalizeCapture(capture)
	if !strings.Contains(normalized, trustPromptNeedle) {
		return false
	}
	return trustOptionAdjacent(locateTrustLines(capture)) || strings.Contains(normalized, trustFooterNeedle)
}

// Startup classifies a pane capture during launch.
// It reports the folder-trust prompt first, since its caret draws the composer marker.
// It then reports the hook-review prompt as still booting, since its options draw the marker too and the launch never answers it.
// It reports ready on any composer-marker line, and still booting otherwise.
func (c *Codex) Startup(capture string) shuttleengine.StartupState {
	if trustPromptShown(capture) {
		return shuttleengine.StartupTrustPrompt
	}
	if strings.Contains(normalizeCapture(capture), hookReviewNeedle) {
		return shuttleengine.StartupPending
	}
	for _, line := range strings.Split(capture, "\n") {
		if isComposerLine(line) {
			return shuttleengine.StartupReady
		}
	}
	return shuttleengine.StartupPending
}

// TrustDismissSequence returns the keys that accept the folder-trust prompt capture shows: Up or Down until the caret sits on "Trust and continue", then Enter.
// It returns no keys when capture shows no caret, no accepting option, or an option that is not next to the caret, so nothing is ever pressed blind.
func (c *Codex) TrustDismissSequence(capture string) []shuttleengine.PaneInput {
	caretLine, acceptLine := locateTrustLines(capture)
	if !trustOptionAdjacent(caretLine, acceptLine) {
		return nil
	}
	steps, key := acceptLine-caretLine, "Down"
	if steps < 0 {
		steps, key = -steps, "Up"
	}
	inputs := make([]shuttleengine.PaneInput, 0, steps+1)
	for range steps {
		inputs = append(inputs, shuttleengine.PaneInput{Key: key, SettleMS: optionMoveSettleMS})
	}
	return append(inputs, shuttleengine.PaneInput{Key: "Enter"})
}

// InterruptSequence returns one Escape, which interrupts Codex's turn.
func (c *Codex) InterruptSequence() []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Key: "Escape"}}
}

// ComposeSend returns the text and, after a settle that moves it out of Codex's paste burst, its Enter.
func (c *Codex) ComposeSend(text string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Text: text, SettleMS: composeSubmitSettleMS}, {Key: "Enter"}}
}

// TypeSequence returns the text step alone: the caller sends the Enter once the composer has settled.
func (c *Codex) TypeSequence(text string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Text: text}}
}

// ColorSequence returns no keys: Codex has no command that colors its session.
func (c *Codex) ColorSequence(segmentcolor.Color) []shuttleengine.PaneInput {
	return nil
}

// RunningTurn reports whether capture shows a turn in progress: a line carrying both "Working (" and "esc to interrupt".
// Bound: a transcript line quoting both reads as a running turn.
func (c *Codex) RunningTurn(capture string) bool {
	for _, line := range strings.Split(capture, "\n") {
		normalized := normalizeCapture(line)
		if strings.Contains(normalized, busyLineNeedle) && strings.Contains(normalized, busyNeedle) {
			return true
		}
	}
	return false
}

// InterruptSettle returns how long after Escape the pane is left alone before it is read again.
func (c *Codex) InterruptSettle() time.Duration {
	return interruptSettle
}

// InputBoxText returns the text the composer holds in capture: the last composer-marker line's text with the marker stripped, joined by single spaces to the non-blank lines under it.
// The placeholder reads as empty text; ok is false when capture shows no composer.
// Bound: a plain capture carries no styling, so a placeholder Codex rewords reads as a draft, which holds a send instead of merging with it.
func (c *Codex) InputBoxText(capture string) (string, bool) {
	lines := strings.Split(capture, "\n")
	composer := -1
	for i, line := range lines {
		if isComposerLine(line) {
			composer = i
		}
	}
	if composer == -1 {
		return "", false
	}
	first := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[composer]), composerMarker))
	if first == composerPlaceholder {
		return "", true
	}
	parts := []string{}
	if first != "" {
		parts = append(parts, first)
	}
	for _, line := range lines[composer+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, " "), true
}

// SubmitSettle returns how long after an Enter the composer needs to be redrawn before it is read.
func (c *Codex) SubmitSettle() time.Duration {
	return time.Duration(c.submitRedrawSettleMS) * time.Millisecond
}

// IdleSession reports whether capture shows Codex idle: no running turn, and the composer on screen and empty.
func (c *Codex) IdleSession(capture string) bool {
	if c.RunningTurn(capture) {
		return false
	}
	text, ok := c.InputBoxText(capture)
	return ok && text == ""
}

// PaneTooShort reports whether capture shows a pane too short to draw the composer: no composer-marker line, and fewer rows than the composer and its status line take.
func (c *Codex) PaneTooShort(capture string) bool {
	if _, ok := c.InputBoxText(capture); ok {
		return false
	}
	return len(strings.Split(strings.TrimSuffix(capture, "\n"), "\n")) < paneMinRows
}

// ClearSessionSequence returns no keys: no clear command of Codex's has been tried.
func (c *Codex) ClearSessionSequence() []shuttleengine.PaneInput { return nil }

// CompactSessionSequence returns no keys: no compact command of Codex's has been tried.
func (c *Codex) CompactSessionSequence(string) []shuttleengine.PaneInput { return nil }

// ReloadPluginsSequence returns no keys: Codex has no command that reloads skills.
func (c *Codex) ReloadPluginsSequence() []shuttleengine.PaneInput { return nil }

// PasteSubmits reports false: shuttle pastes without the Enter and sends its own guarded Enter once the composer settles, which Codex submits.
func (c *Codex) PasteSubmits() bool { return false }

// OwnPastePlaceholder reports whether boxText, trimmed, is the whole collapsed-paste placeholder a paste of text produces: `[Pasted Content N chars]` with N the characters of text.
func (c *Codex) OwnPastePlaceholder(boxText, text string) bool {
	match := pastePlaceholderShape.FindStringSubmatch(strings.TrimSpace(boxText))
	if match == nil {
		return false
	}
	count, err := strconv.Atoi(match[1])
	return err == nil && count == utf8.RuneCountInString(text)
}
