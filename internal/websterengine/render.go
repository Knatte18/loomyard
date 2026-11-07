// render.go composes and renders the producer prompts for webster's Master, forks and fixers.
// Each prompt is composed at render time from stencils read through stencilstore.Read,
// so this file has no go:embed directive of its own.
// The fork and recovery prompts each join the shared implementer body with their own prefix.
// One prompt cannot serve both, because the in-session fork inherits Master's whole context,
// and the cold recovery strand inherits nothing.
// Card content reaches the forks as card-file pointers rather than inlined fields,
// so the implementer reads its own card in its own turn.
// Each Render*Prompt takes a notePath for the friction directive.
// A friction.Directive error is logged and swallowed rather than returned.
// RenderForkPrompt and RenderRecoveryPrompt require a non-blank specsDir for the required specs_dir marker.
// The fill refuses a blank one, which would leave a dead cross-repository reference in the agent's instructions.
// A blank gateTiers renders "none".

package websterengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// MasterTemplate returns the Master prompt template read from stencilsDir.
func MasterTemplate(stencilsDir string) ([]byte, error) {
	return stencilstore.Read(stencilsDir, "webster-template-master")
}

// ImplementerBodyTemplate returns the shared implementer job body read from stencilsDir.
// Each fork and recovery prompt puts its own prefix ahead of it.
func ImplementerBodyTemplate(stencilsDir string) ([]byte, error) {
	return stencilstore.Read(stencilsDir, "webster-body-implementer")
}

// joinTemplateAssets strips each asset's leading banner comment via stencil.StripLeadingComment.
// It then concatenates the two bodies with a blank line between them.
// Stripping both matters: stencil.Fill strips only one leading banner from the joined text.
// Otherwise a seeded stencil's `lym-stencil:` stamp in the second banner reaches the prompt as instruction text.
func joinTemplateAssets(prefix, body []byte) []byte {
	strippedPrefix := stencil.StripLeadingComment(string(prefix))
	strippedBody := stencil.StripLeadingComment(string(body))
	return []byte(strippedPrefix + "\n\n" + strippedBody)
}

// composeForkTemplate returns the thin in-session fork prompt: webster-prefix-fork joined ahead of
// webster-body-implementer, both read from stencilsDir.
func composeForkTemplate(stencilsDir string) ([]byte, error) {
	prefix, err := stencilstore.Read(stencilsDir, "webster-prefix-fork")
	if err != nil {
		return nil, err
	}
	body, err := stencilstore.Read(stencilsDir, "webster-body-implementer")
	if err != nil {
		return nil, err
	}
	return joinTemplateAssets(prefix, body), nil
}

// composeRecoveryTemplate returns the full cold-start recovery prompt: webster-prefix-recovery
// joined ahead of webster-body-implementer, both read from stencilsDir.
func composeRecoveryTemplate(stencilsDir string) ([]byte, error) {
	prefix, err := stencilstore.Read(stencilsDir, "webster-prefix-recovery")
	if err != nil {
		return nil, err
	}
	body, err := stencilstore.Read(stencilsDir, "webster-body-implementer")
	if err != nil {
		return nil, err
	}
	return joinTemplateAssets(prefix, body), nil
}

// ForkTemplate returns the composed thin in-session fork prompt template, read from stencilsDir.
func ForkTemplate(stencilsDir string) ([]byte, error) {
	return composeForkTemplate(stencilsDir)
}

// RecoveryTemplate returns the composed full cold-start recovery prompt template, read from stencilsDir.
func RecoveryTemplate(stencilsDir string) ([]byte, error) {
	return composeRecoveryTemplate(stencilsDir)
}

// cardGateFixerMarker is the implementer body's optional marker the card-gate fixer paragraph fills, right after the card gates.
const cardGateFixerMarker = "card_gate_fixer"

// cardGateFixerParagraph returns the text the card_gate_fixer marker takes when cardFixer is set.
// The text is the webster-fragment-card-fixer fragment, led by the blank line that sets it off from the card gates.
// When cardFixer is unset it returns "", which leaves the rendered prompt without the fixer.
func cardGateFixerParagraph(stencilsDir string, cardFixer bool) (string, error) {
	if !cardFixer {
		return "", nil
	}
	fragment, err := stencilstore.Read(stencilsDir, "webster-fragment-card-fixer")
	if err != nil {
		return "", fmt.Errorf("webster: read the card-gate fixer fragment: %w", err)
	}
	return "\n\n" + strings.Trim(stencil.StripLeadingComment(string(fragment)), "\n"), nil
}

