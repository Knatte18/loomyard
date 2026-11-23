// linelength.go declares the check of a `//` comment line's text length against the prose line-length limit.

package commentlint

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// LineLengthLimit is the number of characters of comment text at which a `//` comment line becomes a finding.
// A line one character shorter passes.
const LineLengthLimit = 120

var (
	// codeSpanPattern matches one backticked code span.
	codeSpanPattern = regexp.MustCompile("`[^`]*`")
	// anyLevelHeadingPattern matches a markdown heading of any level, one to six `#`.
	anyLevelHeadingPattern = regexp.MustCompile(`^#{1,6}(\s|$)`)
)

// countedText returns the text a comment line's length is measured on, given the line's text after `//`:
// that text without its indentation or a list item's marker.
func countedText(raw string) string {
	return listItemPattern.ReplaceAllString(strings.TrimLeft(raw, " \t"), "")
}

// isExemptFromLength reports whether a line's counted text may run past limit:
// it is a heading, a blockquote or a table row,
// or it holds a URL, code span or other unbroken word that alone reaches limit.
func isExemptFromLength(text string, limit int) bool {
	if anyLevelHeadingPattern.MatchString(text) || strings.HasPrefix(text, ">") || strings.HasPrefix(text, "|") {
		return true
	}
	for _, span := range codeSpanPattern.FindAllString(text, -1) {
		if utf8.RuneCountInString(span) >= limit {
			return true
		}
	}
	for _, word := range strings.Fields(text) {
		if utf8.RuneCountInString(word) >= limit {
			return true
		}
	}
	return false
}

// TextReachesLimit reports whether a `//` comment line, given as its text after `//`, breaks a line-length limit:
// its text without indentation or a list item's marker counts limit characters or more,
// and the line is no heading, blockquote or table row,
// and it holds no URL, code span or unbroken word that alone reaches limit.
func TextReachesLimit(raw string, limit int) bool {
	text := countedText(raw)
	return utf8.RuneCountInString(text) >= limit && !isExemptFromLength(text, limit)
}

// findNewLongLines returns the comment lines of path's new text whose counted text reaches LineLengthLimit.
// A line that stands unchanged in a comment block of baseText is not new and is skipped.
// Each occurrence of a line in the base excuses one occurrence in the new text.
func findNewLongLines(path, baseText, newText string) []Finding {
	known := map[string]int{}
	for _, block := range commentBlocks(baseText) {
		for _, line := range block {
			known[strings.TrimSpace(line.literal)]++
		}
	}
	var findings []Finding
	for _, block := range commentBlocks(newText) {
		for _, line := range block {
			literal := strings.TrimSpace(line.literal)
			if known[literal] > 0 {
				known[literal]--
				continue
			}
			if !TextReachesLimit(line.raw(), LineLengthLimit) {
				continue
			}
			findings = append(findings, Finding{File: path, Line: line.number, Rule: RuleLineLength, Text: literal})
		}
	}
	return findings
}
