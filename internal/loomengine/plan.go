// plan.go implements PlanTable, the Plan-Write row's seat-table builder.
// Like the Discussion producer (discussion.go), the Plan producer is not a Go module — it is a
// prompt/profile fed to the seat engine, one lead seat producing one artifact.
// The decision record, `_lyx/discussion/decision-record.md` (see layout.DiscussionDecisionRecord), is its design input;
// `_lyx/discussion/relevant.md` is its head start on the code when present, never scope.
// It reads nothing else from `_lyx/discussion/`, never the support log, and never reads the board.
// Its output is a plan-format flat-card plan written into `_lyx/plan/`: one `NN-<card-slug>.md`
// per card plus `00-overview.md`, written LAST as the run's done-sentinel (see
// contracts/specs/loom-plan-spec.md).
// The producer always writes `approved: false` in `00-overview.md`'s frontmatter — it has no review
// logic of its own beyond the twill lens review below (the Bouncer+Burler segment's separate job); `Plan-Bouncer`'s approved
// settle is what flips `approved` to `true`, immediately before its commit seam fires.
//
// With plan_lenses set the lead carries the twill directive rendered into the plan template between Step 2 and Step 3, and fork subagents are authorized.
// The directive's lenses come from the burler.yaml fan plan_lenses names.
// The lead explores, spawns one fork, fork0, to write the plan, reads the finished plan in full and self-checks it, then spawns the lens forks and judges their findings as a critical fixer.
// The forks hold the exploration and the plan but not fork0's reasoning, and the lead records the lens review in the plan directory's lens-review.md.
// The writer runs `lyx loom validate-plan --lens-review` as its final self-check, and the row's gate requires a recorded lens-review.md for a run whose lead was authorized to fork.
// The lead sets ForkSubagents, so the writer may spawn subagents of type fork.
// While the shuttle config's claude_deny_agent_tool is true, the Agent hook admits fork subagents only and refuses every other subagent type; with it false no Agent hook is installed.
// Forks run on the writer's model, read-only and without the Agent tool by the stencil's boilerplate.
// The plan gates and the Plan-Review segment still bound what lands in the plan.
// With plan_lenses empty the prompt keeps its bytes and ForkSubagents stays false.
//
// PlanTable is a pure builder: it does not stat the decision record,
// does not stat or create `_lyx/plan/`, and does not spawn anything.
// Rotating a stale `_lyx/plan/` before a re-run is owned by `loomshed.NewPlanDirRotator`, run as the Plan-Write producer's fresh-spawn preparation;
// verifying the input exists stays outside PlanTable.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/twilldirective"
)

// planRole is the agent-name role this module's Plan-Write spawn carries.
const planRole = "plan"

// planSkills are the skills the Plan-Write spawn loads, in order.
var planSkills = []string{"scribe:prose", "scribe:testing"}

// planStencil is the stencil that opens the Plan-Write lead seat.
const planStencil = "loom-template-plan"

// PlanTable builds the seatengine.Table of one Plan-Write run: a lead that writes the plan directory's cards and, last, its overview.
// When cfg.PlanLenses is set the lead alone is authorized to spawn fork subagents and carries the twill lens review directive, whose fan is resolved through burler.yaml under dirs; otherwise the prompt keeps its bytes.
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

	forks := cfg.PlanLenses != ""
	var leadValues map[string]string
	if forks {
		lensDirective, err := planLensDirective(layout, dirs, stencilsDir, cfg.PlanLenses)
		if err != nil {
			return seatengine.Table{}, fmt.Errorf("loom: PlanTable: %w", err)
		}
		leadValues = map[string]string{twilldirective.MarkerName: setOffParagraph(lensDirective)}
	}

	overviewPath := planparser.PlanOverview(layout.AnchorPath())
	return seatengine.Table{
		RolePrefix: planRole,
		Segment:    segmentcolor.Plan,
		Timeout:    time.Duration(cfg.PlanTimeoutMin) * time.Minute,
		Values: map[string]string{
			"decision_record_path": DiscussionDecisionRecord(layout),
			"relevant_path":        DiscussionRelevant(layout),
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
			ForkSubagents: forks,
		}},
	}, nil
}
