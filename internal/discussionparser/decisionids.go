// decisionids.go implements CheckDecisionIDs, the decision record's decision-id and acceptance-coverage check.

package discussionparser

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// acceptanceHeading is the H2 heading of the section whose bullets must name every decision id.
const acceptanceHeading = "## Acceptance criteria"

// decisionHeadingPrefix starts a decision heading: `### D<n> — <title>`.
const decisionHeadingPrefix = "### D"

// decisionHeadingSeparator separates a decision heading's id from its title.
const decisionHeadingSeparator = " — "

const (
	checkDecisionHeadingMalformed = "decision-heading-malformed"
	checkDecisionIDDuplicate      = "decision-id-duplicate"
	checkDecisionNotInAcceptance  = "decision-not-in-acceptance"
)

// CheckDecisionIDs checks the record's `## Decisions` section for decision ids and their acceptance coverage.
// Each H3 of the section must read `### D<n> — <title>`, with n a positive integer and the title non-empty, except an added-decision entry heading, which carries no id.
// Ids must be unique, and when the record has an `## Acceptance criteria` section, each id must appear as a whole word in one of its bullets.
// A range such as "D1 to D4" covers only the ids it spells.
// Lines inside fenced blocks are not read.
// A record without a `## Decisions` section yields no finding, and one without an acceptance section gets no coverage finding.
// The findings carry an empty Path, which the caller stamps.
func CheckDecisionIDs(record []byte) []Finding {
	lines := strings.Split(string(record), "\n")
	fenced := fencedLines(lines)

	first, end := sectionBounds(lines, fenced, decisionsHeading)
	if first < 0 {
		return nil
	}

	var findings []Finding
	var ids []string
	firstLine := map[string]int{}
	reportedDuplicate := map[string]bool{}
	for i := first; i < end; i++ {
		if fenced[i] {
			continue
		}
		heading := strings.TrimRight(lines[i], " \t\r")
		if !strings.HasPrefix(heading, "### ") || isAddedDecisionHeading(heading) {
			continue
		}
		id, ok := parseDecisionHeading(heading)
		if !ok {
			findings = append(findings, Finding{
				Check:  checkDecisionHeadingMalformed,
				Detail: fmt.Sprintf("line %d: decision heading %q is not `### D<n> — <title>`; way forward: rewrite the heading as `### D<n> — <title>` when it is a decision, or demote it to H4 or plain text when it is not", i+1, heading),
			})
			continue
		}
		if firstSeen, seen := firstLine[id]; seen {
			if !reportedDuplicate[id] {
				reportedDuplicate[id] = true
				findings = append(findings, Finding{
					Check:  checkDecisionIDDuplicate,
					Detail: fmt.Sprintf("line %d: decision id %s is already used by the heading on line %d; way forward: renumber one of the decisions and every mention of its id", i+1, id, firstSeen),
				})
			}
			continue
		}
		firstLine[id] = i + 1
		ids = append(ids, id)
	}

	acceptanceFirst, acceptanceEnd := sectionBounds(lines, fenced, acceptanceHeading)
	if acceptanceFirst < 0 {
		return findings
	}
	bullets := bulletLines(lines, fenced, acceptanceFirst, acceptanceEnd)
	for _, id := range ids {
		if !anyContainsWord(bullets, id) {
			findings = append(findings, Finding{
				Check:  checkDecisionNotInAcceptance,
				Detail: fmt.Sprintf("decision %s is named in no acceptance criteria bullet; way forward: name it in the criterion that covers it, or add one", id),
			})
		}
	}
	return findings
}

// sectionBounds returns the index of the first line after the H2 heading and the index of the next H2 line, or len(lines) when none follows.
// It returns -1, -1 when the record has no such unfenced heading.
func sectionBounds(lines []string, fenced []bool, heading string) (first, end int) {
	head := -1
	for i, line := range lines {
		if fenced[i] {
			continue
		}
		trimmed := strings.TrimRight(line, " \t\r")
		if head < 0 {
			if trimmed == heading {
				head = i
			}
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			return head + 1, i
		}
	}
	if head < 0 {
		return -1, -1
	}
	return head + 1, len(lines)
}

// parseDecisionHeading returns the id `D<n>` of a `### D<n> — <title>` heading, and false when heading has another shape.
func parseDecisionHeading(heading string) (string, bool) {
	rest := strings.TrimPrefix(heading, decisionHeadingPrefix)
	digits := rest[:len(rest)-len(strings.TrimLeft(rest, "0123456789"))]
	if digits == "" || digits[0] == '0' {
		return "", false
	}
	if _, err := strconv.Atoi(digits); err != nil {
		return "", false
	}
	title, ok := strings.CutPrefix(rest[len(digits):], decisionHeadingSeparator)
	if !ok || strings.TrimSpace(title) == "" {
		return "", false
	}
	return "D" + digits, true
}

// bulletLines returns the unfenced lines in [first, end) that belong to a list item: the bullet line itself and the indented lines continuing it.
func bulletLines(lines []string, fenced []bool, first, end int) []string {
	var bullets []string
	inItem := false
	for i := first; i < end; i++ {
		if fenced[i] {
			inItem = false
			continue
		}
		line := strings.TrimRight(lines[i], " \t\r")
		trimmed := strings.TrimLeft(line, " \t")
		switch {
		case isBulletLine(trimmed):
			inItem = true
		case line == "" || (inItem && trimmed == line):
			inItem = false
		}
		if inItem {
			bullets = append(bullets, line)
		}
	}
	return bullets
}

// isBulletLine reports whether line, left-trimmed, starts a markdown list item.
func isBulletLine(line string) bool {
	if len(line) >= 2 && strings.ContainsRune("-*+", rune(line[0])) && line[1] == ' ' {
		return true
	}
	digits := len(line) - len(strings.TrimLeft(line, "0123456789"))
	return digits > 0 && strings.HasPrefix(line[digits:], ". ")
}

// anyContainsWord reports whether any of lines holds word bounded on both sides by a non-alphanumeric rune or the line's end.
func anyContainsWord(lines []string, word string) bool {
	for _, line := range lines {
		for from := 0; ; {
			at := strings.Index(line[from:], word)
			if at < 0 {
				break
			}
			at += from
			stop := at + len(word)
			before, _ := utf8.DecodeLastRuneInString(line[:at])
			after, _ := utf8.DecodeRuneInString(line[stop:])
			if (at == 0 || !isAlphanumeric(before)) && (stop == len(line) || !isAlphanumeric(after)) {
				return true
			}
			from = at + 1
		}
	}
	return false
}

func isAlphanumeric(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
