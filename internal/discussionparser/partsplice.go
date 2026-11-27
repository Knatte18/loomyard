// partsplice.go implements two functions over the decision record.
// SectionBullets reads the top-level bullets of a record section.
// SplicePart cuts one part's decision record out of the record.

package discussionparser

import (
	"fmt"
	"slices"
	"strings"
)

const (
	goalHeading                = "## Goal"
	scopeHeading               = "## Scope"
	constraintsHeading         = "## Constraints"
	autoModeAssumptionsHeading = "## Auto-mode assumptions"
)

// PartSplice holds the inputs for cutting one part's decision record out of the full record.
// Goal and Scope are the text the caller renders from the part's contract and packages.
// Decisions are the ids of the decisions the part builds.
// OpenRisks and Criteria are the texts of the Open risks and Acceptance criteria bullets the part carries.
// PatternChanges are the ids of the PATTERN entries the part carries, such as `PATTERN-sole-parsers`.
type PartSplice struct {
	Goal           string
	Scope          string
	Decisions      []string
	OpenRisks      []string
	Criteria       []string
	PatternChanges []string
}

// SectionBullets returns the top-level bullets of the record's `## <heading>` section, in order.
// The heading is given without its `## `.
// A bullet is its text without the marker, followed by its continuation lines, each trimmed and joined by a newline;
// a continuation line is an indented line that follows it with no blank line between.
// A bullet inside a fenced block is not read.
// Neither is a bullet inside a `<!-- lym:carry-over … -->` block of the `## Open risks` section.
// A record without the section returns none.
func SectionBullets(record []byte, heading string) []string {
	lines := strings.Split(string(record), "\n")
	fenced := fencedLines(lines)
	first, end := sectionBounds(lines, fenced, "## "+heading)
	if first < 0 {
		return nil
	}
	skipped := make([]bool, len(lines))
	dropCarryOverBlocks(lines, skipped)

	var bullets []string
	var current []string
	flush := func() {
		if current != nil {
			bullets = append(bullets, strings.Join(current, "\n"))
			current = nil
		}
	}
	for i := first; i < end; i++ {
		line := strings.TrimRight(lines[i], " \t\r")
		switch {
		case fenced[i] || skipped[i]:
			flush()
		case isBulletLine(line):
			flush()
			current = []string{bulletBody(line)}
		case current != nil && line != "" && (line[0] == ' ' || line[0] == '\t'):
			current = append(current, strings.TrimSpace(line))
		default:
			flush()
		}
	}
	flush()
	return bullets
}

// bulletBody returns the text of a bullet line after its `-`, `*`, `+` or `<n>.` marker.
func bulletBody(line string) string {
	if strings.ContainsRune("-*+", rune(line[0])) {
		return strings.TrimSpace(line[1:])
	}
	_, body, _ := strings.Cut(line, ". ")
	return strings.TrimSpace(body)
}

// SplicePart returns the decision record of one part, cut from record.
// It holds `## Goal` and `## Scope` from part.
// `## Decisions` holds each listed decision copied whole, heading and id included, in the record's order.
// Every `### Added after Discussion` entry of the record follows them, copied whole.
// `## Constraints` and `## Auto-mode assumptions` are copied whole.
// `## Open risks` and `## Acceptance criteria` hold only the listed bullets.
// `## PATTERN changes` is present only when the part carries one,
// and it holds the listed entries' bullets as the record holds them.
// A listed decision id, bullet or PATTERN entry that the record does not hold is an error naming it.
// A section it must copy that the record lacks is an error naming it too.
func SplicePart(record []byte, part PartSplice) ([]byte, error) {
	lines := strings.Split(string(record), "\n")
	fenced := fencedLines(lines)

	decisions, err := listedDecisions(lines, fenced, part.Decisions)
	if err != nil {
		return nil, err
	}
	_, spans := addedDecisionSpans(lines, fenced)
	for _, span := range spans {
		decisions = append(decisions, strings.Join(lines[span.first:span.last+1], "\n"))
	}
	constraints, err := copiedSection(lines, fenced, constraintsHeading)
	if err != nil {
		return nil, err
	}
	autoMode, err := copiedSection(lines, fenced, autoModeAssumptionsHeading)
	if err != nil {
		return nil, err
	}
	if err := requireBullets(record, "Open risks", part.OpenRisks); err != nil {
		return nil, err
	}
	if err := requireBullets(record, "Acceptance criteria", part.Criteria); err != nil {
		return nil, err
	}
	patternBullets, err := listedPatternBullets(string(record), part.PatternChanges)
	if err != nil {
		return nil, err
	}

	var spliced strings.Builder
	writeSection(&spliced, goalHeading, strings.TrimSpace(part.Goal))
	writeSection(&spliced, scopeHeading, strings.TrimSpace(part.Scope))
	writeSection(&spliced, decisionsHeading, strings.Join(decisions, "\n\n"))
	writeSection(&spliced, constraintsHeading, constraints)
	writeSection(&spliced, autoModeAssumptionsHeading, autoMode)
	writeSection(&spliced, openRisksHeading, renderBullets(part.OpenRisks))
	writeSection(&spliced, acceptanceHeading, renderBullets(part.Criteria))
	if len(patternBullets) > 0 {
		writeSection(&spliced, patternChangesHeading, strings.Join(patternBullets, "\n"))
	}
	return []byte(strings.TrimRight(spliced.String(), "\n") + "\n"), nil
}

