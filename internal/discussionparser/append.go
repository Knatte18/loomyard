// append.go writes the design calls made after a Discussion into the decision record.
// It writes their `## Decisions` entries and `## PATTERN changes` bullets,
// and it parses those entries and checks that a record keeps them.

package discussionparser

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// decisionsHeading is the H2 heading that new decision entries are inserted under.
const decisionsHeading = "## Decisions"

// ErrNoDecisionsHeading reports a decision record with no `## Decisions` heading to insert under.
// The caller can refuse with its own way forward.
var ErrNoDecisionsHeading = errors.New("decision record has no \"## Decisions\" heading")

// AddedDecision is one design call recorded after a Discussion.
// It holds who made the call, the heading title, the decision, its rationale, and the day it was made.
type AddedDecision struct {
	By        string
	Title     string
	Decision  string
	Rationale string
	Date      time.Time
	// PatternChange, when set, is a PATTERN entry the decision adds, changes or removes.
	// It is written as a bullet of the `## PATTERN changes` section in the same write as the decision's entry.
	PatternChange *PatternChange
	// WithdrawnPattern, when set, is the entry ID of a declared PATTERN change the decision takes back.
	// Its bullet is struck with the decision's entry, which names the ID; it is never set with PatternChange.
	WithdrawnPattern string
}

// AppendDecision writes the `## Decisions` section.
// When d.PatternChange or d.WithdrawnPattern is set, it also writes the `## PATTERN changes` section.
// The section holds one bullet per entry ID, so a bullet it already holds for the change's ID is replaced in place.
// Otherwise the bullet goes after the section's last bullet, and the section is created when the record has none.
// The bullet carries the suffix ` — added after Discussion (<by>, <date>)`.
// A bullet of the wrong shape comes back as a finding like any other.
// With d.WithdrawnPattern set, it strikes that entry's bullet and keeps the section's heading.
// It closes the entry with a `Withdraws:` line naming the ID, so the strike does not read as a dropped bullet.
// A record with no bullet for that ID is an error wrapping ErrPatternBulletAbsent, and nothing is written.
//
// It reads the record at decisionRecordPath first; a missing file comes back as an error wrapping os.ErrNotExist,
// so the caller can refuse with its own way forward.
// An empty By, Title, Decision or Rationale, after trimming, and a record with no `## Decisions` heading are errors.
// Neither error writes anything.
//
// AppendDecision adds d as the last entry of the decision record's `## Decisions` section.
// It then runs Validate over the result.
//
// The entry lands after the section's last non-blank line.
// The blank lines that separate the section from the next H2 keep separating it.
// Every other byte of the record is unchanged.
// The support log is read, never written.
//
// When Validate returns findings, the prior bytes are restored and the findings come back with a nil error.
// When Validate returns an error, the prior bytes are restored and the error comes back.
func AppendDecision(decisionRecordPath, supportLogPath string, d AddedDecision) ([]Finding, error) {
	return AppendDecisions(decisionRecordPath, supportLogPath, []AddedDecision{d})
}

// AppendDecisions adds every entry of ds, in order, at the end of the record's `## Decisions` section.
// An entry that sets PatternChange also gets its PATTERN bullet.
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

// insertDecision returns content with d's entry added at the end of its `## Decisions` section.
// When d.PatternChange or d.WithdrawnPattern is set, it also writes or strikes d's `## PATTERN changes` bullet.
// An empty field of d after trimming and a record with no `## Decisions` heading are errors.
// So is a withdrawal of a bullet that content lacks.
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

	var patternAt, patternEnd int
	var insertion string
	switch withdrawn := strings.TrimSpace(d.WithdrawnPattern); {
	case withdrawn != "":
		var found bool
		patternAt, patternEnd, found = patternBulletRange(content, withdrawn)
		if !found {
			return "", fmt.Errorf("%w: %s", ErrPatternBulletAbsent, withdrawn)
		}
		if patternEnd < len(content) {
			patternEnd++
		}
		entry += "\nWithdraws: `" + withdrawn + "`\n"
	case d.PatternChange != nil:
		bullet := patternChangeAppendedBullet(*d.PatternChange, by, d.Date)
		var replace bool
		patternAt, patternEnd, replace = patternBulletRange(content, strings.TrimSpace(d.PatternChange.ID))
		insertion = bullet
		if !replace {
			var needHeading bool
			patternAt, needHeading = patternChangesInsertOffset(content)
			patternEnd = patternAt
			insertion = patternChangesInsertion(content, patternAt, needHeading, bullet)
		}
	default:
		return content[:at] + entry + content[at:], nil
	}
	if patternAt >= at {
		return content[:at] + entry + content[at:patternAt] + insertion + content[patternEnd:], nil
	}
	return content[:patternAt] + insertion + content[patternEnd:at] + entry + content[at:], nil
}

// addedDecisionHeadingPrefix starts the heading of every added-decision entry.
const addedDecisionHeadingPrefix = "### Added after Discussion ("

// ParseAddedDecisions returns every added-decision heading of record outside a fenced block, in order.
// A heading has the form `### Added after Discussion (<by>, <date>): <title>`.
// Each comes back with By, Date (the `2006-01-02` day in UTC) and Title set, and Decision and Rationale empty.
// A heading with that prefix but no non-empty author or no valid date is an error naming its line number.
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

