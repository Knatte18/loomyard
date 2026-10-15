// prompt.go holds the two marker-value helpers the table builders share: setOffParagraph, which sets a block off as its own paragraph, and modeRules, the {{.mode_rules}} block of the discussion prompt.

package loomengine

// setOffParagraph wraps a non-empty block in one newline on each side, so the line it replaces gains a blank line before and after it; an empty block stays empty.
func setOffParagraph(block string) string {
	if block == "" {
		return ""
	}
	return "\n" + block + "\n"
}

// modeRules returns the {{.mode_rules}} block for autonomous or interactive mode.
func modeRules(autonomous bool) string {
	if autonomous {
		return "This session is running autonomously: no operator will " +
			"answer questions. For every decision point, make your own best-judgment " +
			"choice and proceed — never block waiting for input, and never call the " +
			"`AskUserQuestion` tool. Record each self-made pick and its rationale in the " +
			"support log's `## Question ledger`, marked as an auto-pick."
	}
	return "This session is running interactively: an operator is present at the pane. " +
		"Ask questions as plain numbered-list text directly in the pane, and wait for " +
		"the operator's typed reply before proceeding. Batch related questions together " +
		"(at most 5 questions per batch) and always list your recommended answer as " +
		"option 1. Never call the `AskUserQuestion` tool — it opens a modal dialog the " +
		"resume mechanism cannot drive; ask conversationally in the pane instead."
}
