// append.go implements AppendDecision, one of the two write paths into the decision record.
// The other keeps a review segment's carry-over entry in `## Open risks`.
// AppendDecision adds one design call made after the Discussion ended to the record's `## Decisions` section,
// so a later reader of the record sees it beside the decisions the Discussion itself made;
// AppendDecisions adds several such calls in one write.
// CheckAddedDecisionsKept reports the entries a record dropped from its history.

package discussionparser

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// decisionsHeading is the H2 heading AppendDecision inserts under.
const decisionsHeading = "## Decisions"

// ErrNoDecisionsHeading reports a decision record with no `## Decisions` heading to insert under.
// The caller can refuse with its own way forward.
var ErrNoDecisionsHeading = errors.New("decision record has no \"## Decisions\" heading")

// AddedDecision is one design call recorded after the Discussion: who made it, the heading title,
// the decision, why, and the day it was made.
type AddedDecision struct {
	By        string
	Title     string
	Decision  string
	Rationale string
	Date      time.Time
	// PatternChange, when set, is a PATTERN entry the decision adds, changes or removes.
	// AppendDecision declares it as a bullet of the record's `## PATTERN changes` section in the same write as the decision's entry.
	PatternChange *PatternChange
}

// AppendDecision adds d as the last entry of the decision record's `## Decisions` section and then
// runs Validate over the result.
// It writes only that section and, when d.PatternChange is set, one bullet of the `## PATTERN changes` section; the `## Open risks` section has its own write path.
// A bullet the section already holds for the change's entry ID is replaced in place, since the section holds one bullet per ID.
// Otherwise the bullet goes after the section's last bullet, and the section is created when the record has none.
// The bullet carries the suffix ` — added after Discussion (<by>, <date>)`, and a bullet of the wrong shape comes back as a finding like any other.
//
// It reads decisionRecordPath first; a missing file comes back as an error wrapping os.ErrNotExist,
// so the caller can refuse with its own way forward.
// An empty By, Title, Decision or Rationale (after trimming) and a record with no `## Decisions`
// heading are errors, and nothing is written.
//
// The entry lands after the section's last non-blank line, so the blank lines that separated the
// section from the next H2 keep separating it, and every byte of the record before and after the
// insertion is unchanged.
// The support log is only read, by Validate, never written.
//
// When Validate returns findings the prior bytes are restored and the findings come back with a nil
// error; when Validate returns an error the prior bytes are restored and the error comes back.
func AppendDecision(decisionRecordPath, supportLogPath string, d AddedDecision) ([]Finding, error) {
	return AppendDecisions(decisionRecordPath, supportLogPath, []AddedDecision{d})
}

// AppendDecisions adds every entry of ds, in order, at the end of the record's `## Decisions` section, with its PATTERN bullet when the entry sets one.
// The record is written once and Validate runs once over the result, so the batch lands whole or not at all:
// an empty field in any entry, or a record with no `## Decisions` heading, is an error and nothing is written,
// and a finding or an error from Validate restores the prior bytes.
// An empty ds is an error.
func AppendDecisions(decisionRecordPath, supportLogPath string, ds []AddedDecision) ([]Finding, error) {
	if len(ds) == 0 {
		return nil, errors.New("no decision to append")
	}
	prior, err := os.ReadFile(decisionRecordPath)
	if err != nil {
		return nil, err
	}

	updated := string(prior)
	for _, d := range ds {
		if updated, err = insertDecision(updated, d); err != nil {
			if errors.Is(err, ErrNoDecisionsHeading) {
				return nil, fmt.Errorf("%s: %w", decisionRecordPath, err)
			}
			return nil, err
		}
	}

	info, err := os.Stat(decisionRecordPath)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(decisionRecordPath, []byte(updated), info.Mode().Perm()); err != nil {
		return nil, err
	}

	findings, err := Validate(decisionRecordPath, supportLogPath)
	if err == nil && len(findings) == 0 {
		return nil, nil
	}
	if restoreErr := os.WriteFile(decisionRecordPath, prior, info.Mode().Perm()); restoreErr != nil {
		return nil, errors.Join(err, restoreErr)
	}
	return findings, err
}

