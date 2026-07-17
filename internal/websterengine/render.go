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

package websterengine

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/builderengine"
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
// bytes: the caller-required top-level markers are {{.batch_file}},
// {{.batch_name}}, {{.report_path}}, {{.self_fix_cap}}, {{.worktree_root}},
// and {{.prev_digest}} (see fork-template.md's leading banner comment).
// RenderForkPrompt fills it via stencil.Fill before begin-batch writes the
// result to a prompt file Master's Agent-tool fork call reads.
func ForkTemplate() []byte {
	return forkTemplate
}

// noPrecedingBatchDigest is the literal sentinel RenderForkPrompt renders
// into {{.prev_digest}} when prevDigest is empty: batch 1 has no preceding
// batch, and a crash-resumed run re-driving batch 1 fresh carries no digest
// either. Never a blank field — an empty {{.prev_digest}} would violate
// stencil.Fill's required-top-level-marker guarantee.
const noPrecedingBatchDigest = "none (first batch)"

// RenderForkPrompt fills fork-template.md for one batch's fork, called by
// begin-batch immediately before Master forks that batch's implementer.
// batch is the plan batch being forked; planDir is the plan directory
// (resolved by the caller via hubgeometry.PlanDir), used to build the
// batch's absolute file path. prevDigest is the immediately preceding
// batch's persisted digest, ALREADY rendered by the caller as a one-line
// summary (batch name, status, tests, files_changed, plus stuck_reason/drift
// when present) — read from state.json's BatchState.Digest, never
// re-Distilled here against a HEAD that may have since moved; an empty
// prevDigest (batch 1, or any batch with no recorded predecessor) renders
// the literal sentinel "none (first batch)" instead of a blank field.
// reportPath and worktreeRoot are the fork's own OutputFiles target and host
// checkout, and selfFixCap is the config knob bounding the fork's in-session
// self-fix attempts.
func RenderForkPrompt(batch builderengine.PlanBatch, planDir string, prevDigest string, reportPath, worktreeRoot string, selfFixCap int) ([]byte, error) {
	batchFilePath, err := filepath.Abs(filepath.Join(planDir, batch.File))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve batch file path: %w", err)
	}

	digestLine := prevDigest
	if strings.TrimSpace(digestLine) == "" {
		digestLine = noPrecedingBatchDigest
	}

	values := map[string]string{
		"batch_file":    batchFilePath,
		"batch_name":    fmt.Sprintf("%02d-%s", batch.Number, batch.Slug),
		"report_path":   reportPath,
		"self_fix_cap":  fmt.Sprintf("%d", selfFixCap),
		"worktree_root": worktreeRoot,
		"prev_digest":   digestLine,
	}
	prompt, err := stencil.Fill(ForkTemplate(), values)
	if err != nil {
		return nil, fmt.Errorf("webster: fill fork template: %w", err)
	}
	return prompt, nil
}

// RenderMasterPrompt fills master-template.md for one `lyx webster run`
// invocation's Master spawn. plan is the parsed, validated plan; st is the
// current run's in-memory State (nil-safe via RenderProgress's own guard,
// though run always has a freshly loaded/initialized State by the time it
// renders this). outcomePath and summaryPath are Master's two permitted
// output files; selfFixCap and pollWaitS are the config knobs Master's
// prompt states as tuning knobs for its forks and its recover-batch
// re-polling, respectively.
func RenderMasterPrompt(plan *builderengine.Plan, st *State, outcomePath, summaryPath string, selfFixCap, pollWaitS int) ([]byte, error) {
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

// RenderBatchIndex renders plan's Batch Index into the ordered-list text
// {{.batch_index}} fills with: one line per batch, "NN — slug — intent",
// annotated with "(oversized)" and/or "(verify: deferred; chain-end NN)"
// where the batch file's own frontmatter declares them. Webster-local
// (rather than reused from builderengine) because Master's own prompt is
// the sole consumer and the rendering has no dir/state dependency worth
// sharing — mirrors builderengine's own renderBatchIndex line-for-line,
// per the reuse-by-import-never-copy decision's own carve-out for a
// same-shape-different-package leaf this small.
func RenderBatchIndex(plan *builderengine.Plan) string {
	lines := make([]string, 0, len(plan.Batches))
	for _, b := range plan.Batches {
		line := fmt.Sprintf("%02d — %s — %s", b.Number, b.Slug, b.Intent)

		var annotations []string
		if b.Oversized {
			annotations = append(annotations, "oversized")
		}
		if b.VerifyDeferred {
			annotations = append(annotations, fmt.Sprintf("verify: deferred; chain-end %02d", b.ChainEnd))
		}
		if len(annotations) > 0 {
			line += " (" + strings.Join(annotations, "; ") + ")"
		}

		lines = append(lines, line)
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
func RenderProgress(plan *builderengine.Plan, st *State) string {
	if st == nil {
		return "none"
	}

	var lines []string
	for _, b := range plan.Batches {
		bs, ok := st.Batches[b.Number]
		if !ok || !bs.Terminal {
			continue
		}
		lines = append(lines, fmt.Sprintf("%02d-%s: %s", b.Number, b.Slug, bs.Status))
	}
	if len(lines) == 0 {
		return "none"
	}
	return strings.Join(lines, "\n")
}
