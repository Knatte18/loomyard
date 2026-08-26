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
// logic of its own (that is the Bouncer+Burler segment's separate job); `Plan-Bouncer`'s approved
// settle is what flips `approved` to `true`, immediately before its commit seam fires.
//
// PlanSpec is a pure composer, exactly like DiscussionSpec: it does not stat the decision record,
// does not stat or create `_lyx/plan/`, and does not spawn anything.
// Verifying the input exists and rotating a stale `_lyx/plan/` before a re-run are the future loom
// phase machine's responsibility.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// composePlanPrompt builds the Plan producer's prompt by reading the "loom-template-plan" stencil
// from stencilsDir and filling it.
func composePlanPrompt(stencilsDir, decisionRecordPath, planDir, overviewPath, patternDirective string) ([]byte, error) {
	template, err := stencilstore.Read(stencilsDir, "loom-template-plan")
	if err != nil {
		return nil, err
	}

	values := map[string]string{
		"decision_record_path": decisionRecordPath,
		"plan_dir":             planDir,
		"overview_path":        overviewPath,
		"pattern_directive":    patternDirective,
	}

	rendered, err := stencil.FillOptional(template, values, []string{"pattern_directive"})
	if err != nil {
		return nil, fmt.Errorf("loom: compose plan prompt: %w", err)
	}
	return rendered, nil
}

// PlanSpec builds the shuttleengine.Spec for one Plan producer run.
func PlanSpec(layout *lyxcwd.Location, stencilsDir string, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error) {
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

	directive, err := pattern.Directive(layout.AnchorPath(), stencilsDir, pattern.RoleImplementer)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: PlanSpec: %w", err)
	}
	prompt, err := composePlanPrompt(stencilsDir, decisionRecordPath, planDir, overviewPath, directive)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: PlanSpec: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{overviewPath},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Interactive: false,
		Role:        "plan",
		Timeout:     time.Duration(cfg.PlanTimeoutMin) * time.Minute,
	}, nil
}
