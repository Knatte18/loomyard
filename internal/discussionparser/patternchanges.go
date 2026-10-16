// patternchanges.go implements the decision record's optional `## PATTERN changes` section:
// ParsePatternChanges reads and checks its bullets, and DeclaredPatternChanges hands the rendered bullets to the callers that put them in front of an agent.

package discussionparser

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// patternChangesHeading is the H2 heading of the optional PATTERN changes section.
const patternChangesHeading = "## PATTERN changes"

// checkPatternChangeMalformed names a Finding produced when a bullet of the PATTERN changes section breaks the bullet shape.
const checkPatternChangeMalformed = "pattern-change-malformed"

// ErrPatternChangesInvalid reports a decision record whose `## PATTERN changes` section is malformed.
// The caller can refuse with its own way forward.
var ErrPatternChangesInvalid = errors.New("decision record's PATTERN changes section is malformed")

// The actions a PATTERN change bullet declares.
const (
	patternActionAdds    = "adds"
	patternActionChanges = "changes"
	patternActionRemoves = "removes"
)

// patternBulletHead matches the start of a bullet: the backticked entry ID and the em dash, capturing the ID and the rest of the line.
var patternBulletHead = regexp.MustCompile("^- `(PATTERN-[a-z0-9-]+)` — (.+)$")

// patternBulletSuffix matches the suffix `lyx loom decision add` appends to a bullet, capturing the author and the day.
var patternBulletSuffix = regexp.MustCompile(` — added after Discussion \(([^,()]+), ([0-9]{4}-[0-9]{2}-[0-9]{2})\)$`)

// PatternChange is one entry of the decision record's `## PATTERN changes` section:
// a PATTERN entry the task adds, changes or removes.
// By and Date are set only on a bullet `lyx loom decision add` appended after the Discussion.
type PatternChange struct {
	// ID is the entry ID with its `PATTERN-` prefix and no backticks.
	ID string
	// Action is `adds`, `changes` or `removes`.
	Action string
	// Text is the new entry text; empty for `removes`.
	Text string
	By   string
	Date time.Time
}

// Bullet renders the change as the section's bullet, without the appended-after-Discussion suffix.
// This is the form the directives and the plan carry.
func (c PatternChange) Bullet() string {
	bullet := "- `" + c.ID + "` — " + c.Action
	if c.Action == patternActionRemoves {
		return bullet
	}
	return bullet + ": " + c.Text
}

// ParsePatternChanges reads the `## PATTERN changes` section of record, which runs from its heading to the next H2 outside a fenced block, and returns its entries in order.
// Each defect is one pattern-change-malformed finding naming its line: a non-blank line that is not a bullet of the shape, an unknown action, `adds` or `changes` without text, `removes` followed by text, and a repeated entry ID.
// A record without the section returns nil and no finding.
// A finding's Path is left empty for the caller to set.
func ParsePatternChanges(record []byte) ([]PatternChange, []Finding) {
	lines := strings.Split(string(record), "\n")
	fenced := fencedLines(lines)

	start := -1
	for i, line := range lines {
		if !fenced[i] && strings.TrimRight(line, " \t\r") == patternChangesHeading {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil, nil
	}

	var changes []PatternChange
	var findings []Finding
	seen := make(map[string]bool)
	for i := start; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t\r")
		if !fenced[i] && strings.HasPrefix(line, "## ") {
			break
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		change, problem := parsePatternBullet(line)
		if problem == "" && seen[change.ID] {
			problem = "a second bullet for `" + change.ID + "`"
		}
		if problem != "" {
			findings = append(findings, Finding{
				Check:  checkPatternChangeMalformed,
				Detail: fmt.Sprintf("line %d: %s", i+1, problem),
			})
			continue
		}
		seen[change.ID] = true
		changes = append(changes, change)
	}
	return changes, findings
}

// parsePatternBullet parses one non-blank line of the section.
// A non-empty problem names the defect, and the returned change is then meaningless.
func parsePatternBullet(line string) (PatternChange, string) {
	head := patternBulletHead.FindStringSubmatch(line)
	if head == nil {
		return PatternChange{}, fmt.Sprintf("%q is not a bullet of the shape - `PATTERN-<id>` — adds|changes: <text> or removes, with <id> in [a-z0-9-]+", line)
	}
	change := PatternChange{ID: head[1]}
	rest := head[2]

	if suffix := patternBulletSuffix.FindStringSubmatchIndex(rest); suffix != nil {
		date, err := time.Parse("2006-01-02", rest[suffix[4]:suffix[5]])
		if err != nil {
			return PatternChange{}, fmt.Sprintf("the appended suffix of `%s` carries an invalid date", change.ID)
		}
		change.By = strings.TrimSpace(rest[suffix[2]:suffix[3]])
		change.Date = date
		rest = rest[:suffix[0]]
	}

	if rest == patternActionRemoves {
		change.Action = patternActionRemoves
		return change, ""
	}
	if strings.HasPrefix(rest, patternActionRemoves) {
		return PatternChange{}, fmt.Sprintf("`%s` removes an entry and takes no text", change.ID)
	}
	action, text, _ := strings.Cut(rest, ":")
	action, text = strings.TrimSpace(action), strings.TrimSpace(text)
	if action != patternActionAdds && action != patternActionChanges {
		return PatternChange{}, fmt.Sprintf("`%s` has unknown action %q; want adds, changes or removes", change.ID, action)
	}
	if text == "" {
		return PatternChange{}, fmt.Sprintf("`%s` %s an entry and needs its new text after a colon", change.ID, action)
	}
	change.Action = action
	change.Text = text
	return change, ""
}

// DeclaredPatternChanges returns the bullets of the decision record's `## PATTERN changes` section, in order and without the appended suffix, for a caller to render into an agent's prompt.
// An empty recordPath or a record that does not exist returns nil and no error, as does a record without the section.
// A read error is returned.
// A malformed section returns an error wrapping ErrPatternChangesInvalid that names the record, the section and each defect, and ends with a way forward the caller completes with its own re-step.
func DeclaredPatternChanges(recordPath string) ([]string, error) {
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
	changes, findings := ParsePatternChanges(data)
	if len(findings) > 0 {
		details := make([]string, len(findings))
		for i, f := range findings {
			details[i] = f.Detail
		}
		return nil, fmt.Errorf("%w: %s, section %s: %s; way forward: the operator fixes and commits the `%s` section of the decision record (the orch never edits the record and escalates this refusal to the operator), confirms it with `lyx loom validate-discussion`, then",
			ErrPatternChangesInvalid, recordPath, patternChangesHeading, strings.Join(details, "; "), patternChangesHeading)
	}
	var bullets []string
	for _, c := range changes {
		bullets = append(bullets, c.Bullet())
	}
	return bullets, nil
}
