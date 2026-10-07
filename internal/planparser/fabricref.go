// fabricref.go implements the card-fabric-reference check: a command a plan carries that reaches the fabric repo is refused at the plan gate, before an implementer runs it.
// It scans only the raw text the parse step recorded, so planparser stays the sole parser of the plan files.

package planparser

import (
	"fmt"
	"strings"
)

const cardFabricReferenceCheck = "card-fabric-reference"

// FabricReferenceMatcher is the narrow seam through which CheckCardFabricReference receives the fabric-reference rule, so planparser imports neither the fabric side nor its naming.
// Each method returns the matched text and whether cmd matched.
type FabricReferenceMatcher interface {
	// MatchPath reports a path that names a sibling worktree of the fabric repo.
	MatchPath(cmd string) (string, bool)
	// MatchSpelling reports a command spelling that drives the fabric repo.
	MatchSpelling(cmd string) (string, bool)
}

// scanSpan is one piece of plan text handed to the matcher on its own, so quote state never carries between spans.
type scanSpan struct {
	text string
	// section is the overview heading the span sits under, empty for a card file or before the first heading.
	section string
	// spelling reports whether the spelling rule runs on the span besides the path rule.
	spelling bool
}

// CheckCardFabricReference reports one card-fabric-reference finding per distinct command a plan carries that reaches the fabric repo.
// The spans scanned are a card's `**Verify:**` value and the overview's `## verify:` section body (path rule and spelling rule), every fenced code block of every plan file whatever its info string (both rules), and every inline code span of every plan file (path rule only, because a span documenting a command opens with the spelling).
// Prose outside code is never scanned.
// A finding names the card, or for a hit in the overview the section it is in, and the matched text; its way forward is committed test data an earlier card creates, or dropping the command.
// It sits outside ValidateFormat and Validate, since only a caller holding a matcher can run it.
// The check only refuses and removes no guard.
// It misses a spelling in `bash -c "…"` or built from a variable, the rest of a span after an unterminated quote for the bare-word and spelling patterns, a spelling in an inline span or prose, and a fabric-repo path not spelled as a name ending in the suffix; the implementer audit stays the guard for those.
func CheckCardFabricReference(plan *Plan, matcher FabricReferenceMatcher) []ValidationError {
	var findings []ValidationError
	seen := make(map[string]bool)
	report := func(card string, span scanSpan, matched string) {
		where := "card " + card
		if card == "" {
			where = "the overview"
			if span.section != "" {
				where = fmt.Sprintf("the overview's %q section", span.section)
			}
		}
		detail := fmt.Sprintf("%s runs a command that reaches the fabric repo: %q; take the data from committed test data an earlier card creates, or drop the command", where, matched)
		if seen[detail] {
			return
		}
		seen[detail] = true
		findings = append(findings, ValidationError{Check: cardFabricReferenceCheck, Card: card, Detail: detail})
	}
	scan := func(card string, spans []scanSpan) {
		for _, span := range spans {
			if matched, ok := matcher.MatchPath(span.text); ok {
				report(card, span, matched)
			}
			if !span.spelling {
				continue
			}
			if matched, ok := matcher.MatchSpelling(span.text); ok {
				report(card, span, matched)
			}
		}
	}

	scan("", overviewSpans(plan.OverviewText))
	for _, card := range plan.Cards {
		spans := codeSpans(card.Text)
		if card.Verify != "" {
			spans = append(spans, scanSpan{text: card.Verify, spelling: true})
		}
		scan(cardID(card), spans)
	}
	return findings
}

// overviewSpans returns the overview's `## verify:` section body and its code spans, each tagged with the section it sits in.
func overviewSpans(text string) []scanSpan {
	spans := codeSpans(text)
	if body := joinSectionBody(extractSection(text, planVerifyHeading)); body != "" {
		spans = append(spans, scanSpan{text: body, section: planVerifyHeading, spelling: true})
	}
	return spans
}

// codeSpans returns every fenced block body (spelling rule too) and every inline code span (path rule only) in a plan file's text.
// A fence closes on a line that starts with the opening fence's character repeated at least as many times; an unclosed fence runs to the end of the text.
// An inline span is a backtick run closed by a run of the same length on the same line.
func codeSpans(text string) []scanSpan {
	var spans []scanSpan
	section := ""
	var fence string
	var block []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, fence[:1]) == "" {
				spans = append(spans, scanSpan{text: strings.Join(block, "\n"), section: section, spelling: true})
				fence, block = "", nil
				continue
			}
			block = append(block, line)
			continue
		}
		if opener := fenceOpener(trimmed); opener != "" {
			fence = opener
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			section = trimmed
		}
		for _, inline := range inlineSpans(line) {
			spans = append(spans, scanSpan{text: inline, section: section})
		}
	}
	if fence != "" {
		spans = append(spans, scanSpan{text: strings.Join(block, "\n"), section: section, spelling: true})
	}
	return spans
}

// fenceOpener returns the fence run (three or more backticks or tildes) a trimmed line opens, or "" when it opens none.
func fenceOpener(trimmed string) string {
	if trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return ""
	}
	run := len(trimmed) - len(strings.TrimLeft(trimmed, trimmed[:1]))
	if run < 3 {
		return ""
	}
	return trimmed[:run]
}

// inlineSpans returns the content of every inline code span on one line.
func inlineSpans(line string) []string {
	var spans []string
	for i := 0; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		run := len(line[i:]) - len(strings.TrimLeft(line[i:], "`"))
		open := strings.Repeat("`", run)
		rest := line[i+run:]
		end := indexBacktickRun(rest, run)
		if end < 0 {
			i += run
			continue
		}
		spans = append(spans, rest[:end])
		i += run + end + len(open)
	}
	return spans
}

// indexBacktickRun returns the index in s of the first backtick run of exactly n backticks, or -1.
func indexBacktickRun(s string, n int) int {
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		run := len(s[i:]) - len(strings.TrimLeft(s[i:], "`"))
		if run == n {
			return i
		}
		i += run
	}
	return -1
}
