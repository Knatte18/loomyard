// multillm.go implements MultiLLMProducer, the adapter over one seat table of a chair and its advisors.
// The chair's result decides the step as one shuttle run's does, unless a writer seat ended the step first;
// writerExit maps the writer seat's ending instead.

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

// multiLLMEngineLabel is the short engine label that the producer's log lines and error text carry.
const multiLLMEngineLabel = "seats"

// SeatRunner is the seam through which a producer drives one table of seats.
// Probe reports which seats of a previous session are still live.
// Resume completes a table whose chair is live, and Run starts the table fresh.
// It is handle-shaped, not result-shaped:
// the engine behind it holds several live runs at once and sends into the chair's while it waits.
type SeatRunner interface {
	Probe(table seatengine.Table) (seatengine.LiveTable, error)
	Resume(table seatengine.Table, live seatengine.LiveTable) (seatengine.Result, error)
	Run(table seatengine.Table) (seatengine.Result, error)
}

// Compile-time proof that *seatengine.Engine satisfies SeatRunner.
var _ SeatRunner = (*seatengine.Engine)(nil)

// MultiLLMProducer is the shedadapters adapter over one seatengine table.
// Each Call probes the table for a live chair.
// It resumes a live chair, or else archives every seat's stale outputs and runs the table fresh,
// and it maps the chair's result onto the shedengine.ShedProducer contract.
// An advisor's result is logged only, since an advisor's death weakens nothing the chair decided;
// a writer that ended the step decides it instead.
type MultiLLMProducer struct {
	name  string
	table seatengine.Table
	// tables, when set, builds the table for each call and table is unused.
	tables TableSource
	seats  SeatRunner
	now    func() time.Time
	// prepareFreshSpawn, when non-nil, runs on the fresh path after the probe and before the archive.
	// It returns the prompt amendment the lead's fresh prompt ends with.
	prepareFreshSpawn func() (string, error)
	// gateFindingsPath, when non-empty, is the file an exhausted chair gate's findings are kept in.
	gateFindingsPath string
}

var _ shedengine.ShedProducer = (*MultiLLMProducer)(nil)

// TableSource builds the seatengine.Table for one producer call, evaluated once per call.
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

// NewMultiLLMProducerSourced returns a MultiLLMProducer identified as name.
// It builds its table from tables on every call and runs it through seats.
// A non-nil prepareFreshSpawn runs on the fresh path only.
// It runs after the probe finds no live chair and before the stale outputs are archived.
// Its error fails the call with nothing archived and no seat started.
// The text it returns becomes the table's Amendment, which the seat engine appends to the lead's prompt.
// A nil now defaults to time.Now.
func NewMultiLLMProducerSourced(name string, tables TableSource, seats SeatRunner, now func() time.Time, prepareFreshSpawn func() (string, error)) *MultiLLMProducer {
	if now == nil {
		now = time.Now
	}
	return &MultiLLMProducer{name: name, tables: tables, seats: seats, now: now, prepareFreshSpawn: prepareFreshSpawn}
}

// KeepGateFindings sets the file an exhausted chair gate's findings are kept in, which the Stuck reason names.
// It returns the producer so a constructor call can chain it.
// A call whose chair gate passed removes the file; an empty path keeps no findings file.
func (p *MultiLLMProducer) KeepGateFindings(path string) *MultiLLMProducer {
	p.gateFindingsPath = path
	return p
}

// Call runs one MultiLLMProducer iteration.
// It entry-checks the context and probes the table's seats.
// If a chair is live, it resumes the chair and maps its result.
// Otherwise it prepares a fresh spawn;
// it archives every seat's stale outputs, runs the table fresh, and maps the chair's result.
// The table is the source's answer, evaluated once before the probe, or the constructed table when there is no source.
// The probe runs before anything is archived, because archiving renames files a live seat may be about to write.
// A probe that finds the chair not live has already stopped every live advisor;
// the preparation and the archive therefore never run beside a live seat.
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

	freshTable := table
	if p.prepareFreshSpawn != nil {
		amendment, err := p.prepareFreshSpawn()
		if err != nil {
			return p.errorExit(ctx, "prepare fresh spawn", err)
		}
		freshTable.Amendment = amendment
	}
	if err := archiveStaleOutputs(table.Outputs(), p.now); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): archive stale outputs: %w", p.name, multiLLMEngineLabel, err)
	}
	result, err := p.seats.Run(freshTable)
	if err != nil {
		return p.errorExit(ctx, "seat run", err)
	}
	return p.mapOutcome(ctx, result)
}

// errorExit returns the context error when ctx is cancelled;
// otherwise it returns err wrapped with the producer's name, the engine label and what failed.
// An error wrapping seatengine.ErrSeatNotStopped is returned over a cancellation;
// the seat may still be live and writing the step's files.
func (p *MultiLLMProducer) errorExit(ctx context.Context, what string, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, p.name, multiLLMEngineLabel); cerr != nil && !errors.Is(err, seatengine.ErrSeatNotStopped) {
		return "", shedengine.OutputPointer{}, cerr
	}
	return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): %s: %w", p.name, multiLLMEngineLabel, what, err)
}

