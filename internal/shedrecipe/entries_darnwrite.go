// entries_darnwrite.go implements darnWriteEntry, the Constructor for the "DarnWrite" registry row.
// The row runs the told one-seat table of the darn writer on a gated shedadapters.MultiLLMProducer.
// The producer sits inside loomshed.NewDarnWrite's commit decorator.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// darnWriteEntry is the Constructor for the "DarnWrite" registry row.
// It resolves the row's "gates" key, so its "verify" gate runs the "gate_key" key and reads as "<row> gate".
// It validates Env.DarnTable, Env.Seats, the absolute paths of Env.Darn and its seams.
// It then builds a seat-table producer behind loomshed.NewDarnWrite.
// Env.Shuttle is never read: the writer runs through Env.Seats.
//
// The table arrives as an injected func(loomshed.DarnTold), evaluated with the values each built session is told.
// The resolved gate is stamped onto each table it returns, so the row's "gates" key guards the writer.
// Each gate entry re-prompts with the darn line, which names Env.DecisionRecordPath,
// so a recorded decision wins over a finding it forbids fixing.
//
// A "fixer" key on a gate element is read first, and the fixer is applied once every entry carries its re-prompt line.
func darnWriteEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	cfg, fixer, err := splitGateFixer("DarnWrite", cfg, env)
	if err != nil {
		return nil, err
	}
	gate, err := resolveRowGateSpec("DarnWrite", row.Name, cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates", "gate_key"); err != nil {
		return nil, err
	}
	for i := range gate {
		gate[i].Reprompt = loomshed.DarnGateReprompt(env.DecisionRecordPath)
	}
	gate, err = applyGateFixer(row.Name, gate, fixer, env)
	if err != nil {
		return nil, err
	}
	if err := requireSeam("DarnWrite", "DarnTable", env.DarnTable); err != nil {
		return nil, err
	}
	if err := requireSeam("DarnWrite", "Seats", env.Seats); err != nil {
		return nil, err
	}
	dw := env.Darn
	for _, path := range []struct{ field, value string }{
		{"Darn.RecordPath", dw.RecordPath},
		{"Darn.SupportLogPath", dw.SupportLogPath},
		{"Darn.StopMarkerPath", dw.StopMarkerPath},
		{"Darn.ConditionStopMarkerPath", dw.ConditionStopMarkerPath},
	} {
		if err := requireAbsRoot("DarnWrite", path.field, path.value); err != nil {
			return nil, err
		}
	}
	seams := []struct {
		field string
		seam  any
	}{
		{"Darn.Commit", dw.Commit},
		{"Darn.LatestOutcome", dw.LatestOutcome},
		{"Darn.CheckRecord", dw.CheckRecord},
		{"Darn.CommittedRecord", dw.CommittedRecord},
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
		return shedadapters.NewMultiLLMProducerSourced(row.Name, tables, env.Seats, env.Now, nil).KeepGateFindings(env.gateFindingsPath(row.Name))
	}
	return loomshed.NewDarnWrite(row.Name, session, dw), nil
}
