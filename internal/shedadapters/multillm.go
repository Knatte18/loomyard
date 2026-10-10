// multillm.go implements MultiLLMProducer, the shedadapters adapter that wraps one table of seats, a chair and its advisors, behind the shedengine.ShedProducer seam.
// The chair's result decides the step as one shuttle run's does.

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// multiLLMEngineLabel is the short engine label MultiLLMProducer's log lines and error text carry.
const multiLLMEngineLabel = "seats"

// SeatRunner is the handle-shaped seam MultiLLMProducer drives one table of seats through:
// Probe reports which seats of a previous session are still live, Resume completes a table whose chair is live, and Run starts the table fresh.
// It is handle-shaped, not result-shaped, because the engine behind it holds several live runs at once and sends into the chair's while it waits.
type SeatRunner interface {
	Probe(table seatengine.Table) (seatengine.LiveTable, error)
	Resume(table seatengine.Table, live seatengine.LiveTable) (seatengine.Result, error)
	Run(table seatengine.Table) (seatengine.Result, error)
}

// Compile-time proof that *seatengine.Engine satisfies SeatRunner.
var _ SeatRunner = (*seatengine.Engine)(nil)

// MultiLLMProducer is the shedadapters adapter over one seatengine table:
// it probes for a live chair, resumes it or archives every seat's stale outputs and runs the table fresh, and maps the chair's result onto the shedengine.ShedProducer contract.
// The advisors' results are logged and never judged: an advisor's death weakens nothing the chair decided.
type MultiLLMProducer struct {
	name  string
	table seatengine.Table
	// tables, when set, builds the table for each call and table is unused.
	tables TableSource
	seats  SeatRunner
	now    func() time.Time
	// prepareFreshSpawn, when non-nil, runs on the fresh path after the probe and before the archive.
	prepareFreshSpawn func() error
}

var _ shedengine.ShedProducer = (*MultiLLMProducer)(nil)

// TableSource builds the seatengine.Table for one MultiLLMProducer call, evaluated once per Call.
// It is a function so the table's stencils and directives are read at call time, never captured at construction.
type TableSource func() (seatengine.Table, error)

// NewMultiLLMProducer returns a MultiLLMProducer identified as name, running table through seats.
// A nil now defaults to time.Now; it resolves only the archive filename's same-second collision suffix.
// The table is validated by the seat runner on every call.
func NewMultiLLMProducer(name string, table seatengine.Table, seats SeatRunner, now func() time.Time) *MultiLLMProducer {
	if now == nil {
		now = time.Now
	}
	return &MultiLLMProducer{name: name, table: table, seats: seats, now: now}
}

// NewMultiLLMProducerSourced returns a MultiLLMProducer identified as name that builds its table from tables on every call and runs it through seats.
// A non-nil prepareFreshSpawn runs on the fresh path only, after the probe found no live chair and before the stale outputs are archived; its error fails the call with nothing archived and no seat started.
// A nil now defaults to time.Now.
func NewMultiLLMProducerSourced(name string, tables TableSource, seats SeatRunner, now func() time.Time, prepareFreshSpawn func() error) *MultiLLMProducer {
	if now == nil {
		now = time.Now
	}
	return &MultiLLMProducer{name: name, tables: tables, seats: seats, now: now, prepareFreshSpawn: prepareFreshSpawn}
}

// Call runs one MultiLLMProducer iteration: entry-check the context, probe the table's seats, and either resume a live chair and map its result, or prepare a fresh spawn, archive every seat's stale outputs, run the table fresh and map the chair's result.
// The table is the source's answer, evaluated once before the probe, or the constructed table when there is no source.
// The probe runs before anything is archived, because archiving renames files a live seat may be about to write.
// A probe that finds the chair not live has already stopped every live advisor, so the preparation and the archive never run beside a live seat.
func (p *MultiLLMProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name, multiLLMEngineLabel); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	table := p.table
	if p.tables != nil {
		built, err := p.tables()
		if err != nil {
			return p.errorExit(ctx, "build table", err)
		}
		table = built
	}

	live, err := p.seats.Probe(table)
	if err != nil {
		return p.errorExit(ctx, "seat probe", err)
	}
	if live.Chair != nil {
		result, err := p.seats.Resume(table, live)
		if err != nil {
			return p.errorExit(ctx, "seat resume", err)
		}
		return p.mapOutcome(ctx, result)
	}

	if p.prepareFreshSpawn != nil {
		if err := p.prepareFreshSpawn(); err != nil {
			return p.errorExit(ctx, "prepare fresh spawn", err)
		}
	}
	if err := archiveStaleOutputs(table.Outputs(), p.now); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): archive stale outputs: %w", p.name, multiLLMEngineLabel, err)
	}
	result, err := p.seats.Run(table)
	if err != nil {
		return p.errorExit(ctx, "seat run", err)
	}
	return p.mapOutcome(ctx, result)
}

