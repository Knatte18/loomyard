// prompt.go composes the discussion producer's interview prompt: it reads the discussion stencil
// from stencilsDir at call time, builds the marker values it requires, specs_dir among them, and fills it via
// internal/stencil, mirroring internal/burlerengine's composePrompt shape.

package loomengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/twilldirective"
)

// composePrompt builds the discussion producer's interview prompt by reading the
// "loom-template-discussion" stencil from stencilsDir and filling it with the four required
// top-level marker values and the required specs_dir marker, plus the optional pattern_directive and friction_directive markers.
// specsDir is the directory the discussion format spec is deployed to.
// patternDirective is the caller-resolved PATTERN directive -- an empty string means PATTERN is inactive,
// and renders as nothing.
// frictionDirective is the caller-resolved Tier 2 note directive for this run --
// an empty string means Tier 2 is off or the read failed, and renders as nothing.
// parentDirective is the caller-rendered parent directive and renders as nothing when empty.
// twillDirective and twillWriteScope are the caller-rendered twill blocks and render as nothing when empty;
// a non-empty one is set off with a newline on each side, so it sits in a paragraph of its own.
func composePrompt(stencilsDir, specsDir, slug,decisionRecordPath, supportLogPath, patternDirective, frictionDirective, parentDirective, twillDirective, twillWriteScope string, autonomous bool) ([]byte, error) {
	template, err := stencilstore.Read(stencilsDir, "loom-template-discussion")
	if err != nil {
		return nil, err
	}

	friction.WarnIfMarkerAbsent(template, "loom-template-discussion", frictionDirective)

	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("loom: compose discussion prompt: %w", err)
	}

	values := map[string]string{
		"slug":                     slug,
		"specs_dir":                specsDir,
		"decision_record_path":     decisionRecordPath,
		"support_log_path":         supportLogPath,
		"mode_rules":               modeRules(autonomous),
		"pattern_directive":        patternDirective,
		friction.MarkerName:        frictionDirective,
		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,

		twilldirective.MarkerName:           setOffParagraph(twillDirective),
		twilldirective.WriteScopeMarkerName: setOffParagraph(twillWriteScope),
	}

	optional := []string{"pattern_directive", friction.MarkerName, parentdirective.MarkerName, twilldirective.MarkerName, twilldirective.WriteScopeMarkerName}
	rendered, err := stencil.FillOptional(template, values, optional)
	if err != nil {
		return nil, fmt.Errorf("loom: compose discussion prompt: %w", err)
	}
	return rendered, nil
}

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
