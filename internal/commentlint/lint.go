// lint.go declares Lint and the classification of a comment block's line breaks over a base and a new text of one file.

package commentlint

import (
	"go/scanner"
	"go/token"
	"regexp"
	"strings"
)

// Finding is one line of a `//` comment block that ends at a fixed-column wrap.
type Finding struct {
	// File is the slash-separated path relative to the worktree.
	File string
	// Line is the 1-based line number in the new text.
	Line int
	// Text is the comment line, `//` included, without surrounding whitespace.
	Text string
}

var (
	generatedPattern   = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.?\s*$`)
	directivePattern   = regexp.MustCompile(`^(go:|lyx:|testtiming:|nolint\b)`)
	headingPattern     = regexp.MustCompile(`^#(\s|$)`)
	listItemPattern    = regexp.MustCompile(`^([-*]\s|[0-9]{1,3}[.)]\s)`)
	sentenceEndPattern = regexp.MustCompile("[.!?:][)\"'`]*$")
	conjunctions       = map[string]bool{"and": true, "but": true, "or": true, "nor": true}
)

// Lint returns the new fixed-column-wrapped breaks in the `//` comment blocks of the `.go` files that differ in worktree.
// An empty base compares the working tree, untracked files included, against HEAD; a base commit compares it against HEAD.
// Findings are ordered by file path, then line.
func Lint(worktree, base string) ([]Finding, error) {
	changes, err := changedGoFiles(worktree, base)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, change := range changes {
		baseText, newText, err := readChange(worktree, base, change)
		if err != nil {
			return nil, err
		}
		findings = append(findings, findNewWrappedBreaks(change.path, baseText, newText)...)
	}
	return findings, nil
}

// commentLine is one `//` line of a comment block.
type commentLine struct {
	number int
	// literal is the comment as written, `//` included.
	literal string
	// content is the text after `//` and the one conventional space.
	content string
}

// raw is the text after `//`, before the conventional space is removed.
func (l commentLine) raw() string { return strings.TrimPrefix(l.literal, "//") }

// commentBlocks returns the runs of consecutive `//` comment lines in src that each start their line.
// An end-of-line comment belongs to no block and ends the run before it.
func commentBlocks(src string) [][]commentLine {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var lexer scanner.Scanner
	lexer.Init(file, []byte(src), func(token.Position, string) {}, scanner.ScanComments)

	var blocks [][]commentLine
	var current []commentLine
	flush := func() {
		if len(current) > 0 {
			blocks = append(blocks, current)
			current = nil
		}
	}
	for {
		pos, tok, literal := lexer.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.COMMENT || !strings.HasPrefix(literal, "//") {
			continue
		}
		literal = strings.TrimRight(literal, "\r\n")
		line := file.Position(pos).Line
		if !startsLine(src, file.Offset(pos)) {
			flush()
			continue
		}
		if len(current) > 0 && current[len(current)-1].number != line-1 {
			flush()
		}
		current = append(current, commentLine{number: line, literal: literal, content: strings.TrimPrefix(strings.TrimPrefix(literal, "//"), " ")})
	}
	flush()
	return blocks
}

// startsLine reports whether only whitespace precedes offset on its line.
func startsLine(src string, offset int) bool {
	lineStart := strings.LastIndexByte(src[:offset], '\n') + 1
	return strings.TrimSpace(src[lineStart:offset]) == ""
}

// isProseBlock reports whether the block holds only prose lines: no directive, indented code, list item or heading.
func isProseBlock(block []commentLine) bool {
	for _, line := range block {
		raw := line.raw()
		if directivePattern.MatchString(strings.TrimSpace(raw)) {
			return false
		}
		if strings.HasPrefix(raw, "\t") || strings.HasPrefix(line.content, " ") || strings.HasPrefix(line.content, "\t") {
			return false
		}
		if headingPattern.MatchString(line.content) || listItemPattern.MatchString(line.content) {
			return false
		}
	}
	return true
}

// lineEndPair names the two words either side of the break between two prose lines.
func lineEndPair(before, after commentLine) string {
	beforeWords := strings.Fields(before.content)
	afterWords := strings.Fields(after.content)
	return beforeWords[len(beforeWords)-1] + " " + afterWords[0]
}

// adjacentProse reports whether both lines hold words, so a break between them is a break inside one paragraph.
func adjacentProse(before, after commentLine) bool {
	return strings.TrimSpace(before.content) != "" && strings.TrimSpace(after.content) != ""
}

// baseLineEndPairs returns the word pairs adjacent across a line end inside a comment block of src.
func baseLineEndPairs(src string) map[string]bool {
	pairs := map[string]bool{}
	for _, block := range commentBlocks(src) {
		for i := 0; i+1 < len(block); i++ {
			if adjacentProse(block[i], block[i+1]) {
				pairs[lineEndPair(block[i], block[i+1])] = true
			}
		}
	}
	return pairs
}

// endsSemanticBreak reports whether a line may end where it does: at a sentence end, a semicolon, or a comma before a coordinating conjunction.
func endsSemanticBreak(before, after commentLine) bool {
	trimmed := strings.TrimRight(before.content, " \t")
	if sentenceEndPattern.MatchString(trimmed) || strings.HasSuffix(trimmed, ";") {
		return true
	}
	if strings.HasSuffix(trimmed, ",") {
		return conjunctions[strings.ToLower(strings.Fields(after.content)[0])]
	}
	return false
}

// findNewWrappedBreaks returns the lines of path's new text that end at a break, new against baseText, that is not a semantic one.
func findNewWrappedBreaks(path, baseText, newText string) []Finding {
	if generatedPattern.MatchString(newText) {
		return nil
	}
	known := baseLineEndPairs(baseText)
	var findings []Finding
	for _, block := range commentBlocks(newText) {
		if !isProseBlock(block) {
			continue
		}
		for i := 0; i+1 < len(block); i++ {
			before, after := block[i], block[i+1]
			if !adjacentProse(before, after) || known[lineEndPair(before, after)] || endsSemanticBreak(before, after) {
				continue
			}
			findings = append(findings, Finding{File: path, Line: before.number, Text: strings.TrimSpace(before.literal)})
		}
	}
	return findings
}
