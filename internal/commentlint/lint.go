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
	conjunctions       = map[string]bool{"and": true, "but": true, "or": true, "nor": true, "yet": true, "so": true}
	subjectPronouns    = wordSet("it", "they", "we", "he", "she", "i", "you", "this", "that", "these", "those", "there")
	determiners        = wordSet("the", "a", "an", "its", "their", "our", "his", "her", "my", "your", "each", "every", "any", "some", "no", "another", "both", "all")
	auxiliaries        = wordSet("is", "are", "was", "were", "be", "been", "has", "have", "had", "do", "does", "did", "can", "could", "will", "would", "shall", "should", "may", "might", "must")
)

// wordSet returns the set of words.
func wordSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	return set
}

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
// An end-of-line comment belongs to no block and ends the run before it; so does a directive line, which belongs to no block either.
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
		if !startsLine(src, file.Offset(pos)) || directivePattern.MatchString(strings.TrimSpace(strings.TrimPrefix(literal, "//"))) {
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

// paragraphs splits a comment block at its blank comment lines.
func paragraphs(block []commentLine) [][]commentLine {
	var result [][]commentLine
	var current []commentLine
	for _, line := range block {
		if strings.TrimSpace(line.content) == "" {
			if len(current) > 0 {
				result = append(result, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		result = append(result, current)
	}
	return result
}

// isProseParagraph reports whether the paragraph holds only prose lines: no indented code, list item or heading.
func isProseParagraph(paragraph []commentLine) bool {
	for _, line := range paragraph {
		raw := line.raw()
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

// baseLineEndPairs returns the word pairs adjacent across a line end inside a comment block of src.
func baseLineEndPairs(src string) map[string]bool {
	pairs := map[string]bool{}
	for _, block := range commentBlocks(src) {
		for _, paragraph := range paragraphs(block) {
			for i := 0; i+1 < len(paragraph); i++ {
				pairs[lineEndPair(paragraph[i], paragraph[i+1])] = true
			}
		}
	}
	return pairs
}

// endsSemanticBreak reports whether a line may end where it does: at a sentence end, a semicolon, or a comma before a coordinating conjunction that opens a clause.
func endsSemanticBreak(before, after commentLine) bool {
	trimmed := strings.TrimRight(before.content, " \t")
	if sentenceEndPattern.MatchString(trimmed) || strings.HasSuffix(trimmed, ";") {
		return true
	}
	if strings.HasSuffix(trimmed, ",") {
		words := strings.Fields(after.content)
		return conjunctions[strings.ToLower(words[0])] && opensClause(clauseWords(words[1:]))
	}
	return false
}

// clauseWords returns the words up to and including the first one that ends with `,`, `;`, `:` or a sentence end, or all of them when none does.
func clauseWords(words []string) []string {
	for i, word := range words {
		if strings.HasSuffix(word, ",") || strings.HasSuffix(word, ";") || sentenceEndPattern.MatchString(word) {
			return words[:i+1]
		}
	}
	return words
}

// opensClause reports whether the words after a coordinating conjunction open an independent clause:
// they start with a subject and hold a finite-verb candidate after the subject's head word.
// The subject is a subject or demonstrative pronoun, a determiner followed by a word, or a backticked identifier.
// The verb candidate is a closed set of auxiliaries and modals, or a word ending in `s` or `ed`.
// An uncertain case reads as no clause, since joining the two lines always passes.
func opensClause(words []string) bool {
	if len(words) == 0 {
		return false
	}
	first := strings.ToLower(trimWordPunctuation(words[0]))
	headIndex := 0
	switch {
	case strings.HasPrefix(words[0], "`"):
	case subjectPronouns[first]:
	case determiners[first]:
		headIndex = 1
	default:
		return false
	}
	if headIndex+1 > len(words) {
		return false
	}
	for _, word := range words[headIndex+1:] {
		if isFiniteVerbCandidate(strings.ToLower(trimWordPunctuation(word))) {
			return true
		}
	}
	return false
}

// trimWordPunctuation removes the punctuation and backticks around a word.
func trimWordPunctuation(word string) string {
	return strings.Trim(word, ".,;:!?()\"'`")
}

// isFiniteVerbCandidate reports whether a lower-case word may be a finite verb: an auxiliary or modal, or a word ending in `s` or `ed`.
func isFiniteVerbCandidate(word string) bool {
	return auxiliaries[word] || strings.HasSuffix(word, "s") || strings.HasSuffix(word, "ed")
}

// findNewWrappedBreaks returns the lines of path's new text that end at a break, new against baseText, that is not a semantic one.
func findNewWrappedBreaks(path, baseText, newText string) []Finding {
	if generatedPattern.MatchString(newText) {
		return nil
	}
	known := baseLineEndPairs(baseText)
	var findings []Finding
	for _, block := range commentBlocks(newText) {
		for _, paragraph := range paragraphs(block) {
			if !isProseParagraph(paragraph) {
				continue
			}
			for i := 0; i+1 < len(paragraph); i++ {
				before, after := paragraph[i], paragraph[i+1]
				if known[lineEndPair(before, after)] || endsSemanticBreak(before, after) {
					continue
				}
				findings = append(findings, Finding{File: path, Line: before.number, Text: strings.TrimSpace(before.literal)})
			}
		}
	}
	return findings
}