// errorExit returns the context error when ctx is cancelled, else err wrapped with the producer's name, the engine label and what failed.
// An error wrapping seatengine.ErrSeatNotStopped keeps its way forward, since a seat that may still be live may be writing the step's files.
func (p *MultiLLMProducer) errorExit(ctx context.Context, what string, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, p.name, multiLLMEngineLabel); cerr != nil && !errors.Is(err, seatengine.ErrSeatNotStopped) {
		return "", shedengine.OutputPointer{}, cerr
	}
	return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): %s: %w", p.name, multiLLMEngineLabel, what, err)
}

// mapOutcome maps the chair's result onto shedengine's contract, shared by the resume path and the fresh path:
// done maps to Done with the chair's first output as the pointer, unless its gate failed, which maps to Stuck with the pointer and the gate's attempts and reason;
// died and timeout are engine-level errors, wrapping shuttleengine.ErrNotStarted when the chair never started.
// A genuine success survives cancellation, the one exception cancelErr never applies to.
func (p *MultiLLMProducer) mapOutcome(ctx context.Context, result seatengine.Result) (shedengine.Outcome, shedengine.OutputPointer, error) {
	for _, advisor := range result.Advisors {
		logger.Info("shedadapters: advisor seat result", "producer", p.name, "engine", multiLLMEngineLabel, "seat", advisor.Name, "outcome", advisor.Outcome, "startError", advisor.StartError, "notified", advisor.Notified, "strandGUID", advisor.StrandGUID)
	}

	chair := result.Chair
	switch chair.Outcome {
	case shuttleengine.OutcomeDone:
		if len(result.ChairOutputs) == 0 {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): shuttle reported %s but the chair has no outputs", p.name, multiLLMEngineLabel, shuttleengine.OutcomeDone)
		}
		if chair.Gate != nil && !chair.Gate.Passed {
			if cerr := cancelErr(ctx, p.name, multiLLMEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			logger.Warn("shedadapters: chair's gate did not pass", "producer", p.name, "engine", multiLLMEngineLabel, "attempts", chair.Gate.Attempts, "findingsPath", chair.Gate.FindingsPath, "sessionID", chair.SessionID, "strandGUID", chair.StrandGUID)
			return shedengine.Stuck, shedengine.OutputPointer{Path: result.ChairOutputs[0], GateAttempts: gateAttemptsPointer(chair.Gate), Reason: gateFailedReason(chair.Gate)}, nil
		}
		return shedengine.Done, shedengine.OutputPointer{Path: result.ChairOutputs[0], GateAttempts: gateAttemptsPointer(chair.Gate)}, nil

	case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:
		if cerr := cancelErr(ctx, p.name, multiLLMEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("shedadapters: chair died or timed out", "producer", p.name, "engine", multiLLMEngineLabel, "sessionID", chair.SessionID, "strandGUID", chair.StrandGUID, "runDir", chair.RunDir, "outcome", chair.Outcome)
		if chair.NotStarted {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): shuttle run outcome %s: %w", p.name, multiLLMEngineLabel, chair.Outcome, shuttleengine.ErrNotStarted)
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): shuttle run outcome %s", p.name, multiLLMEngineLabel, chair.Outcome)

	default:
		if cerr := cancelErr(ctx, p.name, multiLLMEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("shedadapters: unrecognized shuttle outcome", "producer", p.name, "engine", multiLLMEngineLabel, "sessionID", chair.SessionID, "strandGUID", chair.StrandGUID, "runDir", chair.RunDir, "outcome", chair.Outcome)
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): unrecognized shuttle outcome %q", p.name, multiLLMEngineLabel, chair.Outcome)
	}
}
