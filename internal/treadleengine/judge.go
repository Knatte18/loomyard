// judge.go runs treadle's ephemeral progress-judge calls:
// the per-round circling check and the milestone continuation gate.
// Each call is a fail-safe spawn made through the package-local Shuttle seam.
// Neither call returns an error; any infrastructure failure degrades to the safe default.
// The default leans toward continuing, because a false STUCK is the costly failure, not a few extra bounded rounds.
// Every logger.Warn label starts with the calling engine's name, passed in as name.

package treadleengine

import (
	"os"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentconfig"
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

// Shuttle is the seam judge.go drives its ephemeral calls through, satisfied by
// *shuttleengine.Runner in production and fakes in tests.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
}

// Compile-time proof that *shuttleengine.Runner satisfies Shuttle as-is.
// Production wiring therefore needs no adapter type.
var _ Shuttle = (*shuttleengine.Runner)(nil)

// judgeInputs bundles values for composing a judge call's prompt and shuttle spec.
type judgeInputs struct {
	Round               int
	HardCap             int
	PriorReviews        []string
	VerdictPath         string
	PreviousHandoffPath string
	HandoffPath         string
	Choice              shuttleengine.ModelChoice
	// StencilsDir is the absolute directory the judge prompt templates are read from.
	// The caller supplies it; the judge does not derive it.
	// Templates are read through stencilstore.Read.
	StencilsDir string
	// ParentName is the parent name the judge prompt's directive is rendered for.
	// The caller supplies it.
	ParentName string
}

// runCircling spawns the per-round circling-check progress judge.
// Fail-safe: any failure logs a Warn and returns (JudgeProgressing, "", false) instead of an error.
// That includes a stencilstore.Read failure for the prompt itself.
// ok is false on every fail-safe path and true only when a real verdict was parsed.
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
	return runJudgeCall(sh, name, template, values, framingCircling, in.Round, in.Choice, JudgeProgressing, "circling judge")
}

// runMilestone spawns the milestone continuation-gate progress judge.
// Fail-safe: any failure logs a Warn and returns (JudgeContinue, "", false) instead of an error.
// That includes a stencilstore.Read failure for the prompt itself.
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
	return runJudgeCall(sh, name, template, values, framingMilestone, in.Round, in.Choice, JudgeContinue, "milestone judge")
}

// previousHandoffMarker renders the previous_handoff marker value for a judge prompt.
// It returns the path when a previous handoff exists,
// or the literal "(none)" for the first handoff a block produces.
// Templates have no conditionals and the stencil fill requires every marker to resolve,
// so the "none yet" case needs its own literal rather than an empty string.
func previousHandoffMarker(path string) string {
	if path == "" {
		return "(none)"
	}
	return path
}

// runJudgeCall runs one judge call.
// It composes the prompt, runs the shuttle spec, and parses the verdict file.
// Every failure degrades to (fallback, "", false) and logs the call's name and cause.
// ok is true only on the success path.
func runJudgeCall(sh Shuttle, name string, template []byte, values map[string]string, framing judgeFraming, round int, choice shuttleengine.ModelChoice, fallback JudgeVerdict, label string) (JudgeVerdict, string, bool) {
	prompt, err := stencil.Fill(template, values)
	if err != nil {
		logger.Warn(name+": "+label+" failed, defaulting to "+string(fallback), "round", round, "cause", err)
		return fallback, "", false
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{values["verdict_path"], values["handoff_path"]},
		ModelChoice: choice,
		Role:        judgeRole,
		Segment:     segmentcolor.Review,
		ConfigRole:  agentconfig.RoleTreadleJudge,
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
