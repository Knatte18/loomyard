// entries_planwrite.go implements planWriteEntry, the Constructor for the "PlanWrite" registry
// row: it wraps a shedadapters.SingleLLMProducer in loomshed.NewPlanWrite's rotate-and-commit
// decorator, so it lives in its own file rather than in entries_simple.go, whose header comment
// describes only the plain single-constructor shape.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// planWriteEntry is the Constructor for the "PlanWrite" registry row: it validates Env.PlanSpec, Env.CommitPlan, Env.Shuttle, Env.AnchorPath, Env.StencilsDir and Env.Rework.ArchiveWebster, resolves the row's "gates" Config key through resolveGateSpec, then builds a gated SingleLLMProducer carrying loomshed.NewPlanDirRotator as its fresh-spawn preparation, behind loomshed.NewPlanWrite's post-Done commit decorator.
//
// The Spec arrives as an injected shedadapters.SpecSource closure rather than as recipe Config
// because building it needs a *lyxcwd.Location, which the Shed Recipe Registry Invariant bars this
// package from importing directly; internal/loomcli's wire() is what supplies the closure.
//
// The generic "SingleLLM" entry is not reused here for two reasons: building the Spec needs a
// *lyxcwd.Location the Shed Recipe Registry Invariant bars this package from importing, and a
// generic row's own model/effort Config keys would bypass the "plan" role's own model-spec
// resolution and its plan_timeout_min timeout entirely.
//
// AnchorPath is validated here and threaded through because loomshed.NewPlanDirRotator resolves the
// plan directory itself via planparser.PlanDir, the same split loomshed.NewPlanGate uses between
// that call and planglyph.ValidateFormat's separate worktree-root parameter, which keeps this
// package free of any planparser import.
//
// The row carries exactly one Config key, "gates", per the Config Strictness Invariant.
// It carries a "gates" key even though this entry's own dedicated constructor could imply the validator, so that one key means one thing at every gated site and a reader of the recipe can see which validator guards each row without opening Go;
// the key still names the validator.
// The entry adds only the lens-review closure that shuttle evaluates in place of the "plan" closure for a run whose record carries fork authorization, which PlanSpec sets exactly under plan_producer: twill.
func planWriteEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := planWriteGateSpec(cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireSeam("PlanWrite", "PlanSpec", env.PlanSpec); err != nil {
		return nil, err
	}
	if err := requireSeam("PlanWrite", "CommitPlan", env.CommitPlan); err != nil {
		return nil, err
	}
	if err := requireSeam("PlanWrite", "Shuttle", env.Shuttle); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("PlanWrite", "AnchorPath", env.AnchorPath); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("PlanWrite", "StencilsDir", env.StencilsDir); err != nil {
		return nil, err
	}
	if err := requireSeam("PlanWrite", "Rework.ArchiveWebster", env.Rework.ArchiveWebster); err != nil {
		return nil, err
	}
	// The rotation is handed to the producer as its fresh-spawn preparation, never run as a step
	// ahead of it: it must not touch _lyx/plan until the producer's own attach probe has proved no
	// live plan agent is writing there. See loomshed.NewPlanDirRotator.
	rotate := loomshed.NewPlanDirRotator(env.AnchorPath, env.StencilsDir, env.Rework.ArchiveWebster, env.Now)
	inner := shedadapters.NewSingleLLMProducerGated(name, env.PlanSpec, env.Shuttle, env.Now, rotate, gate)
	return loomshed.NewPlanWrite(name, inner, env.CommitPlan), nil
}

// planWriteGateSpec resolves the PlanWrite row's "gates" key through resolveGateSpec, then sets Forked on every resolved entry named "plan" to loomshed.NewLensReviewPlanGate over the same roots and index.
// Forked is consulted only for a fork-authorized run of this row, and the closure runs the plan gate's own set plus one presence check.
// The "plan" entry's resolution has already required the roots and the index, so the closure is built over checked values.
func planWriteGateSpec(cfg Config, env Env) (shuttleengine.GateSpec, error) {
	spec, err := resolveGateSpec("PlanWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	for i := range spec {
		if spec[i].Name == "plan" {
			spec[i].Forked = loomshed.NewLensReviewPlanGate(env.AnchorPath, env.WorktreeRoot, env.RunScratchDir, env.PlanIndex)
		}
	}
	return spec, nil
}