// noPrecedingBatchDigest is the sentinel the prompt renderers fill into {{.prev_digest}} when there is no digest.
// The first executed batch has no preceding batch,
// and a crash-resumed run re-driving that batch fresh carries no digest either.
// It is never blank: an empty {{.prev_digest}} would violate stencil's required-top-level-marker guarantee.
const noPrecedingBatchDigest = "none (first batch)"

// renderCardPointers renders one `- <planDirDisplay>/<card file name>` bullet per card, in declared order.
// Each card's file name is re-rooted onto planDirDisplay.
// Under hub geometry planDirDisplay is "_lym/plan".
// Under standalone geometry it is the derived state directory's absolute plan dir.
// That is the only spelling a fork running at the target repo can read.
func renderCardPointers(cards []planparser.Card, planDirDisplay string) string {
	bullets := make([]string, 0, len(cards))
	for _, c := range cards {
		bullets = append(bullets, fmt.Sprintf("- `%s`", cardPointerPath(c, planDirDisplay)))
	}
	return strings.Join(bullets, "\n")
}

// cardPointerPath returns c's card file path re-rooted onto planDirDisplay: the path its pointer bullet and its card gate bullet both spell.
func cardPointerPath(c planparser.Card, planDirDisplay string) string {
	return planDirDisplay + "/" + filepath.Base(c.SourcePath)
}

// RenderForkPrompt fills ForkTemplate for one execution batch's in-session fork, read from stencilsDir.
// Cards' pointers are rendered in planDir's display form:
// relative to promptWorktreeRoot, the pane's own cwd, when planDir sits inside it.
// {{.worktree_root}} is filled from the same caller-supplied promptWorktreeRoot.
// prevDigest is already rendered as a one-line summary by the caller.
// notePath is the caller-composed friction note path (friction.NotePath), or "" when Tier 2 is off;
// friction_directive is injected via friction.RoleImplementer when Tier 2 is on;
// a friction.Directive error is logged as a warning rather than returned.
// specsDir is the told deployed-specs directory.
// It fills the shared implementer-job body's required specs_dir marker.
// gateTiers is the hub's rendered tier and gate-list lines, filled verbatim into the body's required gate_tiers marker;
// it renders "none" when no tier is declared.
// renameTools is the hub's structure-aware rename tools, filled as bullets into the rename_tools marker;
// it renders "none" when empty.
// cardGates is the caller-rendered per-card gate step list, filled into the body's required card_gates marker.
// batchGate is the caller-rendered batch gate call, filled into the body's required batch_gate marker.
// outcomePath and summaryPath are Merriam's two contract files, filled into the outcome_path and summary_path markers;
// the fork is told never to write them.
// cardFixer says a failed card gate call goes through a cold fixer first;
// it renders the card-gate fixer paragraph after the card gates, and false leaves the text unchanged.
// The plan is not a parameter, per the fork-context-hygiene rule.
func RenderForkPrompt(batch batcher.Batch, cardGates, batchGate, prevDigest, reportPath, planDir, promptWorktreeRoot, stencilsDir, specsDir, gateTiers string, renameTools []string, outcomePath, summaryPath string, selfFixCap int, notePath string, cardFixer bool) ([]byte, error) {
	digestLine := prevDigest
	if strings.TrimSpace(digestLine) == "" {
		digestLine = noPrecedingBatchDigest
	}

	directive, err := friction.Directive(notePath, stencilsDir, friction.RoleImplementer)
	if err != nil {
		logger.Warn("webster: friction directive failed, continuing without one", "role", "implementer", "stencil", "webster-prefix-fork", "error", err)
		directive = ""
	}

	cardFixerText, err := cardGateFixerParagraph(stencilsDir, cardFixer)
	if err != nil {
		return nil, err
	}

	values := map[string]string{
		"card_pointers":     renderCardPointers(batch.Cards, masterPlanDirDisplay(promptWorktreeRoot, planDir)),
		"card_gates":        cardGates,
		"batch_gate":        batchGate,
		"report_path":       reportPath,
		"self_fix_cap":      fmt.Sprintf("%d", selfFixCap),
		"worktree_root":     promptWorktreeRoot,
		"prev_digest":       digestLine,
		"specs_dir":         specsDir,
		"gate_tiers":        gateTiersOrNone(gateTiers),
		"rename_tools":      languageconfig.BulletListValue(renameTools),
		"outcome_path":      outcomePath,
		"summary_path":      summaryPath,
		friction.MarkerName: directive,
		cardGateFixerMarker: cardFixerText,
	}
	template, err := composeForkTemplate(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("webster: read fork template: %w", err)
	}
	friction.WarnIfMarkerAbsent(template, "webster-prefix-fork", directive)
	prompt, err := stencil.FillOptional(template, values, []string{friction.MarkerName, cardGateFixerMarker})
	if err != nil {
		return nil, fmt.Errorf("webster: fill fork template: %w", err)
	}
	return prompt, nil
}

