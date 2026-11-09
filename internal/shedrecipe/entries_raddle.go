// entries_raddle.go implements the Constructors of the "RaddleWrite" and "RaddleFile" registry rows:
// raddleWriteEntry, raddleFileEntry, and RaddleWriteEnv, which holds the seams and paths the RaddleWrite row reads.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// RaddleWriteEnv carries what the RaddleWrite entry reads:
// the producer's paths and seams, the table source of the raddle agent's seat, and the check the raddle gate runs.
type RaddleWriteEnv struct {
	// Deps are the paths and seams of the Raddle-Write producer.
	Deps loomshed.RaddleWriteDeps
	// Table builds the seat table for one session from the run's mode and the last return reason, empty on a first session.
	// It arrives as a closure because building the table needs a *lymcwd.Location, which this package may not import.
	Table func(mode loomengine.RaddleMode, returnReason string) (seatengine.Table, error)
	// Check carries the proposal and decision record paths and the board context the gate judges the proposal against.
	Check loomshed.RaddleCheck
}

// raddleWriteEntry is the Constructor for the "RaddleWrite" registry row.
// It takes the "gates" Config key alone and validates Env.RaddleWrite and Env.Seats;
// it returns loomshed.NewRaddleWrite over a seat-table producer.
// The table source reads the run's recorded mode and return reason from the raddle state at each Call,
// so a session started after a return or in the other mode gets its own table,
// and the resolved gate is stamped onto each table.
// Every path of Env.RaddleWrite.Deps must be absolute and every seam non-nil, or the refusal names the Env field.
func raddleWriteEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	const entry = "RaddleWrite"
	gate, err := resolveGateSpec(entry, cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}

	deps := env.RaddleWrite.Deps
	for _, path := range []struct{ field, value string }{
		{"RaddleWrite.Deps.StatePath", deps.StatePath},
		{"RaddleWrite.Deps.StateLockPath", deps.StateLockPath},
		{"RaddleWrite.Deps.ProposalPath", deps.ProposalPath},
		{"RaddleWrite.Deps.ReturnedPath", deps.ReturnedPath},
		{"RaddleWrite.Deps.ColdPath", deps.ColdPath},
		{"RaddleWrite.Deps.ApprovalPath", deps.ApprovalPath},
	} {
		if err := requireAbsRoot(entry, path.field, path.value); err != nil {
			return nil, err
		}
	}
	for _, seam := range []struct {
		field string
		value any
	}{
		{"RaddleWrite.Deps.SplitPerch", deps.SplitPerch},
		{"RaddleWrite.Deps.AutoApprove", deps.AutoApprove},
		{"RaddleWrite.Deps.FreshEntry", deps.FreshEntry},
		{"RaddleWrite.Deps.StoreCold", deps.StoreCold},
		{"RaddleWrite.Deps.Commit", deps.Commit},
		{"RaddleWrite.Table", env.RaddleWrite.Table},
		{"Seats", env.Seats},
	} {
		if err := requireSeam(entry, seam.field, seam.value); err != nil {
			return nil, err
		}
	}

	tables := func() (seatengine.Table, error) {
		recorded, err := loomshed.ReadRaddleState(deps.StatePath, deps.StateLockPath)
		if err != nil {
			return seatengine.Table{}, fmt.Errorf("shedrecipe: %s: read the run's state: %w", entry, err)
		}
		table, err := env.RaddleWrite.Table(recorded.Mode, recorded.ReturnReason)
		if err != nil {
			return seatengine.Table{}, err
		}
		table.Gate = gate
		return table, nil
	}
	inner := shedadapters.NewMultiLLMProducerSourced(row.Name, tables, env.Seats, env.Now, nil).KeepGateFindings(env.gateFindingsPath(row.Name))
	return loomshed.NewRaddleWrite(row.Name, inner, deps), nil
}

// raddleFileEntry is the Constructor for the "RaddleFile" registry row.
// It takes no Config key and returns loomshed.NewRaddleFile over Env.RaddleFile.
// Every path of Env.RaddleFile must be absolute and every seam non-nil, or the refusal names the Env field.
func raddleFileEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	const entry = "RaddleFile"
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}

	deps := env.RaddleFile
	for _, path := range []struct{ field, value string }{
		{"RaddleFile.ProposalPath", deps.ProposalPath},
		{"RaddleFile.ColdPath", deps.ColdPath},
		{"RaddleFile.RecordPath", deps.RecordPath},
		{"RaddleFile.StageDir", deps.StageDir},
		{"RaddleFile.RecordsDir", deps.RecordsDir},
		{"RaddleFile.StatePath", deps.StatePath},
		{"RaddleFile.StateLockPath", deps.StateLockPath},
	} {
		if err := requireAbsRoot(entry, path.field, path.value); err != nil {
			return nil, err
		}
	}
	for _, seam := range []struct {
		field string
		value any
	}{
		{"RaddleFile.PartRecordPath", deps.PartRecordPath},
		{"RaddleFile.Check", deps.Check},
		{"RaddleFile.Recheck", deps.Recheck},
		{"RaddleFile.Entry", deps.Entry},
		{"RaddleFile.StoreDone", deps.StoreDone},
		{"RaddleFile.AlreadyFiled", deps.AlreadyFiled},
		{"RaddleFile.File", deps.File},
		{"RaddleFile.CreateBeam", deps.CreateBeam},
		{"RaddleFile.Notify", deps.Notify},
		{"RaddleFile.EndEntry", deps.EndEntry},
		{"RaddleFile.Commit", deps.Commit},
	} {
		if err := requireSeam(entry, seam.field, seam.value); err != nil {
			return nil, err
		}
	}

	return loomshed.NewRaddleFile(row.Name, deps), nil
}
