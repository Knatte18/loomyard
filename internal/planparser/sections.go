// sections.go extracts the overview's three optional plan-level body sections —
// "## Shared Decisions", "## Rename mechanic", and "## verify:" — into Plan's
// SharedDecisions, RenameMechanic, and Verify fields. All three are read once from
// 00-overview.md's body by ParsePlan; nothing else in this package re-parses them.

package planparser

import "strings"

// planVerifyHeading is the exact "## " heading plan-format-v3.md pins for the
// overview's optional plan-level integration verify section.
const planVerifyHeading = "## verify:"

// sharedDecisionsHeading and renameMechanicHeading are the exact "## " headings
// plan-format-v3.md pins for the overview's optional Shared Decisions and Rename
// mechanic sections.
const (
	sharedDecisionsHeading = "## Shared Decisions"
	renameMechanicHeading  = "## Rename mechanic"
)

// extractSection returns the lines strictly between an exact heading match
// (trimmed equality) and the next "## " heading or EOF, or nil when heading is not
// present at all. Because the match requires a two-hash prefix, a "### Card"-style
// sub-heading (not used at the plan level, but mirrored here for consistency with
// the frozen v2 parser's helper shape) never terminates a "## " section early.
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

// joinSectionBody joins section's lines back into a single trimmed string, or
// returns "" for a nil section (heading absent). Preserves the section's own
// internal blank lines and formatting — SharedDecisions and RenameMechanic are
// reproduced for fork prompts verbatim, not reflowed.
func joinSectionBody(section []string) string {
	if section == nil {
		return ""
	}
	return strings.TrimSpace(strings.Join(section, "\n"))
}

// firstNonEmptyLine returns the first non-blank line of section, trimmed, or "" if
// section is nil or entirely blank. Used for "## verify:", which plan-format-v3.md
// defines as carrying a single command line, unlike the prose-shaped Shared
// Decisions / Rename mechanic sections.
func firstNonEmptyLine(section []string) string {
	for _, raw := range section {
		line := strings.TrimSpace(raw)
		if line != "" {
			return line
		}
	}
	return ""
}

// extractPlanSections populates plan's three plan-level body sections
// (SharedDecisions, RenameMechanic, Verify) from the overview's body — the text
// following 00-overview.md's frontmatter fence, as returned by
// parseOverviewFrontmatter. All three are optional; each is left "" when its
// heading is absent.
func extractPlanSections(plan *Plan, body string) {
	plan.SharedDecisions = joinSectionBody(extractSection(body, sharedDecisionsHeading))
	plan.RenameMechanic = joinSectionBody(extractSection(body, renameMechanicHeading))
	plan.Verify = firstNonEmptyLine(extractSection(body, planVerifyHeading))
}
