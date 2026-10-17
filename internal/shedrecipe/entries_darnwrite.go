// entries_darnwrite.go implements darnWriteEntry, the Constructor for the "DarnWrite" registry row:
// it runs the told one-seat table of the darn writer on a gated shedadapters.MultiLLMProducer inside loomshed.NewDarnWrite's commit and rejection decorator, so it lives in its own file like prReworkEntry.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// darnWriteEntry is the Constructor for the "DarnWrite" registry row:
// it resolves the row's "gates" Config key through resolveGateSpec, validates Env.DarnTable, Env.Seats and every seam of Env.Darn, then builds a seat-table producer behind loomshed.NewDarnWrite.
// Env.Shuttle is never read: the writer runs through Env.Seats.
//
// The table arrives as an injected func(loomshed.DarnTold) for the reason planWriteEntry gives, evaluated with the values the producer tells each session it builds.
// The resolved gate is stamped onto each table it returns, so the row's "gates" key guards the writer.
func darnWriteEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("DarnWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireSeam("DarnWrite", "DarnTable", env.DarnTable); err != nil {
		return nil, err
	}
	if err := requireSeam("DarnWrite", "Seats", env.Seats); err != nil {
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
		tables := func() (seatengine.Table, error) {
			table, err := env.DarnTable(told)
			if err != nil {
				return seatengine.Table{}, err
			}
			table.Gate = gate
			return table, nil
		}
		return shedadapters.NewMultiLLMProducerSourced(row.Name, tables, env.Seats, env.Now, nil)
	}
	return loomshed.NewDarnWrite(row.Name, session, dw), nil
}
