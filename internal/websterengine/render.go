// render.go implements the producer prompt assets webster composes and renders (webster-prefix-fork.md, webster-prefix-recovery.md, webster-body-implementer.md, webster-template-master.md, webster-prefix-verify-fix-fork.md, webster-prefix-verify-fix-cold.md, webster-body-verify-fix.md),
// and the rendering functions that fill them.
// RenderForkPrompt is called by begin-batch immediately before each in-session fork, RenderRecoveryPrompt by recover-batch immediately before spawning the separate cold recovery strand, and RenderMasterPrompt by run at Master's own spawn.
// RenderVerifyFixPrompt is called by run for the verify-gate fixer fork Merriam spawns on a gate failure, and RenderVerifyFixColdPrompt by fix-verify for the cold verify-gate fixer strand.
// The file also holds the two sequenced-execution-order renderers those prompts embed (RenderBatchIndex, RenderProgress).
// Every asset ships as an embedded default in the top-level stencils package and is read from a
// told stencils directory at call time via stencilstore.Read, per the runtime-read-not-embed Shared
// Decision — this file carries no //go:embed directive of its own.
//
// Per the fork-context-hygiene Shared Decision, RenderForkPrompt's output feeds two callers with
// opposite context situations — beginbatch.go's in-session fork (which already inherits Master's
// whole context) and recoverbatch.go's cold recovery strand (a separate process that inherits
// nothing) — so one prompt cannot honestly serve both.
// This file instead composes each of the two prompts, at render time, from one shared
// implementer-job body (webster-body-implementer) plus a caller-specific prefix
// (webster-prefix-fork or webster-prefix-recovery): joinTemplateAssets strips each asset's leading
// banner via stencil.StripLeadingComment, then concatenates the stripped bodies before either is
// handed to stencil.Fill/FillOptional, since internal/stencil has no {{template}} include mechanism
// of its own (see stencil.go's own doc comment).
// Stripping every asset's banner, not just the joined result's first one, matters once a seeded
// stencil carries a `lyx-stencil:` stamp: stencil.Fill strips only one leading banner from the
// joined text, so an unstripped second asset's banner would otherwise reach the LLM verbatim as if
// it were instruction text.
// The two verify-gate fixer prompts are composed the same way, from webster-body-verify-fix plus webster-prefix-verify-fix-fork for the in-session fixer fork or webster-prefix-verify-fix-cold for the cold fixer strand.
// Card content reaches both prompts as a worktree-relative card-file pointer
// (planparser.Card.SourcePath, `_lyx/plan/NN-<slug>.md`) rendered verbatim by renderCardPointers,
// never as inlined What/file-op fields — the implementer reads its own card file in its own turn
// instead of trusting a Go-rendered paraphrase of it.
//
// Each of the Render*Prompt functions also takes a notePath parameter (the composed friction note path from friction.NotePath, or "" when Tier 2 is off) and injects {{.friction_directive}} into its own values map.
// RenderForkPrompt, RenderRecoveryPrompt, RenderVerifyFixPrompt and RenderVerifyFixColdPrompt resolve friction.RoleImplementer;
// RenderMasterPrompt resolves friction.RoleOrchestrator, since Master never edits code and only forks.
// A non-nil friction.Directive error is swallowed — logged at Warn via internal/logger, never wrapped and returned — exactly as if Tier 2 were off, per the composer-swallows-a-friction-error Shared Decision;
// this is deliberately unlike the pattern.Directive error handling sitting beside it in RenderRecoveryPrompt, RenderVerifyFixColdPrompt and RenderMasterPrompt, which does propagate.
//
// RenderForkPrompt and RenderRecoveryPrompt each also take a specsDir parameter, immediately after
// their stencilsDir parameter, filled into the shared implementer-job body's required specs_dir
// marker. It is required, not optional: a blank render would silently reproduce a dead
// cross-repository reference in the agent's own instructions.
// Both also take a gateTiers parameter, immediately after specsDir, filled into the same body's required gate_tiers marker.
// RenderMasterPrompt and the two verify-fix renderers gain no such parameter, since neither the Master template nor the verify-fix body cites a normative spec.

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

