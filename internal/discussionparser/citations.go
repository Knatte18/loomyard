// citations.go implements Citations, which extracts the inline code spans of a decision record that the citation check judges.

package discussionparser

import "strings"

// Citation is one backticked inline code span of a decision record.
type Citation struct {
	// Token is the text between the span's two backticks, verbatim.
	Token string
	// Line is the 1-based line of the record holding the span.
	Line int
	// Marked reports whether the closing backtick is followed immediately by ` (new)` or ` (removed)`.
	Marked bool
}

// citationMarks are the suffixes after a closing backtick that mark a span as naming something the task creates or removes.
var citationMarks = []string{" (new)", " (removed)"}

// Citations returns, in record order, one Citation per inline code span of each line of the record, and judges none of them.
// A span runs from a backtick to the next backtick on the same line, and a span with empty text is skipped.
// It skips the lines inside fenced code blocks and the lines of each `### Added after Discussion` entry.
// It also skips a `## PATTERN changes` bullet line that `lym loom decision add` appended, and every line of a `<!-- lym:carry-over … -->` block, an unclosed block running to the end of the record.
// A heading line is read like any other line.
func Citations(record []byte) []Citation {
	lines := strings.Split(string(record), "\n")
	fenced := fencedLines(lines)

	skip := make([]bool, len(lines))
	_, spans := addedDecisionSpans(lines, fenced)
	for _, span := range spans {
		for i := span.first; i <= span.last; i++ {
			skip[i] = true
		}
	}

	inCarryOver, inPatternChanges := false, false
	for i, line := range lines {
		if fenced[i] {
			continue
		}
		trimmed := strings.TrimRight(line, " \t\r")
		if _, isOpen, ok := carryOverMarker(line); ok {
			inCarryOver = isOpen
			skip[i] = true
			continue
		}
		if inCarryOver {
			skip[i] = true
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			inPatternChanges = trimmed == patternChangesHeading
			continue
		}
		if inPatternChanges && strings.HasPrefix(trimmed, "- ") && patternBulletSuffix.MatchString(trimmed) {
			skip[i] = true
		}
	}

	var citations []Citation
	for i, line := range lines {
		if fenced[i] || skip[i] {
			continue
		}
		citations = append(citations, lineCitations(line, i+1)...)
	}
	return citations
}

// lineCitations returns the inline code spans of one line.
func lineCitations(line string, number int) []Citation {
	var citations []Citation
	for from := 0; from < len(line); {
		open := strings.IndexByte(line[from:], '`')
		if open < 0 {
			break
		}
		open += from
		length := strings.IndexByte(line[open+1:], '`')
		if length < 0 {
			break
		}
		closing := open + 1 + length
		from = closing + 1
		if length == 0 {
			continue
		}
		marked := false
		for _, mark := range citationMarks {
			if strings.HasPrefix(line[closing+1:], mark) {
				marked = true
			}
		}
		citations = append(citations, Citation{Token: line[open+1 : closing], Line: number, Marked: marked})
	}
	return citations
}
