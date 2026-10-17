// entries_describe.go implements describeEntry, the Constructor for the "Describe" registry row:
// it runs the told one-seat table of the Describe lead on a shedadapters.MultiLLMProducer behind loomshed.NewDiscussionWrite's commit decorator, so
// the change description is committed by the loop owner once the gate passes.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// describeEntry is the Constructor for the "Describe" registry row: it resolves the row's "gates" Config key through resolveGateSpec, validates Env.DescribeTable, Env.CommitDescription, and Env.Seats, then returns loomshed.NewDiscussionWrite(name, shedadapters.NewMultiLLMProducerSourced(name, tables, env.Seats, env.Now, nil), env.CommitDescription) -- a seat-table producer behind a commit decorator.
// Env.Shuttle is never read: the lead runs through Env.Seats.
//
// The DiscussionWrite decorator is reused rather than copied: its commit-on-non-empty-pointer rule
// is the loop owner's commit of the description, and on gate exhaustion it commits the refused
// file so a halted run leaves a clean, diagnosable tree. The seat producer already
// archives a stale description before a fresh spawn.
//
// The table arrives as an injected shedadapters.TableSource closure because building it needs the fabric's landing geometry, which the Shed Recipe Registry Invariant bars this package from deriving.
// The resolved gate is stamped onto each table the source returns, so the row's "gates" key guards the lead.
func describeEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("Describe", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireSeam("Describe", "DescribeTable", env.DescribeTable); err != nil {
		return nil, err
	}
	if err := requireSeam("Describe", "CommitDescription", env.CommitDescription); err != nil {
		return nil, err
	}
	if err := requireSeam("Describe", "Seats", env.Seats); err != nil {
		return nil, err
	}
	tables := func() (seatengine.Table, error) {
		table, err := env.DescribeTable()
		if err != nil {
			return seatengine.Table{}, err
		}
		table.Gate = gate
		return table, nil
	}
	inner := shedadapters.NewMultiLLMProducerSourced(row.Name, tables, env.Seats, env.Now, nil)
	return loomshed.NewDiscussionWrite(row.Name, inner, env.CommitDescription), nil
}
