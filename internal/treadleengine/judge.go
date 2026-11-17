// judge.go runs treadle's ephemeral progress-judge calls:
// the per-round circling check and the milestone continuation gate.
// Each call is a fail-safe spawn made through the package-local Shuttle seam.
// Neither call returns an error; any infrastructure failure degrades to the safe default.
// The default leans toward continuing, because a false STUCK is the costly failure, not a few extra bounded rounds.
// Every logger.Warn label starts with the calling engine's name, passed in as name.

package treadleengine

import (
	"fmt"
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
// EngineName names the engine a call's model choice resolves to, which the call's prompt is rendered for.
type Shuttle interface {
	shuttleengine.EngineNamer
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

// JudgePromptInputs bundles the values a progress-judge prompt is rendered from.
type JudgePromptInputs struct {
	// Engine is the name of the engine the prompt is rendered for; its edit directive is that engine's copy.
	Engine string
	// StencilsDir is the absolute directory the prompt templates are read from.
	StencilsDir string
	// ParentName is the parent name the prompt's directive is rendered for; empty renders the no-parent variant.
	ParentName          string
	Round               int
	HardCap             int
	PriorReviews        []string
	VerdictPath         string
	PreviousHandoffPath string
	HandoffPath         string
}

// promptInputs returns the render inputs for these judge inputs, rendered for engine.
func (in judgeInputs) promptInputs(engine string) JudgePromptInputs {
	return JudgePromptInputs{
		Engine:              engine,
		StencilsDir:         in.StencilsDir,
		ParentName:          in.ParentName,
		Round:               in.Round,
		HardCap:             in.HardCap,
		PriorReviews:        in.PriorReviews,
		VerdictPath:         in.VerdictPath,
		PreviousHandoffPath: in.PreviousHandoffPath,
		HandoffPath:         in.HandoffPath,
	}
}

// RenderCirclingJudgePrompt renders the per-round circling-check judge prompt for in.Engine.
// It needs only the stencils on disk, so it runs with no shuttle or session.
// It returns an error when a stencil is unreadable or a marker cannot be filled.
func RenderCirclingJudgePrompt(in JudgePromptInputs) (string, error) {
	return renderJudgePrompt("treadle-template-judge-circling", in, map[string]string{})
}

// RenderMilestoneJudgePrompt renders the milestone continuation-gate judge prompt for in.Engine.
// It needs only the stencils on disk, so it runs with no shuttle or session.
// It returns an error when a stencil is unreadable or a marker cannot be filled.
func RenderMilestoneJudgePrompt(in JudgePromptInputs) (string, error) {
	return renderJudgePrompt("treadle-template-judge-milestone", in, map[string]string{"hard_cap": strconv.Itoa(in.HardCap)})
}

// renderJudgePrompt fills the named judge stencil with the markers both judge prompts share plus extra.
func renderJudgePrompt(stencilName string, in JudgePromptInputs, extra map[string]string) (string, error) {
	template, err := stencilstore.Read(in.StencilsDir, stencilName)
	if err != nil {
		return "", fmt.Errorf("template unreadable: %w", err)
	}
	directive, err := parentdirective.Directive(in.StencilsDir, in.ParentName, false)
	if err != nil {
		return "", fmt.Errorf("parent directive unreadable: %w", err)
	}
	editDirective, err := editdirective.Directive(in.StencilsDir, in.Engine)
	if err != nil {
		return "", fmt.Errorf("edit directive unreadable: %w", err)
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
	for marker, value := range extra {
		values[marker] = value
	}
	prompt, err := stencil.Fill(template, values)
	if err != nil {
		return "", fmt.Errorf("fill the prompt: %w", err)
	}
	return string(prompt), nil
}

// runCircling spawns the per-round circling-check progress judge.
// Fail-safe: any failure logs a Warn and returns (JudgeProgressing, "", false) instead of an error.
// That includes a failure to render the prompt.
// ok is false on every fail-safe path and true only when a real verdict was parsed.
func runCircling(sh Shuttle, name string, in judgeInputs) (JudgeVerdict, string, bool) {
	engine := sh.EngineName(in.Choice)
	prompt, err := RenderCirclingJudgePrompt(in.promptInputs(engine))
	if err != nil {
		logger.Warn(name+": circling judge prompt failed, defaulting to "+string(JudgeProgressing), "round", in.Round, "cause", err)
		return JudgeProgressing, "", false
	}
	return runJudgeCall(sh, name, prompt, engine, in, framingCircling, JudgeProgressing, "circling judge")
}

// runMilestone spawns the milestone continuation-gate progress judge.
// Fail-safe: any failure logs a Warn and returns (JudgeContinue, "", false) instead of an error.
// That includes a failure to render the prompt.
func runMilestone(sh Shuttle, name string, in judgeInputs) (JudgeVerdict, string, bool) {
	engine := sh.EngineName(in.Choice)
	prompt, err := RenderMilestoneJudgePrompt(in.promptInputs(engine))
	if err != nil {
		logger.Warn(name+": milestone judge prompt failed, defaulting to "+string(JudgeContinue), "round", in.Round, "cause", err)
		return JudgeContinue, "", false
	}
	return runJudgeCall(sh, name, prompt, engine, in, framingMilestone, JudgeContinue, "milestone judge")
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
// It runs the shuttle spec for the rendered prompt, which was rendered for engine, and parses the verdict file.
// Every failure degrades to (fallback, "", false) and logs the call's name and cause.
// ok is true only on the success path.
func runJudgeCall(sh Shuttle, name, prompt, engine string, in judgeInputs, framing judgeFraming, fallback JudgeVerdict, label string) (JudgeVerdict, string, bool) {
	round := in.Round
	spec := shuttleengine.Spec{
		Prompt:       prompt,
		PromptEngine: engine,
		OutputFiles:  []string{in.VerdictPath, in.HandoffPath},
		ModelChoice:  in.Choice,
		Role:         judgeRole,
		Segment:      segmentcolor.Review,
		ConfigRole:   agentconfig.RoleTreadleJudge,
		Round:        strconv.Itoa(round),
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

	content, err := os.ReadFile(in.VerdictPath)
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
