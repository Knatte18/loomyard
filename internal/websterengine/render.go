// render.go implements the four embedded prompt-template assets
// (fork-prefix.md, recovery-prefix.md, implementer-body.md,
// master-template.md, integration-template.md) and the rendering functions
// that fill them: RenderForkPrompt (called by begin-batch immediately before
// each in-session fork), RenderRecoveryPrompt (called by recover-batch
// immediately before spawning the separate cold recovery strand),
// RenderMasterPrompt (called by run at Master's own spawn), and
// RenderIntegrationPrompt (called for the plan's single dedicated
// integration-suite fork, when ShouldRunIntegration reports true), plus the
// two batch-list/progress renderers those prompts embed (RenderBatchIndex,
// RenderProgress). The go:embed directives and their accessors live here
// rather than in template.go, which stays config-only — mirroring
// builderengine's own split between template.go's
// ConfigTemplate/ImplementerTemplate/OrchestratorTemplate accessors and this
// package's own render-time logic.
//
// Per the fork-context-hygiene Shared Decision, RenderForkPrompt's output
// feeds two callers with opposite context situations — beginbatch.go's
// in-session fork (which already inherits Master's whole context) and
// recoverbatch.go's cold recovery strand (a separate process that inherits
// nothing) — so one prompt cannot honestly serve both. This file instead
// composes each of the two prompts, at render time, from one shared
// implementer-job body (implementer-body.md) plus a caller-specific prefix
// (fork-prefix.md or recovery-prefix.md): joinTemplateAssets concatenates
// the raw template bytes before either is handed to stencil.Fill/
// FillOptional, since internal/stencil has no {{template}} include
// mechanism of its own (see stencil.go's own doc comment). Card content
// reaches both prompts as a worktree-relative card-file pointer
// (planparser.Card.SourcePath, `_lyx/plan/NN-<slug>.md`) rendered verbatim
// by renderCardPointers, never as inlined What/file-op fields — the
// implementer reads its own card file in its own turn instead of trusting a
// Go-rendered paraphrase of it.

package websterengine

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencil"
)

//go:embed master-template.md
var masterTemplate []byte

// MasterTemplate returns the embedded Master-session prompt template's raw
// bytes: the caller-required top-level markers are {{.batch_index}},
// {{.progress}}, {{.outcome_path}}, {{.summary_path}},
// {{.integration_prompt_path}}, {{.self_fix_cap}}, and {{.poll_wait_s}}
// (see master-template.md's leading banner comment).
// RenderMasterPrompt fills it via stencil.Fill before run hands it to
// shuttle as the Master session's Prompt.
func MasterTemplate() []byte {
	return masterTemplate
}

//go:embed fork-prefix.md
var forkPrefix []byte

//go:embed recovery-prefix.md
var recoveryPrefix []byte

//go:embed implementer-body.md
var implementerBody []byte

// joinTemplateAssets concatenates prefix and body's raw template bytes,
// separated by a blank line, into one composed template — the Go
// byte-composition internal/stencil's single-pass text/template needs in
// place of an include mechanism it does not have (see stencil.go's own doc
// comment). Only prefix may carry a leading `<!-- -->` banner comment: body
// (always implementerBody) never does, so stencil.stripLeadingComment strips
// exactly the composed result's one banner and none of body's own markers
// are lost to it.
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

// ForkTemplate returns the composed, thin in-session fork prompt template's
// raw bytes (fork-prefix.md + implementer-body.md). Its required top-level
// markers are {{.card_pointers}}, {{.worktree_root}}, {{.prev_digest}},
// {{.self_fix_cap}}, and {{.report_path}} — see implementer-body.md's own
// section text for each marker's role; fork-prefix.md carries no markers of
// its own. RenderForkPrompt fills it via stencil.Fill before begin-batch
// writes the result to a prompt file Master's Agent-tool fork call reads.
func ForkTemplate() []byte {
	return composeForkTemplate()
}

