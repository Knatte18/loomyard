// split.go holds the splitter that decides where a semantic line break goes.
// It breaks at sentence ends, semicolons, and independent-clause conjunctions.
// The docstring structure handling is not here; reflow.go does that part.

package main

import (
	"regexp"
	"strings"
	"unicode/utf8"
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

// wrapLongLine is a last-resort fallback for a line that no sentence, semicolon or conjunction split can shorten.
// prefixLen is the visual width of everything before the content on its rendered line: indentation plus "// ".
// The content is returned unchanged as a single-element slice when the line fits.
// It fits when prefixLen plus the content's width is at most maxWidth, or when maxWidth is non-positive (disabled).
//
// This is a targeted exception to semantic line breaks, not general fixed-column wrapping.
// It fires only on a line the boundary splitting could not shorten,
// and it greedily packs words up to the column budget rather than breaking mid-phrase at an arbitrary column.
func wrapLongLine(content string, prefixLen, maxWidth int) []string {
	if maxWidth <= 0 || prefixLen+utf8.RuneCountInString(content) <= maxWidth {
		return []string{content}
	}
	budget := maxWidth - prefixLen
	words := strings.Fields(content)
	if budget < 1 || len(words) == 0 {
		return []string{content} // pathological config or nothing to wrap -- leave as-is
	}

	var lines []string
	cur := words[0]
	curLen := utf8.RuneCountInString(cur)
	for _, w := range words[1:] {
		wLen := utf8.RuneCountInString(w)
		if curLen+1+wLen <= budget {
			cur += " " + w
			curLen += 1 + wLen
			continue
		}
		lines = append(lines, cur)
		cur, curLen = w, wLen
	}
	lines = append(lines, cur)
	return lines
}
