// rework.go implements ReworkSpec, the PR-Rework producer's Spec factory, and its composeReworkPrompt prompt composer.
// Like PlanSpec, the rework agent is a prompt/profile fed to shuttle.Run, one shuttle.Run producing one artifact.
// It turns the pending rejection's findings into a whole new plan generation, written into the emptied plan directory, and writes the coverage file last as its completion signal.
// Rework is planning, so it reuses the plan role's model-spec and timeout rather than carrying config keys of its own.
//
// ReworkSpec is a pure composer: it stats nothing and spawns nothing.
// Archiving the retired generation and classifying the new one are owned by the PR-Rework producer, not by this file.

package loomengine

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// reworkStencilName is the registered name of the rework stencil.
const reworkStencilName = "loom-template-rework"

// reworkRole is the agent-name role this module's PR-Rework spawn carries.
const reworkRole = "rework"

// reworkSkills are the skills the PR-Rework spawn loads, in order.
var reworkSkills = []string{"scribe:prose", "scribe:testing"}

// reworkPaths are the told paths composeReworkPrompt fills into the rework stencil.
type reworkPaths struct {
	rejection      string
	planDir        string
	overview       string
	decisionRecord string
	coverage       string
	planStencil    string
	priorPlan      string
}

// composeReworkPrompt builds the rework prompt by reading the "loom-template-rework" stencil from stencilsDir and filling it.
// firstCard is the number the new generation's first card takes.
// Only pattern_directive, the friction marker and the parent directive marker are optional; specs_dir and every path marker fail composition when empty.
func composeReworkPrompt(stencilsDir, specsDir string, p reworkPaths, firstCard int, patternDirective, frictionDirective, parentDirective string) ([]byte, error) {
	template, err := stencilstore.Read(stencilsDir, reworkStencilName)
	if err != nil {
		return nil, err
	}

	friction.WarnIfMarkerAbsent(template, reworkStencilName, frictionDirective)

	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("loom: compose rework prompt: %w", err)
	}

	values := map[string]string{
		"rejection_path":           p.rejection,
		"plan_dir":                 p.planDir,
		"overview_path":            p.overview,
		"decision_record_path":     p.decisionRecord,
		"coverage_path":            p.coverage,
		"plan_stencil_path":        p.planStencil,
		"first_card":               strconv.Itoa(firstCard),
		"prior_plan_dir":           p.priorPlan,
		"specs_dir":                specsDir,
		"pattern_directive":        patternDirective,
		friction.MarkerName:        frictionDirective,
		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
	}

	rendered, err := stencil.FillOptional(template, values, []string{"pattern_directive", friction.MarkerName, parentdirective.MarkerName})
	if err != nil {
		return nil, fmt.Errorf("loom: compose rework prompt: %w", err)
	}
	return rendered, nil
}

// ReworkSpec builds the shuttleengine.Spec for one PR-Rework agent run.
// stencilsDir, specsDir and parentName are told, never derived: this package is bound by the Told-Geometry Invariant.
// An empty parentName renders the no-parent directive.
// firstCard is the number the new generation's first card takes, one past the highest card of the retired generation,
// and priorPlanDir is where that generation's plan was archived; both are told as values because the session cannot derive them from the worktree it runs in.
// The plan role's model-spec and PlanTimeoutMin are reused.
// Segment is the webster segment, because rework belongs to the Webster row of the loom.
func ReworkSpec(layout *lyxcwd.Location, stencilsDir, specsDir, parentName string, cfg Config, reg modelspec.Registry, firstCard int, priorPlanDir string) (shuttleengine.Spec, error) {
	spec, err := modelspec.Parse(cfg.Plan)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: ReworkSpec: plan role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: ReworkSpec: plan role model-spec: %w", err)
	}

	paths := reworkPaths{
		rejection:      LoomRejectionPath(layout),
		planDir:        planparser.PlanDir(layout.AnchorPath()),
		overview:       planparser.PlanOverview(layout.AnchorPath()),
		decisionRecord: DiscussionDecisionRecord(layout),
		coverage:       LoomReworkCoveragePath(layout),
		planStencil:    stencilstore.Path(stencilsDir, "loom-template-plan"),
		priorPlan:      priorPlanDir,
	}

	directive, err := pattern.Directive(layout.WorktreePath(), stencilsDir, pattern.RoleImplementer)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: ReworkSpec: %w", err)
	}

	parentDirective, err := parentdirective.Directive(stencilsDir, parentName, false)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: ReworkSpec: parent directive: %w", err)
	}

	var frictionDir string
	if cfg.Friction != "" {
		frictionDir = LoomFrictionDir(layout)
	}
	notePath := friction.NotePath(frictionDir, "PR-Rework")
	frictionDirective, err := friction.Directive(notePath, stencilsDir, friction.RoleImplementer)
	if err != nil {
		// A friction.Directive error is swallowed, never returned: it is optional bookkeeping, so a transient stencil read failure must never fail the rework spawn.
		logger.Warn("loom: friction directive failed, continuing without one", "role", "rework", "stencil", reworkStencilName, "error", err)
		frictionDirective = ""
	}

	prompt, err := composeReworkPrompt(stencilsDir, specsDir, paths, firstCard, directive, frictionDirective, parentDirective)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: ReworkSpec: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{paths.coverage},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Interactive: false,
		Role:        reworkRole,
		Segment:     segmentcolor.Webster,
		Skills:      reworkSkills,
		Timeout:     time.Duration(cfg.PlanTimeoutMin) * time.Minute,
	}, nil
}
