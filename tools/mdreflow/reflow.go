// reflow.go implements the semantic-line-break reflow rule from the
// mill:markdown skill / CLAUDE.md's "Markdown: semantic line breaks" section:
// one sentence per line, plus a break at an internal independent-clause
// boundary (semicolon, or comma+coordinating-conjunction+explicit-subject).
//
// This is a Go port of millhouse's plugins/mill/scripts/tools/mdreflow/mdreflow.py
// (same algorithm, same edge cases). Line breaks only -- never rewords,
// reorders, or drops content. Never touches fenced code blocks (nested fences
// tracked by backtick/tilde-run-length matching, not a naive toggle), inline
// code spans, links/autolinks, YAML frontmatter, headings, tables,
// blockquotes, thematic breaks, or link-reference definitions.
//
// The one real porting wrinkle: Go's regexp (RE2) has no lookaround or
// backreferences, which mdreflow.py's code-span mask relied on
// ("(`+)(.+?)(?<!`)\1(?!`)"). maskCodeSpans below replaces that with a
// two-step scan: find a maximal backtick run (an "opener"), then scan
// forward for the next maximal run of the same length (the "closer"),
// backtracking the opener's length on failure -- equivalent to CommonMark's
// backtick-string code-span rule, and to what the Python regex actually
// matched in practice.
package main

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// maskDelim is a Private Use Area rune, chosen (matching the Python original)
// because it cannot occur in real markdown prose, so masked-span placeholders
// can never collide with surrounding text.
const maskDelim = ''

var (
	listMarkerRE       = regexp.MustCompile(`^(?P<indent>[ \t]*)(?P<marker>[-*+]|\d{1,9}[.)])(?P<spaces>\s+)(?P<rest>.*)$`)
	headingRE          = regexp.MustCompile(`^\s*#{1,6}(\s|$)`)
	blockquoteRE       = regexp.MustCompile(`^\s*>`)
	hrRE               = regexp.MustCompile(`^\s*-{3,}\s*$|^\s*\*{3,}\s*$|^\s*_{3,}\s*$`)
	linkdefRE          = regexp.MustCompile(`^\s*\[[^\]]+\]:\s`)
	htmlBlockRE        = regexp.MustCompile(`^\s*<[a-zA-Z!/]`)
	tableSepRE         = regexp.MustCompile(`^:?-+:?(\s*\|\s*:?-+:?)+$`)
	leadingWSRE        = regexp.MustCompile(`^\s*`)
	fenceOpenRE        = regexp.MustCompile("^\\s*(`{3,}|~{3,})")
	fenceCloseBacktick = regexp.MustCompile("^\\s*(`{3,})\\s*$")
	fenceCloseTilde    = regexp.MustCompile(`^\s*(~{3,})\s*$`)
	whitespaceRunRE    = regexp.MustCompile(`\s+`)
	wordRE             = regexp.MustCompile(`[A-Za-z]+`)
	verbSuffixRE       = regexp.MustCompile(`(s|es|ed|ing)$`)

	autolinkRE = regexp.MustCompile(`<(?:https?|ftp)://[^>\s]+>`)
	mdLinkRE   = regexp.MustCompile(`\[([^\]\n]*)\]\(([^)\n]*)\)`)
	refLinkRE  = regexp.MustCompile(`\[([^\]\n]*)\]\[[^\]\n]*\]`)
	bareURLRE  = regexp.MustCompile(`https?://\S+`)

	maskRefRE = regexp.MustCompile(string(maskDelim) + `(\d+)` + string(maskDelim))

	// boundaryRE finds sentence/clause-boundary candidates: sentence-ending
	// punctuation, a semicolon, or a comma immediately followed by a
	// coordinating conjunction and an explicit subject.
	boundaryRE = regexp.MustCompile(
		`(?P<term>[.!?])(?P<closers>["'\)\]\*_` + "`" + `]*)(?P<sp1>\s+)` +
			`|(?P<semi>;)(?P<sp2>\s+)` +
			`|,(?P<sp3>\s+)\b(?P<conj>and|but|or)\b(?P<sp4>\s+)(?P<subj>[A-Za-z]+)`,
	)
)

var abbreviations = toSet([]string{
	"e.g", "i.e", "etc", "vs", "cf", "approx", "no", "fig", "eq", "al", "et",
	"a.m", "p.m", "u.s", "u.k", "vol", "ch", "pp", "p", "dr", "mr", "mrs",
	"ms", "prof", "sr", "jr", "st", "ave", "inc", "ltd", "co", "corp",
})