// writeSection writes heading and, when it is not empty, body, each followed by a blank line.
func writeSection(b *strings.Builder, heading, body string) {
	b.WriteString(heading + "\n\n")
	if body != "" {
		b.WriteString(body + "\n\n")
	}
}

// listedDecisions returns each decision of the `## Decisions` section whose id is in ids, whole and in record order.
// A decision runs from its heading to the next H2 or H3 outside a fenced block, trailing blank lines left out;
// an id the section holds twice yields its first decision.
func listedDecisions(lines []string, fenced []bool, ids []string) ([]string, error) {
	first, end := sectionBounds(lines, fenced, decisionsHeading)
	if first < 0 {
		return nil, ErrNoDecisionsHeading
	}
	held := map[string]bool{}
	var blocks []string
	walkDecisionHeadings(lines, fenced, first, end, func(i int, _, id string, ok bool) {
		if !ok || held[id] || !slices.Contains(ids, id) {
			return
		}
		held[id] = true
		next := i + 1
		for next < end && (fenced[next] || !isSectionOrDecisionHeading(lines[next])) {
			next++
		}
		blocks = append(blocks, strings.Join(trimBlankLines(lines[i:next]), "\n"))
	})
	for _, id := range ids {
		if !held[id] {
			return nil, fmt.Errorf("the decision record holds no decision %s", id)
		}
	}
	return blocks, nil
}

// isSectionOrDecisionHeading reports whether line is an H2 or an H3 heading.
func isSectionOrDecisionHeading(line string) bool {
	return strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ")
}

// copiedSection returns the body of the record's section under heading, without leading or trailing blank lines.
func copiedSection(lines []string, fenced []bool, heading string) (string, error) {
	first, end := sectionBounds(lines, fenced, heading)
	if first < 0 {
		return "", fmt.Errorf("the decision record holds no %q section", heading)
	}
	return strings.Join(trimBlankLines(lines[first:end]), "\n"), nil
}

// trimBlankLines returns lines without its leading and trailing blank lines.
func trimBlankLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// requireBullets returns an error naming the first text in listed that is no bullet of the record's section heading.
func requireBullets(record []byte, heading string, listed []string) error {
	held := SectionBullets(record, heading)
	for _, text := range listed {
		if !slices.Contains(held, text) {
			return fmt.Errorf("the decision record's %q section holds no bullet %q", heading, text)
		}
	}
	return nil
}

// listedPatternBullets returns the `## PATTERN changes` bullet line of each entry id, verbatim and in ids' order.
func listedPatternBullets(content string, ids []string) ([]string, error) {
	var bullets []string
	for _, id := range ids {
		start, end, found := patternBulletRange(content, id)
		if !found {
			return nil, fmt.Errorf("the decision record's %q section holds no bullet for %s", patternChangesHeading, id)
		}
		bullets = append(bullets, content[start:end])
	}
	return bullets, nil
}

// renderBullets renders each text as a bullet, with its continuation lines indented.
// Each bullet follows the previous one with no blank line between.
func renderBullets(texts []string) string {
	var rendered []string
	for _, text := range texts {
		first, rest, _ := strings.Cut(text, "\n")
		bullet := "- " + first
		if rest != "" {
			bullet += "\n  " + strings.ReplaceAll(rest, "\n", "\n  ")
		}
		rendered = append(rendered, bullet)
	}
	return strings.Join(rendered, "\n")
}
