// plan.go implements PlanTable, the Plan-Write row's seat-table builder.
// The Plan producer, like the Discussion producer in discussion.go, is not a Go module.
// It is a prompt and profile fed to the seat engine: one lead seat produces one artifact.
// The decision record, `_lym/discussion/decision-record.md` (see layout.DiscussionDecisionRecord), is its design input;
// `_lym/discussion/relevant.md` is its head start on the code when present, never scope.
// It reads nothing else from `_lym/discussion/`, never the support log, and never reads the board.
// Its output is a plan-format flat-card plan written into `_lym/plan/`:
// one `NN-<card-slug>.md` per card plus `00-overview.md`, written last as the run's done-sentinel.
// The format is in contracts/specs/loom-plan-spec.md.
// The producer always writes `approved: false` in the frontmatter of `00-overview.md`.
// It has no review logic of its own beyond the twill lens review below;
// the plan review is the Bouncer+Burler segment's separate job.
// The approved settle of `Plan-Bouncer` flips `approved` to `true`, immediately before its commit seam fires.
//
// With plan_lenses set, the lead carries the twill directive rendered into the plan template between Step 2 and Step 3.
// Fork subagents are authorized.
// The directive's lenses come from the lenses.yaml fan plan_lenses names.
// The lead explores, spawns one fork, fork0, to write the plan, reads the finished plan in full and self-checks it;
// then it spawns the lens forks and judges their findings as a critical fixer.
// The forks hold the exploration and the plan but not fork0's reasoning;
// the lead records the lens review in the plan directory's lens-review.md.
// The writer runs `lym loom validate-plan --lens-review` as its final self-check,
// and the row's gate requires a recorded lens-review.md for a run whose lead was authorized to fork.
// The lead sets ForkSubagents, so the writer may spawn subagents of type fork.
// Forks run on the writer's model, read-only and without the Agent tool by the stencil's boilerplate.
// The plan gates and the Plan-Review segment still bound what lands in the plan.
// With plan_lenses empty, the prompt keeps its bytes and ForkSubagents stays false.
//
// PlanTable is a pure builder: it does not stat the decision record,
// does not stat or create `_lym/plan/`, and does not spawn anything.
// Rotating a stale `_lym/plan/` before a re-run is owned by `loomshed.NewPlanDirRotator`;
// it runs as the Plan-Write producer's fresh-spawn preparation.
// Verifying the input exists stays outside PlanTable.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/twilldirective"
)

// planRole is the agent-name role this module's Plan-Write spawn carries.
const planRole = "plan"

// planSkills are the skills the Plan-Write spawn loads, in order.
var planSkills = []string{"scribe:prose", "scribe:testing"}

// planStencil is the stencil that opens the Plan-Write lead seat.
const planStencil = "loom-template-plan"

// planSelfCheckStencil and planSelfCheckGatedStencil are the fragments that fill the plan template's self-check step.
// The first serves a writer that runs the validator itself; the second serves one whose gate has a cold fixer.
const (
	planSelfCheckStencil      = "loom-fragment-self-check-plan"
	planSelfCheckGatedStencil = "loom-fragment-self-check-plan-gated"
)

// PlanTable builds the seatengine.Table of one Plan-Write run.
// The lead writes the plan directory's cards and, last, its overview.
// When cfg.PlanLenses is set, only the lead is authorized to spawn fork subagents,
// and the lead carries the twill lens review directive, whose fan is resolved through lenses.yaml under dirs;
// otherwise the prompt keeps its bytes.
// specsDir is told, never derived: this package is bound by the Told-Geometry Invariant.
// The seat's opening stencil is read at call time from stencilsDir,
// and the seat engine renders the parent and edit directives from its geometry.
// gateTiers is the hub's rendered tier and gate-list lines, or "none" when no tier is declared;
// it fills the prompt's gate_tiers marker verbatim.
// cfg.SubagentModel names the model of the fresh subagents the lead's prompt tells it to hand lookups to.
// The table carries no gate; the producer entry sets it.
// A pattern directive that cannot be rendered fails the table;
// a friction directive that cannot be rendered is left empty, and the failure is logged.
// fixerOn reports that a cold fixer clears the row's plan gate,
// so the prompt tells the writer to run no validator and the twill directive asks for none;
// false keeps the writer's own validator run.
func PlanTable(layout *lymcwd.Location, dirs configengine.Dirs, stencilsDir, specsDir string, cfg Config, reg modelspec.Registry, gateTiers string, fixerOn bool) (seatengine.Table, error) {
	choice, err := resolveModelChoice(cfg.Plan, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PlanTable: plan role model-spec: %w", err)
	}

	declaredChanges, err := discussionparser.DeclaredPatternChanges(DiscussionDecisionRecord(layout))
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PlanTable: %w re-step the row with %q", err, "lym loom start")
	}
	patternDirective, err := pattern.Directive(layout.WorktreePath(), stencilsDir, pattern.RoleImplementer, declaredChanges)
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

	selfCheck, err := selfCheckBlock(stencilsDir, fixerOn, planSelfCheckStencil, planSelfCheckGatedStencil)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: PlanTable: self-check step: %w", err)
	}
	forks := cfg.PlanLenses != ""
	leadValues := map[string]string{"self_check": selfCheck}
	if forks {
		lensDirective, err := planLensDirective(layout, dirs, stencilsDir, specsDir, cfg.PlanLenses, fixerOn)
		if err != nil {
			return seatengine.Table{}, fmt.Errorf("loom: PlanTable: %w", err)
		}
		leadValues[twilldirective.MarkerName] = setOffParagraph(lensDirective)
	}

	lead := seatengine.Seat{
		Stencil:       planStencil,
		ModelChoice:   choice,
		ModelKey:      shuttleengine.ModelKey{Module: loomModule, Key: "plan"},
		Outputs:       []string{planparser.PlanOverview(layout.AnchorPath())},
		Skills:        planSkills,
		Values:        leadValues,
		ForkSubagents: forks,
	}
	if forks {
		// The lens directive tells the lead to fork and to end its turn while forks are outstanding, which a provider must wake it from.
		lead.Requires = []shuttleengine.Requirement{shuttleengine.RequirementBackgroundWake}
	}

	overviewPath := lead.Outputs[0]
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
			"gate_tiers":           gateTiers,
			subagentModelKey:       cfg.SubagentModel,
			"pattern_directive":    patternDirective,
			friction.MarkerName:    frictionDirective,
		},
		Optional: []string{"pattern_directive", friction.MarkerName, twilldirective.MarkerName},
		Seats:    []seatengine.Seat{lead},
	}, nil
}
