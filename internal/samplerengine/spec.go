// spec.go — the spec the sampler agent's interactive session starts under.

package samplerengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// Role is the agent-name role of the sampler agent's session.
const Role = "sampler"

// stencilName is the registered name of the sampler agent's opening prompt.
const stencilName = "sampler-template-agent"

// patternDirectiveMarker is the stencil's one optional marker.
// It is filled with the PATTERN directive and left empty when the worktree has no PATTERN overview.
const patternDirectiveMarker = "pattern_directive"

// SessionInputs holds the caller-supplied values the sampler agent's session spec is built from, none of them derived.
type SessionInputs struct {
	// StencilsDir is the directory the stencils are read from.
	StencilsDir string
	// ParentName names the session the agent asks, empty for a run with no parent.
	ParentName string
	// Slug is the run's task slug.
	Slug string
	// WorktreeRoot is the task worktree the agent works in.
	WorktreeRoot string
	// DecisionRecordPath is the decision record the agent reads as the task's brief.
	DecisionRecordPath string
	// LanguageSkills are the hub's language skills the agent loads after the sampler role's agents.yaml skills.
	// The agent edits code, so it loads them before it writes any; empty for none.
	LanguageSkills []string
	// Engines names the engine the agent's model choice resolves to, which the prompt is rendered for.
	Engines shuttleengine.EngineNamer
}

// SessionSpec builds the spec the sampler agent's interactive session starts under.
// It refuses an empty StencilsDir, Slug, WorktreeRoot or DecisionRecordPath, or a nil Engines, by name.
// It also refuses an agent model-spec that does not resolve against reg.
// The prompt is rendered for the engine in.Engines names for the resolved model choice,
// and the spec's PromptEngine carries that engine.
func SessionSpec(in SessionInputs, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error) {
	required := []struct{ field, value string }{
		{"StencilsDir", in.StencilsDir},
		{"Slug", in.Slug},
		{"WorktreeRoot", in.WorktreeRoot},
		{"DecisionRecordPath", in.DecisionRecordPath},
	}
	for _, r := range required {
		if r.value == "" {
			return shuttleengine.Spec{}, fmt.Errorf("sampler: SessionSpec: %s must not be empty", r.field)
		}
	}
	if in.Engines == nil {
		return shuttleengine.Spec{}, errors.New("sampler: SessionSpec: Engines must not be nil")
	}

	parsed, err := modelspec.Parse(cfg.Agent)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("sampler: SessionSpec: agent model-spec: %w", err)
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("sampler: SessionSpec: agent model-spec: %w", err)
	}

	choice := shuttleengine.NewModelChoice(resolved)
	engine := in.Engines.EngineName(choice)
	prompt, err := RenderSessionPrompt(in, engine)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("sampler: SessionSpec: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:       prompt,
		PromptEngine: engine,
		ModelChoice:  choice,
		Interactive:  true,
		Unawaited:    true,
		DenyPublish:  true,
		AllowRawTmux: true,
		Role:         Role,
		Segment:      segmentcolor.Darn,
		ConfigRole:   agentconfig.RoleSampler,
		Skills:       in.LanguageSkills,
	}, nil
}

// RenderSessionPrompt renders the sampler agent's opening prompt for engine.
// It needs only the stencils and the worktree on disk, so it runs with no shuttle or session; in.Engines is unused.
// The prompt carries paths only, never file content, bar the PATTERN overview.
// The implementer directive renders that overview from WorktreeRoot, since the agent writes and commits code.
// It returns an error when a stencil or directive cannot be rendered.
func RenderSessionPrompt(in SessionInputs, engine string) (string, error) {
	template, err := stencilstore.ReadForEngine(in.StencilsDir, engine, stencilName)
	if err != nil {
		return "", err
	}
	parentDirective, err := parentdirective.Directive(in.StencilsDir, in.ParentName, true)
	if err != nil {
		return "", err
	}
	editDirective, err := editdirective.Directive(in.StencilsDir, engine)
	if err != nil {
		return "", err
	}
	patternDirective, err := pattern.Directive(in.WorktreeRoot, in.StencilsDir, pattern.RoleImplementer, nil)
	if err != nil {
		return "", err
	}
	prompt, err := stencil.FillOptional(template, map[string]string{
		"slug":                     in.Slug,
		"task_worktree":            in.WorktreeRoot,
		"decision_record_path":     in.DecisionRecordPath,
		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
		patternDirectiveMarker:     patternDirective,
	}, []string{patternDirectiveMarker})
	if err != nil {
		return "", fmt.Errorf("fill the sampler prompt: %w", err)
	}
	return string(prompt), nil
}
