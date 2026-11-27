// entries_planwrite.go holds planWriteEntry, the Constructor for the "PlanWrite" registry row.
// It runs the told one-seat table of the Plan-Write lead on a shedadapters.MultiLLMProducer.
// A loomshed.NewPlanWrite commit decorator wraps that producer.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// planWriteEntry is the Constructor for the "PlanWrite" registry row.
// It validates Env.PlanTable, Env.CommitPlan, Env.Seats, Env.AnchorPath, Env.StencilsDir and Env.Rework.ArchiveWebster.
// It resolves the row's "gates" Config key through resolveGateSpec.
// It then builds a seat-table producer with loomshed.NewPlanDirRotator as its fresh-spawn preparation.
// The producer sits behind loomshed.NewPlanWrite's post-Done commit decorator.
// Env.Shuttle is never read: the lead runs through Env.Seats.
//
// The table arrives as a shedadapters.TableSource closure, evaluated once per Call,
// so the lead's stencil is read at call time.
// The table is a closure rather than recipe Config, because building it needs a *lymcwd.Location.
// The Shed Recipe Registry Invariant bars this package from importing that directly;
// internal/loomcli's wire() supplies the source.
// The resolved gate is stamped onto each table the source returns.
// The "gates" key therefore guards the lead whatever table the source builds.
//
// AnchorPath is validated here and threaded through.
// loomshed.NewPlanDirRotator resolves the plan directory itself through planparser.PlanDir.
// Threading AnchorPath through keeps this package free of any planparser import.
//
// The row carries exactly one Config key, "gates", per the Config Strictness Invariant.
// The key names the validator, although the constructor alone could imply it.
// A reader of the recipe then sees which validator guards each row without opening Go.
//
// The entry adds only the lens-review closure, which shuttle evaluates in place of the "plan" closure.
// It runs only for a run whose lead carries fork authorization.
// The table sets that authorization exactly when tapestry.yaml's plan_lenses is set.
//
// A "fixer" key on a gate element is read through splitGateFixer.
// The fixer's Forked follows the tapestry.plan_lenses setting,
// and its SkipForked leaves a failing forked evaluation that only lacks the lens review to the writer.
func planWriteEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	cfg, fixer, err := splitGateFixer("PlanWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	gate, err := planWriteGateSpec(cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if fixer != nil {
		fixer.Forked, err = planLensesSet(env)
		if err != nil {
			return nil, err
		}
		fixer.SkipForked = loomshed.LensReviewOnly
	}
	gate, err = applyGateFixer(row.Name, gate, fixer, env)
	if err != nil {
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
	// The rotation is the producer's fresh-spawn preparation, never a step ahead of it.
	// It must not touch _lym/plan until the producer's own seat probe has proved no live plan agent is writing there.
	rotate := loomshed.NewPlanDirRotator(env.AnchorPath, env.StencilsDir, env.Rework.ArchiveWebster, env.Now)
	tables := func() (seatengine.Table, error) {
		table, err := env.PlanTable()
		if err != nil {
			return seatengine.Table{}, err
		}
		table.Gate = gate
		return table, nil
	}
	inner := shedadapters.NewMultiLLMProducerSourced(row.Name, tables, env.Seats, env.Now, rotate).KeepGateFindings(env.gateFindingsPath(row.Name))
	return loomshed.NewPlanWrite(row.Name, inner, env.CommitPlan), nil
}

// planLensesSet reports whether the tapestry.plan_lenses setting is non-empty.
// A non-empty setting gives the Plan-Write lead fork authorization.
func planLensesSet(env Env) (bool, error) {
	value, err := env.Setting("tapestry", "plan_lenses")
	if err != nil {
		return false, fmt.Errorf("shedrecipe: PlanWrite: setting tapestry.plan_lenses: %w", err)
	}
	lenses, ok := value.(string)
	if !ok {
		return false, fmt.Errorf("shedrecipe: PlanWrite: setting tapestry.plan_lenses holds %v, want a string; way forward: set plan_lenses in tapestry.yaml to a fan name or \"\"", value)
	}
	return lenses != "", nil
}

// planWriteGateSpec resolves the PlanWrite row's "gates" key and sets Forked on each entry named "plan".
// The Forked closure is loomshed.NewLensReviewPlanGate over the same roots and index.
// Forked runs only for a run whose lead carries fork authorization, which plan_lenses in tapestry.yaml sets.
// The closure runs the plan gate's own checks plus one presence check.
// The "plan" entry's resolution has already checked the roots and the index.
func planWriteGateSpec(cfg Config, env Env) (shuttleengine.GateSpec, error) {
	spec, err := resolveGateSpec("PlanWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	for i := range spec {
		if spec[i].Name == "plan" {
			spec[i].Forked = loomshed.NewLensReviewPlanGate(env.AnchorPath, env.WorktreeRoot, env.RunScratchDir, env.PlanIndex, planContextOf(env))
		}
	}
	return spec, nil
}