// mapOutcome maps a table's result onto shedengine's contract, shared by the resume path and the fresh path.
// The first writer seat in table order that ended the step decides it through writerExit, whatever the chair's outcome.
// Otherwise the chair's result decides:
// done maps to Done with the chair's first output as the pointer;
// when its gate failed, it maps to Stuck with the pointer and the gate's attempts and reason;
// died and timeout are engine-level errors, wrapping shuttleengine.ErrNotStarted when the chair never started;
// it carries the chair's start error text when shuttle reported one;
// a chair that started and died wraps shuttleengine.ErrAgentDied.
// A genuine success survives cancellation, the one exception cancelErr never applies to.
func (p *MultiLLMProducer) mapOutcome(ctx context.Context, result seatengine.Result) (shedengine.Outcome, shedengine.OutputPointer, error) {
	for _, advisor := range result.Advisors {
		logger.Info("shedadapters: advisor seat result", "producer", p.name, "engine", multiLLMEngineLabel, "seat", advisor.Name, "outcome", advisor.Outcome, "startError", advisor.StartError, "notified", advisor.Notified, "strandGUID", advisor.StrandGUID)
	}

	for _, seat := range result.Advisors {
		if seat.Writer && seat.EndedStep {
			return p.writerExit(ctx, seat)
		}
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
			return shedengine.Stuck, shedengine.OutputPointer{Path: result.ChairOutputs[0], GateAttempts: gateAttemptsPointer(chair.Gate), GateSpans: gateSpansOf(chair.Gate), Reason: gateFailedReason(chair.Gate, p.gateFindingsPath)}, nil
		}
		if chair.Gate != nil {
			clearGateFindings(p.gateFindingsPath)
		}
		return shedengine.Done, shedengine.OutputPointer{Path: result.ChairOutputs[0], GateAttempts: gateAttemptsPointer(chair.Gate), GateSpans: gateSpansOf(chair.Gate)}, nil

	case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:
		if cerr := cancelErr(ctx, p.name, multiLLMEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("shedadapters: chair died or timed out", "producer", p.name, "engine", multiLLMEngineLabel, "sessionID", chair.SessionID, "strandGUID", chair.StrandGUID, "runDir", chair.RunDir, "outcome", chair.Outcome)
		if chair.NotStarted {
			if chair.StartError != "" {
				return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): shuttle run outcome %s: %w: %s", p.name, multiLLMEngineLabel, chair.Outcome, shuttleengine.ErrNotStarted, chair.StartError)
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): shuttle run outcome %s: %w", p.name, multiLLMEngineLabel, chair.Outcome, shuttleengine.ErrNotStarted)
		}
		if chair.Outcome == shuttleengine.OutcomeDied {
			return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): shuttle run outcome %s: %w", p.name, multiLLMEngineLabel, chair.Outcome, shuttleengine.ErrAgentDied)
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

// writerResumeWayForward is the way forward every writer exit ends with.
const writerResumeWayForward = "way forward: re-run the row with lym loom resume, which starts every part over unless the chair is still live"

// writerExit maps a writer seat that ended the step onto the producer contract.
// The seat's name is also the name of its part.
// A start failure wraps shuttleengine.ErrNotStarted.
// A died run or a wait error wraps shuttleengine.ErrAgentDied and names the writer.
// A timeout, a failed gate, an Accept error, and a writer lost at resume with its outputs missing are Stuck.
// The pointer is the seat's first output, so the part's files are committed.
// The reason names the seat and its part.
// Any other end is an error naming the writer and its outcome.
// A cancelled context returns the context error first: no writer end is a success.
func (p *MultiLLMProducer) writerExit(ctx context.Context, seat seatengine.SeatResult) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, p.name, multiLLMEngineLabel); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	logger.Warn("shedadapters: writer seat ended the step", "producer", p.name, "engine", multiLLMEngineLabel, "seat", seat.Name, "outcome", seat.Outcome, "startError", seat.StartError, "strandGUID", seat.StrandGUID)

	label := fmt.Sprintf("writer seat %s of part %s", seat.Name, seat.Name)
	stuck := func(reason string, gate *shuttleengine.GateOutcome) (shedengine.Outcome, shedengine.OutputPointer, error) {
		return shedengine.Stuck, shedengine.OutputPointer{Path: seat.Outputs[0], GateAttempts: gateAttemptsPointer(gate), GateSpans: gateSpansOf(gate), Reason: reason + "; " + writerResumeWayForward}, nil
	}
	gate := seat.Result.Gate

	switch {
	case seat.StartError != "":
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): %s did not start: %w: %s; %s", p.name, multiLLMEngineLabel, label, shuttleengine.ErrNotStarted, seat.StartError, writerResumeWayForward)
	case seat.Outcome == shuttleengine.OutcomeDied || seat.WaitErr != nil:
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): %s ended without finishing (outcome %q, wait error %v): %w; %s", p.name, multiLLMEngineLabel, label, seat.Outcome, seat.WaitErr, shuttleengine.ErrAgentDied, writerResumeWayForward)
	case seat.Outcome == shuttleengine.OutcomeTimeout:
		return stuck(label+" timed out", nil)
	case seat.Outcome == shuttleengine.OutcomeDone && gate != nil && !gate.Passed:
		return stuck(label+": "+gateFailedReason(gate, seat.GateFindingsPath), gate)
	case seat.AcceptErr != nil:
		return stuck(fmt.Sprintf("%s: its part did not pass the part check: %v", label, seat.AcceptErr), gate)
	case seat.StrandGUID == "" && seat.Outcome == "":
		return stuck(label+": its outputs were missing at resume", nil)
	default:
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): %s ended with the unrecognized outcome %q; %s", p.name, multiLLMEngineLabel, label, seat.Outcome, writerResumeWayForward)
	}
}