// insertDecision returns content with d's entry added at the end of its `## Decisions` section and, when d.PatternChange is set, d's bullet written to its `## PATTERN changes` section.
// An empty field of d after trimming and content with no `## Decisions` heading are errors.
func insertDecision(content string, d AddedDecision) (string, error) {
	by, title := strings.TrimSpace(d.By), strings.TrimSpace(d.Title)
	decision, rationale := strings.TrimSpace(d.Decision), strings.TrimSpace(d.Rationale)
	for name, value := range map[string]string{"by": by, "title": title, "decision": decision, "rationale": rationale} {
		if value == "" {
			return "", fmt.Errorf("decision %s is empty", name)
		}
	}

	at, err := decisionsInsertOffset(content)
	if err != nil {
		return "", err
	}

	entry := fmt.Sprintf("\n### Added after Discussion (%s, %s): %s\n\nDecision: %s\n\nRationale: %s\n",
		by, d.Date.UTC().Format("2006-01-02"), title, decision, rationale)
	if at > 0 && content[at-1] != '\n' {
		entry = "\n" + entry
	}

	if d.PatternChange == nil {
		return content[:at] + entry + content[at:], nil
	}
	bullet := patternChangeAppendedBullet(*d.PatternChange, by, d.Date)
	patternAt, patternEnd, replace := patternBulletRange(content, strings.TrimSpace(d.PatternChange.ID))
	insertion := bullet
	if !replace {
		var needHeading bool
		patternAt, needHeading = patternChangesInsertOffset(content)
		patternEnd = patternAt
		insertion = patternChangesInsertion(content, patternAt, needHeading, bullet)
	}
	if patternAt >= at {
		return content[:at] + entry + content[at:patternAt] + insertion + content[patternEnd:], nil
	}
	return content[:patternAt] + insertion + content[patternEnd:at] + entry + content[at:], nil
}

// addedDecisionHeadingPrefix starts the heading of every entry AppendDecision writes.
const addedDecisionHeadingPrefix = "### Added after Discussion ("

