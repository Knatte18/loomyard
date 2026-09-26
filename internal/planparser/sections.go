// sections.go extracts the overview's three optional plan-level body sections — "## Shared
// Decisions", "## Rename mechanic", and "## verify:" — into Plan's SharedDecisions, RenameMechanic,
// and Verify fields.
// All three are read once from 00-overview.md's body by ParsePlan; nothing else in this package
// re-parses them.

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

// joinVerifyCommands returns section's non-blank lines, trimmed, chained with verifyCommandJoiner
// into the single command line webster runs, or "" if section is nil or entirely blank.
// The plan stencil tells the planner the section holds one or more commands, one per line;
// keeping only the first line silently dropped every later one, so a plan whose section read
// `go vet ./...` then `go test ./...` had its tests skipped by the integration gate.
func joinVerifyCommands(section []string) string {
	var commands []string
	for _, raw := range section {
		line := strings.TrimSpace(raw)
		if line != "" {
			commands = append(commands, line)
		}
	}
	return strings.Join(commands, verifyCommandJoiner)
}

// extractPlanSections populates plan's three plan-level body sections from the overview's body.
func extractPlanSections(plan *Plan, body string) {
	plan.SharedDecisions = joinSectionBody(extractSection(body, sharedDecisionsHeading))
	plan.RenameMechanic = joinSectionBody(extractSection(body, renameMechanicHeading))
	plan.Verify = joinVerifyCommands(extractSection(body, planVerifyHeading))
}
