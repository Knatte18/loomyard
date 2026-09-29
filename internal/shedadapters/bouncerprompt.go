// bouncerprompt.go holds the two variants of the focus-schema text both Bouncer stencils carry, so
// the stencils stay conditional-free while the exclude_lenses channel is requested only from a
// Bouncer told its round has a cluster fan.

package shedadapters

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
