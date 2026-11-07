// bouncerprompt.go holds the two variants of the focus-schema text that both Bouncer stencils carry.
// It also holds the judge's decision rule, whose CIRCLING bullet appears only from the circling checkpoint round on.
// The variants live here so the stencils stay conditional-free:
// the exclude_lenses channel is requested only from a Bouncer whose round has a cluster fan.

package shedadapters

// decisionRuleMarker returns the value of the judge stencil's decision_rule marker for round and checkpoint.
//
// Round 1 rules CONTINUE over any gating-class finding at MEDIUM or worse, or any BLOCKING finding.
// From round 2 on the rule is lighter: a BLOCKING finding rules CONTINUE.
// A gating-class finding at MEDIUM or worse on a key the facts list as open in an earlier round rules CONTINUE too.
// A gating finding on a key first raised in the latest round does not block it.
// It goes into the decision record's `## Open risks` for a later check.
// A parse-error line in the facts' earlier-open list means the judge rules by round 1's rule.
//
// Below the checkpoint the rule offers CONVERGED and CONTINUE only and names no CIRCLING,
// so a judge cannot rule on no-progress evidence before the run has had rounds to show progress.
// At or above it the rule adds CIRCLING.
// CIRCLING is earned only by a gating finding on a key the facts list as open in an earlier round.
// The key must still be open in this round.
// The variant lives here so the stencil stays conditional-free.
func decisionRuleMarker(round, checkpoint int) string {
	const roundOneContinue = "`CONTINUE` while the latest round carries a gating-class finding at MEDIUM or worse, or any BLOCKING finding.\n"
	rule := "A finding is gating when its class is `design`.\n" +
		"\n"
	if round < 2 {
		rule += "- " + roundOneContinue +
			"- `CONVERGED` when the latest round carries neither.\n"
	} else {
		rule += "- `CONTINUE` while the latest round carries any BLOCKING finding, or a gating-class finding at MEDIUM or worse that you map to a key in the facts' list of keys open in an earlier round, unchanged or reopened.\n" +
			"- `CONVERGED` when the latest round carries neither.\n" +
			"  A gating-class finding at MEDIUM or worse on a key first raised in the latest round does not block it:\n" +
			"  Go carries that finding into the decision record's `## Open risks` for a later check.\n" +
			"  A parse-error line in the facts' list of keys open in an earlier round means that list cannot be trusted, so rule by round 1's rule instead: " + roundOneContinue
	}
	if round >= checkpoint {
		rule += "- `CIRCLING` when a gating finding sits on a key the facts list as open in an earlier round, unchanged or reopened, and is still open in this round.\n" +
			"  A rising count of findings or new keys alone is never circling, and with progress the verdict is `CONTINUE`.\n" +
			"  A parse-error row is no evidence.\n"
	}
	rule += "- A parse-error row for the latest round means `CONTINUE`.\n"
	return rule
}

// focusSchemaMarkers returns the values of the three focus-schema markers for the Bouncer stencils:
// focus_example_lists, focus_list_rules, and approved_focus_lists.
//
// The variants live here, not in the stencils, because stencil.Fill requires every marker non-empty:
// a marker inside an {{if}} branch renders silently blank when present but empty,
// so a conditional stencil could not hold the two variants safely.
// Every value renders non-empty in both modes.
//
// With clusterExcludes true the values ask the judge for exclude_lenses and focus.
// They also state when a lens may be excluded.
// With it false they ask for focus alone and name no exclude_lenses key anywhere.
// A judge invited to exclude lenses from a round with no cluster fan produces a breach the round can only drop.
func focusSchemaMarkers(clusterExcludes bool) map[string]string {
	if clusterExcludes {
		return map[string]string{
			"focus_example_lists": "exclude_lenses: []\nfocus: []",
			"focus_list_rules": "- `exclude_lenses` is a list of strings, possibly empty.\n" +
				"- `focus` is a list of strings, possibly empty.\n" +
				"- Both list keys are always present, even when empty -- never omit either key, and never write a\n" +
				"  scalar where a list is required.\n" +
				"- An exclusion is permanent for the segment generation: an excluded lens never runs again.\n" +
				"- Exclude a lens only when the latest round ran it (the facts file's `## Lenses` section lists them)\n" +
				"  and its area is settled: the latest round's findings from it carry nothing at MEDIUM or worse,\n" +
				"  and its area is not in the next round's `focus`.\n" +
				"- Earlier exclusions are carried by Go: never restate them in `exclude_lenses`.",
			"approved_focus_lists": "an empty `exclude_lenses` and an empty `focus`",
		}
	}
	return map[string]string{
		"focus_example_lists": "focus: []",
		"focus_list_rules": "- `focus` is a list of strings, possibly empty.\n" +
			"- The `focus` key is always present, even when empty --\n" +
			"  never omit it, and never write a scalar where a list is required.",
		"approved_focus_lists": "an empty `focus`",
	}
}
