// judge.go implements treadle's two ephemeral LLM utility calls — the progress judge (per-round
// circling check, milestone continuation gate) and the asking-triage call — as fail-safe spawns
// over a package-local Shuttle seam, mirroring burlerengine.Engine's Shuttle pattern.
// Unlike a round-runner attempt, none of the three calls here ever returns an error: any
// infrastructure failure degrades to the safe default and logs a logger.Warn, per the original
// error-and-fail-safe-posture decision (03-judge-triage.md) — a false STUCK is the costly failure
// mode, not a few extra bounded rounds.
// Every Warn label is prefixed with the calling engine's name (threaded in as name), per the
// name-parameterized-diagnostics shared decision.

package treadleengine

import (
	"os"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// Shuttle is the seam judge.go drives its three ephemeral calls through, satisfied by
// *shuttleengine.Runner in production and fakes in tests.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
}

// var _ Shuttle = (*shuttleengine.Runner)(nil) is the compile-time proof
// that *shuttleengine.Runner satisfies Shuttle as-is, so production wiring
// never needs an adapter type.
var _ Shuttle = (*shuttleengine.Runner)(nil)

// judgeInputs bundles values for composing a judge call's prompt and shuttle spec.
type judgeInputs struct {
	Round               int
	HardCap             int
	PriorReviews        []string
	VerdictPath         string
	PreviousHandoffPath string
	HandoffPath         string
	Model               string
	Effort              string
	// StencilsDir is the absolute stencils directory runCircling and
	// runMilestone read their prompt from via stencilstore.Read, told by the
	// caller rather than derived — see Options.StencilsDir.
	StencilsDir string
}

// runCircling spawns the per-round circling-check progress judge. Fail-safe:
// any failure — including a stencilstore.Read failure for the prompt itself —
// logs a Warn and returns (JudgeProgressing, "", false) rather than an error.
// ok is false on every fail-safe path and true only when a real verdict was
// parsed.
func runCircling(sh Shuttle, name string, in judgeInputs) (JudgeVerdict, string, bool) {
	template, err := stencilstore.Read(in.StencilsDir, "treadle-template-judge-circling")
	if err != nil {
		logger.Warn(name+": circling judge template unreadable, defaulting to "+string(JudgeProgressing), "round", in.Round, "cause", err)
		return JudgeProgressing, "", false
	}
	values := map[string]string{
		"round":            strconv.Itoa(in.Round),
		"prior_reviews":    strings.Join(in.PriorReviews, "\n"),
		"verdict_path":     in.VerdictPath,
		"previous_handoff": previousHandoffMarker(in.PreviousHandoffPath),
		"handoff_path":     in.HandoffPath,
	}
	return runJudgeCall(sh, name, template, values, framingCircling, in.Round, in.Model, in.Effort, JudgeProgressing, "circling judge")
}

// runMilestone spawns the milestone continuation-gate progress judge. Fail-safe
// posture mirrors runCircling: defaults to (JudgeContinue, "", false) on any
// failure, including a stencilstore.Read failure for the prompt itself.
func runMilestone(sh Shuttle, name string, in judgeInputs) (JudgeVerdict, string, bool) {
	template, err := stencilstore.Read(in.StencilsDir, "treadle-template-judge-milestone")
	if err != nil {
		logger.Warn(name+": milestone judge template unreadable, defaulting to "+string(JudgeContinue), "round", in.Round, "cause", err)
		return JudgeContinue, "", false
	}
	values := map[string]string{
		"round":            strconv.Itoa(in.Round),
		"hard_cap":         strconv.Itoa(in.HardCap),
		"prior_reviews":    strings.Join(in.PriorReviews, "\n"),
		"verdict_path":     in.VerdictPath,
		"previous_handoff": previousHandoffMarker(in.PreviousHandoffPath),
		"handoff_path":     in.HandoffPath,
	}
	return runJudgeCall(sh, name, template, values, framingMilestone, in.Round, in.Model, in.Effort, JudgeContinue, "milestone judge")
}

// previousHandoffMarker renders a judgeInputs.PreviousHandoffPath value into
// the previous_handoff stencil marker: the path itself when a previous
// handoff exists, or the literal "(none)" when this is the first handoff a
// block has ever produced — stencil.Fill requires every marker to resolve
// to some value (no conditionals in templates), so the "none yet" case
// needs its own literal rather than an empty string.
func previousHandoffMarker(path string) string {
	if path == "" {
		return "(none)"
	}
	return path
}