// RenderRecoveryPrompt fills RecoveryTemplate for one batch's cold-start recovery strand, read from stencilsDir.
// The recovery strand inherits nothing;
// its prompt orients from plan/00-overview.md and from any PATTERN entries before the shared implementer-job body runs.
// {{.worktree_root}} is filled from the caller-supplied promptWorktreeRoot, the pane's own cwd;
// the card pointers are spelled relative to it.
// repoRoot is the repository's worktree root holding PATTERN.md, and it feeds pattern.Directive alone.
// pattern_directive is injected if PATTERN is active.
// notePath is the caller-composed friction note path (friction.NotePath), or "" when Tier 2 is off;
// friction_directive is injected via friction.RoleImplementer when Tier 2 is on;
// a friction.Directive error is logged as a warning rather than returned.
// specsDir is the told deployed-specs directory.
// It fills the shared implementer-job body's required specs_dir marker.
// gateTiers is the hub's rendered tier and gate-list lines, filled verbatim into the body's required gate_tiers marker;
// it renders "none" when no tier is declared.
// renameTools is the hub's structure-aware rename tools, filled as bullets into the rename_tools marker;
// it renders "none" when empty.
// patternChanges is the task's declared PATTERN changes as rendered bullet lines;
// it is passed to the pattern directive, and nil renders none.
// cardGates is the caller-rendered per-card gate step list, and batchGate the caller-rendered batch gate call.
// parentName is the told parent agent name; an empty one renders parentdirective's no-parent variant.
// failureDigest is the prior failed record's reasons and suspect paths, or "" when the batch was not failed;
// it fills the optional failure_digest marker, rendered as "none" when empty.
// Being optional, the marker leaves an older deployed stencil rendering.
// uncommittedPaths is the caller-rendered block of the worktree's uncommitted paths grouped by who wrote them;
// it fills the optional uncommitted_paths marker, rendered as "none" when empty.
// cardFixer says a failed card gate call goes through a cold fixer first;
// it renders the card-gate fixer paragraph after the card gates, and false leaves the text unchanged.
func RenderRecoveryPrompt(batch batcher.Batch, cardGates, batchGate, prevDigest, failureDigest, uncommittedPaths, reportPath, repoRoot, planDir, promptWorktreeRoot, stencilsDir, specsDir, gateTiers string, renameTools, patternChanges []string, selfFixCap int, notePath, parentName string, cardFixer bool) ([]byte, error) {
	digestLine := prevDigest
	if strings.TrimSpace(digestLine) == "" {
		digestLine = noPrecedingBatchDigest
	}
	failureLine := failureDigest
	if strings.TrimSpace(failureLine) == "" {
		failureLine = "none"
	}
	uncommittedLine := uncommittedPaths
	if strings.TrimSpace(uncommittedLine) == "" {
		uncommittedLine = "none"
	}

	directive, err := pattern.Directive(repoRoot, stencilsDir, pattern.RoleImplementer, patternChanges)
	if err != nil {
		return nil, fmt.Errorf("webster: recovery prompt directive: %w", err)
	}

	parentDirective, err := parentdirective.Directive(stencilsDir, parentName, false)
	if err != nil {
		return nil, fmt.Errorf("webster: recovery prompt parent directive: %w", err)
	}

	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("webster: recovery prompt edit directive: %w", err)
	}

	frictionDirective, err := friction.Directive(notePath, stencilsDir, friction.RoleImplementer)
	if err != nil {
		logger.Warn("webster: friction directive failed, continuing without one", "role", "implementer", "stencil", "webster-prefix-recovery", "error", err)
		frictionDirective = ""
	}

	cardFixerText, err := cardGateFixerParagraph(stencilsDir, cardFixer)
	if err != nil {
		return nil, err
	}

	values := map[string]string{
		"card_pointers":     renderCardPointers(batch.Cards, masterPlanDirDisplay(promptWorktreeRoot, planDir)),
		"card_gates":        cardGates,
		"batch_gate":        batchGate,
		"report_path":       reportPath,
		"self_fix_cap":      fmt.Sprintf("%d", selfFixCap),
		"worktree_root":     promptWorktreeRoot,
		"prev_digest":       digestLine,
		cardGateFixerMarker: cardFixerText,
		"failure_digest":    failureLine,
		"uncommitted_paths": uncommittedLine,
		"pattern_directive": directive,
		"specs_dir":         specsDir,
		"gate_tiers":        gateTiersOrNone(gateTiers),
		"rename_tools":      languageconfig.BulletListValue(renameTools),
		friction.MarkerName: frictionDirective,

		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
	}
	template, err := composeRecoveryTemplate(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("webster: read recovery template: %w", err)
	}
	friction.WarnIfMarkerAbsent(template, "webster-prefix-recovery", frictionDirective)
	prompt, err := stencil.FillOptional(template, values, []string{"pattern_directive", "failure_digest", "uncommitted_paths", cardGateFixerMarker, friction.MarkerName, parentdirective.MarkerName})
	if err != nil {
		return nil, fmt.Errorf("webster: fill recovery template: %w", err)
	}
	return prompt, nil
}

