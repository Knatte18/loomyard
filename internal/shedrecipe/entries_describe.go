// entries_describe.go implements describeEntry, the Constructor for the "Describe" registry row.
// The row runs the told one-seat table of the Describe lead on a shedadapters.MultiLLMProducer.
// The producer sits behind loomshed.NewDiscussionWrite's commit decorator.
// The loop owner commits the change description once the gate passes.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// describeEntry is the Constructor for the "Describe" registry row.
// It resolves the row's "gates" key and validates Env.DescribeTable, Env.CommitDescription and Env.Seats.
// It returns the seat-table producer wrapped in loomshed.NewDiscussionWrite's commit decorator.
// Env.Shuttle is never read: the lead runs through Env.Seats.
//
// The commit decorator commits the description when the pointer it is given is non-empty.
// On gate exhaustion it commits the refused file, so a halted run leaves a clean, diagnosable tree.
// The seat producer archives a stale description before a fresh spawn.
//
// The table arrives as an injected shedadapters.TableSource closure.
// Building it needs the fabric's landing geometry.
// The Shed Recipe Registry Invariant bars this package from deriving that geometry.
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
	inner := shedadapters.NewMultiLLMProducerSourced(row.Name, tables, env.Seats, env.Now, nil).KeepGateFindings(env.gateFindingsPath(row.Name))
	return loomshed.NewDiscussionWrite(row.Name, inner, env.CommitDescription), nil
}
