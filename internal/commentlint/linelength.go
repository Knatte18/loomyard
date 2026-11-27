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

// countedText returns the text of a comment line that its length is measured on:
// the text after `//`, without its indentation or a list item's marker.
func countedText(line commentLine) string {
	return listItemPattern.ReplaceAllString(strings.TrimLeft(line.raw(), " \t"), "")
}

// isExemptFromLength reports whether a line's counted text may run past the limit:
// it is a heading, a blockquote or a table row,
// or it holds a URL, code span or other unbroken word that alone reaches the limit.
func isExemptFromLength(text string) bool {
	if anyLevelHeadingPattern.MatchString(text) || strings.HasPrefix(text, ">") || strings.HasPrefix(text, "|") {
		return true
	}
	for _, span := range codeSpanPattern.FindAllString(text, -1) {
		if utf8.RuneCountInString(span) >= LineLengthLimit {
			return true
		}
	}
	for _, word := range strings.Fields(text) {
		if utf8.RuneCountInString(word) >= LineLengthLimit {
			return true
		}
	}
	return false
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
			text := countedText(line)
			literal := strings.TrimSpace(line.literal)
			if known[literal] > 0 {
				known[literal]--
				continue
			}
			if utf8.RuneCountInString(text) < LineLengthLimit || isExemptFromLength(text) {
				continue
			}
			findings = append(findings, Finding{File: path, Line: line.number, Rule: RuleLineLength, Text: literal})
		}
	}
	return findings
}
