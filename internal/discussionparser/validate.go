// validate.go implements Validate, which checks that a discussion's support log and decision record exist.
// It also checks that the decision record carries every required H2 section.
// CheckRecordSections runs that check on a record's bytes alone.

package discussionparser

import (
	"errors"
	"os"
	"strings"
)

// Finding is one defect found in a discussion.
// It records the check that tripped, the absolute path it concerns, and a human-readable detail.
type Finding struct {
	Check  string
	Path   string
	Detail string
}

// Error implements the error interface as the check name, then the detail.
// Path is left out.
func (f Finding) Error() string {
	return f.Check + ": " + f.Detail
}

// checkFileMissing names a Finding produced when a required `_lym/discussion/` file does not exist.
const checkFileMissing = "discussion-file-missing"

// checkSectionMissing names a Finding produced when the decision record lacks a required H2 heading.
const checkSectionMissing = "discussion-section-missing"

// requiredDiscussionSections are the H2 headings the decision record must carry.
// The list follows `contracts/specs/loom-discussion-spec.md`.
var requiredDiscussionSections = []string{
	"## Goal",
	"## Scope",
	"## Decisions",
	"## Constraints",
	"## Auto-mode assumptions",
	"## Open risks",
	"## Acceptance criteria",
}

// Validate checks a discussion's two files and reports each defect as a Finding.
// The support log is checked for existence first, then the decision record.
// A missing file yields one Finding and a nil error, and ends the check.
// Any other stat or read failure is returned as the error, with no findings.
//
// Each missing required H2 heading is one Finding.
// The optional `## PATTERN changes` section, when present, yields one Finding per malformed bullet.
// It yields another per `adds` or `changes` bullet whose rule text is longer than a PATTERN.md entry's rule text may be.
// A record without that section passes it.
//
// Validate never returns a non-empty findings slice together with a non-nil error.
// A clean run returns a nil or empty slice and a nil error.
//
// Section order is not checked, and neither is an extra H2.
// The optional `## Notes for the plan writer` section is never required.
func Validate(decisionRecordPath, supportLogPath string) ([]Finding, error) {
	if _, err := os.Stat(supportLogPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return []Finding{{Check: checkFileMissing, Path: supportLogPath, Detail: "support log does not exist"}}, nil
	}

	data, err := os.ReadFile(decisionRecordPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return []Finding{{Check: checkFileMissing, Path: decisionRecordPath, Detail: "decision record does not exist"}}, nil
	}

	var findings []Finding
	for _, f := range CheckRecordSections(data) {
		f.Path = decisionRecordPath
		findings = append(findings, f)
	}
	return findings, nil
}

// CheckRecordSections checks the sections of a decision record held as bytes, with no support log beside it.
// A caller that keeps a record apart from its discussion directory, such as the board's stored record, uses it.
// Each missing required H2 heading is one Finding.
// The optional `## PATTERN changes` section, when present, yields one Finding per malformed bullet.
// It yields another per `adds` or `changes` bullet whose rule text is longer than a PATTERN.md entry's rule text may be.
// A record without that section passes it.
// Section order is not checked, and neither is an extra H2.
// The findings carry an empty Path, which the caller stamps.
func CheckRecordSections(record []byte) []Finding {
	var findings []Finding
	for _, missing := range missingSections(string(record), requiredDiscussionSections) {
		findings = append(findings, Finding{
			Check:  checkSectionMissing,
			Detail: "decision record is missing required section " + missing,
		})
	}
	changes, patternFindings := ParsePatternChanges(record)
	findings = append(findings, patternFindings...)
	return append(findings, patternOverCapFindings(changes)...)
}

// missingSections returns every heading in required that no line of content matches outside a fenced block.
// A line matches after its trailing spaces, tabs and carriage returns are trimmed.
// A heading nested inside a fenced block, or part of a longer line, never counts.
// The result keeps the order of required.
//
// It splits content by newline rather than reading it with a bufio.Scanner.
// A Scanner stops at the first line longer than bufio.MaxScanTokenSize.
// It reports that only through Err, so every heading below such a line would count as missing.
func missingSections(content string, required []string) []string {
	found := make(map[string]bool, len(required))

	lines := strings.Split(content, "\n")
	fenced := fencedLines(lines)
	for i, raw := range lines {
		if !fenced[i] {
			found[strings.TrimRight(raw, " \t\r")] = true
		}
	}

	var missing []string
	for _, h := range required {
		if !found[h] {
			missing = append(missing, h)
		}
	}
	return missing
}
