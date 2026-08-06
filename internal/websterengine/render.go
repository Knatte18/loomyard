// render.go implements the four embedded prompt-template assets (fork-prefix.md, recovery-prefix.md, implementer-body.md, master-template.md, integration-template.md) and the rendering functions that fill them: RenderForkPrompt (called by begin-batch immediately before each in-session fork), RenderRecoveryPrompt (called by recover-batch immediately before spawning the separate cold recovery strand), RenderMasterPrompt (called by run at Master's own spawn), and RenderIntegrationPrompt (called for the plan's single dedicated integration-suite fork, when ShouldRunIntegration reports true), plus the two batch-list/progress renderers those prompts embed (RenderBatchIndex, RenderProgress).
// The go:embed directives and their accessors live here rather than in template.go, which stays config-only — mirroring builderengine's own split between template.go's ConfigTemplate/ImplementerTemplate/OrchestratorTemplate accessors and this package's own render-time logic.
//
// Per the fork-context-hygiene Shared Decision, RenderForkPrompt's output feeds two callers with opposite context situations — beginbatch.go's in-session fork (which already inherits Master's whole context) and recoverbatch.go's cold recovery strand (a separate process that inherits nothing) — so one prompt cannot honestly serve both.
// This file instead composes each of the two prompts, at render time, from one shared implementer-job body (implementer-body.md) plus a caller-specific prefix (fork-prefix.md or recovery-prefix.md): joinTemplateAssets concatenates the raw template bytes before either is handed to stencil.Fill/ FillOptional, since internal/stencil has no {{template}} include mechanism of its own (see stencil.go's own doc comment).
// Card content reaches both prompts as a worktree-relative card-file pointer (planparser.Card.SourcePath, `_lyx/plan/NN-<slug>.md`) rendered verbatim by renderCardPointers, never as inlined What/file-op fields — the implementer reads its own card file in its own turn instead of trusting a Go-rendered paraphrase of it.

package websterengine

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencil"
)

//go:embed master-template.md
var masterTemplate []byte

// MasterTemplate returns the embedded Master-session prompt template's raw bytes.
func MasterTemplate() []byte {
	return masterTemplate
}

//go:embed fork-prefix.md
var forkPrefix []byte

//go:embed recovery-prefix.md
var recoveryPrefix []byte

//go:embed implementer-body.md
var implementerBody []byte

// joinTemplateAssets concatenates prefix and body's raw template bytes
// separated by a blank line. stencil.stripLeadingComment removes the leading banner.
func joinTemplateAssets(prefix, body []byte) []byte {
	joined := append([]byte{}, prefix...)
	joined = append(joined, []byte("\n\n")...)
	joined = append(joined, body...)
	return joined
}

// composeForkTemplate returns the thin in-session fork prompt: fork-prefix.md
// joined ahead of implementer-body.md.
func composeForkTemplate() []byte {
	return joinTemplateAssets(forkPrefix, implementerBody)
}

// composeRecoveryTemplate returns the full cold-start recovery prompt:
// recovery-prefix.md joined ahead of implementer-body.md.
func composeRecoveryTemplate() []byte {
	return joinTemplateAssets(recoveryPrefix, implementerBody)
}

// ForkTemplate returns the composed thin in-session fork prompt template.
func ForkTemplate() []byte {
	return composeForkTemplate()
}

// RecoveryTemplate returns the composed full cold-start recovery prompt template.
func RecoveryTemplate() []byte {
	return composeRecoveryTemplate()
}

// ImplementerBodyTemplate returns implementer-body.md's raw bytes — the shared job body both ForkTemplate and RecoveryTemplate compose with their own prefix.
func ImplementerBodyTemplate() []byte {
	return implementerBody
}

//go:embed integration-template.md
var integrationTemplate []byte

// IntegrationTemplate returns the embedded integration-suite fork prompt template's raw bytes.
func IntegrationTemplate() []byte {
	return integrationTemplate
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
	prompt, err := stencil.Fill(composeForkTemplate(), values)
	if err != nil {
		return nil, fmt.Errorf("webster: fill fork template: %w", err)
	}
	return prompt, nil
}

// RenderRecoveryPrompt fills RecoveryTemplate for one batch's cold-start recovery strand.
// Unlike RenderForkPrompt, the recovery strand inherits nothing, so its prompt orients from plan/overview.md and CONSTRAINTS.md before the shared implementer-job body runs.
// pattern_directive is injected if PATTERN is active.
func RenderRecoveryPrompt(batch batcher.Batch, prevDigest, reportPath string, l *lyxcwd.Location, selfFixCap int) ([]byte, error) {
	digestLine := prevDigest
	if strings.TrimSpace(digestLine) == "" {
		digestLine = noPrecedingBatchDigest
	}

	values := map[string]string{
		"card_pointers":     renderCardPointers(batch.Cards),
		"report_path":       reportPath,
		"self_fix_cap":      fmt.Sprintf("%d", selfFixCap),
		"worktree_root":     l.AnchorPath(),
		"prev_digest":       digestLine,
		"pattern_directive": pattern.Directive(l, pattern.RoleImplementer),
	}
	prompt, err := stencil.FillOptional(composeRecoveryTemplate(), values, []string{"pattern_directive"})
	if err != nil {
		return nil, fmt.Errorf("webster: fill recovery template: %w", err)
	}
	return prompt, nil
}

// RenderIntegrationPrompt fills integration-template.md for the plan's single integration-suite fork.
// Returns an error if plan.Verify is empty.
func RenderIntegrationPrompt(plan *planparser.Plan, reportPath, worktreeRoot string) ([]byte, error) {
	verify := strings.TrimSpace(plan.Verify)
	if verify == "" {
		return nil, fmt.Errorf("webster: render integration prompt: plan carries no plan-level \"## verify:\" section")
	}

	values := map[string]string{
		"verify":        verify,
		"report_path":   reportPath,
		"worktree_root": worktreeRoot,
	}
	prompt, err := stencil.Fill(IntegrationTemplate(), values)
	if err != nil {
		return nil, fmt.Errorf("webster: fill integration template: %w", err)
	}
	return prompt, nil
}

// noIntegrationPromptPath is the sentinel RenderMasterPrompt renders when no integration prompt file.
const noIntegrationPromptPath = "none (this plan has no \"## verify:\" section)"

// RenderMasterPrompt fills master-template.md for one `lyx webster run` invocation.
// pattern_directive is injected via pattern.RoleOrchestrator if PATTERN is active (Master never edits code, only forks).
func RenderMasterPrompt(plan *planparser.Plan, st *State, outcomePath, summaryPath, integrationPromptPath string, selfFixCap, pollWaitS int, l *lyxcwd.Location) ([]byte, error) {
	integrationPrompt := strings.TrimSpace(integrationPromptPath)
	if integrationPrompt == "" {
		integrationPrompt = noIntegrationPromptPath
	}

	values := map[string]string{
		"batch_index":             RenderBatchIndex(plan),
		"progress":                RenderProgress(plan, st),
		"outcome_path":            outcomePath,
		"summary_path":            summaryPath,
		"integration_prompt_path": integrationPrompt,
		"self_fix_cap":            fmt.Sprintf("%d", selfFixCap),
		"poll_wait_s":             fmt.Sprintf("%d", pollWaitS),
		"pattern_directive":       pattern.Directive(l, pattern.RoleOrchestrator),
	}
	prompt, err := stencil.FillOptional(MasterTemplate(), values, []string{"pattern_directive"})
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

// RenderProgress renders {{.progress}}'s per-batch state summary for resume, built from st's persisted BatchState entries.
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
