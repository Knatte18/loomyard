// prompt.go composes the discussion producer's interview prompt: it builds
// the four marker values the embedded template (prompttemplate.go) requires
// and fills it via internal/stencil, mirroring internal/burlerengine's
// composePrompt shape.

package loomengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// composePrompt builds the discussion producer's interview prompt by
// composing each of the template's four top-level marker values (slug, the
// two output-file paths, and the mode-specific instructions for how to
// obtain answers) and filling discussionTemplate with them via
// stencil.Fill.
func composePrompt(slug, decisionRecordPath, supportLogPath string, autonomous bool) ([]byte, error) {
	values := map[string]string{
		"slug":                 slug,
		"decision_record_path": decisionRecordPath,
		"support_log_path":     supportLogPath,
		"mode_rules":           modeRules(autonomous),
	}

	rendered, err := stencil.Fill(discussionTemplate, values)
	if err != nil {
		return nil, fmt.Errorf("loom: compose discussion prompt: %w", err)
	}
	return rendered, nil
}

// modeRules returns the {{.mode_rules}} block: autonomous mode tells the
// agent no operator will answer, so it must make its own best-judgment
// pick at every decision point and record it in the support log's Question
// ledger; interactive mode tells the agent an operator is at the pane and
// how to ask it questions there. Both branches return non-empty prose so
// the mode_rules top-level marker is never left blank.
func modeRules(autonomous bool) string {
	if autonomous {
		return "This session is running in autonomous (`--auto`) mode: no operator will " +
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
