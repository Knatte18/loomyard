// render.go implements the five producer prompt assets webster composes and renders
// (webster-prefix-fork.md, webster-prefix-recovery.md, webster-body-implementer.md,
// webster-template-master.md, webster-template-integration.md) and the rendering functions that
// fill them: RenderForkPrompt (called by begin-batch immediately before each in-session fork),
// RenderRecoveryPrompt (called by recover-batch immediately before spawning the separate cold
// recovery strand), RenderMasterPrompt (called by run at Master's own spawn), and
// RenderIntegrationPrompt (called for the plan's single dedicated integration-suite fork, when
// ShouldRunIntegration reports true), plus the two batch-list/progress renderers those prompts
// embed (RenderBatchIndex, RenderProgress).
// Every asset ships as an embedded default in the top-level stencils package and is read from the
// hub's stencils directory (fabricengine.StencilsDir) at call time via stencilstore.Read, per the
// runtime-read-not-embed Shared Decision — this file carries no //go:embed directive of its own.
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
// Card content reaches both prompts as a worktree-relative card-file pointer
// (planparser.Card.SourcePath, `_lyx/plan/NN-<slug>.md`) rendered verbatim by renderCardPointers,
// never as inlined What/file-op fields — the implementer reads its own card file in its own turn
// instead of trusting a Go-rendered paraphrase of it.

package websterengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
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

