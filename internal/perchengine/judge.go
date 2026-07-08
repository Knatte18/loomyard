// judge.go implements perch's two ephemeral LLM utility calls — the
// progress judge (per-round circling check, milestone continuation gate)
// and the asking-triage call — as fail-safe spawns over a package-local
// Shuttle seam, mirroring burlerengine.Engine's Shuttle pattern. Unlike a
// burler round, none of the three calls here ever returns an error: any
// infrastructure failure degrades to the safe default and logs a
// logger.Warn, per the error-and-fail-safe-posture decision
// (03-judge-triage.md) — a false STUCK is the costly failure mode, not a
// few extra bounded rounds.

package perchengine

import (
	"os"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// Shuttle is the seam judge.go drives its three ephemeral calls through:
// the subset of shuttleengine's API each call needs, satisfied as-is by
// *shuttleengine.Runner in production and by a fake in unit tests. Kept
// package-local rather than shared with burlerengine's own Shuttle
// interface, mirroring that seam's own rationale: it lets perchengine stay
// engine-agnostic and testable without wiring mux or an LLM provider.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
}

// var _ Shuttle = (*shuttleengine.Runner)(nil) is the compile-time proof
// that *shuttleengine.Runner satisfies Shuttle as-is, so production wiring
// never needs an adapter type.
var _ Shuttle = (*shuttleengine.Runner)(nil)

// judgeInputs bundles the values every judge call (either framing) needs to
// compose its prompt and shuttle spec. HardCap is only read by the
// milestone framing; PriorReviews is rendered into the prior_reviews marker
// as a newline-separated absolute-path list — the judge agent reads the
// files itself, so its input is self-contained with no memory carried
// between calls.
type judgeInputs struct {
	Round        int
	HardCap      int
	PriorReviews []string
	VerdictPath  string
	Model        string
	Effort       string
}

// runCircling spawns the per-round circling-check progress judge: does the
// newest BLOCKING round's findings recur across prior rounds, or is the
// block still moving forward? Fail-safe: any failure — stencil fill,
// shuttle Run error, non-done Outcome, verdict file read, or parse — logs a
// logger.Warn naming the round and cause, and returns (JudgeProgressing,
// "") rather than an error, since a false CIRCLING permanently kills a
// converging block while a false PROGRESSING only costs a few more bounded
// rounds.
func runCircling(sh Shuttle, in judgeInputs) (JudgeVerdict, string) {
	values := map[string]string{
		"round":         strconv.Itoa(in.Round),
		"prior_reviews": strings.Join(in.PriorReviews, "\n"),
		"verdict_path":  in.VerdictPath,
	}
	return runJudgeCall(sh, judgeCirclingTemplate, values, framingCircling, in.Round, in.Model, in.Effort, JudgeProgressing, "circling judge")
}

// runMilestone spawns the milestone continuation-gate progress judge: has a
// block reached a soft cap whose trajectory still justifies continuing
// toward HardCap? Fail-safe posture mirrors runCircling exactly, defaulting
// to (JudgeContinue, "") on any failure — a false STOP permanently kills a
// converging block while a false CONTINUE only spends the remaining rounds
// up to the hard cap, which still catches a genuinely stuck block.
func runMilestone(sh Shuttle, in judgeInputs) (JudgeVerdict, string) {
	values := map[string]string{
		"round":         strconv.Itoa(in.Round),
		"hard_cap":      strconv.Itoa(in.HardCap),
		"prior_reviews": strings.Join(in.PriorReviews, "\n"),
		"verdict_path":  in.VerdictPath,
	}
	return runJudgeCall(sh, judgeMilestoneTemplate, values, framingMilestone, in.Round, in.Model, in.Effort, JudgeContinue, "milestone judge")
}

// runJudgeCall is the shared body runCircling and runMilestone drive
// through their respective template/framing/default: compose the prompt,
// build and run the shuttle spec (Role "judge" for both framings), then
// read and parse the verdict file. Every failure point degrades to
// fallback with an empty rationale rather than an error, logging label
// (the call's human-facing name, e.g. "circling judge") alongside round and
// cause so an operator can tell which of the two framings failed.
func runJudgeCall(sh Shuttle, template []byte, values map[string]string, framing judgeFraming, round int, model, effort string, fallback JudgeVerdict, label string) (JudgeVerdict, string) {
	prompt, err := stencil.Fill(template, values)
	if err != nil {
		logger.Warn("perch: "+label+" failed, defaulting to "+string(fallback), "round", round, "cause", err)
		return fallback, ""
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{values["verdict_path"]},
		Model:       model,
		Effort:      effort,
		Role:        "judge",
		Round:       strconv.Itoa(round),
	}

	result, err := sh.Run(spec)
	if err != nil {
		logger.Warn("perch: "+label+" shuttle run failed, defaulting to "+string(fallback), "round", round, "cause", err)
		return fallback, ""
	}
	if result.Outcome != shuttleengine.OutcomeDone {
		logger.Warn("perch: "+label+" did not complete, defaulting to "+string(fallback), "round", round, "outcome", result.Outcome)
		return fallback, ""
	}

	content, err := os.ReadFile(values["verdict_path"])
	if err != nil {
		logger.Warn("perch: "+label+" verdict file unreadable, defaulting to "+string(fallback), "round", round, "cause", err)
		return fallback, ""
	}

	verdict, rationale, err := ParseJudgeVerdict(content, framing)
	if err != nil {
		logger.Warn("perch: "+label+" verdict file unparseable, defaulting to "+string(fallback), "round", round, "cause", err)
		return fallback, ""
	}
	return verdict, rationale
}

// runTriage spawns the asking-triage call: a review agent stopped mid-round
// asking question rather than finishing, and this call classifies whether
// a fresh retry can plausibly proceed (RETRY) or the round profile itself
// is broken (GIVE_UP). Fail-safe: any failure — stencil fill, shuttle Run
// error, non-done Outcome, verdict file read, or parse — logs a
// logger.Warn naming the round and cause, and returns (TriageRetry, "")
// rather than an error.
func runTriage(sh Shuttle, round int, question, verdictPath, model, effort string) (TriageVerdict, string) {
	values := map[string]string{
		"round":        strconv.Itoa(round),
		"question":     question,
		"verdict_path": verdictPath,
	}

	prompt, err := stencil.Fill(triageTemplate, values)
	if err != nil {
		logger.Warn("perch: triage failed, defaulting to retry", "round", round, "cause", err)
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
		logger.Warn("perch: triage shuttle run failed, defaulting to retry", "round", round, "cause", err)
		return TriageRetry, ""
	}
	if result.Outcome != shuttleengine.OutcomeDone {
		logger.Warn("perch: triage did not complete, defaulting to retry", "round", round, "outcome", result.Outcome)
		return TriageRetry, ""
	}

	content, err := os.ReadFile(verdictPath)
	if err != nil {
		logger.Warn("perch: triage verdict file unreadable, defaulting to retry", "round", round, "cause", err)
		return TriageRetry, ""
	}

	verdict, rationale, err := ParseTriageVerdict(content)
	if err != nil {
		logger.Warn("perch: triage verdict file unparseable, defaulting to retry", "round", round, "cause", err)
		return TriageRetry, ""
	}
	return verdict, rationale
}
