// Package operatoragent builds the session the operator starts from a status-bar agent button:
// an interactive, unawaited agent in the worktree the button belongs to.
// The session opens on the `operatoragent-template-start` stencil.
// That stencil tells the agent what it is and which hub and worktree it sits in.
// It also names the board entry the worktree serves, or says that the worktree is the prime.
// The session starts as a run session does.
// It gets the settings, shell guards, standing passages and skills every lym session of its engine gets.
// Its skills are the agents.yaml list of the agent role, followed by the hub's language skills the caller passes.
// Unlike a run session, it takes no earlier session's operator instructions over, and its window closes when it exits.
//
// The package is told every path and name it renders and imports no resolver.
package operatoragent

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// stencilName is the registered name of the agent's opening prompt.
const stencilName = "operatoragent-template-start"

// patternDirectiveMarker is the stencil's one optional marker.
// It is filled with the PATTERN directive and left empty when the worktree has no PATTERN overview.
const patternDirectiveMarker = "pattern_directive"

// paneRules fills the mode_rules marker.
// The parent directive is written for a run's agent, so these rules override it for a session the operator drives.
const paneRules = "The operator started this session and works with you in this pane, " +
	"so these rules take precedence over the section on your parent above.\n" +
	"Put a question or a blocker to the operator here and wait for the answer: " +
	"post none to a parent, and end no turn to wait on a run.\n" +
	"You have no report to write.\n" +
	"Of that section, only what it says about creel mail applies to you."

// Inputs holds the caller-supplied values the agent's session spec is built from.
type Inputs struct {
	// StencilsDir is the directory the stencils are read from.
	StencilsDir string
	// Name is the strand role the operator chose, already in the role grammar.
	Name string
	// HubShortname and HubPath name the hub the worktree belongs to.
	HubShortname string
	HubPath      string
	// WorktreeRoot is the worktree the agent works in.
	WorktreeRoot string
	// Slug is the board entry the worktree serves, empty in the prime.
	Slug string
	// ParentName names the session that spawned the worktree's run, empty for a worktree with none.
	ParentName string
	// LanguageSkills are the hub's language skills the agent loads after the agent role's agents.yaml skills.
	LanguageSkills []string
}

// SessionSpec builds the spec the agent's interactive session starts under on choice, its prompt rendered for engine.
// The session is unawaited, opens focused in a window of its own, and carries the agent role's skills.
// It ends with its provider, and it neither takes over nor leaves behind operator instructions.
// It refuses an empty StencilsDir, Name, HubShortname, HubPath or WorktreeRoot by name.
func SessionSpec(in Inputs, choice shuttleengine.ModelChoice, engine string) (shuttleengine.Spec, error) {
	prompt, err := RenderPrompt(in, engine)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	return shuttleengine.Spec{
		Prompt:          prompt,
		PromptEngine:    engine,
		ModelChoice:     choice,
		Interactive:     true,
		Unawaited:       true,
		EndsWithSession: true,
		NoCarryOver:     true,
		Role:            in.Name,
		Display:         render.Display{Anchor: render.AnchorOwnWindow, Focus: true},
		ConfigRole:      agentconfig.RoleAgent,
		Skills:          in.LanguageSkills,
	}, nil
}

// RenderPrompt renders the agent's opening prompt for engine.
// The implementer PATTERN directive is rendered from WorktreeRoot, since the agent may change and commit code.
// It refuses an empty required input by name, and returns an error when a stencil or directive cannot be rendered.
func RenderPrompt(in Inputs, engine string) (string, error) {
	required := []struct{ field, value string }{
		{"StencilsDir", in.StencilsDir},
		{"Name", in.Name},
		{"HubShortname", in.HubShortname},
		{"HubPath", in.HubPath},
		{"WorktreeRoot", in.WorktreeRoot},
	}
	for _, r := range required {
		if r.value == "" {
			return "", errors.New("operatoragent: " + r.field + " must not be empty")
		}
	}
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
		"hub_shortname":            in.HubShortname,
		"hub_path":                 in.HubPath,
		"worktree":                 in.WorktreeRoot,
		"place":                    placeClause(in.Slug),
		parentdirective.MarkerName: parentDirective,
		"mode_rules":               paneRules,
		editdirective.MarkerName:   editDirective,
		patternDirectiveMarker:     patternDirective,
	}, []string{patternDirectiveMarker})
	if err != nil {
		return "", fmt.Errorf("operatoragent: fill the agent prompt: %w", err)
	}
	return string(prompt), nil
}

// placeClause names what the worktree serves: the board entry slug names, or the prime for an empty slug.
func placeClause(slug string) string {
	if slug == "" {
		return "the hub's prime worktree, where the hub's orch runs and the board's entries are filed"
	}
	return "the worktree of the board entry `" + slug + "`, where that entry's run works"
}