// composeVerifyFixTemplate returns a verify-gate fixer prompt template:
// the prefix stencil named prefix joined ahead of webster-body-verify-fix, both read from stencilsDir.
func composeVerifyFixTemplate(stencilsDir, prefix string) ([]byte, error) {
	prefixText, err := stencilstore.Read(stencilsDir, prefix)
	if err != nil {
		return nil, err
	}
	body, err := stencilstore.Read(stencilsDir, verifyFixBodyStencil)
	if err != nil {
		return nil, err
	}
	return joinTemplateAssets(prefixText, body), nil
}

// verifyFixPromptFileName is the verify-gate fixer fork's prompt file name inside a webster prompts dir.
const verifyFixPromptFileName = "verify-fix.md"

// verifyFixValues returns the shared verify-fix body's marker values, with the friction directive for notePath.
// It returns an error when reportPath or fixGate is empty.
// A friction.Directive error is logged as a warning naming stencilName rather than returned.
func verifyFixValues(reportPath, worktreeRoot, planDir, stencilsDir, outcomePath, summaryPath, notePath, fixGate, stencilName string) (map[string]string, error) {
	if strings.TrimSpace(reportPath) == "" {
		return nil, fmt.Errorf("webster: render %s prompt: report path is empty", stencilName)
	}
	if strings.TrimSpace(fixGate) == "" {
		return nil, fmt.Errorf("webster: render %s prompt: fix gate call is empty", stencilName)
	}

	directive, err := friction.Directive(notePath, stencilsDir, friction.RoleImplementer)
	if err != nil {
		logger.Warn("webster: friction directive failed, continuing without one", "role", "implementer", "stencil", stencilName, "error", err)
		directive = ""
	}

	return map[string]string{
		"report_path":       reportPath,
		"worktree_root":     worktreeRoot,
		"plan_dir":          masterPlanDirDisplay(worktreeRoot, planDir),
		"outcome_path":      outcomePath,
		"summary_path":      summaryPath,
		"fix_gate":          fixGate,
		friction.MarkerName: directive,
	}, nil
}

