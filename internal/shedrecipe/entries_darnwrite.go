// entries_darnwrite.go implements darnWriteEntry, the Constructor for the "DarnWrite" registry row:
// it wraps a gated shedadapters.SingleLLMProducer in loomshed.NewDarnWrite's commit and rejection decorator, so it lives in its own file like prReworkEntry.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// darnWriteEntry is the Constructor for the "DarnWrite" registry row:
// it resolves the row's "gates" Config key through resolveGateSpec, validates Env.DarnSpec, Env.Shuttle and every seam of Env.Darn, then builds a gated SingleLLMProducer behind loomshed.NewDarnWrite.
//
// The Spec arrives as an injected func(loomshed.DarnTold) for the reason planWriteEntry gives, evaluated with the values the producer tells each session it builds.
func darnWriteEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("DarnWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireSeam("DarnWrite", "DarnSpec", env.DarnSpec); err != nil {
		return nil, err
	}
	if err := requireSeam("DarnWrite", "Shuttle", env.Shuttle); err != nil {
		return nil, err
	}
	dw := env.Darn
	seams := []struct {
		field string
		seam  any
	}{
		{"Darn.ReadRejection", dw.ReadRejection},
		{"Darn.ClearRejection", dw.ClearRejection},
		{"Darn.Commit", dw.Commit},
		{"Darn.LatestOutcome", dw.LatestOutcome},
		{"Darn.PublishFailure", dw.PublishFailure},
	}
	for _, s := range seams {
		if err := requireSeam("DarnWrite", s.field, s.seam); err != nil {
			return nil, err
		}
	}
	session := func(told loomshed.DarnTold) shedengine.ShedProducer {
		spec := func() (shuttleengine.Spec, error) { return env.DarnSpec(told) }
		return shedadapters.NewSingleLLMProducerGated(name, spec, env.Shuttle, env.Now, nil, gate)
	}
	return loomshed.NewDarnWrite(name, session, dw), nil
}
