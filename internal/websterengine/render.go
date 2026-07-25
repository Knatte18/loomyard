// render.go implements the two embedded prompt-template assets
// (fork-template.md, master-template.md) and the rendering functions that
// fill them: RenderForkPrompt (called by begin-batch immediately before each
// fork) and RenderMasterPrompt (called by run at Master's own spawn), plus
// the two batch-list/progress renderers those prompts embed
// (RenderBatchIndex, RenderProgress). The two go:embed directives and their
// accessors live here rather than in template.go, which stays config-only —
// mirroring builderengine's own split between template.go's
// ConfigTemplate/ImplementerTemplate/OrchestratorTemplate accessors and this
// package's own render-time logic.
//
// This file is retargeted onto planparser.Plan / batcher.Batch (the flat
// card-list model) and away from builderengine.Plan/PlanBatch. A fork
// prompt no longer points at a batch file on disk — plan-format v3's Card
// carries no on-disk path of its own (see internal/planparser.Card) — it
// renders each of the batch's cards' own fields (What/Context/Edits/
// Creates/Deletes/Moves) directly into the prompt, per the
// fork-prompt-plan-level-context Shared Decision.

package websterengine

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencil"
)

//go:embed master-template.md
var masterTemplate []byte

// MasterTemplate returns the embedded Master-session prompt template's raw
// bytes: the caller-required top-level markers are {{.batch_index}},
// {{.progress}}, {{.outcome_path}}, {{.summary_path}}, {{.self_fix_cap}},
// and {{.poll_wait_s}} (see master-template.md's leading banner comment).
// RenderMasterPrompt fills it via stencil.Fill before run hands it to
// shuttle as the Master session's Prompt.
func MasterTemplate() []byte {
	return masterTemplate
}

//go:embed fork-template.md
var forkTemplate []byte

// ForkTemplate returns the embedded fork-implementer prompt template's raw
// bytes (see fork-template.md's leading banner comment for its top-level
// markers). RenderForkPrompt fills it via stencil.Fill before begin-batch
// writes the result to a prompt file Master's Agent-tool fork call reads.
func ForkTemplate() []byte {
	return forkTemplate
}

// noPrecedingBatchDigest is the literal sentinel RenderForkPrompt renders
// into {{.prev_digest}} when prevDigest is empty: the first executed batch
// has no preceding batch, and a crash-resumed run re-driving that batch
// fresh carries no digest either. Never a blank field — an empty
// {{.prev_digest}} would violate stencil.Fill's required-top-level-marker
// guarantee.
const noPrecedingBatchDigest = "none (first batch)"

// noSharedDecisions is the literal sentinel RenderForkPrompt renders into
// {{.shared_decisions}} when plan.SharedDecisions is empty (a plan with no
// "## Shared Decisions" section) — the marker is unconditionally injected
// into every fork prompt per the fork-prompt-plan-level-context Shared
// Decision, so it must never render blank.
const noSharedDecisions = "none"

// RenderForkPrompt fills fork-template.md for one execution batch's fork,
// called by begin-batch immediately before Master forks that batch's
// implementer. plan is the whole parsed plan; batch is the specific
// batcher.Batch being forked — the ordered group of cards this fork
// implements. prevDigest is the immediately preceding batch's persisted
// digest, ALREADY rendered by the caller as a one-line summary — read from
// state.json's BatchState.Digest, never re-distilled here against a HEAD
// that may have since moved; an empty prevDigest renders the literal
// sentinel "none (first batch)" instead of a blank field. reportPath and
// worktreeRoot are the fork's own OutputFiles target and host checkout, and
// selfFixCap is the config knob bounding the fork's in-session self-fix
// attempts.
//
// Per the fork-prompt-plan-level-context Shared Decision, this function
// ALWAYS injects plan's plan-level "## Shared Decisions" body
// (plan.SharedDecisions, or the sentinel "none" when absent), and injects
// the canonical "## Rename mechanic" body (plan.RenameMechanic) ONLY when
// batch contains at least one card with a non-empty Moves field — every
// other batch's rendered value for that marker is the empty string, which
// the fork template's own conditional section (card 28) is responsible for
// rendering as nothing.
func RenderForkPrompt(plan *planparser.Plan, batch batcher.Batch, prevDigest string, reportPath, worktreeRoot string, selfFixCap int) ([]byte, error) {
	digestLine := prevDigest
	if strings.TrimSpace(digestLine) == "" {
		digestLine = noPrecedingBatchDigest
	}

	sharedDecisions := strings.TrimSpace(plan.SharedDecisions)
	if sharedDecisions == "" {
		sharedDecisions = noSharedDecisions
	}

	renameMechanic := ""
	if batchHasMove(batch) {
		renameMechanic = plan.RenameMechanic
	}

	values := map[string]string{
		"cards":            renderBatchCards(batch.Cards),
		"report_path":      reportPath,
		"self_fix_cap":     fmt.Sprintf("%d", selfFixCap),
		"worktree_root":    worktreeRoot,
		"prev_digest":      digestLine,
		"shared_decisions": sharedDecisions,
		"rename_mechanic":  renameMechanic,
	}
	prompt, err := stencil.Fill(ForkTemplate(), values)
	if err != nil {
		return nil, fmt.Errorf("webster: fill fork template: %w", err)
	}
	return prompt, nil
}

