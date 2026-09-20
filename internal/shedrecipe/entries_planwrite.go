// entries_planwrite.go implements planWriteEntry, the Constructor for the "PlanWrite" registry
// row: it wraps a shedadapters.SingleLLMProducer in loomshed.NewPlanWrite's rotate-and-commit
// decorator, so it lives in its own file rather than in entries_simple.go, whose header comment
// describes only the plain single-constructor shape.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// planWriteEntry is the Constructor for the "PlanWrite" registry row: it validates Env.PlanSpec,
// Env.CommitPlan, Env.Shuttle, and Env.AnchorPath, resolves the row's "gate"/"gate_attempts"
// Config keys through resolveGateSpec, then builds a gated SingleLLMProducer carrying
// loomshed.NewPlanDirRotator as its fresh-spawn preparation, behind loomshed.NewPlanWrite's
// post-Done commit decorator.
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
// plan directory itself via planparser.PlanDir, the same split planValidateEntry already uses,
// which keeps this package free of any planparser import.
//
// The row carries exactly two Config keys, "gate" and "gate_attempts", per the Config Strictness
// Invariant. It carries a "gate" key even though this entry's own dedicated constructor could
// imply the validator, so that one key means one thing at all four gated sites and a reader of the
// recipe can see which validator guards each row without opening Go; this entry therefore never
// hard-codes a validator choice of its own.
func planWriteEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("PlanWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gate", "gate_attempts"); err != nil {
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
	// The rotation is handed to the producer as its fresh-spawn preparation, never run as a step
	// ahead of it: it must not touch _lyx/plan until the producer's own attach probe has proved no
	// live plan agent is writing there. See loomshed.NewPlanDirRotator.
	rotate := loomshed.NewPlanDirRotator(env.AnchorPath, env.Now)
	inner := shedadapters.NewSingleLLMProducerGated(name, env.PlanSpec, env.Shuttle, env.Now, rotate, gate)
	return loomshed.NewPlanWrite(name, inner, env.CommitPlan), nil
}
