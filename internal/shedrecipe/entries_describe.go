// entries_describe.go implements describeEntry, the Constructor for the "Describe" registry row:
// it wraps a shedadapters.SingleLLMProducer in loomshed.NewDiscussionWrite's commit decorator, so
// the change description is committed by the loop owner once the gate passes.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// describeEntry is the Constructor for the "Describe" registry row: it resolves the row's "gates"
// Config key through resolveGateSpec, validates Env.DescribeSpec,
// Env.CommitDescription, and Env.Shuttle, then returns
// loomshed.NewDiscussionWrite(name, shedadapters.NewSingleLLMProducerGated(name, env.DescribeSpec,
// env.Shuttle, env.Now, nil, gate), env.CommitDescription).
//
// The DiscussionWrite decorator is reused rather than copied: its commit-on-non-empty-pointer rule
// is the loop owner's commit of the description, and on gate exhaustion it commits the refused
// file so a halted run leaves a clean, diagnosable tree. The shared SingleLLM adapter already
// archives a stale description before a fresh spawn.
//
// The Spec arrives as an injected shedadapters.SpecSource closure because building it needs a
// *lyxcwd.Location, which the Shed Recipe Registry Invariant bars this package from importing.
func describeEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("Describe", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireSeam("Describe", "DescribeSpec", env.DescribeSpec); err != nil {
		return nil, err
	}
	if err := requireSeam("Describe", "CommitDescription", env.CommitDescription); err != nil {
		return nil, err
	}
	if err := requireSeam("Describe", "Shuttle", env.Shuttle); err != nil {
		return nil, err
	}
	inner := shedadapters.NewSingleLLMProducerGated(name, env.DescribeSpec, env.Shuttle, env.Now, nil, gate)
	return loomshed.NewDiscussionWrite(name, inner, env.CommitDescription), nil
}
