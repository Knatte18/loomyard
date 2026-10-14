// plan.go implements PlanSpec, the Plan producer's Spec factory, and its composePlanPrompt prompt
// composer.
// Like the Discussion producer (discussion.go), the Plan producer is not a Go module — it is a
// prompt/profile fed to shuttle.Run, one shuttle.Run producing one artifact.
// Its sole input is `_lyx/discussion/decision-record.md` (see layout.DiscussionDecisionRecord);
// it never reads the support log and never reads the board.
// Its output is a plan-format flat-card plan written into `_lyx/plan/`: one `NN-<card-slug>.md`
// per card plus `00-overview.md`, written LAST as the run's done-sentinel (see
// contracts/specs/loom-plan-spec.md).
// The producer always writes `approved: false` in `00-overview.md`'s frontmatter — it has no review
// logic of its own beyond the twill lens review below (the Bouncer+Burler segment's separate job); `Plan-Bouncer`'s approved
// settle is what flips `approved` to `true`, immediately before its commit seam fires.
//
// With plan_producer twill the spec is the single writer's with the twill directive rendered into the plan template between Step 2 and Step 3, and fork subagents authorized.
// The directive's lenses come from the twill-plan fan in burler.yaml, the writer spawns the forks once its plan is written and passes the self-check, so they review the plan their inherited context holds, and the writer records the lens review in the plan directory's lens-review.md.
// The writer runs `lyx loom validate-plan --lens-review` as its final self-check, and the row's gate requires a recorded lens-review.md for a twill-spawned run.
// The spec sets ForkSubagents, so the writer may spawn subagents of type fork.
// While the shuttle config's claude_deny_agent_tool is true, the Agent hook admits fork subagents only and refuses every other subagent type; with it false no Agent hook is installed.
// Forks run on the writer's model, read-only and without the Agent tool by the stencil's boilerplate.
// The plan gates and the Plan-Review segment still bound what lands in the plan.
// With plan_producer single the prompt keeps its bytes and ForkSubagents stays false.
//
// PlanSpec is a pure composer, exactly like DiscussionSpec: it does not stat the decision record,
// does not stat or create `_lyx/plan/`, and does not spawn anything.
// Rotating a stale `_lyx/plan/` before a re-run is owned by `loomshed.NewPlanDirRotator`, run as the Plan-Write producer's fresh-spawn preparation;
// verifying the input exists stays outside PlanSpec.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/twilldirective"
)

// planRole is the agent-name role this module's Plan-Write spawn carries.
const planRole = "plan"

// planSkills are the skills the Plan-Write spawn loads, in order.
var planSkills = []string{"scribe:prose", "scribe:testing"}

// composePlanPrompt builds the Plan producer's prompt by reading the "loom-template-plan" stencil
// from stencilsDir and filling it. parentDirective is the caller-rendered parent directive and renders as nothing when empty.
// frictionDirective is the caller-resolved Tier 2 note directive for
// this run -- an empty string means Tier 2 is off or the read failed, and renders as nothing.
// specsDir is the told deployed-specs directory, filled into the required specs_dir marker: unlike
// pattern_directive and friction.MarkerName, specs_dir stays out of the optional-names slice, so a
// prompt composed without a specs directory fails loudly at composition instead of rendering a blank
// path into the agent's instructions.
func composePlanPrompt(stencilsDir, specsDir, decisionRecordPath, planDir, overviewPath, patternDirective, frictionDirective, parentDirective, twillDirective string) ([]byte, error) {
	template, err := stencilstore.Read(stencilsDir, "loom-template-plan")
	if err != nil {
		return nil, err
	}

	friction.WarnIfMarkerAbsent(template, "loom-template-plan", frictionDirective)

	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return nil, fmt.Errorf("loom: compose plan prompt: %w", err)
	}

	values := map[string]string{
		"decision_record_path":     decisionRecordPath,
		"plan_dir":                 planDir,
		"overview_path":            overviewPath,
		"pattern_directive":        patternDirective,
		"specs_dir":                specsDir,
		friction.MarkerName:        frictionDirective,
		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
		twilldirective.MarkerName:  setOffParagraph(twillDirective),
	}

	rendered, err := stencil.FillOptional(template, values, []string{"pattern_directive", friction.MarkerName, parentdirective.MarkerName, twilldirective.MarkerName})
	if err != nil {
		return nil, fmt.Errorf("loom: compose plan prompt: %w", err)
	}
	return rendered, nil
}

