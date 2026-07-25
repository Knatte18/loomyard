// plan.go implements PlanSpec, the Plan producer's Spec factory, and its
// composePlanPrompt prompt composer. Like the Discussion producer
// (discussion.go), the Plan producer is not a Go module — it is a
// prompt/profile fed to shuttle.Run, one shuttle.Run producing one
// artifact. Its sole input is `_lyx/discussion/decision-record.md` (see
// layout.DiscussionDecisionRecord); it never reads the support log and
// never reads the board. Its output is a plan-format-v3 flat-card plan
// written into `_lyx/plan/`: one `NN-<card-slug>.md` per card plus
// `00-overview.md`, written LAST as the run's done-sentinel (see
// docs/reference/plan-format-v3.md). The producer always writes
// `approved: false` in `00-overview.md`'s frontmatter — it has no review
// logic of its own (that is perch/burler's separate job); flipping
// `approved` to `true` after perch returns APPROVED is the future loom
// orchestrator's job, not built here.
//
// PlanSpec is a pure composer, exactly like DiscussionSpec: it does not
// stat the decision record, does not stat or create `_lyx/plan/`, and does
// not spawn anything. Verifying the input exists and rotating a stale
// `_lyx/plan/` before a re-run are the future loom phase machine's
// responsibility.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// composePlanPrompt builds the Plan producer's prompt by composing the
// template's three top-level marker values (the decision record path, the
// plan directory the agent writes into, and the overview file path it must
// write last) and filling planTemplate with them via stencil.Fill. Unlike
// composePrompt, there is no mode-specific branch to compose: the Plan
// producer is autonomous-only, so plan-template.md carries a single,
// unconditional instruction set.
func composePlanPrompt(decisionRecordPath, planDir, overviewPath string) ([]byte, error) {
	values := map[string]string{
		"decision_record_path": decisionRecordPath,
		"plan_dir":             planDir,
		"overview_path":        overviewPath,
	}

	rendered, err := stencil.Fill(planTemplate, values)
	if err != nil {
		return nil, fmt.Errorf("loom: compose plan prompt: %w", err)
	}
	return rendered, nil
}

// PlanSpec builds the shuttleengine.Spec for one Plan producer run against
// layout, using cfg's plan role model-spec and timeout knob and reg to
// resolve that model-spec. Unlike DiscussionSpec, PlanSpec takes no slug
// and no autonomous param: the Plan producer's sole input is
// layout.DiscussionDecisionRecord() (self-contained, no board read), and
// it is always autonomous (Interactive is always false).
//
// The Resolved→Spec field mapping mirrors DiscussionSpec's: Spec.Model =
// resolved.Model, Spec.Effort = resolved.Params["effort"], Spec.Version =
// resolved.Params["version"].
//
// PlanSpec does not stat or create any file (see the package doc comment
// above): shuttleengine's Spec.validate rejects a Spec naming a
// pre-existing output file, and creating `_lyx/plan/` is the plan agent's
// own write concern (see plan-template.md's Step 3).
func PlanSpec(layout *hubgeometry.Layout, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error) {
	// Resolve the plan role's model-spec now, before composing the prompt
	// or naming output paths, so an unknown alias or malformed spec fails
	// loud before anything else about this run is constructed.
	spec, err := modelspec.Parse(cfg.Plan)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: PlanSpec: plan role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("loom: PlanSpec: plan role model-spec: %w", err)
	}

	decisionRecordPath := layout.DiscussionDecisionRecord()
	planDir := layout.PlanDir()
	overviewPath := layout.PlanOverview()

	prompt, err := composePlanPrompt(decisionRecordPath, planDir, overviewPath)
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
