// lint.go declares Lint, its Finding, and the classification of a comment block's line breaks over two texts of one file.

package commentlint

import (
	"fmt"
	"go/scanner"
	"go/token"
	"regexp"
	"slices"
	"strings"
)

// Rule names the comment line rule a Finding breaks.
type Rule string

// The comment line rules Lint checks.
const (
	// RuleWrappedBreak is a line that ends at a fixed-column wrap instead of a sentence or clause boundary.
	RuleWrappedBreak Rule = "wrapped break"
	// RuleLineLength is a line whose text reaches LineLengthLimit characters.
	RuleLineLength Rule = "line length"
)

// Finding is one line of a `//` comment block that breaks a comment line rule.
type Finding struct {
	// File is the slash-separated path relative to the worktree.
	File string
	// Line is the 1-based line number in the new text.
	Line int
	// Rule is the rule the line breaks.
	Rule Rule
	// Text is the comment line, `//` included, without surrounding whitespace.
	Text string
}

// String renders the finding as `file:line: rule: text`.
func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.File, f.Line, f.Rule, f.Text)
}

var (
	generatedPattern   = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.?\s*$`)
	directivePattern   = regexp.MustCompile(`^(go:|lym:|testtiming:|nolint\b)`)
	headingPattern     = regexp.MustCompile(`^#(\s|$)`)
	listItemPattern    = regexp.MustCompile(`^([-*]\s|[0-9]{1,3}[.)]\s)`)
	sentenceEndPattern = regexp.MustCompile("[.!?:][)\"'`]*$")
	conjunctions       = map[string]bool{"and": true, "but": true, "or": true, "nor": true, "yet": true, "so": true}
	subjectPronouns    = wordSet("it", "they", "we", "he", "she", "i", "you", "this", "that", "these", "those", "there")
	determiners        = wordSet("the", "a", "an", "its", "their", "our", "his", "her", "my", "your", "each", "every", "any", "some", "no", "another", "both", "all")
	auxiliaries        = wordSet("is", "are", "was", "were", "be", "been", "has", "have", "had", "do", "does", "did", "can", "could", "will", "would", "shall", "should", "may", "might", "must")
)

func wordSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	return set
}

// Lint returns the comment line findings of the `.go` files that differ in worktree:
// the new fixed-column-wrapped breaks in their `//` comment blocks,
// and the new or changed comment lines whose text reaches LineLengthLimit.
// An empty base compares the working tree, untracked files included, against HEAD;
// a base commit compares it against HEAD.
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
		findings = append(findings, findFileFindings(change.path, baseText, newText)...)
	}
	return findings, nil
}

// findFileFindings returns the findings of every rule in path's new text against its base text, ordered by line.
// It returns nil for a generated file.
func findFileFindings(path, baseText, newText string) []Finding {
	if generatedPattern.MatchString(newText) {
		return nil
	}
	findings := append(findNewWrappedBreaks(path, baseText, newText), findNewLongLines(path, baseText, newText)...)
	slices.SortStableFunc(findings, func(a, b Finding) int { return a.Line - b.Line })
	return findings
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
// An end-of-line comment belongs to no block and ends the run before it;
// so does a directive line, which belongs to no block either.
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

// linePair is the words of the two lines either side of a break.
type linePair struct {
	before, after []string
}

// baseBreaks is the line breaks inside the comment blocks of a base text.
type baseBreaks struct {
	// wordPairs holds the word pairs adjacent across a base line end.
	wordPairs map[string]bool
	// linePairsByShape holds the base line pairs, keyed by the word counts of their two lines.
	linePairsByShape map[[2]int][]linePair
}

// newBaseBreaks collects the line breaks inside the comment blocks of src.
func newBaseBreaks(src string) baseBreaks {
	breaks := baseBreaks{wordPairs: map[string]bool{}, linePairsByShape: map[[2]int][]linePair{}}
	for _, block := range commentBlocks(src) {
		for _, paragraph := range paragraphs(block) {
			for i := 0; i+1 < len(paragraph); i++ {
				before, after := paragraph[i], paragraph[i+1]
				breaks.wordPairs[lineEndPair(before, after)] = true
				pair := linePair{before: strings.Fields(before.content), after: strings.Fields(after.content)}
				shape := [2]int{len(pair.before), len(pair.after)}
				breaks.linePairsByShape[shape] = append(breaks.linePairsByShape[shape], pair)
			}
		}
	}
	return breaks
}

// carriesOver reports whether the break between two new lines already stood in the base text.
// It holds when the base holds the same word pair across a line end.
// It also holds when the base holds a line pair that differs from the two lines only by words substituted in place.
// A substitution such as a renamed word counts when the unchanged words outnumber the substituted ones.
func (b baseBreaks) carriesOver(before, after commentLine) bool {
	if b.wordPairs[lineEndPair(before, after)] {
		return true
	}
	pair := linePair{before: strings.Fields(before.content), after: strings.Fields(after.content)}
	for _, basePair := range b.linePairsByShape[[2]int{len(pair.before), len(pair.after)}] {
		unchanged := countEqualPositions(basePair.before, pair.before) + countEqualPositions(basePair.after, pair.after)
		if 2*unchanged > len(pair.before)+len(pair.after) {
			return true
		}
	}
	return false
}

// countEqualPositions returns the number of positions at which two equally long word lists hold the same word.
func countEqualPositions(first, second []string) int {
	count := 0
	for i := range first {
		if first[i] == second[i] {
			count++
		}
	}
	return count
}

// endsSemanticBreak reports whether a line may end where it does:
// at a sentence end, a semicolon, or a comma before a coordinating conjunction that opens a clause.
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

// clauseWords returns the words up to and including the first one that ends with `,`, `;`, `:` or a sentence end.
// It returns all the words when none does.
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

// isFiniteVerbCandidate reports whether a lower-case word may be a finite verb:
// an auxiliary or modal, or a word ending in `s` or `ed`.
func isFiniteVerbCandidate(word string) bool {
	return auxiliaries[word] || strings.HasSuffix(word, "s") || strings.HasSuffix(word, "ed")
}

// findNewWrappedBreaks returns the lines of path's new text that end at a break:
// a break that is not semantic and does not carry over from baseText.
func findNewWrappedBreaks(path, baseText, newText string) []Finding {
	known := newBaseBreaks(baseText)
	var findings []Finding
	for _, block := range commentBlocks(newText) {
		for _, paragraph := range paragraphs(block) {
			if !isProseParagraph(paragraph) {
				continue
			}
			for i := 0; i+1 < len(paragraph); i++ {
				before, after := paragraph[i], paragraph[i+1]
				if endsSemanticBreak(before, after) || known.carriesOver(before, after) {
					continue
				}
				findings = append(findings, Finding{File: path, Line: before.number, Rule: RuleWrappedBreak, Text: strings.TrimSpace(before.literal)})
			}
		}
	}
	return findings
}
