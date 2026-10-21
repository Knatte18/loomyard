// identity.go implements IdentityText, the decision record's identity under re-entry.
// It strips what the record's two write paths add after the Discussion, so only the reviewed text decides whether the record changed.

package discussionparser

import (
	"strings"
)

// IdentityText returns record without what a settle and `lyx loom decision add` write into it:
// every `<!-- lyx:carry-over … -->` block of the `## Open risks` section, marker lines included,
// and every `### Added after Discussion (…)` entry of the `## Decisions` section.
// Each removal takes the blank line that separated the block from the line above it, as WriteCarryOver's removal and AppendDecision's insertion do,
// so a record with such blocks and the same record without them give equal text.
// Every other byte stays, so an edit anywhere else changes the text.
// A section that is missing, or a carry-over marker set that is malformed, removes nothing from that section.
// It is a pure function of record.
func IdentityText(record []byte) []byte {
	lines := strings.Split(string(record), "\n")
	fenced := fencedLines(lines)
	drop := make([]bool, len(lines))
	dropCarryOverBlocks(lines, drop)
	dropAddedDecisionEntries(lines, fenced, drop)

	kept := make([]string, 0, len(lines))
	for i, line := range lines {
		if !drop[i] {
			kept = append(kept, line)
		}
	}
	return []byte(strings.Join(kept, "\n"))
}

// dropCarryOverBlocks marks every well-formed carry-over block of the `## Open risks` section, and the blank line above it, in drop.
func dropCarryOverBlocks(lines []string, drop []bool) {
	section, err := locateOpenRisks(lines)
	if err != nil {
		return
	}
	pairs, err := scanCarryOverPairs(lines, section)
	if err != nil {
		return
	}
	for _, pair := range pairs {
		for i := pair.open; i <= pair.close; i++ {
			drop[i] = true
		}
		dropBlankAbove(lines, drop, pair.open, section.head)
	}
}

// dropAddedDecisionEntries marks every added-decision entry of the `## Decisions` section, and the blank line above it, in drop.
// An entry runs from its heading to its last non-blank line before the next heading of any level.
func dropAddedDecisionEntries(lines []string, fenced, drop []bool) {
	head, end := -1, len(lines)
	for i, line := range lines {
		if fenced[i] {
			continue
		}
		trimmed := strings.TrimRight(line, " \t\r")
		if head < 0 {
			if trimmed == decisionsHeading {
				head = i
			}
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			end = i
			break
		}
	}
	if head < 0 {
		return
	}

	for i := head + 1; i < end; i++ {
		if fenced[i] || !isAddedDecisionHeading(lines[i]) {
			continue
		}
		next := i + 1
		for next < end && (fenced[next] || !isHeadingLine(lines[next])) {
			next++
		}
		last := next - 1
		for last > i && strings.TrimSpace(lines[last]) == "" {
			last--
		}
		for j := i; j <= last; j++ {
			drop[j] = true
		}
		dropBlankAbove(lines, drop, i, head)
		i = last
	}
}

// dropBlankAbove marks the line above first in drop when it is blank and below the section's heading line.
func dropBlankAbove(lines []string, drop []bool, first, head int) {
	if first-1 > head && strings.TrimSpace(lines[first-1]) == "" {
		drop[first-1] = true
	}
}

// isAddedDecisionHeading reports whether line is a well-formed `### Added after Discussion (<by>, <date>): <title>` heading.
func isAddedDecisionHeading(line string) bool {
	rest, ok := strings.CutPrefix(strings.TrimRight(line, " \t\r"), addedDecisionHeadingPrefix)
	if !ok {
		return false
	}
	_, ok = parseAddedDecisionHeading(rest)
	return ok
}

// isHeadingLine reports whether line is a markdown ATX heading: one to six `#` and a space.
func isHeadingLine(line string) bool {
	hashes := len(line) - len(strings.TrimLeft(line, "#"))
	return hashes >= 1 && hashes <= 6 && strings.HasPrefix(line[hashes:], " ")
}