// RecoveryTemplate returns the composed, full cold-start recovery prompt
// template's raw bytes (recovery-prefix.md + implementer-body.md). It
// carries the same five required markers ForkTemplate does, PLUS
// recovery-prefix.md's one optional marker, {{.pattern_directive}}.
// RenderRecoveryPrompt fills it via stencil.FillOptional before
// recover-batch starts the separate cold recovery strand.
func RecoveryTemplate() []byte {
	return composeRecoveryTemplate()
}

// ImplementerBodyTemplate returns implementer-body.md's raw, pre-Fill bytes
// — the one shared job body both ForkTemplate and RecoveryTemplate compose
// with their own prefix. It exists so a test can assert the reuse guarantee
// (both composed templates carry this exact byte sequence) without
// byte-comparing the two renderers' own final output, which legitimately
// diverges on per-caller values.
func ImplementerBodyTemplate() []byte {
	return implementerBody
}

//go:embed integration-template.md
var integrationTemplate []byte

// IntegrationTemplate returns the embedded integration-suite fork prompt
// template's raw bytes (see integration-template.md's leading banner
// comment for its top-level markers), mirroring ForkTemplate/MasterTemplate's
// own accessor shape. RenderIntegrationPrompt fills it via stencil.Fill
// before the single dedicated integration fork's prompt file is written.
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

// renderCardPointers renders one `- <SourcePath>` bullet per card, in
// declared order, joined by "\n" — the pointer list both RenderForkPrompt
// and RenderRecoveryPrompt inject in place of any inlined card content. It
// reads c.SourcePath VERBATIM: the worktree-relative `_lyx/plan/NN-<slug>.md`
// token planparser already built (see planparser.Card.SourcePath). Per the
// card-pointer-relative-via-hubgeometry Shared Decision this function must
// NOT filepath.Rel/filepath.Join the pointer against any Layout field, must
// NOT rebuild the NN-<slug>.md filename from Card.Number/Slug, and must NOT
// name a literal `_lyx` — all three would duplicate authority the Hub
// Geometry Invariant and the Planparser Sole-Parser Invariant already
// reserve elsewhere.
func renderCardPointers(cards []planparser.Card) string {
	bullets := make([]string, 0, len(cards))
	for _, c := range cards {
		bullets = append(bullets, fmt.Sprintf("- `%s`", c.SourcePath))
	}
	return strings.Join(bullets, "\n")
}

// RenderForkPrompt fills the composed, thin in-session fork prompt
// (ForkTemplate) for one execution batch's fork, called by begin-batch
// immediately before Master forks that batch's implementer. batch is the
// specific batcher.Batch being forked — the ordered group of cards this fork
// implements; its cards' SourcePath pointers are rendered verbatim by
// renderCardPointers, never inlined What/file-op fields, since the fork
// already inherits Master's whole plan-level context and reads its own card
// files directly (see the fork-context-hygiene Shared Decision). prevDigest
// is the immediately preceding batch's persisted digest, ALREADY rendered by
// the caller as a one-line summary — read from state.json's
// BatchState.Digest, never re-distilled here against a HEAD that may have
// since moved; an empty prevDigest renders the literal sentinel "none (first
// batch)" instead of a blank field. reportPath is the fork's own
// OutputFiles target; l is the resolved Layout this batch's worktree runs
// in — the source of {{.worktree_root}}. selfFixCap is the config knob
// bounding the fork's in-session self-fix attempts.
//
// {{.worktree_root}} is filled from l.Cwd, NOT l.WorktreeRoot: every caller
// of this function assigns l.Cwd to the Layout field this parameter
// replaced, so filling from WorktreeRoot instead would silently change what
// the fork prompt calls its worktree root at any RelPath != "." — the exact
// geometry this plumbing exists for. On a Resolve-built Layout the two are
// byte-identical at RelPath == ".", but that holds because both are
// Cwd-equivalent there, not because the fields are interchangeable in
// general.
//
// This function takes no *planparser.Plan: the thinned fork prompt injects
// nothing plan-level (no Shared Decisions body, no Rename mechanic, no
// PATTERN directive) — all of that is already in the fork's inherited
// Master context, per the fork-context-hygiene Shared Decision.
func RenderForkPrompt(batch batcher.Batch, prevDigest, reportPath string, l *hubgeometry.Layout, selfFixCap int) ([]byte, error) {
	digestLine := prevDigest
	if strings.TrimSpace(digestLine) == "" {
		digestLine = noPrecedingBatchDigest
	}

	values := map[string]string{
		"card_pointers": renderCardPointers(batch.Cards),
		"report_path":   reportPath,
		"self_fix_cap":  fmt.Sprintf("%d", selfFixCap),
		"worktree_root": l.Cwd,
		"prev_digest":   digestLine,
	}
	prompt, err := stencil.Fill(composeForkTemplate(), values)
	if err != nil {
		return nil, fmt.Errorf("webster: fill fork template: %w", err)
	}
	return prompt, nil
}

