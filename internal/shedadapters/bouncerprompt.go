// bouncerprompt.go holds the two variants of the focus-schema text both Bouncer stencils carry, so
// the stencils stay conditional-free while the exclude_lenses channel is requested only from a
// Bouncer told its round has a cluster fan.
// It also holds the judge's decision rule, whose CIRCLING bullet appears only from the circling checkpoint round on.

package shedadapters

// decisionRuleMarker returns the value of the judge stencil's decision_rule marker for round and checkpoint.
//
// Below the checkpoint the rule offers CONVERGED and CONTINUE only and names no CIRCLING,
// so a judge cannot rule on no-progress evidence before the run has had rounds to show progress.
// At or above it the rule adds CIRCLING, earned only by a gating finding that sits on a key the facts list as open in an earlier round and is still open in this one.
// The variant lives here rather than in the stencil for the same reason focusSchemaMarkers does:
// the stencil stays conditional-free.
func decisionRuleMarker(round, checkpoint int) string {
	rule := "A finding is gating when its class is `design`.\n" +
		"\n" +
		"- `CONTINUE` while the latest round carries a gating-class finding at MEDIUM or worse, or any BLOCKING finding.\n" +
		"- `CONVERGED` when the latest round carries neither.\n"
	if round >= checkpoint {
		rule += "- `CIRCLING` when a gating finding sits on a key the facts list as open in an earlier round, unchanged or reopened, and is still open in this round.\n" +
			"  A rising count of findings or new keys alone is never circling, and with progress the verdict is `CONTINUE`.\n" +
			"  A parse-error row is no evidence.\n"
	}
	rule += "- A parse-error row for the latest round means `CONTINUE`.\n"
	return rule
}

// focusSchemaMarkers returns the values of the three focus-schema markers -- focus_example_lists,
// focus_list_rules, and approved_focus_lists -- for the Bouncer stencils.
//
// The variant lives here rather than in the stencils because stencil.Fill's every-marker-non-empty
// rule makes a marker inside a {{if}} branch render silently blank when present-but-empty, so a
// conditional stencil could not hold the two variants safely.
// Every value renders non-empty in both modes.
//
// With clusterExcludes true the values ask the judge for both exclude_lenses and focus;
// with it false they ask for focus alone and name no exclude_lenses key anywhere, since a judge
// invited to exclude lenses from a round with no cluster fan produces a breach the round can only drop.
func focusSchemaMarkers(clusterExcludes bool) map[string]string {
	if clusterExcludes {
		return map[string]string{
			"focus_example_lists": "exclude_lenses: []\nfocus: []",
			"focus_list_rules": "- `exclude_lenses` is a list of strings, possibly empty.\n" +
				"- `focus` is a list of strings, possibly empty.\n" +
				"- Both list keys are always present, even when empty -- never omit either key, and never write a\n" +
				"  scalar where a list is required.",
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