// MasterTemplate reads webster-template-master's current content from stencilsDir via
// stencilstore.Read.
func MasterTemplate(stencilsDir string) ([]byte, error) {
	return stencilstore.Read(stencilsDir, "webster-template-master")
}

// ImplementerBodyTemplate reads webster-body-implementer's current content from stencilsDir via
// stencilstore.Read — the shared job body both ForkTemplate and RecoveryTemplate compose with
// their own prefix.
func ImplementerBodyTemplate(stencilsDir string) ([]byte, error) {
	return stencilstore.Read(stencilsDir, "webster-body-implementer")
}

// joinTemplateAssets concatenates prefix and body's raw template bytes separated by a blank line,
// after stripping each asset's own leading banner comment via stencil.StripLeadingComment.
// Stripping both, not relying on stencil.Fill to strip only the joined result's first banner, is
// load-bearing: once a seeded stencil carries a `lyx-stencil:` stamp in its banner, Fill would
// otherwise deliver the second asset's stamp into the composed prompt as if it were instruction
// text.
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

// RecoveryTemplate returns the composed full cold-start recovery prompt template, read from
// stencilsDir.
func RecoveryTemplate(stencilsDir string) ([]byte, error) {
	return composeRecoveryTemplate(stencilsDir)
}

// noPrecedingBatchDigest is the literal sentinel RenderForkPrompt and
// RenderRecoveryPrompt render into {{.prev_digest}} when prevDigest is
// empty: the first executed batch has no preceding batch, and a
// crash-resumed run re-driving that batch fresh carries no digest either.
// Never a blank field — an empty {{.prev_digest}} would violate stencil's
// required-top-level-marker guarantee.
const noPrecedingBatchDigest = "none (first batch)"

// renderCardPointers renders one `- <planDirDisplay>/<card file name>` bullet per card in declared
// order: each card's SourcePath contributes its file name, re-rooted onto the told plan-directory
// display. Under hub geometry the display is "_lyx/plan" and the bullets render byte-identically
// to the old verbatim-SourcePath form; under standalone geometry the display is the derived state
// directory's absolute plan dir — the only spelling a fork running at the target repo can actually
// read (crucible round fable5-high-r3, F-A4).
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

// RenderForkPrompt fills ForkTemplate for one execution batch's in-session fork, read from
// stencilsDir.
// Cards' pointers are rendered re-rooted onto planDir's display form (see renderCardPointers) —
// promptWorktreeRoot, the pane's own cwd, is the display's relative-spelling base;
// {{.worktree_root}} is filled from the same caller-supplied promptWorktreeRoot.
// prevDigest is already rendered as a one-line summary by the caller.
// notePath is the caller-composed friction note path (friction.NotePath), or "" when Tier 2 is off;
// friction_directive is injected via friction.RoleImplementer when Tier 2 is on, with a
// friction.Directive error swallowed as a Warn rather than propagated.
// specsDir is the told deployed-specs directory, filled into the shared implementer-job body's
// required specs_dir marker.
// gateTiers is the hub's rendered tier and gate-list lines, filled verbatim into the body's required gate_tiers marker; "none" when no tier is declared.
// renameTools is the hub's structure-aware rename tools, filled into the body's required rename_tools marker as bullets, or as "none" when empty.
// cardGates is the caller-rendered per-card gate step list (renderCardGates), filled into the body's required card_gates marker.
// batchGate is the caller-rendered batch gate call (renderBatchGate), filled into the body's required batch_gate marker.
// outcomePath and summaryPath are Merriam's two contract files, filled into the fork prefix's outcome_path and summary_path markers so the fork is told never to write them.
// The plan is not a parameter, per the fork-context-hygiene rule.
func RenderForkPrompt(batch batcher.Batch, cardGates, batchGate, prevDigest, reportPath, planDir, promptWorktreeRoot, stencilsDir, specsDir, gateTiers string, renameTools []string, outcomePath, summaryPath string, selfFixCap int, notePath string) ([]byte, error) {
	digestLine := prevDigest
	if strings.TrimSpace(digestLine) == "" {
		digestLine = noPrecedingBatchDigest
	}

	directive, err := friction.Directive(notePath, stencilsDir, friction.RoleImplementer)
	if err != nil {
		logger.Warn("webster: friction directive failed, continuing without one", "role", "implementer", "stencil", "webster-prefix-fork", "error", err)
		directive = ""
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
	}
	template, err := composeForkTemplate(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("webster: read fork template: %w", err)
	}
	friction.WarnIfMarkerAbsent(template, "webster-prefix-fork", directive)
	prompt, err := stencil.FillOptional(template, values, []string{friction.MarkerName})
	if err != nil {
		return nil, fmt.Errorf("webster: fill fork template: %w", err)
	}
	return prompt, nil
}