// runJudgeCall composes the prompt, builds and runs the shuttle spec, then
// reads and parses the verdict file. Every failure point degrades to
// (fallback, "", false) rather than an error, logging the call's name and
// cause. ok is true only on the success path.
func runJudgeCall(sh Shuttle, name string, template []byte, values map[string]string, framing judgeFraming, round int, model, effort string, fallback JudgeVerdict, label string) (JudgeVerdict, string, bool) {
	prompt, err := stencil.Fill(template, values)
	if err != nil {
		logger.Warn(name+": "+label+" failed, defaulting to "+string(fallback), "round", round, "cause", err)
		return fallback, "", false
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{values["verdict_path"], values["handoff_path"]},
		Model:       model,
		Effort:      effort,
		Role:        "judge",
		Round:       strconv.Itoa(round),
	}

	result, err := sh.Run(spec)
	if err != nil {
		logger.Warn(name+": "+label+" shuttle run failed, defaulting to "+string(fallback), "round", round, "cause", err)
		return fallback, "", false
	}
	if result.Outcome != shuttleengine.OutcomeDone {
		logger.Warn(name+": "+label+" did not complete, defaulting to "+string(fallback), "round", round, "outcome", result.Outcome)
		return fallback, "", false
	}

	content, err := os.ReadFile(values["verdict_path"])
	if err != nil {
		logger.Warn(name+": "+label+" verdict file unreadable, defaulting to "+string(fallback), "round", round, "cause", err)
		return fallback, "", false
	}

	verdict, rationale, err := ParseJudgeVerdict(content, framing)
	if err != nil {
		logger.Warn(name+": "+label+" verdict file unparseable, defaulting to "+string(fallback), "round", round, "cause", err)
		return fallback, "", false
	}
	return verdict, rationale, true
}

// runTriage spawns the asking-triage call: a review agent stopped mid-round
// asking question rather than finishing, and this call classifies whether
// a fresh retry can plausibly proceed (RETRY) or the round profile itself
// is broken (GIVE_UP). Fail-safe: any failure — the prompt's stencilstore.Read,
// stencil fill, shuttle Run error, non-done Outcome, verdict file read, or
// parse — logs a name-prefixed logger.Warn naming the round and cause, and
// returns (TriageRetry, "") rather than an error. stencilsDir is the
// absolute stencils directory this call reads its prompt from, leading
// rather than trailing so a mis-ordered call site still compiles (see the
// composePrompt convention this mirrors).
func runTriage(stencilsDir string, sh Shuttle, name string, round int, question, verdictPath, model, effort string) (TriageVerdict, string) {
	values := map[string]string{
		"round":        strconv.Itoa(round),
		"question":     question,
		"verdict_path": verdictPath,
	}

	triageTemplate, err := stencilstore.Read(stencilsDir, "treadle-template-triage")
	if err != nil {
		logger.Warn(name+": triage template unreadable, defaulting to retry", "round", round, "cause", err)
		return TriageRetry, ""
	}

	prompt, err := stencil.Fill(triageTemplate, values)
	if err != nil {
		logger.Warn(name+": triage failed, defaulting to retry", "round", round, "cause", err)
		return TriageRetry, ""
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{verdictPath},
		Model:       model,
		Effort:      effort,
		Role:        "triage",
		Round:       strconv.Itoa(round),
	}

	result, err := sh.Run(spec)
	if err != nil {
		logger.Warn(name+": triage shuttle run failed, defaulting to retry", "round", round, "cause", err)
		return TriageRetry, ""
	}
	if result.Outcome != shuttleengine.OutcomeDone {
		logger.Warn(name+": triage did not complete, defaulting to retry", "round", round, "outcome", result.Outcome)
		return TriageRetry, ""
	}

	content, err := os.ReadFile(verdictPath)
	if err != nil {
		logger.Warn(name+": triage verdict file unreadable, defaulting to retry", "round", round, "cause", err)
		return TriageRetry, ""
	}

	verdict, rationale, err := ParseTriageVerdict(content)
	if err != nil {
		logger.Warn(name+": triage verdict file unparseable, defaulting to retry", "round", round, "cause", err)
		return TriageRetry, ""
	}
	return verdict, rationale
}
