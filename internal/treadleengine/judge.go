// judge.go implements treadle's ephemeral progress-judge LLM calls (per-round circling check, milestone continuation gate) as fail-safe spawns over a package-local Shuttle seam, mirroring burlerengine.Engine's Shuttle pattern.
// Unlike a round-runner attempt, neither call here ever returns an error:
// any infrastructure failure degrades to the safe default and logs a logger.Warn, per the original
// error-and-fail-safe-posture decision (03-judge-triage.md) — a false STUCK is the costly failure
// mode, not a few extra bounded rounds.
// Every Warn label is prefixed with the calling engine's name (threaded in as name), per the
// name-parameterized-diagnostics shared decision.

package treadleengine

import (
	"os"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// judgeRole is the agent-name role this module's judge spawn carries.
// The spawn names the review segment.
const judgeRole = "judge"

// judgeSkills are the skills the judge spawns load before their prompt.
var judgeSkills = []string{"scribe:prose"}

// Shuttle is the seam judge.go drives its ephemeral calls through, satisfied by
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
	// ParentName is the told parent name the judge prompt's directive is
	// rendered for — see Options.ParentName.
	ParentName string
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
	directive, err := parentdirective.Directive(in.StencilsDir, in.ParentName, false)
	if err != nil {
		logger.Warn(name+": circling judge parent directive unreadable, defaulting to "+string(JudgeProgressing), "round", in.Round, "cause", err)
		return JudgeProgressing, "", false
	}
	editDirective, err := editdirective.Directive(in.StencilsDir)
	if err != nil {
		logger.Warn(name+": circling judge edit directive unreadable, defaulting to "+string(JudgeProgressing), "round", in.Round, "cause", err)
		return JudgeProgressing, "", false
	}
	values := map[string]string{
		"round":                    strconv.Itoa(in.Round),
		"prior_reviews":            strings.Join(in.PriorReviews, "\n"),
		"verdict_path":             in.VerdictPath,
		"previous_handoff":         previousHandoffMarker(in.PreviousHandoffPath),
		"handoff_path":             in.HandoffPath,
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
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
	directive, err := parentdirective.Directive(in.StencilsDir, in.ParentName, false)
	if err != nil {
		logger.Warn(name+": milestone judge parent directive unreadable, defaulting to "+string(JudgeContinue), "round", in.Round, "cause", err)
		return JudgeContinue, "", false
	}
	editDirective, err := editdirective.Directive(in.StencilsDir)
	if err != nil {
		logger.Warn(name+": milestone judge edit directive unreadable, defaulting to "+string(JudgeContinue), "round", in.Round, "cause", err)
		return JudgeContinue, "", false
	}
	values := map[string]string{
		"round":                    strconv.Itoa(in.Round),
		"hard_cap":                 strconv.Itoa(in.HardCap),
		"prior_reviews":            strings.Join(in.PriorReviews, "\n"),
		"verdict_path":             in.VerdictPath,
		"previous_handoff":         previousHandoffMarker(in.PreviousHandoffPath),
		"handoff_path":             in.HandoffPath,
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
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
		Role:        judgeRole,
		Segment:     segmentcolor.Review,
		Skills:      judgeSkills,
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