// RenderVerifyFixPrompt fills the verify-gate fixer fork's prompt, read from stencilsDir.
// It joins webster-prefix-verify-fix-fork ahead of webster-body-verify-fix.
// reportPath is the verify-gate report the fork reads.
// outcomePath and summaryPath are Merriam's two contract files, which the fixer is told never to write.
// The fork never names a record path.
// fixGate is the rendered gate call the fixer runs before it reports, filled into the body's required fix_gate marker.
// Returns an error if reportPath or fixGate is empty.
// planDir is rendered relative to worktreeRoot when it sits inside it, and absolute otherwise.
// notePath is the caller-composed friction note path (friction.NotePath), or "" when Tier 2 is off;
// friction_directive is injected via friction.RoleImplementer when Tier 2 is on;
// a friction.Directive error is logged as a warning rather than returned.
func RenderVerifyFixPrompt(reportPath, worktreeRoot, planDir, stencilsDir, outcomePath, summaryPath, notePath, fixGate string) ([]byte, error) {
	const prefix = "webster-prefix-verify-fix-fork"
	values, err := verifyFixValues(reportPath, worktreeRoot, planDir, stencilsDir, outcomePath, summaryPath, notePath, fixGate, prefix)
	if err != nil {
		return nil, err
	}
	template, err := composeVerifyFixTemplate(stencilsDir, prefix)
	if err != nil {
		return nil, fmt.Errorf("webster: read verify-fix template: %w", err)
	}
	friction.WarnIfMarkerAbsent(template, prefix, values[friction.MarkerName])
	prompt, err := stencil.FillOptional(template, values, []string{friction.MarkerName})
	if err != nil {
		return nil, fmt.Errorf("webster: fill verify-fix template: %w", err)
	}
	return prompt, nil
}

// RenderPlanFixPrompt fills webster-template-plan-fix for the cold plan fixer strand, read from stencilsDir.
// recordPath is the plan-conflict record the fixer reads, and replyPath the reply file it writes as its final action;
// both must be non-empty.
// planDir is rendered relative to worktreeRoot when it sits inside it, and absolute otherwise;
// specsDir points the fixer at the plan spec.
// parentName renders the parent directive, an empty one its no-parent variant.
// notePath is the caller-composed friction note path (friction.NotePath), or "" when Tier 2 is off;
// friction_directive is injected via friction.RoleImplementer when Tier 2 is on;
// a friction.Directive error is logged as a warning rather than returned.
func RenderPlanFixPrompt(recordPath, replyPath, worktreeRoot, planDir, stencilsDir, specsDir, notePath, parentName string) ([]byte, error) {
	const stencilName = "webster-template-plan-fix"
	if strings.TrimSpace(recordPath) == "" || strings.TrimSpace(replyPath) == "" {
		return nil, fmt.Errorf("webster: render %s prompt: record path %q or reply path %q is empty", stencilName, recordPath, replyPath)
	}

	frictionDirective, err := friction.Directive(notePath, stencilsDir, friction.RoleImplementer)
	if err != nil {
		logger.Warn("webster: friction directive failed, continuing without one", "role", "implementer", "stencil", stencilName, "error", err)
		frictionDirective = ""
	}
	parentDirective, err := parentdirective.Directive(stencilsDir, parentName, false)
	if err != nil {
		return nil, fmt.Errorf("webster: plan fixer prompt parent directive: %w", err)
	}
	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("webster: plan fixer prompt edit directive: %w", err)
	}
	values := map[string]string{
		"record_path":              recordPath,
		"reply_path":               replyPath,
		"worktree_root":            worktreeRoot,
		"plan_dir":                 masterPlanDirDisplay(worktreeRoot, planDir),
		"specs_dir":                specsDir,
		friction.MarkerName:        frictionDirective,
		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
	}

	template, err := stencilstore.Read(stencilsDir, stencilName)
	if err != nil {
		return nil, fmt.Errorf("webster: read plan fixer template: %w", err)
	}
	friction.WarnIfMarkerAbsent(template, stencilName, frictionDirective)
	prompt, err := stencil.FillOptional(template, values, []string{friction.MarkerName, parentdirective.MarkerName})
	if err != nil {
		return nil, fmt.Errorf("webster: fill plan fixer template: %w", err)
	}
	return prompt, nil
}

