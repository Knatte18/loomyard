// carryover.go implements WriteCarryOver, the decision record's second write path.
// It keeps one review segment's carry-over entry in the record's `## Open risks` section,
// so the findings a closing review round left open reach the sessions that read the record next.

package discussionparser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// openRisksHeading is the H2 heading WriteCarryOver writes under.
const openRisksHeading = "## Open risks"

const (
	carryOverOpenPrefix  = "<!-- lyx:carry-over "
	carryOverClosePrefix = "<!-- /lyx:carry-over "
	carryOverMarkerEnd   = " -->"
)

// ErrNoOpenRisksHeading reports a decision record with no `## Open risks` heading to write under.
// The caller can refuse with its own way forward.
var ErrNoOpenRisksHeading = errors.New("decision record has no \"## Open risks\" heading")

// CarryOverClosing says how a review segment's closing round ended.
type CarryOverClosing string

const (
	// CarryOverConverged is a round closed by a CONVERGED verdict.
	// Each listed finding was addressed by the round's fixer, and no fresh reviewer has seen the fix.
	CarryOverConverged CarryOverClosing = "converged"

	// CarryOverAccepted is a round closed by a recorded circling accept.
	// A listed finding may also be unfixed.
	CarryOverAccepted CarryOverClosing = "accepted"
)

// CarryOverFinding is one finding that was open when its segment closed.
// It holds the key, the class and severity labels (the caller passes "unlabelled" for a missing label),
// and the ledger rounds the key was open in.
type CarryOverFinding struct {
	Key      string
	Class    string
	Severity string
	Rounds   []int
}

// CarryOver is one review segment's carry-over entry.
// It holds the segment, the round that closed it and how it closed.
// It also holds the anchor-relative slash-separated paths of that round's review and fixer report, and the findings still open.
// A CarryOver with no Findings asks WriteCarryOver to remove the segment's entry.
type CarryOver struct {
	Segment         string
	Round           int
	Closing         CarryOverClosing
	ReviewPath      string
	FixerReportPath string
	Findings        []CarryOverFinding
}

// WriteCarryOver writes entry into the `## Open risks` section of the decision record at decisionRecordPath.
// It replaces the segment's existing entry or appends one after the section's last non-blank line.
// When entry has no findings, it removes the segment's entry.
// An entry is delimited by marker lines naming the segment.
// The rendering is deterministic, so a replay writes the same bytes.
//
// Every line outside the written entry stays byte-identical.
// A record with no entry for the segment and an entry without findings is not rewritten.
// A malformed marker set in the section is an error, and nothing is written.
// So is a record with no `## Open risks` heading (ErrNoOpenRisksHeading) and a Segment that is empty or holds anything but letters, digits and hyphens.
// The new bytes are checked against the prior ones before they replace the record through a sibling temporary file.
// A failed write or check leaves the record unchanged.
//
// Every error is prefixed with decisionRecordPath; a missing record wraps os.ErrNotExist.
func WriteCarryOver(decisionRecordPath string, entry CarryOver) error {
	if err := writeCarryOver(decisionRecordPath, entry); err != nil {
		return fmt.Errorf("%s: %w", decisionRecordPath, err)
	}
	return nil
}

func writeCarryOver(path string, entry CarryOver) error {
	if err := validateCarryOverSegment(entry.Segment); err != nil {
		return err
	}
	if len(entry.Findings) > 0 && entry.Closing != CarryOverConverged && entry.Closing != CarryOverAccepted {
		return fmt.Errorf("unknown carry-over closing %q", entry.Closing)
	}

	prior, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(prior), "\n")
	section, err := locateOpenRisks(lines)
	if err != nil {
		return err
	}
	pairs, err := scanCarryOverPairs(lines, section)
	if err != nil {
		return err
	}
	existing, found := pairs[entry.Segment]

	var next []string
	var priorStart, priorEnd, nextEnd int
	switch {
	case len(entry.Findings) == 0 && !found:
		return nil
	case len(entry.Findings) == 0:
		priorStart, priorEnd = existing.open, existing.close+1
		if priorStart > section.head+1 && strings.TrimSpace(lines[priorStart-1]) == "" {
			priorStart--
		}
		next = slices.Concat(lines[:priorStart], lines[priorEnd:])
		nextEnd = priorStart
	case found:
		rendered := renderCarryOver(entry)
		priorStart, priorEnd = existing.open, existing.close+1
		next = slices.Concat(lines[:priorStart], rendered, lines[priorEnd:])
		nextEnd = priorStart + len(rendered)
	default:
		rendered := renderCarryOver(entry)
		priorStart = lastNonBlankEnd(lines, section)
		priorEnd = priorStart
		next = slices.Concat(lines[:priorStart], []string{""}, rendered, lines[priorStart:])
		nextEnd = priorStart + 1 + len(rendered)
	}

	if err := checkCarryOverSplice(lines, next, priorStart, priorEnd, nextEnd, entry.Segment, len(entry.Findings) > 0); err != nil {
		return fmt.Errorf("refusing to write: %w", err)
	}
	return replaceFile(path, []byte(strings.Join(next, "\n")), info.Mode().Perm())
}