// PlanSpec builds the shuttleengine.Spec for one Plan producer run. specsDir is told, never
// derived: this package is bound by the Told-Geometry Invariant and derives no path of its own.
// parentName is told likewise; an empty one renders the no-parent directive.
// dirs is the told config geometry the twill fan's burler.yaml is read through.
// Segment is the plan segment, because the run is the Plan-Write phase.
func PlanSpec(layout *lyxcwd.Location, dirs configengine.Dirs, stencilsDir, specsDir, parentName string, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error) {
	spec, err := modelspec.Parse(cfg.Plan)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: PlanSpec: plan role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: PlanSpec: plan role model-spec: %w", err)
	}

	decisionRecordPath := DiscussionDecisionRecord(layout)
	planDir := planparser.PlanDir(layout.AnchorPath())
	overviewPath := planparser.PlanOverview(layout.AnchorPath())

	directive, err := pattern.Directive(layout.WorktreePath(), stencilsDir, pattern.RoleImplementer)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: PlanSpec: %w", err)
	}

	parentDirective, err := parentdirective.Directive(stencilsDir, parentName, false)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: PlanSpec: parent directive: %w", err)
	}

	var frictionDir string
	if cfg.Friction != "" {
		frictionDir = LoomFrictionDir(layout)
	}
	notePath := friction.NotePath(frictionDir, "Plan-Write")
	frictionDirective, err := friction.Directive(notePath, stencilsDir, friction.RoleImplementer)
	if err != nil {
		// Unlike the pattern.Directive call immediately above, a friction.Directive error is
		// swallowed, never returned: it is optional bookkeeping, not a binding constraint, so a
		// transient stencil read failure here must never fail the whole Plan-Write spawn.
		logger.Warn("loom: friction directive failed, continuing without one", "role", "plan-write", "stencil", "loom-template-plan", "error", err)
		frictionDirective = ""
	}

	var twillDirective string
	twill := cfg.PlanProducer == PlanProducerTwill
	if twill {
		twillDirective, err = planTwillDirective(layout, dirs, stencilsDir)
		if err != nil {
			return shuttleengine.Spec{}, fmt.Errorf("loom: PlanSpec: %w", err)
		}
	}

	prompt, err := composePlanPrompt(stencilsDir, specsDir, decisionRecordPath, planDir, overviewPath, directive, frictionDirective, parentDirective, twillDirective)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: PlanSpec: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:        string(prompt),
		OutputFiles:   []string{overviewPath},
		Model:         resolved.Model,
		Effort:        resolved.Params["effort"],
		Version:       resolved.Params["version"],
		Interactive:   false,
		Role:          planRole,
		Segment:       segmentcolor.Plan,
		Skills:        planSkills,
		Timeout:       time.Duration(cfg.PlanTimeoutMin) * time.Minute,
		ForkSubagents: twill,
	}, nil
}

// planStencil is the stencil that opens the Plan-Write lead seat.
const planStencil = "loom-template-plan"

// PlanTable builds the seatengine.Table of one Plan-Write run: a lead that writes the plan directory's cards and, last, its overview.
// Under plan_producer twill the lead alone is authorized to spawn fork subagents and carries the twill lens review directive, whose twill-plan fan is resolved through burler.yaml under dirs; otherwise the prompt keeps its bytes.
// specsDir is told, never derived: this package is bound by the Told-Geometry Invariant.
// The seat's opening stencil is read at call time from stencilsDir, and the seat engine renders the parent and edit directives from its geometry.
// The table carries no gate; the producer entry sets it.
// A PATTERN directive that cannot be rendered fails the table; a friction directive that cannot be rendered is left empty, the failure logged.
func PlanTable(layout *lyxcwd.Location, dirs configengine.Dirs, stencilsDir, specsDir string, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	model, effort, version, err := resolveModelChoice(cfg.Plan, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PlanTable: plan role model-spec: %w", err)
	}

	patternDirective, err := pattern.Directive(layout.WorktreePath(), stencilsDir, pattern.RoleImplementer)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PlanTable: %w", err)
	}

	var frictionDir string
	if cfg.Friction != "" {
		frictionDir = LoomFrictionDir(layout)
	}
	frictionDirective, err := friction.Directive(friction.NotePath(frictionDir, "Plan-Write"), stencilsDir, friction.RoleImplementer)
	if err != nil {
		// A friction directive is optional bookkeeping, so a stencil read failure here must never fail the Plan-Write spawn.
		logger.Warn("loom: friction directive failed, continuing without one", "role", "plan-write", "stencil", planStencil, "error", err)
		frictionDirective = ""
	}

	twill := cfg.PlanProducer == PlanProducerTwill
	var leadValues map[string]string
	if twill {
		twillDirective, err := planTwillDirective(layout, dirs, stencilsDir)
		if err != nil {
			return seatengine.Table{}, fmt.Errorf("loom: PlanTable: %w", err)
		}
		leadValues = map[string]string{twilldirective.MarkerName: setOffParagraph(twillDirective)}
	}

	overviewPath := planparser.PlanOverview(layout.AnchorPath())
	return seatengine.Table{
		RolePrefix: planRole,
		Segment:    segmentcolor.Plan,
		Timeout:    time.Duration(cfg.PlanTimeoutMin) * time.Minute,
		Values: map[string]string{
			"decision_record_path": DiscussionDecisionRecord(layout),
			"plan_dir":             planparser.PlanDir(layout.AnchorPath()),
			"overview_path":        overviewPath,
			"specs_dir":            specsDir,
			"pattern_directive":    patternDirective,
			friction.MarkerName:    frictionDirective,
		},
		Optional: []string{"pattern_directive", friction.MarkerName, twilldirective.MarkerName},
		Seats: []seatengine.Seat{{
			Stencil:       planStencil,
			Model:         model,
			Effort:        effort,
			Version:       version,
			Outputs:       []string{overviewPath},
			Skills:        planSkills,
			Values:        leadValues,
			ForkSubagents: twill,
		}},
	}, nil
}
