// entries_planwrite.go implements planWriteEntry, the Constructor for the "PlanWrite" registry row:
// it runs the told one-seat table of the Plan-Write lead on a shedadapters.MultiLLMProducer behind loomshed.NewPlanWrite's commit decorator,
// so it lives in its own file rather than in entries_simple.go, whose header comment describes only the plain single-constructor shape.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// planWriteEntry is the Constructor for the "PlanWrite" registry row: it validates Env.PlanTable, Env.CommitPlan, Env.Seats, Env.AnchorPath, Env.StencilsDir and Env.Rework.ArchiveWebster, resolves the row's "gates" Config key through resolveGateSpec, then builds a seat-table producer carrying loomshed.NewPlanDirRotator as its fresh-spawn preparation, behind loomshed.NewPlanWrite's post-Done commit decorator.
// Env.Shuttle is never read: the lead runs through Env.Seats.
//
// The table arrives as a shedadapters.TableSource closure evaluated once per Call, so the lead's stencil is read at call time.
// It is a closure rather than recipe Config because building the table needs a *lyxcwd.Location, which the Shed Recipe Registry Invariant bars this package from importing directly; internal/loomcli's wire() supplies it.
// The resolved gate is stamped onto each table the source returns, so the row's "gates" key guards the lead whatever table the source builds.
//
// AnchorPath is validated here and threaded through because loomshed.NewPlanDirRotator resolves the
// plan directory itself via planparser.PlanDir, the same split loomshed.NewPlanGate uses between
// that call and planglyph.ValidateFormat's separate worktree-root parameter, which keeps this
// package free of any planparser import.
//
// The row carries exactly one Config key, "gates", per the Config Strictness Invariant.
// It carries a "gates" key even though this entry's own dedicated constructor could imply the validator, so that one key means one thing at every gated site and a reader of the recipe can see which validator guards each row without opening Go;
// the key still names the validator.
// The entry adds only the lens-review closure that shuttle evaluates in place of the "plan" closure for a run whose lead carries fork authorization, which the table sets exactly when plan_lenses is set.
func planWriteEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := planWriteGateSpec(cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireSeam("PlanWrite", "PlanTable", env.PlanTable); err != nil {
		return nil, err
	}
	if err := requireSeam("PlanWrite", "CommitPlan", env.CommitPlan); err != nil {
		return nil, err
	}
	if err := requireSeam("PlanWrite", "Seats", env.Seats); err != nil {
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
	// ahead of it: it must not touch _lyx/plan until the producer's own seat probe has proved no
	// live plan agent is writing there. See loomshed.NewPlanDirRotator.
	rotate := loomshed.NewPlanDirRotator(env.AnchorPath, env.StencilsDir, env.Rework.ArchiveWebster, env.Now)
	tables := func() (seatengine.Table, error) {
		table, err := env.PlanTable()
		if err != nil {
			return seatengine.Table{}, err
		}
		table.Gate = gate
		return table, nil
	}
	inner := shedadapters.NewMultiLLMProducerSourced(name, tables, env.Seats, env.Now, rotate)
	return loomshed.NewPlanWrite(name, inner, env.CommitPlan), nil
}

// planWriteGateSpec resolves the PlanWrite row's "gates" key through resolveGateSpec, then sets Forked on every resolved entry named "plan" to loomshed.NewLensReviewPlanGate over the same roots and index.
// Forked is consulted only for a run whose lead carries fork authorization, which loom.yaml's plan_lenses sets, and the closure runs the plan gate's own set plus one presence check.
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
