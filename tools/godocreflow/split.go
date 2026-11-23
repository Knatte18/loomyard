// split.go holds the splitter that decides where a semantic line break goes.
// It breaks at sentence ends, semicolons, and independent-clause conjunctions,
// and it keeps only the breaks the comment line rules accept, splitting a line that reaches the length limit.
// The docstring structure handling is not here; reflow.go does that part.

package main

import (
	"regexp"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/commentlint"
)

// abbreviations lists lowercase words whose trailing period must never be treated as a sentence end.
var abbreviations = map[string]bool{
	"e.g": true, "i.e": true, "etc": true, "vs": true, "cf": true, "approx": true,
	"no": true, "fig": true, "eq": true, "pp": true, "dr": true, "mr": true,
	"mrs": true, "ms": true, "st": true, "inc": true, "ltd": true, "u.s": true,
	"u.k": true, "vol": true, "ch": true,
}

var (
	sentenceEndRE     = regexp.MustCompile(`([.!?])(["')\]]*)(\s+)`)
	clauseRE          = regexp.MustCompile(`,\s+(and|but|or|nor)\s+`)
	semicolonRE       = regexp.MustCompile(`;\s+`)
	precedingWordRE   = regexp.MustCompile(`[A-Za-z]+$`)
	listMarkerRE      = regexp.MustCompile(`(?:^|[\s(])[0-9]{1,3}$`)
	bareParticipleRE  = regexp.MustCompile(`^[A-Za-z]+(?:ed|ing)\b`)
	clauseTailRE      = regexp.MustCompile(`^[^.,;]*`)
	wordRE            = regexp.MustCompile(`[A-Za-z0-9_']+`)
	alnumFirstByteSet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// inBacktickSpan reports whether pos falls inside a single- or double-backtick code span.
func inBacktickSpan(text string, pos int) bool {
	before := text[:pos]
	for _, marker := range []string{"``", "`"} {
		if strings.Count(before, marker)%2 == 1 {
			return true
		}
	}
	return false
}

// inParenSpan reports whether pos falls inside an unclosed '(' ... ')' span.
func inParenSpan(text string, pos int) bool {
	depth := 0
	for _, ch := range text[:pos] {
		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth > 0
}

func precedingWord(text string, pos int) string {
	return precedingWordRE.FindString(text[:pos])
}

func looksLikeURL(text string, pos int) bool {
	start := lastIndexAny(text[:pos], " \n") + 1
	token := text[start:pos]
	return strings.Contains(token, "://") || strings.HasPrefix(token, "www.")
}

func lastIndexAny(s string, cutset string) int {
	best := -1
	for _, c := range cutset {
		if i := strings.LastIndexByte(s, byte(c)); i > best {
			best = i
		}
	}
	return best
}

// looksLikeListMarker reports whether the period at pos ends a numbered-list marker.
// Such a marker is a number like "1." at the start of an item, not a sentence end.
func looksLikeListMarker(text string, pos int) bool {
	return listMarkerRE.MatchString(text[:pos])
}

// splitSentences splits a prose string into a list of sentences (no trailing whitespace).
func splitSentences(text string) []string {
	var sentences []string
	last := 0
	for _, m := range sentenceEndRE.FindAllStringSubmatchIndex(text, -1) {
		// m: [wholeStart, wholeEnd, punctStart, punctEnd, quotesStart, quotesEnd, wsStart, wsEnd]
		wholeEnd := m[1]
		punctStart := m[2]
		quotesEnd := m[5]

		if wholeEnd >= len(text) || !strings.ContainsRune(alnumFirstByteSet, rune(text[wholeEnd])) {
			continue
		}
		word := precedingWord(text, punctStart)
		if abbreviations[strings.ToLower(word)] {
			continue
		}
		if looksLikeURL(text, punctStart) {
			continue
		}
		if inBacktickSpan(text, punctStart) {
			continue
		}
		if looksLikeListMarker(text, punctStart) {
			continue
		}
		end := quotesEnd
		sentences = append(sentences, strings.TrimSpace(text[last:end]))
		last = wholeEnd
	}
	tail := strings.TrimSpace(text[last:])
	if tail != "" {
		sentences = append(sentences, tail)
	}
	if len(sentences) == 0 && strings.TrimSpace(text) != "" {
		sentences = append(sentences, strings.TrimSpace(text))
	}
	return sentences
}

// splitClauses splits one sentence at semicolons and at independent-clause commas.
func splitClauses(sentence string) []string {
	var pieces []string
	for _, chunk := range semicolonSplit(sentence) {
		pieces = append(pieces, conjunctionSplit(chunk)...)
	}
	out := pieces[:0]
	for _, p := range pieces {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// semicolonSplit splits text at semicolons.
// It skips a semicolon inside a backtick or paren span.
// It also skips one followed by a bare past-participle or gerund phrase such as "serialised to X".
// That phrase continues the sentence rather than starting a new clause.
func semicolonSplit(text string) []string {
	var parts []string
	last := 0
	for _, m := range semicolonRE.FindAllStringIndex(text, -1) {
		start, end := m[0], m[1]
		if inBacktickSpan(text, start) || inParenSpan(text, start) {
			continue
		}
		tail := strings.TrimLeft(text[end:], " \t\n")
		if bareParticipleRE.MatchString(tail) {
			continue
		}
		parts = append(parts, text[last:start+1])
		last = end
	}
	parts = append(parts, text[last:])
	return parts
}

// hasSubjectAndVerb is a cheap proxy for "has its own subject and verb".
// An independent clause needs at least two real words.
// A bare one-word tail such as "or None." is a compound object or alternative, not a new clause.
func hasSubjectAndVerb(clauseTail string) bool {
	return len(wordRE.FindAllString(clauseTail, -1)) >= 2
}

// conjunctionSplit splits at ", and/but/or/nor " when the text after it looks like an independent clause.
// A list item or a compound predicate is not split.
//
// Heuristic: only a comma with no earlier comma before it in the piece being split is eligible.
// A later comma belongs to an enumerated list (an Oxford comma), not a two-clause join.
// The clause after the conjunction must contain at least two words, a proxy for "has its own subject and verb".
func conjunctionSplit(text string) []string {
	var parts []string
	last := 0
	searchFrom := 0
	for {
		rel := clauseRE.FindStringSubmatchIndex(text[searchFrom:])
		if rel == nil {
			break
		}
		// offset every index by searchFrom to get absolute positions.
		m := make([]int, len(rel))
		for i, v := range rel {
			if v < 0 {
				m[i] = v
			} else {
				m[i] = v + searchFrom
			}
		}
		commaPos := m[0]
		conjStart, conjEnd := m[2], m[3]
		wholeEnd := m[1]

		if inBacktickSpan(text, commaPos) || inParenSpan(text, commaPos) {
			searchFrom = wholeEnd
			continue
		}
		if strings.Contains(text[last:commaPos], ",") {
			searchFrom = wholeEnd
			continue
		}
		tail := clauseTailRE.FindString(text[wholeEnd:])
		if !hasSubjectAndVerb(tail) {
			searchFrom = wholeEnd
			continue
		}
		parts = append(parts, text[last:commaPos+1])
		last = conjStart // keep "and"/"but"/"or"/"nor" with the new clause
		searchFrom = conjEnd
	}
	parts = append(parts, text[last:])
	return parts
}

// reflowText collapses existing hard-wraps in text and re-splits it into semantic lines.
// Each returned line has no indentation and no trailing whitespace.
func reflowText(text string) []string {
	joined := strings.Join(strings.Fields(text), " ")
	var lines []string
	for _, sentence := range splitSentences(joined) {
		lines = append(lines, splitClauses(sentence)...)
	}
	return lines
}

// layoutParagraph lays the text of one prose paragraph out as semantic lines, given the paragraph's lines as written.
// It starts from the sentence and clause splits and the written breaks the comment line rules accept.
// It keeps a break only where those rules accept it or the paragraph as written already breaks there.
// A line whose text reaches limit is split at such a break, as late as keeps each piece under limit.
// It returns nil when the layout holds a line that reaches limit and that the paragraph does not hold as written:
// such a line is new to the lint, so the paragraph is better left as written.
func layoutParagraph(written []string, limit int) []string {
	writtenBreaks := map[int]bool{}
	acceptedWrittenBreaks := map[int]bool{}
	wordsBefore := 0
	for i, line := range written[:len(written)-1] {
		wordsBefore += len(strings.Fields(line))
		writtenBreaks[wordsBefore] = true
		if commentlint.IsSemanticBreak(line, written[i+1]) {
			acceptedWrittenBreaks[wordsBefore] = true
		}
	}

	var lines [][]string
	for _, line := range reflowText(strings.Join(written, " ")) {
		lines = append(lines, strings.Fields(line))
	}
	lines = splitAtBreaks(lines, acceptedWrittenBreaks)
	lines = joinRejectedBreaks(lines, writtenBreaks)
	var split [][]string
	wordsBefore = 0
	for _, line := range lines {
		split = append(split, splitLongLine(line, wordsBefore, writtenBreaks, limit)...)
		wordsBefore += len(line)
	}
	lines = joinRejectedBreaks(split, writtenBreaks)

	writtenTexts := map[string]bool{}
	for _, line := range written {
		writtenTexts[strings.TrimSpace(line)] = true
	}
	result := make([]string, len(lines))
	for i, line := range lines {
		text := strings.Join(line, " ")
		if commentlint.TextReachesLimit(text, limit) && !writtenTexts[text] {
			return nil
		}
		result[i] = text
	}
	return result
}

// splitAtBreaks splits lines, each given as its words, at every break that falls inside a line.
// breaks holds the paragraph's word counts before each break.
func splitAtBreaks(lines [][]string, breaks map[int]bool) [][]string {
	var result [][]string
	wordsBefore := 0
	for _, line := range lines {
		start := 0
		for n := 1; n < len(line); n++ {
			if breaks[wordsBefore+n] {
				result = append(result, line[start:n])
				start = n
			}
		}
		result = append(result, line[start:])
		wordsBefore += len(line)
	}
	return result
}

// joinRejectedBreaks joins each line, given as its words, onto the line before it across a rejected break.
// A break is rejected when the comment line rules do not accept it and writtenBreaks does not hold it.
// writtenBreaks holds the paragraph's word counts before each break as written.
func joinRejectedBreaks(lines [][]string, writtenBreaks map[int]bool) [][]string {
	joined := [][]string{lines[0]}
	wordsBefore := len(lines[0])
	for _, line := range lines[1:] {
		last := len(joined) - 1
		if writtenBreaks[wordsBefore] || commentlint.IsSemanticBreak(strings.Join(joined[last], " "), strings.Join(line, " ")) {
			joined = append(joined, line)
		} else {
			joined[last] = slices.Concat(joined[last], line)
		}
		wordsBefore += len(line)
	}
	return joined
}

// splitLongLine splits a line, given as its words, whose text reaches limit.
// It splits only where the comment line rules accept a break or where writtenBreaks holds one.
// At each split it takes the latest such break that keeps the piece under limit, or the earliest when none does.
// wordsBefore is the paragraph's word count before the line, the offset into writtenBreaks.
func splitLongLine(words []string, wordsBefore int, writtenBreaks map[int]bool, limit int) [][]string {
	reachesLimit := func(piece []string) bool {
		return commentlint.TextReachesLimit(strings.Join(piece, " "), limit)
	}
	var pieces [][]string
	start := 0
	for start < len(words) {
		rest := words[start:]
		if !reachesLimit(rest) {
			pieces = append(pieces, rest)
			break
		}
		cut := 0
		for n := 1; n < len(rest); n++ {
			breakAccepted := writtenBreaks[wordsBefore+start+n] ||
				commentlint.IsSemanticBreak(strings.Join(rest[:n], " "), strings.Join(rest[n:], " "))
			if !breakAccepted {
				continue
			}
			if reachesLimit(rest[:n]) {
				if cut == 0 {
					cut = n
				}
				break
			}
			cut = n
		}
		if cut == 0 {
			pieces = append(pieces, rest)
			break
		}
		pieces = append(pieces, rest[:cut])
		start += cut
	}
	return pieces
}