// AddedDecisionTexts returns the entries of the decision record at recordPath, in record order.
// Callers render the texts into an agent's prompt.
// Each text is one `### Added after Discussion` entry, heading and body verbatim.
// An entry runs from its heading to its last non-blank line before the next heading of any level.
// A heading inside a fenced block starts no entry.
// An empty recordPath, a missing record and a record without an entry return nil and no error;
// a read error is returned.
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

// checkAddedDecisionDropped names the Finding for a record that lacks an added-decision entry an earlier version holds.
const checkAddedDecisionDropped = "added-decision-dropped"

// CheckAddedDecisionsKept returns one Finding per added-decision entry an earlier version holds and record lacks.
// An entry is keyed by its author, date and title, and findings come in order of first appearance across versions.
// Each finding's Detail names the entry's full heading and says to restore it from the record's history, since an added entry is never the writer's to remove.
//
// It then returns one pattern-change-dropped Finding per entry ID of an appended `## PATTERN changes` bullet some of versions holds for which record holds no appended bullet, in the same order and with a Detail that names the bullet and says to restore it.
// The ID alone is the key, because AppendDecision replaces the bullet it finds for an ID with one carrying the new call's author and date.
// An ID a `Withdraws:` line of an added-decision entry of record names is no finding, since that call struck the bullet; such a line exempts the ID for good, so a bullet re-added after the withdrawal is not held to the record either.
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

// checkPatternChangeDropped is the Check of a Finding that reports a dropped appended PATTERN change bullet.
const checkPatternChangeDropped = "pattern-change-dropped"

// checkDroppedPatternBullets returns one pattern-change-dropped Finding per ID of an appended PATTERN change bullet.
// An ID is reported when some version holds an appended bullet for it and the record holds none.
// An ID the `Withdraws:` lines of the record's added-decision entries name is exempt, and so is an ID already reported.
// A version whose PATTERN changes section does not parse contributes nothing,
// and a record whose own section does not parse yields no findings.
func checkDroppedPatternBullets(record []byte, versions [][]byte) []Finding {
	keptBullets, problems := ParsePatternChanges(record)
	if len(problems) > 0 {
		return nil
	}
	seen := withdrawnPatternIDs(record)
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

// withdrawnPatternIDs returns the entry IDs the `Withdraws:` lines of record's added-decision entries name, outside fenced blocks.
func withdrawnPatternIDs(record []byte) map[string]bool {
	lines := strings.Split(string(record), "\n")
	fenced := fencedLines(lines)
	withdrawn := map[string]bool{}
	_, spans := addedDecisionSpans(lines, fenced)
	for _, span := range spans {
		for i := span.first; i <= span.last; i++ {
			if match := patternWithdrawalLine.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r")); match != nil && !fenced[i] {
				withdrawn[match[1]] = true
			}
		}
	}
	return withdrawn
}

// checkDroppedAddedDecisions returns one added-decision-dropped Finding per entry of versions that kept lacks.
// An entry matches on its author, date and title.
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

// parseAddedDecisionHeading parses the part of an added-decision heading after its opening parenthesis:
// `<by>, <date>): <title>`.
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

// decisionsInsertOffset returns the byte offset in content where a new entry goes:
// just after the last non-blank line of the `## Decisions` section.
// The section ends at the next H2 line or at the end of content.
// It returns ErrNoDecisionsHeading when content has no such heading.
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

// fencedLines reports for each line whether it is a fence line or lies inside a fenced block.
// A heading or marker on such a line is ordinary text.
//
// A fence follows CommonMark's rule for a top-level fenced code block.
// It opens on a line of at most three leading spaces and a run of at least three backticks or tildes;
// a backtick run whose info string holds a backtick opens none.
// An open fence closes only on a line of at most three leading spaces and a run of the opening character.
// That run must be at least as long as the opening run, and nothing but spaces or tabs may follow it.
// Any other line inside an open fence is body text, and an unclosed fence runs to the end of the lines.
// A line indented four or more spaces neither opens nor closes.
// A list item or blockquote is not read, so a fence nested in one counts as a top-level line.
func fencedLines(lines []string) []bool {
	fenced := make([]bool, len(lines))
	var openChar byte
	openRun := 0
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		char, run, rest := fenceRun(line)
		if openRun == 0 {
			if run >= 3 && !(char == '`' && strings.Contains(rest, "`")) {
				openChar, openRun = char, run
				fenced[i] = true
			}
			continue
		}
		fenced[i] = true
		if char == openChar && run >= openRun && strings.Trim(rest, " \t") == "" {
			openRun = 0
		}
	}
	return fenced
}

// fenceRun splits line into the fence character, the length of its leading run and the text after the run.
// It returns a zero run when the line is indented four or more spaces.
// It also returns a zero run when, after at most three spaces, the line does not start with a backtick or tilde.
func fenceRun(line string) (char byte, run int, rest string) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 || indent == len(line) || (line[indent] != '`' && line[indent] != '~') {
		return 0, 0, ""
	}
	char = line[indent]
	end := indent
	for end < len(line) && line[end] == char {
		end++
	}
	return char, end - indent, line[end:]
}
