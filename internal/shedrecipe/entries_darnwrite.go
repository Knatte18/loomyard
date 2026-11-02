// entries_darnwrite.go implements darnWriteEntry, the Constructor for the "DarnWrite" registry row:
// it runs the told one-seat table of the darn writer on a gated shedadapters.MultiLLMProducer inside loomshed.NewDarnWrite's commit decorator, so it lives in its own file like prReworkEntry.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// darnWriteEntry is the Constructor for the "DarnWrite" registry row:
// it resolves the row's "gates" Config key through resolveRowGateSpec, so its "verify" gate runs the "gate_key" key and reads as "<row> gate", validates Env.DarnTable, Env.Seats, the three absolute paths and every seam of Env.Darn, then builds a seat-table producer behind loomshed.NewDarnWrite.
// Env.Shuttle is never read: the writer runs through Env.Seats.
//
// The table arrives as an injected func(loomshed.DarnTold) for the reason planWriteEntry gives, evaluated with the values the producer tells each session it builds.
// The resolved gate is stamped onto each table it returns, so the row's "gates" key guards the writer.
// Each gate entry re-prompts with the darn line, which names Env.DecisionRecordPath so a recorded decision wins over a finding it forbids fixing.
//
// A "fixer" key on a gate element is read through splitGateFixer, and the spec, once every entry carries its re-prompt line, goes through applyGateFixer.
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
