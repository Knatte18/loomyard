// patternentry.go implements the pattern-entry-line-cap check: a card that prescribes a verbatim PATTERN.md entry line whose rule text is over the cap fails the plan gate, before an implementer writes a line the pattern check would refuse.
// It scans only the raw text the parse step recorded, so planparser stays the sole parser of the plan files.

package planparser

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/pattern"
)

const patternEntryLineCapCheck = "pattern-entry-line-cap"

// patternEntryLinePrefix opens a PATTERN.md entry line.
const patternEntryLinePrefix = "- `PATTERN-"

// checkPatternEntryLineCap implements pattern-entry-line-cap: every line inside a card's fenced code block that opens like a PATTERN.md entry line must keep its rule text, the part between the name and a trailing "(test)" or background link, within pattern.MaxEntryTextChars characters.
// Inline code spans are never scanned, since an entry line carries backticks of its own and no span can hold one whole.
// It measures verbatim lines only; a prose instruction to append a sentence to an entry is outside it.
func checkPatternEntryLineCap(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		for _, entry := range CardPatternEntries(c) {
			length := len([]rune(pattern.EntryRuleText(entry.Line)))
			if length <= pattern.MaxEntryTextChars {
				continue
			}
			findings = append(findings, ValidationError{
				Check: patternEntryLineCapCheck,
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d prescribes the entry line for `%s` with a rule text at %d characters, over the %d cap; shorten the text or move the detail to the entry's background file",
					c.Number, entry.ID, length, pattern.MaxEntryTextChars,
				),
			})
		}
	}

	return findings
}

// PatternEntryLine is one verbatim PATTERN.md entry line a card prescribes in a fenced code block.
// ID is the entry's ID without backticks, such as PATTERN-cwd-resolution, and Line is the whole line as written with any trailing carriage return trimmed.
type PatternEntryLine struct {
	ID   string
	Line string
}

// CardPatternEntries returns, in the card's text order, every line inside one of its fenced code blocks that opens like a PATTERN.md entry line.
// Inline code spans and prose are never read, and every line is kept, a repeated ID included.
func CardPatternEntries(card Card) []PatternEntryLine {
	var entries []PatternEntryLine
	for _, span := range codeSpans(card.Text) {
		if !span.spelling {
			continue
		}
		for _, line := range strings.Split(span.text, "\n") {
			line = strings.TrimRight(line, "\r")
			if !strings.HasPrefix(line, patternEntryLinePrefix) {
				continue
			}
			id, _, _ := strings.Cut(strings.TrimPrefix(line, "- `"), "`")
			entries = append(entries, PatternEntryLine{ID: id, Line: line})
		}
	}
	return entries
}

// CardNamesPatternEntry reports whether card's text names the PATTERN entry id in backticks, anywhere in prose, a code span or a fenced block.
// It marks the card that carries out a change to the entry which the plan states without fencing a line, such as a removal.
func CardNamesPatternEntry(card Card, id string) bool {
	return strings.Contains(card.Text, "`"+id+"`")
}
