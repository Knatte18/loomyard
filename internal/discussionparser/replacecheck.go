// replacecheck.go implements the checks a whole-record replacement runs before it writes.
// A replacement file may drop added entries and appended PATTERN bullets, but it must not touch what Go wrote.

package discussionparser

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const (
	checkCarryOverChanged       = "carry-over-changed"
	checkAddedDecisionRewritten = "added-decision-rewritten"
)

// CheckCarryOversKept returns one Finding per carry-over block of the `## Open risks` section.
// It reports each block that next drops, changes or adds.
// A block is its lines from the opening marker through the closing marker, compared byte for byte.
// Findings come sorted by segment, and each Detail names the segment and says that only Go writes carry-over blocks.
// A record with no `## Open risks` heading holds no blocks.
// A malformed marker set in current or in next yields one Finding carrying the scan error, for each record that has one.
func CheckCarryOversKept(current, next []byte) []Finding {
	currentBlocks, currentErr := carryOverBlockTexts(current)
	nextBlocks, nextErr := carryOverBlockTexts(next)
	var findings []Finding
	if currentErr != nil {
		findings = append(findings, Finding{Check: checkCarryOverChanged,
			Detail: fmt.Sprintf("the current record's carry-over blocks are malformed: %v", currentErr)})
	}
	if nextErr != nil {
		findings = append(findings, Finding{Check: checkCarryOverChanged,
			Detail: fmt.Sprintf("the file's carry-over blocks are malformed: %v", nextErr)})
	}
	if len(findings) > 0 {
		return findings
	}

	segments := make([]string, 0, len(currentBlocks)+len(nextBlocks))
	for segment := range currentBlocks {
		segments = append(segments, segment)
	}
	for segment := range nextBlocks {
		if _, held := currentBlocks[segment]; !held {
			segments = append(segments, segment)
		}
	}
	slices.Sort(segments)

	for _, segment := range segments {
		currentText, inCurrent := currentBlocks[segment]
		nextText, inNext := nextBlocks[segment]
		var what string
		switch {
		case !inNext:
			what = "dropped"
		case !inCurrent:
			what = "added"
		case currentText != nextText:
			what = "changed"
		default:
			continue
		}
		findings = append(findings, Finding{
			Check:  checkCarryOverChanged,
			Detail: fmt.Sprintf("the file %s the carry-over block for segment %q; only Go writes carry-over blocks", what, segment),
		})
	}
	return findings
}

// carryOverBlockTexts returns the text of each carry-over block of record's `## Open risks` section, keyed by segment.
// A record with no such section yields no blocks; a malformed marker set is an error.
func carryOverBlockTexts(record []byte) (map[string]string, error) {
	lines := strings.Split(string(record), "\n")
	section, err := locateOpenRisks(lines)
	if errors.Is(err, ErrNoOpenRisksHeading) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pairs, err := scanCarryOverPairs(lines, section)
	if err != nil {
		return nil, err
	}
	texts := make(map[string]string, len(pairs))
	for segment, pair := range pairs {
		texts[segment] = strings.Join(lines[pair.open:pair.close+1], "\n")
	}
	return texts, nil
}

// CheckAddedEntriesSubset returns one Finding per `### Added after Discussion` entry of next.
// It reports each entry that no unmatched entry of current equals.
// Each entry of current matches at most one entry of next, so an entry the file repeats is a finding.
// Entries are compared as heading and body with trailing whitespace trimmed, and findings come in record order.
// Each Detail names the entry's heading line and says the file may drop an added entry but never insert or rewrite one.
// A fenced heading starts no entry.
func CheckAddedEntriesSubset(current, next []byte) []Finding {
	currentLines := strings.Split(string(current), "\n")
	_, currentSpans := addedDecisionSpans(currentLines, fencedLines(currentLines))
	held := make(map[string]int, len(currentSpans))
	for _, span := range currentSpans {
		held[addedDecisionSpanText(currentLines, span)]++
	}

	nextLines := strings.Split(string(next), "\n")
	_, nextSpans := addedDecisionSpans(nextLines, fencedLines(nextLines))
	var findings []Finding
	for _, span := range nextSpans {
		text := addedDecisionSpanText(nextLines, span)
		if held[text] > 0 {
			held[text]--
			continue
		}
		heading := strings.TrimRight(nextLines[span.first], " \t\r")
		findings = append(findings, Finding{
			Check:  checkAddedDecisionRewritten,
			Detail: fmt.Sprintf("the added entry %q is not an entry of the current record; the file may drop an added entry but never insert or rewrite one", heading),
		})
	}
	return findings
}