// ParseAddedDecisions returns every `### Added after Discussion (<by>, <date>): <title>` heading of record outside a fenced block, in order.
// Each comes back with By, Date (the `2006-01-02` day in UTC) and Title set, and Decision and Rationale empty.
// A heading that starts that way but does not parse to a non-empty author and a valid date is an error naming its line number.
func ParseAddedDecisions(record []byte) ([]AddedDecision, error) {
	lines := strings.Split(string(record), "\n")
	fenced := fencedLines(lines)
	var decisions []AddedDecision
	for i, line := range lines {
		if fenced[i] {
			continue
		}
		heading := strings.TrimRight(line, " \t\r")
		rest, ok := strings.CutPrefix(heading, addedDecisionHeadingPrefix)
		if !ok {
			continue
		}
		decision, ok := parseAddedDecisionHeading(rest)
		if !ok {
			return nil, fmt.Errorf("line %d: %q is not an added-decision heading with an author and a date", i+1, heading)
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

// AddedDecisionTexts returns every `### Added after Discussion` entry of the decision record at recordPath, heading and body verbatim, in record order, for a caller to render into an agent's prompt.
// An entry runs from its heading to its last non-blank line before the next heading of any level, and a heading inside a fenced block starts none.
// An empty recordPath, a record that does not exist and a record without an entry return nil and no error; a read error is returned.
func AddedDecisionTexts(recordPath string) ([]string, error) {
	if recordPath == "" {
		return nil, nil
	}
	data, err := os.ReadFile(recordPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	_, spans := addedDecisionSpans(lines, fencedLines(lines))
	var texts []string
	for _, span := range spans {
		texts = append(texts, strings.TrimRight(strings.Join(lines[span.first:span.last+1], "\n"), " \t\r"))
	}
	return texts, nil
}

// checkAddedDecisionDropped names a Finding produced when a record lacks an added-decision entry an earlier version of it held.
const checkAddedDecisionDropped = "added-decision-dropped"

// CheckAddedDecisionsKept returns one Finding per distinct added-decision entry, by author, date and title, that some of versions holds and record does not, in the order of first appearance across versions.
// Each finding's Detail names the entry's full heading and says to restore it from the record's history, since an added entry is never the writer's to remove.
//
// It then returns one pattern-change-dropped Finding per entry ID of an appended `## PATTERN changes` bullet some of versions holds for which record holds no appended bullet, in the same order and with a Detail that names the bullet and says to restore it.
// The ID alone is the key, because AppendDecision replaces the bullet it finds for an ID with one carrying the new call's author and date.
//
// A record whose added-decision heading does not parse is one finding carrying the parse error, because the writer can fix it.
// A version whose heading does not parse contributes no entries: only `lym loom decision add` writes these headings, and a garbled historical copy must not block the run.
// A version whose PATTERN changes section does not parse contributes no bullets, and a record whose own section does not parse yields no bullet finding, since Validate reports it.
// It reads nothing from disk.
func CheckAddedDecisionsKept(record []byte, versions [][]byte) []Finding {
	kept, err := ParseAddedDecisions(record)
	if err != nil {
		return []Finding{{Check: checkAddedDecisionDropped, Detail: err.Error()}}
	}
	return append(checkDroppedAddedDecisions(kept, versions), checkDroppedPatternBullets(record, versions)...)
}

// checkPatternChangeDropped names a Finding produced when a record lacks an appended PATTERN change bullet an earlier version of it held.
const checkPatternChangeDropped = "pattern-change-dropped"

// checkDroppedPatternBullets returns the pattern-change-dropped findings CheckAddedDecisionsKept reports.
func checkDroppedPatternBullets(record []byte, versions [][]byte) []Finding {
	keptBullets, problems := ParsePatternChanges(record)
	if len(problems) > 0 {
		return nil
	}
	seen := make(map[string]bool, len(keptBullets))
	for _, c := range keptBullets {
		if c.By != "" {
			seen[c.ID] = true
		}
	}

	var findings []Finding
	for _, version := range versions {
		held, problems := ParsePatternChanges(version)
		if len(problems) > 0 {
			continue
		}
		for _, c := range held {
			if c.By == "" || seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			findings = append(findings, Finding{
				Check: checkPatternChangeDropped,
				Detail: fmt.Sprintf("the record dropped the `%s` bullet added after Discussion (%s, %s) from its `%s` section; restore it from the record's history, since an appended bullet is never the writer's to remove",
					c.ID, c.By, c.Date.UTC().Format("2006-01-02"), patternChangesHeading),
			})
		}
	}
	return findings
}

// checkDroppedAddedDecisions returns the added-decision-dropped findings for the entries of versions that kept lacks.
func checkDroppedAddedDecisions(kept []AddedDecision, versions [][]byte) []Finding {
	type entryKey struct{ by, date, title string }
	keyOf := func(d AddedDecision) entryKey {
		return entryKey{d.By, d.Date.UTC().Format("2006-01-02"), d.Title}
	}
	seen := make(map[entryKey]bool, len(kept))
	for _, d := range kept {
		seen[keyOf(d)] = true
	}

	var findings []Finding
	for _, version := range versions {
		held, err := ParseAddedDecisions(version)
		if err != nil {
			continue
		}
		for _, d := range held {
			key := keyOf(d)
			if seen[key] {
				continue
			}
			seen[key] = true
			findings = append(findings, Finding{
				Check: checkAddedDecisionDropped,
				Detail: fmt.Sprintf("the record dropped the entry `%s%s, %s): %s`; restore it from the record's history, since an added entry is never the writer's to remove",
					addedDecisionHeadingPrefix, key.by, key.date, key.title),
			})
		}
	}
	return findings
}

// parseAddedDecisionHeading parses the part of an added-decision heading after its opening parenthesis: `<by>, <date>): <title>`.
func parseAddedDecisionHeading(rest string) (AddedDecision, bool) {
	inside, title, ok := strings.Cut(rest, "): ")
	if !ok {
		return AddedDecision{}, false
	}
	by, dateText, ok := strings.Cut(inside, ", ")
	by = strings.TrimSpace(by)
	if !ok || by == "" {
		return AddedDecision{}, false
	}
	date, err := time.Parse("2006-01-02", strings.TrimSpace(dateText))
	if err != nil {
		return AddedDecision{}, false
	}
	return AddedDecision{By: by, Date: date, Title: strings.TrimSpace(title)}, true
}

// decisionsInsertOffset returns the byte offset in content where a new entry goes: just after the
// last non-blank line of the `## Decisions` section, which ends at the next H2 line or at the end
// of content.
// Headings inside a fenced code block are not headings.
func decisionsInsertOffset(content string) (int, error) {
	lines := strings.Split(content, "\n")
	starts := make([]int, len(lines))
	offset := 0
	for i, line := range lines {
		starts[i] = offset
		offset += len(line) + 1
	}

	head, end := -1, len(lines)
	fenced := fencedLines(lines)
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
		return 0, ErrNoDecisionsHeading
	}

	for end-1 > head && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if end == len(lines) {
		return len(content), nil
	}
	return starts[end], nil
}

// fencedLines reports for each line whether it is a ``` or ~~~ fence line or lies inside a fenced block.
// A heading or marker on such a line is ordinary text.
func fencedLines(lines []string) []bool {
	fenced := make([]bool, len(lines))
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			fenced[i] = true
			continue
		}
		fenced[i] = inFence
	}
	return fenced
}