// validateCarryOverSegment refuses a segment that cannot be spelled inside a marker line.
func validateCarryOverSegment(segment string) error {
	if segment == "" {
		return errors.New("carry-over segment is empty")
	}
	for _, r := range segment {
		if !isSegmentRune(r) {
			return fmt.Errorf("carry-over segment %q holds a character other than a letter, digit or hyphen", segment)
		}
	}
	return nil
}

func isSegmentRune(r rune) bool {
	return r == '-' || (r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)))
}

// renderCarryOver renders entry as its marker-delimited lines.
func renderCarryOver(entry CarryOver) []string {
	var closing string
	switch entry.Closing {
	case CarryOverConverged:
		closing = "converged; each finding below was addressed by the round's fixer, and no fresh reviewer has seen the fix"
	case CarryOverAccepted:
		closing = "closed by an accepted circling decision; a finding below was addressed by the round's fixer or is still unfixed"
	}

	lines := []string{
		carryOverOpenPrefix + entry.Segment + carryOverMarkerEnd,
		fmt.Sprintf("- %s, round %d: %s.", entry.Segment, entry.Round, closing),
		fmt.Sprintf("  Review: %s; fixer report: %s.", sanitizeCarryOverField(entry.ReviewPath), sanitizeCarryOverField(entry.FixerReportPath)),
	}
	for _, finding := range entry.Findings {
		rounds := make([]string, len(finding.Rounds))
		for i, round := range finding.Rounds {
			rounds[i] = strconv.Itoa(round)
		}
		lines = append(lines, fmt.Sprintf("  - %s (class %s, severity %s, open in rounds %s)",
			sanitizeCarryOverField(finding.Key), sanitizeCarryOverField(finding.Class),
			sanitizeCarryOverField(finding.Severity), strings.Join(rounds, ", ")))
	}
	return append(lines, carryOverClosePrefix+entry.Segment+carryOverMarkerEnd)
}