var pronouns = toSet([]string{
	"i", "you", "he", "she", "it", "we", "they",
	"this", "that", "these", "those",
})

var determiners = toSet([]string{
	"a", "an", "the", "this", "that", "these", "those", "my",
	"your", "his", "her", "its", "our", "their", "no", "some",
	"any", "each", "every", "many", "most", "such",
})

var prepositions = toSet([]string{
	"to", "of", "in", "on", "at", "for", "with", "from", "by",
	"as", "into", "onto", "about", "over", "under", "between",
	"among", "through", "during", "per", "via", "within",
	"without", "against", "across", "after", "before",
})

var verbAux = toSet([]string{
	"is", "are", "was", "were", "be", "been", "being", "has",
	"have", "had", "do", "does", "did", "will", "would", "can",
	"could", "shall", "should", "may", "might", "must", "took",
	"went", "came", "made", "said", "got", "put", "ran", "saw",
	"gave", "found", "told", "knew", "felt", "kept", "held",
	"meant", "began", "showed", "called", "needed", "seems",
	"needs", "means", "brought", "thought", "became",
})

var openMarkup = map[byte]bool{'*': true, '_': true, '`': true, '"': true, '\'': true, '[': true}

func toSet(words []string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// maskCodeSpans replaces every CommonMark-style code span (a backtick run,
// content, then the first later backtick run of equal length) with a
// placeholder token, appending the original span text to *spans. It mirrors
// mdreflow.py's `(`+)(.+?)(?<!`)\1(?!`)` regex by scanning maximal backtick
// runs directly instead of relying on lookaround: the opener's length is
// tried from its full run down to 1 (mirroring backtracking a greedy `+`
// quantifier) before the scan advances past a non-matching backtick.
func maskCodeSpans(text string, spans *[]string) string {
	var b strings.Builder
	n := len(text)
	i := 0
	for i < n {
		if text[i] != '`' {
			b.WriteByte(text[i])
			i++
			continue
		}
		j := i
		for j < n && text[j] == '`' {
			j++
		}
		maxLen := j - i
		matched := false
		for l := maxLen; l >= 1 && !matched; l-- {
			k := i + l
			for k < n {
				if text[k] != '`' {
					k++
					continue
				}
				k2 := k
				for k2 < n && text[k2] == '`' {
					k2++
				}
				runLen := k2 - k
				if runLen == l {
					*spans = append(*spans, text[i:k2])
					idx := len(*spans) - 1
					b.WriteRune(maskDelim)
					b.WriteString(strconv.Itoa(idx))
					b.WriteRune(maskDelim)
					i = k2
					matched = true
					break
				}
				k = k2
			}
		}
		if !matched {
			b.WriteByte(text[i])
			i++
		}
	}
	return b.String()
}

func maskRegex(text string, re *regexp.Regexp, spans *[]string) string {
	return re.ReplaceAllStringFunc(text, func(match string) string {
		*spans = append(*spans, match)
		idx := len(*spans) - 1
		return string(maskDelim) + strconv.Itoa(idx) + string(maskDelim)
	})
}

func maskText(text string) (string, []string) {
	var spans []string
	text = maskCodeSpans(text, &spans)
	text = maskRegex(text, autolinkRE, &spans)
	text = maskRegex(text, mdLinkRE, &spans)
	text = maskRegex(text, refLinkRE, &spans)
	text = maskRegex(text, bareURLRE, &spans)
	return text, spans
}

// unmaskText resolves placeholder tokens back to their original spans.
// Masking is layered -- code-span masking runs before the link/URL masks,
// so a link whose text contains a code span (e.g. `[`hardener`](url)`)
// captures a span that itself still contains a placeholder token. Each
// substitution pass can only replace placeholders present in its input, not
// ones freshly introduced by that same pass's replacements, so resolving
// fully requires looping until no placeholder remains.
func unmaskText(text string, spans []string) string {
	for maskRefRE.MatchString(text) {
		text = maskRefRE.ReplaceAllStringFunc(text, func(m string) string {
			sub := maskRefRE.FindStringSubmatch(m)
			idx, _ := strconv.Atoi(sub[1])
			return spans[idx]
		})
	}
	return text
}

func precedingWord(s string, pos int) string {
	j := pos
	for j > 0 && (isAlpha(s[j-1]) || s[j-1] == '.') {
		j--
	}
	return s[j:pos]
}

func nextSentenceStartOK(s string, pos int) bool {
	j := pos
	n := len(s)
	for j < n && openMarkup[s[j]] {
		j++
	}
	if j >= n {
		return false
	}
	c := s[j]
	return c >= 'A' && c <= 'Z'
}

// runeWindow returns the substring of s starting at byte offset pos, up to
// runeCount runes long (never fewer bytes than that many runes need, but
// also never splitting a multi-byte rune at the window edge -- unlike a
// fixed byte-length slice, which can truncate a multi-byte character such
// as an em dash mid-sequence).
func runeWindow(s string, pos int, runeCount int) string {
	if pos >= len(s) {
		return ""
	}
	tail := s[pos:]
	count := 0
	end := 0
	for end < len(tail) && count < runeCount {
		_, size := utf8.DecodeRuneInString(tail[end:])
		end += size
		count++
	}
	return tail[:end]
}

func hasVerbBeforePreposition(s string, pos int) bool {
	words := wordRE.FindAllString(runeWindow(s, pos, 60), -1)
	if len(words) > 6 {
		words = words[:6]
	}
	for _, w := range words {
		lw := strings.ToLower(w)
		if prepositions[lw] {
			return false
		}
		if verbAux[lw] || verbSuffixRE.MatchString(lw) {
			return true
		}
	}
	return false
}

func namedGroups(re *regexp.Regexp, m []string) map[string]string {
	result := make(map[string]string, len(m))
	for i, name := range re.SubexpNames() {
		if name != "" && i < len(m) {
			result[name] = m[i]
		}
	}
	return result
}

// findCuts locates sentence/clause-boundary cut points in s (already
// code-span/link masked), each as a [leftEnd, rightStart) byte-offset pair:
// text[leftEnd:rightStart] is the boundary whitespace/punctuation to drop.
func findCuts(s string) [][2]int {
	var cuts [][2]int
	matches := boundaryRE.FindAllStringSubmatchIndex(s, -1)
	names := boundaryRE.SubexpNames()
	idxOf := func(name string) int {
		for i, n := range names {
			if n == name {
				return i
			}
		}
		return -1
	}
	iTerm, iSp1 := idxOf("term"), idxOf("sp1")
	iSemi, iSp2 := idxOf("semi"), idxOf("sp2")
	iConj, iSubj := idxOf("conj"), idxOf("subj")

	get := func(m []int, i int) (int, int, bool) {
		if i < 0 || 2*i+1 >= len(m) {
			return -1, -1, false
		}
		st, en := m[2*i], m[2*i+1]
		if st < 0 {
			return -1, -1, false
		}
		return st, en, true
	}

	for _, m := range matches {
		if st, _, ok := get(m, iTerm); ok {
			word := strings.ToLower(precedingWord(s, st))
			if abbreviations[word] {
				continue
			}
			if len(word) == 1 {
				continue
			}
			sp1St, sp1En, _ := get(m, iSp1)
			if !nextSentenceStartOK(s, sp1En) {
				continue
			}
			cuts = append(cuts, [2]int{sp1St, sp1En})
			continue
		}
		if _, _, ok := get(m, iSemi); ok {
			sp2St, sp2En, _ := get(m, iSp2)
			cuts = append(cuts, [2]int{sp2St, sp2En})
			continue
		}
		if conjSt, _, ok := get(m, iConj); ok {
			subjSt, subjEn, _ := get(m, iSubj)
			subj := strings.ToLower(s[subjSt:subjEn])
			if !pronouns[subj] && !determiners[subj] {
				continue
			}
			lookahead := runeWindow(s, subjEn, 25)
			if strings.Contains(lookahead, "—") || strings.Contains(lookahead, "–") {
				continue
			}
			if !hasVerbBeforePreposition(s, subjEn) {
				continue
			}
			commaPos := m[0]
			cuts = append(cuts, [2]int{commaPos + 1, conjSt})
		}
	}
	return cuts
}

func splitSemantic(content string) []string {
	masked, spans := maskText(content)
	cuts := findCuts(masked)

	var chunks []string
	last := 0
	for _, c := range cuts {
		leftEnd, rightStart := c[0], c[1]
		chunks = append(chunks, strings.TrimSpace(masked[last:leftEnd]))
		last = rightStart
	}
	chunks = append(chunks, strings.TrimSpace(masked[last:]))

	var filtered []string
	for _, c := range chunks {
		if c != "" {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) == 0 {
		filtered = []string{strings.TrimSpace(masked)}
	}

	result := make([]string, len(filtered))
	for i, c := range filtered {
		result[i] = unmaskText(c, spans)
	}
	return result
}

func flushParagraph(buf *[]string, out *[]string) {
	b := *buf
	if len(b) == 0 {
		return
	}

	first := b[0]
	var prefixFirst, firstContent string
	if m := listMarkerRE.FindStringSubmatch(first); m != nil {
		g := namedGroups(listMarkerRE, m)
		prefixFirst = g["indent"] + g["marker"] + g["spaces"]
		firstContent = strings.TrimSpace(g["rest"])
	} else {
		prefixFirst = leadingWSRE.FindString(first)
		firstContent = strings.TrimSpace(first)
	}
	prefixCont := strings.Repeat(" ", len(prefixFirst))

	parts := make([]string, 0, len(b))
	parts = append(parts, firstContent)
	for _, line := range b[1:] {
		parts = append(parts, strings.TrimSpace(line))
	}
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	content := strings.Join(nonEmpty, " ")
	if content == "" {
		*buf = nil
		return
	}

	chunks := splitSemantic(content)
	*out = append(*out, prefixFirst+chunks[0])
	for _, c := range chunks[1:] {
		*out = append(*out, prefixCont+c)
	}
	*buf = nil
}

func classifySpecial(line string) string {
	if strings.TrimSpace(line) == "" {
		return "blank"
	}
	if headingRE.MatchString(line) {
		return "heading"
	}
	if blockquoteRE.MatchString(line) {
		return "blockquote"
	}
	if hrRE.MatchString(line) {
		return "hr"
	}
	if linkdefRE.MatchString(line) {
		return "linkdef"
	}
	if htmlBlockRE.MatchString(line) {
		return "html"
	}
	if isTableLine(line) {
		return "table"
	}
	return ""
}

func isTableLine(line string) bool {
	stripped := strings.TrimSpace(line)
	if stripped == "" {
		return false
	}
	if !strings.Contains(stripped, "|") {
		return false
	}
	noSpace := strings.ReplaceAll(stripped, " ", "")
	if tableSepRE.MatchString(noSpace) {
		return true
	}
	return strings.Contains(stripped, "|")
}

func fenceOpenMatch(line string) (char byte, count int, ok bool) {
	m := fenceOpenRE.FindStringSubmatch(line)
	if m == nil {
		return 0, 0, false
	}
	run := m[1]
	return run[0], len(run), true
}

func fenceCloseMatch(line string, char byte, count int) bool {
	var re *regexp.Regexp
	if char == '`' {
		re = fenceCloseBacktick
	} else {
		re = fenceCloseTilde
	}
	m := re.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	return len(m[1]) >= count
}

// reflowText applies the semantic-line-break rule to a whole markdown file's
// contents, leaving fenced code, inline code spans, links, YAML frontmatter,
// headings, tables, blockquotes, thematic breaks, and link-reference
// definitions untouched.
func reflowText(text string) string {
	lines := strings.Split(text, "\n")
	trailingNewline := strings.HasSuffix(text, "\n")
	var out []string
	var buf []string
	n := len(lines)
	i := 0

	if n > 0 && strings.TrimSpace(lines[0]) == "---" {
		j := 1
		for j < n && strings.TrimSpace(lines[j]) != "---" {
			j++
		}
		if j < n {
			out = append(out, lines[0:j+1]...)
			i = j + 1
		}
	}

	var fenceChar byte
	var fenceCount int
	inFence := false

	for i < n {
		line := lines[i]

		if inFence {
			out = append(out, line)
			if fenceCloseMatch(line, fenceChar, fenceCount) {
				inFence = false
			}
			i++
			continue
		}

		if ch, cnt, ok := fenceOpenMatch(line); ok {
			flushParagraph(&buf, &out)
			out = append(out, line)
			fenceChar, fenceCount = ch, cnt
			inFence = true
			i++
			continue
		}

		if kind := classifySpecial(line); kind != "" {
			flushParagraph(&buf, &out)
			out = append(out, line)
			i++
			continue
		}

		if listMarkerRE.MatchString(line) {
			flushParagraph(&buf, &out)
			buf = append(buf, line)
			i++
			continue
		}

		buf = append(buf, line)
		i++
	}

	flushParagraph(&buf, &out)

	result := strings.Join(out, "\n")
	if trailingNewline && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result
}

// collapsed collapses all whitespace runs to a single space, for the
// safety-invariant check: the reflowed text and the original must be
// byte-identical once collapsed this way, or the reflow altered content
// rather than just line breaks.
func collapsed(text string) string {
	return strings.TrimSpace(whitespaceRunRE.ReplaceAllString(text, " "))
}
