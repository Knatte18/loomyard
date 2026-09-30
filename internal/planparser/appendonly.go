// appendonly.go compares a base plan and an extended plan for the append-only rule the rework row
// enforces: the extended plan is the base plus one or more appended cards, and nothing else changed.
// It works over planparser's own parsed model, so a whitespace-only reflow the parser normalizes
// away is not a violation.

package planparser

import (
	"fmt"
	"reflect"
)

// CheckAppendOnly returns one human-readable violation per breach of the append-only rule, and nil
// when extended is base plus one or more appended cards.
// It checks the frontmatter fields, the plan-level sections, every pre-existing card by position,
// and that at least one card was appended.
// Card numbering after the base cards is ValidateFormat's concern and is not re-checked here.
func CheckAppendOnly(base, extended *Plan) []string {
	var violations []string

	if base.Format != extended.Format {
		violations = append(violations, fmt.Sprintf("frontmatter format changed from %d to %d", base.Format, extended.Format))
	}
	if base.Approved != extended.Approved {
		violations = append(violations, fmt.Sprintf("frontmatter approved changed from %t to %t", base.Approved, extended.Approved))
	}
	if base.Root != extended.Root {
		violations = append(violations, fmt.Sprintf("frontmatter root changed from %q to %q", base.Root, extended.Root))
	}
	if base.Language != extended.Language {
		violations = append(violations, fmt.Sprintf("frontmatter language changed from %q to %q", base.Language, extended.Language))
	}

	if base.Framing != extended.Framing {
		violations = append(violations, "framing section changed")
	}
	if base.SharedDecisions != extended.SharedDecisions {
		violations = append(violations, "Shared Decisions section changed")
	}
	if base.RenameMechanic != extended.RenameMechanic {
		violations = append(violations, "Rename mechanic section changed")
	}
	if base.Verify != extended.Verify {
		violations = append(violations, "verify section changed")
	}

	for i, baseCard := range base.Cards {
		if i >= len(extended.Cards) {
			violations = append(violations, fmt.Sprintf("card %d (%s) is missing from the extended plan", baseCard.Number, baseCard.Slug))
			continue
		}
		if !reflect.DeepEqual(baseCard, extended.Cards[i]) {
			violations = append(violations, fmt.Sprintf("card %d (%s) differs from its base form", baseCard.Number, baseCard.Slug))
		}
	}

	if len(extended.Cards) <= len(base.Cards) {
		violations = append(violations, "no card was appended")
	}

	return violations
}