// RenderRecoveryPrompt fills the composed, full cold-start recovery prompt
// (RecoveryTemplate) for one execution batch's recovery strand, called by
// recover-batch immediately before spawning that separate process. Unlike
// RenderForkPrompt's in-session fork, the recovery strand inherits NOTHING —
// no codebase orientation, no plan framing, no constraints — so its prompt
// points the strand at `_lyx/plan/00-overview.md`, `CONSTRAINTS.md`, and (when
// PATTERN is active) `_pattern/PATTERN.md` before the shared implementer-job
// body runs, per the fork-context-hygiene Shared Decision. batch, prevDigest,
// reportPath, l, and selfFixCap share RenderForkPrompt's exact meaning and
// plan-less signature shape. pattern_directive is injected via
// pattern.Directive(l, pattern.RoleImplementer) — RoleImplementer, not
// RoleOrchestrator, because the recovery strand DOES edit code (unlike
// Master, which only forks) — and filled through stencil.FillOptional, so it
// renders as nothing when PATTERN is inactive.
func RenderRecoveryPrompt(batch batcher.Batch, prevDigest, reportPath string, l *hubgeometry.Layout, selfFixCap int) ([]byte, error) {
	digestLine := prevDigest
	if strings.TrimSpace(digestLine) == "" {
		digestLine = noPrecedingBatchDigest
	}

	values := map[string]string{
		"card_pointers":     renderCardPointers(batch.Cards),
		"report_path":       reportPath,
		"self_fix_cap":      fmt.Sprintf("%d", selfFixCap),
		"worktree_root":     l.Cwd,
		"prev_digest":       digestLine,
		"pattern_directive": pattern.Directive(l, pattern.RoleImplementer),
	}
	prompt, err := stencil.FillOptional(composeRecoveryTemplate(), values, []string{"pattern_directive"})
	if err != nil {
		return nil, fmt.Errorf("webster: fill recovery template: %w", err)
	}
	return prompt, nil
}

// RenderIntegrationPrompt fills integration-template.md for the plan's
// single, dedicated integration-suite fork's own prompt: run once (if at
// all), after every batch has reached a terminal-done state, only when
// ShouldRunIntegration(plan) is true (integration.go). plan is the whole
// parsed plan — this function injects its own plan-level "## verify:" text
// (plan.Verify). reportPath and worktreeRoot are the fork's own OutputFiles
// target and host checkout, mirroring RenderForkPrompt's own two path
// parameters. Returns an error when plan.Verify is empty: callers gate this
// call on ShouldRunIntegration first, so an empty plan-level verify reaching
// this function is a caller bug, not a value this function papers over with
// a sentinel the way RenderForkPrompt's own prev_digest marker does.
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

