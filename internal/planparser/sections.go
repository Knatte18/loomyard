// sections.go extracts the overview's four optional plan-level body sections into Plan's SharedDecisions, RenameMechanic, Verify and PatternChanges fields.
// The sections are "## Shared Decisions", "## Rename mechanic", "## verify:" and "## PATTERN changes".
// All four are read once from 00-overview.md's body by ParsePlan; nothing else in this package re-parses them.

package planparser

import "strings"

// planVerifyHeading is the exact "## " heading loom-plan-spec.md pins for the overview's optional plan-level verify section.
const planVerifyHeading = "## verify:"

// sharedDecisionsHeading and renameMechanicHeading are the exact "## " headings loom-plan-spec.md pins for the optional sections.
const (
	sharedDecisionsHeading = "## Shared Decisions"
	renameMechanicHeading  = "## Rename mechanic"
)

// extractSection returns the lines strictly between an exact heading match and the next "## " heading or EOF.
func extractSection(body, heading string) []string {
	lines := strings.Split(body, "\n")

	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return nil
	}

	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	return lines[start:end]
}

// joinSectionBody joins section's lines back into a single trimmed string, preserving internal blank lines and formatting.
func joinSectionBody(section []string) string {
	if section == nil {
		return ""
	}
	return strings.TrimSpace(strings.Join(section, "\n"))
}

// verifyCommandJoiner chains the verify section's command lines into one shell command line,
// so the first failing command fails the whole check.
const verifyCommandJoiner = " && "

// joinVerifyCommands returns section's command lines — the non-blank lines left after skipping fence and comment lines — trimmed and chained with verifyCommandJoiner into the single command line webster runs,
// or "" if section is nil or holds only blank, fence, and comment lines.
// Two kinds of line are skipped, judged on the trimmed line:
// a markdown code-fence line (starts with "```" or "~~~", so info strings, longer fences, and indented fences need no open/close tracking),
// and a full-line shell comment (starts with "#").
// A chained fence marker is a shell syntax error at every commit,
// and a chained comment turns every later command into comment text, a silent false pass.
// The plan stencil tells the planner the section holds one or more commands, one per line;
// keeping only the first line silently dropped every later one, so a plan whose section read `go vet ./...` then `go test ./...` had its tests skipped by the integration gate.
func joinVerifyCommands(section []string) string {
	var commands []string
	for _, raw := range section {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") || strings.HasPrefix(line, "#") {
			continue
		}
		commands = append(commands, line)
	}
	return strings.Join(commands, verifyCommandJoiner)
}

// patternChangesHeading is the exact "## " heading loom-plan-spec.md pins for the overview's optional PATTERN changes section.
const patternChangesHeading = "## PATTERN changes"

// patternEntryPrefix opens the backticked entry id of a PATTERN changes bullet.
const patternEntryPrefix = "PATTERN-"

// patternChangeSeparator joins a bullet's entry id to its action.
const patternChangeSeparator = " — "

// PatternChange is one bullet of the overview's "## PATTERN changes" section: the PATTERN entry the task changes, how, and the declared text.
// A line fitting no bullet shape is kept with an empty ID and Action and the whole line as Text, so a comparison against the decision record flags it.
type PatternChange struct {
	// ID is the backticked entry id without the backticks, such as "PATTERN-cwd-resolution".
	ID string
	// Action is the verb before the colon, or "removes" for a bullet that ends there.
	Action string
	// Text is the trimmed declared text after the colon, empty for "removes".
	Text string
}

// parsePatternChanges reads each non-blank, non-fence line of section as a PATTERN change.
// A line of the shape "- `PATTERN-<id>` — <action>: <text>" or "- `PATTERN-<id>` — removes" yields its id, action and text;
// any other line yields an entry with only Text set to the trimmed line.
// It returns nil when the section holds no such line.
func parsePatternChanges(section []string) []PatternChange {
	var changes []PatternChange
	for _, raw := range section {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			continue
		}
		if change, ok := parsePatternChangeBullet(line); ok {
			changes = append(changes, change)
			continue
		}
		changes = append(changes, PatternChange{Text: line})
	}
	return changes
}

// parsePatternChangeBullet reports line's id, action and text when it fits the PATTERN change bullet shape.
func parsePatternChangeBullet(line string) (PatternChange, bool) {
	rest, ok := strings.CutPrefix(line, "- `"+patternEntryPrefix)
	if !ok {
		return PatternChange{}, false
	}
	id, rest, ok := strings.Cut(rest, "`"+patternChangeSeparator)
	if !ok || id == "" {
		return PatternChange{}, false
	}
	change := PatternChange{ID: patternEntryPrefix + id}
	if rest == "removes" {
		change.Action = rest
		return change, true
	}
	action, text, ok := strings.Cut(rest, ": ")
	if !ok || action == "" || strings.ContainsAny(action, " \t") {
		return PatternChange{}, false
	}
	change.Action = action
	change.Text = strings.TrimSpace(text)
	return change, true
}

// extractPlanSections populates plan's four plan-level body sections from the overview's body.
func extractPlanSections(plan *Plan, body string) {
	plan.SharedDecisions = joinSectionBody(extractSection(body, sharedDecisionsHeading))
	plan.RenameMechanic = joinSectionBody(extractSection(body, renameMechanicHeading))
	plan.Verify = joinVerifyCommands(extractSection(body, planVerifyHeading))
	plan.PatternChanges = parsePatternChanges(extractSection(body, patternChangesHeading))
}