// masterPlanDirDisplay returns the plan-directory spelling the rendered prompts carry:
// planDir relative to paneCwd when it sits inside it, and the absolute planDir otherwise.
// Hub geometry runs its pane at the anchor the plan's `_lym/plan` junction hangs off,
// so the byte-exact "_lym/plan" keeps the prompt's own never-reference-_lym-by-another-path rule self-consistent.
// Standalone geometry keeps its plan in the derived state directory, out of reach of any relative spelling.
// The base is the pane's cwd (Geometry.PaneCwd), where every session runs, never AnchorRoot:
// in standalone geometry AnchorRoot is the state directory containing the plan,
// so an anchor-based split would render an unreachable relative spelling.
func masterPlanDirDisplay(paneCwd, planDir string) string {
	if rel, err := filepath.Rel(paneCwd, planDir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return planDir
}

// RenderMasterPrompt fills webster-template-master for one `lym webster run` invocation.
// It reads the template from the caller-supplied stencilsDir.
// batches is the execution order, the batchifier's own order; nothing in this function reorders it.
// It fills no {{.worktree_root}} key.
// repoRoot, the repository's worktree root holding PATTERN.md, feeds pattern.Directive alone;
// worktreeRoot, the pane's own cwd, feeds only the plan directory's display spelling.
// planDir is the told plan directory (Geometry.PlanDir).
// It is rendered so a standalone Master can find what the prompt tells it to read;
// its plan lives in the derived state directory, not at the pane's own `_lym/plan`.
// verifyFixPromptPath is the Go-rendered verify-gate fixer fork prompt file Merriam forwards to its one fixer fork.
// pattern_directive is injected via pattern.RoleOrchestrator if PATTERN is active.
// Master never edits code, only forks.
// patternChanges is the task's declared PATTERN changes as rendered bullet lines;
// it is passed to the pattern directive, and nil renders none.
// parentDecisions is every decision the run's parent recorded after the Discussion, each entry's text verbatim;
// it renders into {{.parent_decisions}}, so a fresh Master knows the answers its parent already gave;
// nil renders `none`.
// parentName is the told parent agent name; an empty one renders parentdirective's no-parent variant.
// notePath is the caller-composed friction note path (friction.NotePath), or "" when Tier 2 is off;
// friction_directive is injected via friction.RoleOrchestrator when Tier 2 is on;
// a friction.Directive error is logged as a warning rather than returned.
func RenderMasterPrompt(batches []batcher.Batch, st *State, outcomePath, summaryPath, verifyFixPromptPath, planDir string, selfFixCap int, worktreeRoot, repoRoot, stencilsDir string, patternChanges, parentDecisions []string, notePath, parentName string) ([]byte, error) {
	directive, err := pattern.Directive(repoRoot, stencilsDir, pattern.RoleOrchestrator, patternChanges)
	if err != nil {
		return nil, fmt.Errorf("webster: master prompt directive: %w", err)
	}

	parentDirective, err := parentdirective.Directive(stencilsDir, parentName, false)
	if err != nil {
		return nil, fmt.Errorf("webster: master prompt parent directive: %w", err)
	}

	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("webster: master prompt edit directive: %w", err)
	}

	frictionDirective, err := friction.Directive(notePath, stencilsDir, friction.RoleOrchestrator)
	if err != nil {
		logger.Warn("webster: friction directive failed, continuing without one", "role", "orchestrator", "stencil", "webster-template-master", "error", err)
		frictionDirective = ""
	}

	values := map[string]string{
		"batch_index":            RenderBatchIndex(batches),
		"progress":               RenderProgress(batches, st),
		"remaining":              RenderRemaining(batches, st),
		"outcome_path":           outcomePath,
		"summary_path":           summaryPath,
		"verify_fix_prompt_path": verifyFixPromptPath,
		"plan_dir":               masterPlanDirDisplay(worktreeRoot, planDir),
		"self_fix_cap":           fmt.Sprintf("%d", selfFixCap),
		"parent_decisions":       renderParentDecisions(parentDecisions),
		"pattern_directive":      directive,
		friction.MarkerName:      frictionDirective,

		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
	}
	template, err := MasterTemplate(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("webster: read master template: %w", err)
	}
	friction.WarnIfMarkerAbsent(template, "webster-template-master", frictionDirective)
	prompt, err := stencil.FillOptional(template, values, []string{"pattern_directive", friction.MarkerName, parentdirective.MarkerName})
	if err != nil {
		return nil, fmt.Errorf("webster: fill master template: %w", err)
	}
	return prompt, nil
}