// batchHasMove reports whether any of batch's cards declares a non-empty
// Moves field — the fork-prompt-plan-level-context trigger for injecting
// the canonical "## Rename mechanic" section into that batch's fork prompt.
func batchHasMove(batch batcher.Batch) bool {
	for _, c := range batch.Cards {
		if len(c.Moves) > 0 {
			return true
		}
	}
	return false
}

// renderBatchCards renders every one of cards' own fields — What (from
// Intent, the machine-read summary), and the five typed file-op fields — as
// one markdown block per card, joined by a blank line, in declared order.
// This is what lets a fork implement its batch entirely from the injected
// prompt text, with no separate batch file on disk to read (plan-format v3
// cards carry no stored file path of their own).
func renderBatchCards(cards []planparser.Card) string {
	blocks := make([]string, 0, len(cards))
	for _, c := range cards {
		blocks = append(blocks, renderCard(c))
	}
	return strings.Join(blocks, "\n\n")
}

// renderCard renders one card's own fields as a markdown block: its
// "### Card N — <title>" heading, its What (Intent), and the five typed
// file-op fields (each "none" when empty, matching plan-format v3's own
// none-sentinel convention), plus its optional per-card verify: line when
// present.
func renderCard(c planparser.Card) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### Card %d — %s\n\n", c.Number, c.Title)
	fmt.Fprintf(&b, "**What:** %s\n", c.Intent)
	b.WriteString(renderFileOpField("Context", c.ContextFiles))
	b.WriteString(renderFileOpField("Edits", c.EditsFiles))
	b.WriteString(renderFileOpField("Creates", c.CreatesFiles))
	b.WriteString(renderFileOpField("Deletes", c.DeletesFiles))
	b.WriteString(renderMovesField(c.Moves))
	if strings.TrimSpace(c.Verify) != "" {
		fmt.Fprintf(&b, "**verify:** %s\n", c.Verify)
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderFileOpField renders one of a card's four non-Moves typed file-op
// fields: the bold label line, then one backtick-wrapped path per
// sub-bullet, or the literal "none" on the label line when files is empty.
func renderFileOpField(label string, files []string) string {
	if len(files) == 0 {
		return fmt.Sprintf("**%s:** none\n", label)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**%s:**\n", label)
	for _, f := range files {
		fmt.Fprintf(&b, "- `%s`\n", f)
	}
	return b.String()
}

// renderMovesField renders a card's Moves field: the bold label line, then
// one "- `old` -> `new`" sub-bullet per pair, or the literal "none" on the
// label line when moves is empty.
func renderMovesField(moves []planparser.MovePair) string {
	if len(moves) == 0 {
		return "**Moves:** none\n"
	}
	var b strings.Builder
	b.WriteString("**Moves:**\n")
	for _, m := range moves {
		fmt.Fprintf(&b, "- `%s` -> `%s`\n", m.Old, m.New)
	}
	return b.String()
}

// RenderMasterPrompt fills master-template.md for one `lyx webster run`
// invocation's Master spawn. plan is the parsed, validated plan; st is the
// current run's in-memory State (nil-safe via RenderProgress's own guard,
// though run always has a freshly loaded/initialized State by the time it
// renders this). outcomePath and summaryPath are Master's two permitted
// output files; selfFixCap and pollWaitS are the config knobs Master's
// prompt states as tuning knobs for its forks and its recover-batch
// re-polling, respectively.
func RenderMasterPrompt(plan *planparser.Plan, st *State, outcomePath, summaryPath string, selfFixCap, pollWaitS int) ([]byte, error) {
	values := map[string]string{
		"batch_index":  RenderBatchIndex(plan),
		"progress":     RenderProgress(plan, st),
		"outcome_path": outcomePath,
		"summary_path": summaryPath,
		"self_fix_cap": fmt.Sprintf("%d", selfFixCap),
		"poll_wait_s":  fmt.Sprintf("%d", pollWaitS),
	}
	prompt, err := stencil.Fill(MasterTemplate(), values)
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
