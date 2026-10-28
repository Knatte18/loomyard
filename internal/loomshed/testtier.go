// testtier.go holds the plan gate's test-tier-unstated check: a card creating a test file as a file target must state the file's build tier in its Intent.

package loomshed

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// testTierUnstatedCheck names the finding testTierFindings appends for a created test file whose naming Intent lines do not state exactly one tier.
const testTierUnstatedCheck = "test-tier-unstated"

// untaggedWord is the Intent word that states a created test file holds support only and carries no build line.
const untaggedWord = "untagged"

// testTierWayForward is the way-forward clause every test-tier-unstated finding carries.
const testTierWayForward = "way forward: add an Intent line naming the file and its `//go:build` line, such as `//go:build tier0`, or the word `untagged` when the file holds support only (TestMain, helpers) and carries no build line, on a line that names no other test file; where the lines name several tags, join them with || only for a helper several tiers share, else keep one; drop untagged or the tag when both appear"

// testTierFindings returns one test-tier-unstated finding per test file that a card of cards creates as a file target and whose naming Intent lines state no valid tier.
// It returns nothing unless plan is a go plan and tierTags is non-empty.
// A file passes when its naming lines hold exactly one distinct tag; several tags, each consecutive pair joined by || on one line; or the word untagged and no tag.
func testTierFindings(plan *planparser.Plan, cards []planparser.Card, tierTags []string) []planindex.Finding {
	if !plan.GoToolchain() || len(tierTags) == 0 {
		return nil
	}
	var findings []planindex.Finding
	for _, card := range cards {
		lines := strings.Split(card.Intent, "\n")
		for _, group := range card.TargetGroups {
			if group.Type != planparser.CardTypeCreate {
				continue
			}
			for _, ref := range group.Refs {
				path, ok := planparser.PathEntryPath(ref)
				if !ok || !strings.HasSuffix(path, "_test.go") {
					continue
				}
				statement := collectTierStatement(lines, path, tierTags)
				if statement.valid() {
					continue
				}
				findings = append(findings, planindex.Finding{
					Check:    testTierUnstatedCheck,
					Card:     card.ID(),
					Detail:   fmt.Sprintf("card %d creates %s and its Intent lines naming it state %s; %s", card.Number, path, statement.describe(), testTierWayForward),
					Severity: planindex.SeverityBlocking,
				})
			}
		}
	}
	return findings
}

// tierStatement is what the Intent lines naming one test file say about its tier.
type tierStatement struct {
	// tags are the distinct tier tags the lines name, in order of first appearance.
	tags []string
	// untagged reports whether any line carries the untagged word.
	untagged bool
	// joinedCounts holds, per line whose tag occurrences are all joined by ||, the number of distinct tags it names.
	joinedCounts []int
}

// valid reports whether the statement is exactly one tag, a || join of several, or untagged alone.
func (s tierStatement) valid() bool {
	switch {
	case s.untagged:
		return len(s.tags) == 0
	case len(s.tags) == 1:
		return true
	default:
		return len(s.tags) > 1 && slices.Contains(s.joinedCounts, len(s.tags))
	}
}

// describe renders the statement for a finding's Detail.
func (s tierStatement) describe() string {
	parts := slices.Clone(s.tags)
	if s.untagged {
		parts = append(parts, untaggedWord)
	}
	if len(parts) == 0 {
		return "no tier"
	}
	return strings.Join(parts, ", ")
}

// collectTierStatement reads the lines that name path, by its full path or its base name as a token of its own, for the tier tags and untagged word they state.
func collectTierStatement(lines []string, path string, tierTags []string) tierStatement {
	base := path[strings.LastIndex(path, "/")+1:]
	var statement tierStatement
	for _, line := range lines {
		if !namesFile(line, path) && !namesFile(line, base) {
			continue
		}
		words := wordSpans(line)
		var occurrences []wordSpan
		for _, word := range words {
			if word.text == untaggedWord {
				statement.untagged = true
			}
			if slices.Contains(tierTags, word.text) {
				occurrences = append(occurrences, word)
				if !slices.Contains(statement.tags, word.text) {
					statement.tags = append(statement.tags, word.text)
				}
			}
		}
		if joinsAll(line, occurrences) {
			statement.joinedCounts = append(statement.joinedCounts, distinctCount(occurrences))
		}
	}
	return statement
}

// wordSpan is one word of a line and its byte offsets.
type wordSpan struct {
	text       string
	start, end int
}

// wordSpans splits line into runs of letters, digits and underscores.
func wordSpans(line string) []wordSpan {
	var spans []wordSpan
	start := -1
	for i, r := range line {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			spans = append(spans, wordSpan{line[start:i], start, i})
			start = -1
		}
	}
	if start >= 0 {
		spans = append(spans, wordSpan{line[start:], start, len(line)})
	}
	return spans
}

// joinsAll reports whether the tag occurrences of one line are two or more and each consecutive pair is separated by only ||.
func joinsAll(line string, occurrences []wordSpan) bool {
	if len(occurrences) < 2 {
		return false
	}
	for i := 1; i < len(occurrences); i++ {
		if strings.TrimSpace(line[occurrences[i-1].end:occurrences[i].start]) != "||" {
			return false
		}
	}
	return true
}

// distinctCount returns how many different words occurrences holds.
func distinctCount(occurrences []wordSpan) int {
	seen := make(map[string]bool, len(occurrences))
	for _, occurrence := range occurrences {
		seen[occurrence.text] = true
	}
	return len(seen)
}

// namesFile reports whether line holds name as a token of its own: not the tail of a longer path and not the head of a longer file name.
func namesFile(line, name string) bool {
	for offset := 0; ; {
		at := strings.Index(line[offset:], name)
		if at < 0 {
			return false
		}
		start := offset + at
		end := start + len(name)
		if tokenBoundary(line, start, end) {
			return true
		}
		offset = start + 1
	}
}

// tokenBoundary reports whether line[start:end] is not embedded in a longer path-like token.
func tokenBoundary(line string, start, end int) bool {
	if start > 0 && isPathByte(line[start-1]) {
		return false
	}
	if end < len(line) {
		next := line[end]
		if isPathByte(next) && next != '.' {
			return false
		}
		if next == '.' && end+1 < len(line) && isPathByte(line[end+1]) && line[end+1] != '.' {
			return false
		}
	}
	return true
}

// isPathByte reports whether b can be part of a file path token.
func isPathByte(b byte) bool {
	return b == '_' || b == '-' || b == '.' || b == '/' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