// renderParentDecisions renders the decisions for {{.parent_decisions}}:
// the entries in order, joined by a blank line, or `none` when there are none.
func renderParentDecisions(decisions []string) string {
	if len(decisions) == 0 {
		return "none"
	}
	return strings.Join(decisions, "\n\n")
}

// RenderBatchIndex renders batches as ordered-list text for {{.batch_index}}, one line per batch.
// The lines keep the slice's order, which the caller sequences and which is the execution order.
// The number on each line is the batch's identity, which the verbs key on, not its position in the list.
// Each line reads "%02d — %s — %s":
// the number and slug from batchIdentity(b), then the Summary of every card in the batch joined with "; ".
// A batch with no cards is skipped rather than emitting a "00 —" line.
func RenderBatchIndex(batches []batcher.Batch) string {
	lines := make([]string, 0, len(batches))
	for _, b := range batches {
		if len(b.Cards) == 0 {
			continue
		}
		number, slug := batchIdentity(b)
		summaries := make([]string, len(b.Cards))
		for i, c := range b.Cards {
			summaries[i] = c.Summary
		}
		lines = append(lines, fmt.Sprintf("%02d — %s — %s", number, slug, strings.Join(summaries, "; ")))
	}
	return strings.Join(lines, "\n")
}

// RenderProgress renders {{.progress}}'s per-batch state summary for resume.
// It walks batches in the sequenced execution order the caller hands it:
// each one is looked up in st's persisted BatchState entries by its own identity.
// Returns "none" when no batch has reached terminal state or waits on its parent.
// A waiting batch's line names its need and the verb that resumes it.
// st may be nil (as-yet-uninitialized run).
// Only the numbers of the walked batches are looked up, and each is positive,
// so a legacy -1 record in st.Batches never surfaces here.
func RenderProgress(batches []batcher.Batch, st *State) string {
	if st == nil {
		return "none"
	}

	var lines []string
	for _, b := range batches {
		number, slug := batchIdentity(b)
		// The nil-value check is not redundant with ok: a state.json carrying an explicit null for a
		// batch key parses to a present-but-nil entry.
		// Every other reader of this map already guards it.
		// Without the check, the nil entry panics while the Master prompt renders.
		// That takes the run down instead of surfacing a diagnosable error.
		bs, ok := st.Batches[number]
		if waitsOnParent(bs) {
			lines = append(lines, waitingProgressLine(bs, number, slug))
			continue
		}
		if !ok || bs == nil || !bs.Terminal {
			continue
		}
		line := fmt.Sprintf("%02d-%s: %s", number, slug, bs.Status)
		if bs.Digest != nil && len(bs.Digest.Departures) > 0 {
			line += fmt.Sprintf(" (pattern departures: %s)", strings.Join(bs.Digest.Departures, "; "))
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "none"
	}
	return strings.Join(lines, "\n")
}

// RenderRemaining renders {{.remaining}}: the batches with no terminal record yet, in execution
// order, as one comma-separated list of "NN-slug", or "none" when every batch is terminal.
// It serves crash-resume within one generation.
// Without it, a resumed run shows Master a long trail of done batches.
// The unfinished ones would then have to be spotted in the index alone.
func RenderRemaining(batches []batcher.Batch, st *State) string {
	var names []string
	for _, b := range batches {
		number, slug := batchIdentity(b)
		if len(b.Cards) == 0 {
			continue
		}
		if st != nil {
			if bs, ok := st.Batches[number]; ok && bs != nil && bs.Terminal {
				continue
			}
		}
		names = append(names, fmt.Sprintf("%02d-%s", number, slug))
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// gateTiersOrNone returns gateTiers, or "none" when it is empty.
// The required gate_tiers marker refuses an empty value.
func gateTiersOrNone(gateTiers string) string {
	if gateTiers == "" {
		return "none"
	}
	return gateTiers
}