// sanitizeCarryOverField makes a field render on one line without opening a heading, a fence or a marker line.
// Line breaks and other control characters become spaces, and backticks are escaped.
func sanitizeCarryOverField(field string) string {
	var b strings.Builder
	for _, r := range field {
		switch {
		case r == '`':
			b.WriteString("\\`")
		case unicode.IsControl(r), r == ' ', r == ' ':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// openRisksSection is the line range of the `## Open risks` section.
// head is the heading line, end the next H2 line or len(lines), and inFence marks the lines inside a fenced block.
type openRisksSection struct {
	head, end int
	inFence   []bool
}

// locateOpenRisks finds the `## Open risks` section, reading headings and fences with the same rule as the `## Decisions` insertion.
func locateOpenRisks(lines []string) (openRisksSection, error) {
	section := openRisksSection{head: -1, end: len(lines), inFence: make([]bool, len(lines))}
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			section.inFence[i] = true
			continue
		}
		section.inFence[i] = inFence
		if inFence {
			continue
		}
		if section.head < 0 {
			if trimmed == openRisksHeading {
				section.head = i
			}
			continue
		}
		if section.end == len(lines) && strings.HasPrefix(trimmed, "## ") {
			section.end = i
		}
	}
	if section.head < 0 {
		return section, ErrNoOpenRisksHeading
	}
	return section, nil
}

// lastNonBlankEnd returns the index just after the section's last non-blank line.
func lastNonBlankEnd(lines []string, section openRisksSection) int {
	end := section.end
	for end-1 > section.head && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end
}

// carryOverPair is the line span of one well-formed entry: its opening and closing marker lines.
type carryOverPair struct{ open, close int }

// carryOverMarker reports whether line is a marker line, and which kind and segment.
func carryOverMarker(line string) (segment string, isOpen, ok bool) {
	trimmed := strings.TrimRight(line, " \t\r")
	prefix := carryOverOpenPrefix
	isOpen = true
	if strings.HasPrefix(trimmed, carryOverClosePrefix) {
		prefix = carryOverClosePrefix
		isOpen = false
	}
	if !strings.HasPrefix(trimmed, prefix) || !strings.HasSuffix(trimmed, carryOverMarkerEnd) {
		return "", false, false
	}
	segment = strings.TrimSuffix(strings.TrimPrefix(trimmed, prefix), carryOverMarkerEnd)
	if validateCarryOverSegment(segment) != nil {
		return "", false, false
	}
	return segment, isOpen, true
}

// scanCarryOverPairs returns the section's entries by segment, or an error naming the first malformed marker.
// Marker text outside the section or inside a fence is ordinary prose.
func scanCarryOverPairs(lines []string, section openRisksSection) (map[string]carryOverPair, error) {
	pairs := map[string]carryOverPair{}
	openSegment, openLine := "", -1
	for i := section.head + 1; i < section.end; i++ {
		if section.inFence[i] {
			continue
		}
		segment, isOpen, ok := carryOverMarker(lines[i])
		if !ok {
			continue
		}
		switch {
		case isOpen && openLine >= 0:
			return nil, fmt.Errorf("carry-over entry %q opens at line %d inside the entry %q opened at line %d", segment, i+1, openSegment, openLine+1)
		case isOpen:
			if _, dup := pairs[segment]; dup {
				return nil, fmt.Errorf("carry-over entry %q appears twice; second opening at line %d", segment, i+1)
			}
			openSegment, openLine = segment, i
		case openLine < 0:
			return nil, fmt.Errorf("carry-over closing line %d for %q has no opening line", i+1, segment)
		case segment != openSegment:
			return nil, fmt.Errorf("carry-over closing line %d names %q but the open entry is %q", i+1, segment, openSegment)
		default:
			pairs[segment] = carryOverPair{open: openLine, close: i}
			openSegment, openLine = "", -1
		}
	}
	if openLine >= 0 {
		return nil, fmt.Errorf("carry-over entry %q opened at line %d has no closing line", openSegment, openLine+1)
	}
	return pairs, nil
}

// checkCarryOverSplice verifies the spliced lines before they are written.
// They must keep the same H2 heading sequence and a well-formed marker set with exactly one entry for the written segment (none after a removal),
// and every line outside the replaced span must equal the prior file's.
// The replaced span is prior[priorStart:priorEnd] and next[priorStart:nextEnd].
func checkCarryOverSplice(prior, next []string, priorStart, priorEnd, nextEnd int, segment string, wantEntry bool) error {
	nextSection, err := locateOpenRisks(next)
	if err != nil {
		return err
	}
	if !slices.Equal(h2Headings(prior), h2Headings(next)) {
		return errors.New("the splice changed the record's H2 headings")
	}
	pairs, err := scanCarryOverPairs(next, nextSection)
	if err != nil {
		return err
	}
	if _, present := pairs[segment]; present != wantEntry {
		return fmt.Errorf("the splice left the entry for %q present=%t, want %t", segment, present, wantEntry)
	}
	if !slices.Equal(prior[:priorStart], next[:priorStart]) || !slices.Equal(prior[priorEnd:], next[nextEnd:]) {
		return errors.New("the splice changed a line outside the replaced span")
	}
	return nil
}

// h2Headings returns the H2 lines outside fenced blocks, in order.
func h2Headings(lines []string) []string {
	var headings []string
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(trimmed, "## ") {
			headings = append(headings, trimmed)
		}
	}
	return headings
}

// replaceFile writes data to a sibling temporary file and renames it over path.
func replaceFile(path string, data []byte, perm os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	_, writeErr := temp.Write(data)
	closeErr := temp.Close()
	if err := errors.Join(writeErr, closeErr, os.Chmod(tempPath, perm)); err != nil {
		os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return err
	}
	return nil
}
