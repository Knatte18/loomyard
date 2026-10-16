// rework.go implements ReworkTable, the PR-Rework row's seat-table builder, and ResolveRework, which picks its model.
// Like PlanTable, the rework agent is a prompt/profile fed to the seat engine, one lead seat producing one artifact.
// It turns the pending rejection's findings into a whole new plan generation, written into the emptied plan directory, and writes the coverage file last as its completion signal.
// Rework is planning, so it reuses the plan role's timeout and takes the model-spec of the rework key, which falls back to plan.
//
// ReworkTable is a pure builder: it stats nothing and spawns nothing.
// Archiving the retired generation and classifying the new one are owned by the PR-Rework producer, not by this file.

package loomengine

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// reworkStencilName is the registered name of the rework stencil.
const reworkStencilName = "loom-template-rework"

// reworkRole is the agent-name role this module's PR-Rework spawn carries.
const reworkRole = "rework"

// reworkSkills are the skills the PR-Rework spawn loads, in order.
var reworkSkills = []string{"scribe:prose", "scribe:testing"}

// reworkPaths are the told paths ReworkTable fills into the rework stencil.
type reworkPaths struct {
	rejection      string
	planDir        string
	overview       string
	decisionRecord string
	coverage       string
	planStencil    string
	priorPlan      string
}

// ResolveRework parses and resolves the rework role's model-spec from cfg: cfg.Rework when non-empty, else cfg.Plan.
// An error names the rework role and the key the value came from.
func ResolveRework(cfg Config, reg modelspec.Registry) (shuttleengine.ModelChoice, error) {
	key, raw := "plan", cfg.Plan
	if cfg.Rework != "" {
		key, raw = "rework", cfg.Rework
	}
	spec, err := modelspec.Parse(raw)
	if err != nil {
		return shuttleengine.ModelChoice{}, fmt.Errorf("loom: ResolveRework: rework role model-spec from key %q: %w", key, err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return shuttleengine.ModelChoice{}, fmt.Errorf("loom: ResolveRework: rework role model-spec from key %q: %w", key, err)
	}
	return shuttleengine.NewModelChoice(resolved), nil
}

// ReworkTable builds the seatengine.Table of one PR-Rework run: a lead that turns the pending rejection's findings into a whole new plan generation and writes the coverage file last as its completion signal.
// stencilsDir and specsDir are told, never derived: this package is bound by the Told-Geometry Invariant.
// firstCard is the number the new generation's first card takes, one past the highest card of the retired generation, and priorPlanDir is where that generation's plan was archived; both are told as values because the session cannot derive them from the worktree it runs in.
// The model is ResolveRework's and the timeout is PlanTimeoutMin.
// Segment is the webster segment, because rework belongs to the Webster row of the loom.
// The lead's opening stencil is read at call time from stencilsDir, and the seat engine renders the parent and edit directives from its geometry.
// The table carries no gate; the producer entry sets it.
// A PATTERN directive that cannot be rendered fails the table; a friction directive that cannot be rendered is left empty, the failure logged.
func ReworkTable(layout *lyxcwd.Location, stencilsDir, specsDir string, cfg Config, reg modelspec.Registry, firstCard int, priorPlanDir string) (seatengine.Table, error) {
	model, err := ResolveRework(cfg, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: ReworkTable: %w", err)
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

	declaredChanges, err := discussionparser.DeclaredPatternChanges(paths.decisionRecord)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: ReworkTable: %w re-step the row with %q", err, "lyx loom start")
	}
	patternDirective, err := pattern.Directive(layout.WorktreePath(), stencilsDir, pattern.RoleImplementer, declaredChanges)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: ReworkTable: %w", err)
	}

	var frictionDir string
	if cfg.Friction != "" {
		frictionDir = LoomFrictionDir(layout)
	}
	frictionDirective, err := friction.Directive(friction.NotePath(frictionDir, "PR-Rework"), stencilsDir, friction.RoleImplementer)
	if err != nil {
		// A friction directive is optional bookkeeping, so a stencil read failure here must never fail the rework spawn.
		logger.Warn("loom: friction directive failed, continuing without one", "role", "rework", "stencil", reworkStencilName, "error", err)
		frictionDirective = ""
	}

	return seatengine.Table{
		RolePrefix: reworkRole,
		Segment:    segmentcolor.Webster,
		Timeout:    time.Duration(cfg.PlanTimeoutMin) * time.Minute,
		Values: map[string]string{
			"rejection_path":       paths.rejection,
			"plan_dir":             paths.planDir,
			"overview_path":        paths.overview,
			"decision_record_path": paths.decisionRecord,
			"coverage_path":        paths.coverage,
			"plan_stencil_path":    paths.planStencil,
			"first_card":           strconv.Itoa(firstCard),
			"prior_plan_dir":       paths.priorPlan,
			"specs_dir":            specsDir,
			"pattern_directive":    patternDirective,
			friction.MarkerName:    frictionDirective,
		},
		Optional: []string{"pattern_directive", friction.MarkerName},
		Seats: []seatengine.Seat{{
			Stencil:     reworkStencilName,
			ModelChoice: model,
			Outputs:     []string{paths.coverage},
			Skills:      reworkSkills,
		}},
	}, nil
}