// noIntegrationPromptPath is the literal sentinel RenderMasterPrompt renders
// into {{.integration_prompt_path}} when integrationPromptPath is empty: the
// plan carries no plan-level "## verify:" section, so no integration prompt
// file was ever rendered. Never a blank field — an empty top-level marker
// would violate stencil.Fill's required-marker guarantee, and the master
// template's own integration section already tells Master to skip the stage
// when the plan has no "## verify:".
const noIntegrationPromptPath = "none (this plan has no \"## verify:\" section)"

// RenderMasterPrompt fills master-template.md for one `lyx webster run`
// invocation's Master spawn. plan is the parsed, validated plan; st is the
// current run's in-memory State (nil-safe via RenderProgress's own guard,
// though run always has a freshly loaded/initialized State by the time it
// renders this). outcomePath and summaryPath are Master's two permitted
// output files; integrationPromptPath is the pre-rendered integration fork
// prompt file's path (written by run when ShouldRunIntegration reports
// true; empty otherwise, rendering the noIntegrationPromptPath sentinel —
// Master never renders or writes a prompt file itself, since any Master
// write beyond its two contract files is a parent-write audit violation);
// selfFixCap and pollWaitS are the config knobs Master's prompt states as
// tuning knobs for its forks and its recover-batch re-polling,
// respectively. l is the resolved Layout the caller's own PATTERN active
// check runs against — RenderMasterPrompt had neither a root nor a Layout
// parameter before this.
//
// pattern_directive is injected via pattern.Directive(l,
// pattern.RoleOrchestrator) — RoleOrchestrator, not RoleImplementer: this
// template states in as many words that Master never edits code, so an
// implementer-worded directive would be one Master cannot carry out; Master
// qualifies on the context-inheritance clause instead, since its forks are
// in-session and thin precisely because they inherit everything Master has
// read. It is filled through stencil.FillOptional, so it renders as nothing
// when PATTERN is inactive.
func RenderMasterPrompt(plan *planparser.Plan, st *State, outcomePath, summaryPath, integrationPromptPath string, selfFixCap, pollWaitS int, l *hubgeometry.Layout) ([]byte, error) {
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

// RenderBatchIndex renders plan's flat card list into the ordered-list text
// {{.batch_index}} fills with: one line per card, "NN — slug — intent".
// Unlike builderengine's own renderBatchIndex, there are no v2 batch
// annotations left to render — "(oversized)" and "(verify: deferred;
// chain-end NN)" described PlanBatch fields (Oversized, VerifyDeferred,
// ChainEnd) that do not exist on planparser.Card, since the flat format has
// no oversized/chained escape mechanism at all.
func RenderBatchIndex(plan *planparser.Plan) string {
	lines := make([]string, 0, len(plan.Cards))
	for _, c := range plan.Cards {
		lines = append(lines, fmt.Sprintf("%02d — %s — %s", c.Number, c.Slug, c.Intent))
	}
	return strings.Join(lines, "\n")
}

// RenderProgress renders {{.progress}}'s per-batch state summary for
// resume, built strictly from st's PERSISTED BatchState entries — never by
// re-parsing report files the way builderengine's renderProgress does,
// since webster already keeps this exact record in state.json (the
// digest-persistence decision). A batch with no BatchState entry yet, or
// one recorded but not yet Terminal (still in flight, or never started), is
// omitted entirely — only a terminal batch (done or stuck) is listed, one
// "NN-slug: <status>" line per batch, in plan order. Returns the literal
// word "none" when no batch has reached a terminal state yet (a fresh run,
// or a resume before the first batch ever finished). st may be nil (an
// as-yet-uninitialized run); RenderProgress then returns "none" rather than
// panicking, since Master's very first render call happens before any batch
// has run.
//
// st.Batches is keyed by execution-batch number, not plan card number; this
// walks plan.Cards and looks each card's own Number up directly in
// st.Batches. That is exact under today's identity batchifier (batch ≡
// card, so the numbering spaces coincide) and is the same v0 assumption
// CardSHAs documents — a future grouping batchifier needs its own progress
// rendering, not a change here.
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
