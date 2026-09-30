// entries_prrework.go implements prReworkEntry, the Constructor for the "PRRework" registry row: it
// wraps a gated shedadapters.SingleLLMProducer in loomshed.NewPRRework's append-only-check and
// round-record decorator, so it lives in its own file like planWriteEntry.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// prReworkEntry is the Constructor for the "PRRework" registry row: it resolves the row's
// "gate"/"gate_attempts" Config keys through resolveGateSpec, validates Env.ReworkSpec,
// Env.Shuttle, every seam of Env.Rework, and the absolute Env.Rework.PlanDir and
// Env.Rework.ReworkDir, then builds a gated SingleLLMProducer behind loomshed.NewPRRework.
//
// No fresh-spawn preparation is passed: the adapter already archives a stale coverage file on a
// fresh spawn, and nothing else may be moved out from under the plan.
//
// The Spec arrives as an injected shedadapters.SpecSource for the reason planWriteEntry gives.
func prReworkEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("PRRework", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gate", "gate_attempts"); err != nil {
		return nil, err
	}
	if err := requireSeam("PRRework", "ReworkSpec", env.ReworkSpec); err != nil {
		return nil, err
	}
	if err := requireSeam("PRRework", "Shuttle", env.Shuttle); err != nil {
		return nil, err
	}
	rw := env.Rework
	seams := []struct {
		field string
		seam  any
	}{
		{"Rework.ReadCommitted", rw.ReadCommitted},
		{"Rework.ReadRejection", rw.ReadRejection},
		{"Rework.ClearRejection", rw.ClearRejection},
		{"Rework.Commit", rw.Commit},
		{"Rework.Rebaseline", rw.Rebaseline},
	}
	for _, s := range seams {
		if err := requireSeam("PRRework", s.field, s.seam); err != nil {
			return nil, err
		}
	}
	if err := requireAbsRoot("PRRework", "Rework.PlanDir", rw.PlanDir); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("PRRework", "Rework.ReworkDir", rw.ReworkDir); err != nil {
		return nil, err
	}
	inner := shedadapters.NewSingleLLMProducerGated(name, env.ReworkSpec, env.Shuttle, env.Now, nil, gate)
	return loomshed.NewPRRework(name, inner, rw), nil
}