// IntegrationTemplate reads webster-template-integration's current content from stencilsDir via
// stencilstore.Read.
func IntegrationTemplate(stencilsDir string) ([]byte, error) {
	return stencilstore.Read(stencilsDir, "webster-template-integration")
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

// renderCardPointers renders one `- <SourcePath>` bullet per card in declared order.
func renderCardPointers(cards []planparser.Card) string {
	bullets := make([]string, 0, len(cards))
	for _, c := range cards {
		bullets = append(bullets, fmt.Sprintf("- `%s`", c.SourcePath))
	}
	return strings.Join(bullets, "\n")
}

// RenderForkPrompt fills ForkTemplate for one execution batch's in-session fork.
// Cards' SourcePath pointers are rendered verbatim;
// {{.worktree_root}} is filled from l.AnchorPath().
// prevDigest is already rendered as a one-line summary by the caller.
func RenderForkPrompt(batch batcher.Batch, prevDigest, reportPath string, l *lyxcwd.Location, selfFixCap int) ([]byte, error) {
	digestLine := prevDigest
	if strings.TrimSpace(digestLine) == "" {
		digestLine = noPrecedingBatchDigest
	}

	values := map[string]string{
		"card_pointers": renderCardPointers(batch.Cards),
		"report_path":   reportPath,
		"self_fix_cap":  fmt.Sprintf("%d", selfFixCap),
		"worktree_root": l.AnchorPath(),
		"prev_digest":   digestLine,
	}
	template, err := composeForkTemplate(fabricengine.StencilsDir(l.HubPath))
	if err != nil {
		return nil, fmt.Errorf("webster: read fork template: %w", err)
	}
	prompt, err := stencil.Fill(template, values)
	if err != nil {
		return nil, fmt.Errorf("webster: fill fork template: %w", err)
	}
	return prompt, nil
}

// RenderRecoveryPrompt fills RecoveryTemplate for one batch's cold-start recovery strand.
// Unlike RenderForkPrompt, the recovery strand inherits nothing, so its prompt orients from
// plan/overview.md and CONSTRAINTS.md before the shared implementer-job body runs.
// pattern_directive is injected if PATTERN is active.
func RenderRecoveryPrompt(batch batcher.Batch, prevDigest, reportPath string, l *lyxcwd.Location, selfFixCap int) ([]byte, error) {
	digestLine := prevDigest
	if strings.TrimSpace(digestLine) == "" {
		digestLine = noPrecedingBatchDigest
	}

	stencilsDir := fabricengine.StencilsDir(l.HubPath)
	directive, err := pattern.Directive(l.AnchorPath(), stencilsDir, pattern.RoleImplementer)
	if err != nil {
		return nil, fmt.Errorf("webster: recovery prompt directive: %w", err)
	}

	values := map[string]string{
		"card_pointers":     renderCardPointers(batch.Cards),
		"report_path":       reportPath,
		"self_fix_cap":      fmt.Sprintf("%d", selfFixCap),
		"worktree_root":     l.AnchorPath(),
		"prev_digest":       digestLine,
		"pattern_directive": directive,
	}
	template, err := composeRecoveryTemplate(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("webster: read recovery template: %w", err)
	}
	prompt, err := stencil.FillOptional(template, values, []string{"pattern_directive"})
	if err != nil {
		return nil, fmt.Errorf("webster: fill recovery template: %w", err)
	}
	return prompt, nil
}

// RenderIntegrationPrompt fills webster-template-integration for the plan's single
// integration-suite fork, read from stencilsDir.
// Returns an error if plan.Verify is empty.
func RenderIntegrationPrompt(plan *planparser.Plan, reportPath, worktreeRoot, stencilsDir string) ([]byte, error) {
	verify := strings.TrimSpace(plan.Verify)
	if verify == "" {
		return nil, fmt.Errorf("webster: render integration prompt: plan carries no plan-level \"## verify:\" section")
	}

	values := map[string]string{
		"verify":        verify,
		"report_path":   reportPath,
		"worktree_root": worktreeRoot,
	}
	template, err := IntegrationTemplate(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("webster: read integration template: %w", err)
	}
	prompt, err := stencil.Fill(template, values)
	if err != nil {
		return nil, fmt.Errorf("webster: fill integration template: %w", err)
	}
	return prompt, nil
}

// noIntegrationPromptPath is the sentinel RenderMasterPrompt renders when no integration prompt file.
const noIntegrationPromptPath = "none (this plan has no \"## verify:\" section)"

// RenderMasterPrompt fills webster-template-master for one `lyx webster run` invocation, read from
// the hub's stencils directory (fabricengine.StencilsDir(l.HubPath)).
// pattern_directive is injected via pattern.RoleOrchestrator if PATTERN is active (Master never
// edits code, only forks).
func RenderMasterPrompt(plan *planparser.Plan, st *State, outcomePath, summaryPath, integrationPromptPath string, selfFixCap, pollWaitS int, l *lyxcwd.Location) ([]byte, error) {
	integrationPrompt := strings.TrimSpace(integrationPromptPath)
	if integrationPrompt == "" {
		integrationPrompt = noIntegrationPromptPath
	}

	stencilsDir := fabricengine.StencilsDir(l.HubPath)
	directive, err := pattern.Directive(l.AnchorPath(), stencilsDir, pattern.RoleOrchestrator)
	if err != nil {
		return nil, fmt.Errorf("webster: master prompt directive: %w", err)
	}

	values := map[string]string{
		"batch_index":             RenderBatchIndex(plan),
		"progress":                RenderProgress(plan, st),
		"outcome_path":            outcomePath,
		"summary_path":            summaryPath,
		"integration_prompt_path": integrationPrompt,
		"self_fix_cap":            fmt.Sprintf("%d", selfFixCap),
		"poll_wait_s":             fmt.Sprintf("%d", pollWaitS),
		"pattern_directive":       directive,
	}
	template, err := MasterTemplate(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("webster: read master template: %w", err)
	}
	prompt, err := stencil.FillOptional(template, values, []string{"pattern_directive"})
	if err != nil {
		return nil, fmt.Errorf("webster: fill master template: %w", err)
	}
	return prompt, nil
}

// RenderBatchIndex renders plan's flat card list into ordered-list text for {{.batch_index}}.
func RenderBatchIndex(plan *planparser.Plan) string {
	lines := make([]string, 0, len(plan.Cards))
	for _, c := range plan.Cards {
		lines = append(lines, fmt.Sprintf("%02d — %s — %s", c.Number, c.Slug, c.Intent))
	}
	return strings.Join(lines, "\n")
}

// RenderProgress renders {{.progress}}'s per-batch state summary for resume, built from st's
// persisted BatchState entries.
// Returns "none" when no batch has reached terminal state.
// st may be nil (as-yet-uninitialized run).
func RenderProgress(plan *planparser.Plan, st *State) string {
	if st == nil {
		return "none"
	}

	var lines []string
	for _, c := range plan.Cards {
		bs, ok := st.Batches[c.Number]
		if !ok || !bs.Terminal {
			continue
		}
		lines = append(lines, fmt.Sprintf("%02d-%s: %s", c.Number, c.Slug, bs.Status))
	}
	if len(lines) == 0 {
		return "none"
	}
	return strings.Join(lines, "\n")
}
