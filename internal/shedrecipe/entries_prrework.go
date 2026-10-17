// entries_prrework.go implements prReworkEntry, the Constructor for the "PRRework" registry row:
// it runs the told one-seat table of the PR-Rework lead on a gated shedadapters.MultiLLMProducer inside loomshed.NewPRRework's generation-archive and round-record decorator, so it lives in its own file like planWriteEntry.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// prReworkEntry is the Constructor for the "PRRework" registry row:
// it resolves the row's "gates" Config key through resolveGateSpec, validates Env.ReworkTable, Env.Seats, every seam of Env.Rework, and the absolute Env.Rework.PlanDir, Env.Rework.ReworkDir and Env.Rework.ReviewsDir, then builds a seat-table producer behind loomshed.NewPRRework.
// Env.Shuttle is never read: the lead runs through Env.Seats.
//
// No fresh-spawn preparation is passed: the adapter already archives a stale coverage file on a fresh spawn,
// and nothing else may be moved out from under the plan.
//
// The table arrives as an injected func(loomshed.ReworkTold) for the reason planWriteEntry gives, evaluated with the value the producer tells each session it builds.
// The resolved gate is stamped onto each table it returns, so the row's "gates" key guards the lead.
func prReworkEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("PRRework", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireSeam("PRRework", "ReworkTable", env.ReworkTable); err != nil {
		return nil, err
	}
	if err := requireSeam("PRRework", "Seats", env.Seats); err != nil {
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
		{"Rework.ArchiveWebster", rw.ArchiveWebster},
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
	if err := requireAbsRoot("PRRework", "Rework.ReviewsDir", rw.ReviewsDir); err != nil {
		return nil, err
	}
	session := func(told loomshed.ReworkTold) shedengine.ShedProducer {
		tables := func() (seatengine.Table, error) {
			table, err := env.ReworkTable(told)
			if err != nil {
				return seatengine.Table{}, err
			}
			table.Gate = gate
			return table, nil
		}
		return shedadapters.NewMultiLLMProducerSourced(row.Name, tables, env.Seats, env.Now, nil)
	}
	return loomshed.NewPRRework(row.Name, session, rw), nil
}