// RenderRecoveryPrompt fills RecoveryTemplate for one batch's cold-start recovery strand, read from
// stencilsDir.
// Unlike RenderForkPrompt, the recovery strand inherits nothing, so its prompt orients from
// plan/overview.md and any PATTERN entries in its prompt before the shared implementer-job body runs.
// {{.worktree_root}} is filled from the caller-supplied promptWorktreeRoot, which — as the pane's
// own cwd — is also the card pointers' relative-spelling base (see renderCardPointers);
// repoRoot, the repository's worktree root holding PATTERN.md, feeds pattern.Directive alone.
// pattern_directive is injected if PATTERN is active.
// notePath is the caller-composed friction note path (friction.NotePath), or "" when Tier 2 is off;
// friction_directive is injected via friction.RoleImplementer when Tier 2 is on, with a
// friction.Directive error swallowed as a Warn rather than propagated — deliberately unlike the
// pattern.Directive call immediately above, which does propagate.
// specsDir is the told deployed-specs directory, filled into the shared implementer-job body's
// required specs_dir marker.
// gateTiers is the hub's rendered tier and gate-list lines, filled verbatim into the body's required gate_tiers marker; "none" when no tier is declared.
// renameTools is the hub's structure-aware rename tools, filled into the body's required rename_tools marker as bullets, or as "none" when empty.
// patternChanges is the task's declared PATTERN changes as rendered bullet lines, passed to the pattern directive; nil renders none.
// cardGates is the caller-rendered per-card gate step list, and batchGate the caller-rendered batch gate call.
// parentName is the told parent agent name; an empty one renders parentdirective's no-parent variant.
// failureDigest is the prior failed record's reasons and suspect paths, or "" when the batch was not failed;
// it fills the optional failure_digest marker, rendered as "none" when empty, and being optional it leaves an older deployed stencil rendering.
// uncommittedPaths is the caller-rendered block of the worktree's uncommitted paths grouped by who wrote them, or "" for a clean tree;
// it fills the optional uncommitted_paths marker the same way.
func RenderRecoveryPrompt(batch batcher.Batch, cardGates, batchGate, prevDigest, failureDigest, uncommittedPaths, reportPath, repoRoot, planDir, promptWorktreeRoot, stencilsDir, specsDir, gateTiers string, renameTools, patternChanges []string, selfFixCap int, notePath, parentName string) ([]byte, error) {
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

	values := map[string]string{
		"card_pointers":     renderCardPointers(batch.Cards, masterPlanDirDisplay(promptWorktreeRoot, planDir)),
		"card_gates":        cardGates,
		"batch_gate":        batchGate,
		"report_path":       reportPath,
		"self_fix_cap":      fmt.Sprintf("%d", selfFixCap),
		"worktree_root":     promptWorktreeRoot,
		"prev_digest":       digestLine,
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
	prompt, err := stencil.FillOptional(template, values, []string{"pattern_directive", "failure_digest", "uncommitted_paths", friction.MarkerName, parentdirective.MarkerName})
	if err != nil {
		return nil, fmt.Errorf("webster: fill recovery template: %w", err)
	}
	return prompt, nil
}

// composeVerifyFixTemplate returns a verify-gate fixer prompt template: the prefix stencil named prefix joined ahead of webster-body-verify-fix, both read from stencilsDir.
func composeVerifyFixTemplate(stencilsDir, prefix string) ([]byte, error) {
	prefixText, err := stencilstore.Read(stencilsDir, prefix)
	if err != nil {
		return nil, err
	}
	body, err := stencilstore.Read(stencilsDir, "webster-body-verify-fix")
	if err != nil {
		return nil, err
	}
	return joinTemplateAssets(prefixText, body), nil
}

// verifyFixPromptFileName is the verify-gate fixer fork's prompt file name inside a webster prompts dir.
const verifyFixPromptFileName = "verify-fix.md"

// verifyFixValues checks reportPath and fixGate are non-empty and returns the shared verify-fix body's marker values, with the friction directive for notePath.
// A friction.Directive error is swallowed as a Warn naming stencilName rather than propagated.
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

// RenderVerifyFixPrompt fills webster-prefix-verify-fix-fork joined ahead of webster-body-verify-fix for the verify-gate fixer fork, read from stencilsDir.
// reportPath is the verify-gate report the fork reads (VerifyGateReportPath).
// outcomePath and summaryPath are Merriam's two contract files, which the fixer is told never to write.
// The fork never names a record path.
// fixGate is the rendered gate call the fixer runs before it reports, filled into the body's required fix_gate marker.
// Returns an error if reportPath or fixGate is empty.
// planDir is rendered in the display form masterPlanDirDisplay gives it, relative to worktreeRoot when it sits inside it.
// notePath is the caller-composed friction note path (friction.NotePath), or "" when Tier 2 is off;
// friction_directive is injected via friction.RoleImplementer when Tier 2 is on, with a friction.Directive error swallowed as a Warn rather than propagated.
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

// RenderVerifyFixColdPrompt fills webster-prefix-verify-fix-cold joined ahead of webster-body-verify-fix for the cold verify-gate fixer strand, read from stencilsDir.
// It takes RenderVerifyFixPrompt's parameters, plus replyPath, the reply file the strand writes as its final action, which must be non-empty.
// repoRoot and patternChanges render the PATTERN directive, whose error is returned;
// parentName renders the parent directive, an empty one its no-parent variant.
func RenderVerifyFixColdPrompt(reportPath, replyPath, worktreeRoot, repoRoot, planDir, stencilsDir, outcomePath, summaryPath, notePath, fixGate string, patternChanges []string, parentName string) ([]byte, error) {
	const prefix = "webster-prefix-verify-fix-cold"
	if strings.TrimSpace(replyPath) == "" {
		return nil, fmt.Errorf("webster: render %s prompt: reply path is empty", prefix)
	}
	values, err := verifyFixValues(reportPath, worktreeRoot, planDir, stencilsDir, outcomePath, summaryPath, notePath, fixGate, prefix)
	if err != nil {
		return nil, err
	}

	patternDirective, err := pattern.Directive(repoRoot, stencilsDir, pattern.RoleImplementer, patternChanges)
	if err != nil {
		return nil, fmt.Errorf("webster: cold fixer prompt directive: %w", err)
	}
	parentDirective, err := parentdirective.Directive(stencilsDir, parentName, false)
	if err != nil {
		return nil, fmt.Errorf("webster: cold fixer prompt parent directive: %w", err)
	}
	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("webster: cold fixer prompt edit directive: %w", err)
	}
	values["reply_path"] = replyPath
	values["pattern_directive"] = patternDirective
	values[parentdirective.MarkerName] = parentDirective
	values[editdirective.MarkerName] = editDirective

	template, err := composeVerifyFixTemplate(stencilsDir, prefix)
	if err != nil {
		return nil, fmt.Errorf("webster: read cold fixer template: %w", err)
	}
	friction.WarnIfMarkerAbsent(template, prefix, values[friction.MarkerName])
	prompt, err := stencil.FillOptional(template, values, []string{"pattern_directive", friction.MarkerName, parentdirective.MarkerName})
	if err != nil {
		return nil, fmt.Errorf("webster: fill cold fixer template: %w", err)
	}
	return prompt, nil
}

