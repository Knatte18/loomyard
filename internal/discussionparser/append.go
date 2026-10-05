// append.go implements AppendDecision, the only write path into the decision record: it adds one
// design call made after the Discussion ended to the record's `## Decisions` section, so a later
// reader of the record sees it beside the decisions the Discussion itself made.

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
}

// AppendDecision adds d as the last entry of the decision record's `## Decisions` section and then
// runs Validate over the result.
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
	prior, err := os.ReadFile(decisionRecordPath)
	if err != nil {
		return nil, err
	}

	by, title := strings.TrimSpace(d.By), strings.TrimSpace(d.Title)
	decision, rationale := strings.TrimSpace(d.Decision), strings.TrimSpace(d.Rationale)
	for name, value := range map[string]string{"by": by, "title": title, "decision": decision, "rationale": rationale} {
		if value == "" {
			return nil, fmt.Errorf("decision %s is empty", name)
		}
	}

	content := string(prior)
	at, err := decisionsInsertOffset(content)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", decisionRecordPath, err)
	}

	entry := fmt.Sprintf("\n### Added after Discussion (%s, %s): %s\n\nDecision: %s\n\nRationale: %s\n",
		by, d.Date.UTC().Format("2006-01-02"), title, decision, rationale)
	if at > 0 && content[at-1] != '\n' {
		entry = "\n" + entry
	}

	info, err := os.Stat(decisionRecordPath)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(decisionRecordPath, []byte(content[:at]+entry+content[at:]), info.Mode().Perm()); err != nil {
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
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
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
