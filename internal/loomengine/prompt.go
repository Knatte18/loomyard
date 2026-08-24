// prompt.go composes the discussion producer's interview prompt: it reads the discussion stencil
// from stencilsDir at call time, builds the four marker values it requires, and fills it via
// internal/stencil, mirroring internal/burlerengine's composePrompt shape.

package loomengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// composePrompt builds the discussion producer's interview prompt by reading the
// "loom-template-discussion" stencil from stencilsDir and filling it with the four top-level marker
// values.
func composePrompt(stencilsDir, slug, decisionRecordPath, supportLogPath string, autonomous bool) ([]byte, error) {
	template, err := stencilstore.Read(stencilsDir, "loom-template-discussion")
	if err != nil {
		return nil, err
	}

	values := map[string]string{
		"slug":                 slug,
		"decision_record_path": decisionRecordPath,
		"support_log_path":     supportLogPath,
		"mode_rules":           modeRules(autonomous),
	}

	rendered, err := stencil.Fill(template, values)
	if err != nil {
		return nil, fmt.Errorf("loom: compose discussion prompt: %w", err)
	}
	return rendered, nil
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