// masterPlanDirDisplay returns the plan-directory spelling the rendered prompts carry: planDir
// relative to paneCwd when it sits inside it — hub geometry, whose pane runs at the anchor the
// plan's `_lyx/plan` junction hangs off, so the byte-exact "_lyx/plan" keeps the prompt's own
// never-reference-_lyx-by-another-path rule self-consistent — and the absolute planDir otherwise,
// which is standalone geometry, whose plan lives in the derived state directory and is reachable
// from the pane's cwd by no relative spelling at all.
// The base is the PANE's cwd (Geometry.WorktreeRoot, where every session actually runs), never
// AnchorRoot: standalone's AnchorRoot IS the state directory containing the plan, so an
// anchor-based split rendered the unreachable relative spelling for exactly the mode this function
// exists to fix (proven live in round fable5-high-r3's second E2E attempt).
func masterPlanDirDisplay(paneCwd, planDir string) string {
	if rel, err := filepath.Rel(paneCwd, planDir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return planDir
}

// RenderMasterPrompt fills webster-template-master for one `lyx webster run` invocation, read from
// the caller-supplied stencilsDir.
// batches is the execution order, the batchifier's own order — nothing in this function reorders it.
// It fills no {{.worktree_root}} key at all — repoRoot, the repository's worktree root holding PATTERN.md, feeds pattern.Directive alone, and
// worktreeRoot (the pane's own cwd) feeds only masterPlanDirDisplay's relative-spelling decision.
// planDir is the told plan directory (Geometry.PlanDir), rendered so a standalone Master — whose plan lives in the derived state directory, not at the pane's own `_lyx/plan` — can actually find what the prompt tells it to read (found live in crucible round fable5-high-r3, F-A4).
// verifyFixPromptPath is the Go-rendered verify-gate fixer fork prompt file Merriam forwards to its one fixer fork.
// pattern_directive is injected via pattern.RoleOrchestrator if PATTERN is active (Master never
// edits code, only forks).
// patternChanges is the task's declared PATTERN changes as rendered bullet lines, passed to the pattern directive; nil renders none.
// parentName is the told parent agent name; an empty one renders parentdirective's no-parent variant.
// notePath is the caller-composed friction note path (friction.NotePath), or "" when Tier 2 is off;
// friction_directive is injected via friction.RoleOrchestrator when Tier 2 is on — Master never
// edits code, only forks — with a friction.Directive error swallowed as a Warn rather than
// propagated, deliberately unlike the pattern.Directive call immediately above, which does
// propagate.
func RenderMasterPrompt(batches []batcher.Batch, st *State, outcomePath, summaryPath, verifyFixPromptPath, planDir string, selfFixCap int, worktreeRoot, repoRoot, stencilsDir string, patternChanges []string, notePath, parentName string) ([]byte, error) {
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

// RenderBatchIndex renders batches into ordered-list text for {{.batch_index}}, one line per
// batch in the slice's own order — the caller is responsible for handing it a sequenced slice.
// The rendered order is the execution order; the number on each line is the batch's identity,
// which the verbs key on, not its position in this list.
// Each line follows the "%02d — %s — %s" shape, where the number and slug come from
// batchIdentity(b) and the third field is every card's Summary in that batch joined with "; ".
// Under the identity batchifier a batch holds exactly one card, so the joined form renders
// byte-identically to the flat-card-list rendering this function replaced.
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

// RenderProgress renders {{.progress}}'s per-batch state summary for resume, walking batches in
// the sequenced execution order the caller hands it and looking each one up in st's persisted
// BatchState entries by its own identity.
// Returns "none" when no batch has reached terminal state.
// st may be nil (as-yet-uninitialized run).
// Walking batches rather than cards preserves the property doc.go already relies on: every batch
// number is positive, so a legacy -1 record in State.Batches can never surface here.
func RenderProgress(batches []batcher.Batch, st *State) string {
	if st == nil {
		return "none"
	}

	var lines []string
	for _, b := range batches {
		number, slug := batchIdentity(b)
		// The nil-value check is not redundant with ok: a state.json carrying an explicit null for a
		// batch key parses to a present-but-nil entry, and every other reader of this map
		// (completedCards, verifyEveryBatchDone, accumulatedCardSHAs, reclaimEntryTimeStrands,
		// predecessorDigestLine) already guards it. Without it this line panics INSIDE Run's Master
		// prompt render, taking the run down instead of surfacing a diagnosable error.
		bs, ok := st.Batches[number]
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
// It serves crash-resume within one generation: a resumed run otherwise shows Master a long trail of done batches and leaves the unfinished ones to be spotted in the index alone.
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

// gateTiersOrNone returns gateTiers, or "none" when it is empty, since the required gate_tiers marker refuses an empty value.
func gateTiersOrNone(gateTiers string) string {
	if gateTiers == "" {
		return "none"
	}
	return gateTiers
}
