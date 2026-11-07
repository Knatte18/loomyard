// baseline.go implements the Drift-Baseline producer.
// It records the task's regression scope from the drift config without an agent,
// and it takes the baseline capture that scope calls for.

package driftengine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// driftScopeKey is the drift config key that sets a task's regression scope;
// a scope record set from it names this key as its source.
const driftScopeKey = "drift.scope"

// deferrableCaptureHalt opens the reason of every Drift-Baseline Stuck whose way forward includes `lym drift defer`.
// CaptureHaltPrefix stays first, so a capture halt is still told from any other halt of the row.
const deferrableCaptureHalt = CaptureHaltPrefix + DeferHaltPrefix

// DriftBaseline is the Drift-Baseline producer.
// It records a task's regression scope from the drift config and takes the baseline capture that scope calls for.
type DriftBaseline struct {
	name string
	row  Row
	deps Deps
}

var _ shedengine.ShedProducer = (*DriftBaseline)(nil)

// NewDriftBaseline returns the Drift-Baseline producer named name; the name prefixes its errors.
func NewDriftBaseline(name string, row Row, deps Deps) *DriftBaseline {
	return &DriftBaseline{name: name, row: row, deps: deps}
}

// Call runs one Drift-Baseline pass: with drift switched off it is Done at once, spawning, starting and committing nothing.
// Otherwise it ends the live chairs of the removed drift-baseline and drift-verify agents,
// and it records the scope when none is recorded, spawning no agent.
// Then, by the recorded scope, it takes the baseline capture:
// scope off, a set awaited_at and a passed capture take none, and a capture that holds the lock is left running.
// Otherwise the capture is a detached child and the pass returns once the child's record exists;
// when a detached start is refused breakaway, the capture runs in this process.
// A failed or refused capture is a Stuck, and so is a capture child that ended or stalled before it recorded its start.
// Its reason opens with CaptureHaltPrefix and DeferHaltPrefix and names its ways forward, `lym drift defer` among them.
// A child's halt carries the error its log ends on.
// For an error the child marked lasting, the reason drops the resume that captures again.
// It names only `lym drift defer` and a fix of the cause.
// A start that fails for another reason is an error:
// an unresolvable binary, an unwritable child log or a held capture lock.
// The scope write is committed with the records;
// the durable drift directory is committed on the way out only once a capture has ended in this process.
func (p *DriftBaseline) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	err := requireSeams(p.name,
		seam{"Config", d.Config != nil},
		seam{"Status", d.Status != nil},
		seam{"Head", d.Head != nil},
		seam{"BranchPoint", d.BranchPoint != nil},
		seam{"DirtyPaths", d.DirtyPaths != nil},
		seam{"Commit", d.Commit != nil},
		seam{"CommitWithTrailers", d.CommitWithTrailers != nil},
		seam{"TrailerCommit", d.TrailerCommit != nil},
		seam{"FileAt", d.FileAt != nil},
		seam{"EndRoles", d.EndRoles != nil},
		seam{"StartCapture", d.StartCapture != nil},
		seam{"RunCapture", d.RunCapture != nil},
	)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if outcome, pointer, stop := passWhenSwitchedOff(d.Config); stop {
		return outcome, pointer, nil
	}

	var commitAll bool
	outcome, pointer, err := p.drive(ctx, &commitAll)
	if !commitAll {
		return outcome, pointer, err
	}
	return commitOnExit(p.name, d.Commit, []string{DirRel()}, outcome, pointer, err)
}

// drive runs one pass without the commit; it sets commitAll once a capture has ended in this process.
func (p *DriftBaseline) drive(ctx context.Context, commitAll *bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps

	cfg, err := d.Config()
	if err != nil {
		return shedengine.Stuck, shedengine.OutputPointer{Reason: fmt.Sprintf("%v; then lym loom resume", err)}, nil
	}
	if err := d.EndRoles([]string{baselineRole, verifyRole}); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: end the chairs of the removed drift agents: %w", p.name, err)
	}
	scope, err := p.recordScope(cfg)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	if scope.Scope == ScopeOff || scope.AwaitedAt != "" {
		return shedengine.Done, shedengine.OutputPointer{}, nil
	}

	held, err := ProbeLock(d.Paths.CaptureLock)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if held {
		return shedengine.Done, shedengine.OutputPointer{}, nil
	}
	capture, captureFound, err := ReadCapture(d.Paths)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if captureFound && capture.State == CapturePassed {
		return shedengine.Done, shedengine.OutputPointer{}, nil
	}
	return p.captureAgain(ctx, commitAll)
}

// recordScope returns the scope record, writing it first when none is recorded.
// The written scope is off when the config key drift.scope sets it so;
// otherwise it is full, with the config's diff mode and key patterns.
// A config that sets scope partial is recorded full, with one Warn naming the key.
// A scope already recorded, off, full or an old partial, is kept whatever the config says now.
func (p *DriftBaseline) recordScope(cfg Config) (ScopeRecord, error) {
	d := p.deps
	records, err := ReadRecords(d)
	if err != nil {
		return ScopeRecord{}, err
	}
	if records.ScopeFound {
		return records.Scope, nil
	}

	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	scope := ScopeRecord{Scope: ScopeFull, Diff: cfg.Diff, KeyPatterns: cfg.KeyPatterns, Volatile: cfg.Volatile, RecordedAt: stamp(now())}
	switch {
	case cfg.Scope == ScopeOff:
		scope = ScopeRecord{Scope: ScopeOff, ScopeSource: driftScopeKey, RecordedAt: stamp(now())}
	case cfg.ScopeWasPartial:
		logger.Warn("drift: the config key sets scope partial, which is retired; the scope is recorded as full", "key", driftScopeKey)
	}
	err = WriteRecords(d, func(recorded *ScopeRecord, _ *JudgedRecord) error {
		*recorded = scope
		return nil
	})
	return scope, err
}

// captureAgain checks the capture's precondition and takes the capture, detached or in this process.
func (p *DriftBaseline) captureAgain(ctx context.Context, commitAll *bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps

	cfg, err := d.Config()
	if err == nil {
		_, err = cfg.Step(stepCapture)
	}
	if err != nil {
		return shedengine.Stuck, shedengine.OutputPointer{Reason: fmt.Sprintf("%v; then lym loom resume", err)}, nil
	}
	refusal, err := p.checkCapture()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if refusal != nil {
		return shedengine.Stuck, shedengine.OutputPointer{Reason: deferrableCaptureHalt + refusal.Error()}, nil
	}

	startErr := d.StartCapture()
	var noRecord childNoRecordError
	switch {
	case startErr == nil:
		return shedengine.Done, shedengine.OutputPointer{}, nil
	case errors.As(startErr, &noRecord) && noRecord.lasting:
		reason := fmt.Sprintf("%sthe baseline capture did not start: %v; lym loom resume cannot change this error, since it starts the capture child the same way; way forward: lym drift defer followed by lym loom resume to go on with regression deferred, or a fix of the error's cause", deferrableCaptureHalt, startErr)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	case errors.As(startErr, &noRecord):
		reason := fmt.Sprintf("%sthe baseline capture did not start: %v; way forward: lym loom resume to capture again, or lym drift defer followed by lym loom resume to go on with regression deferred", deferrableCaptureHalt, startErr)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	case !errors.Is(startErr, ErrBreakawayRefused):
		return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: start the baseline capture: %w", p.name, startErr)
	}

	*commitAll = true
	record, err := d.RunCapture(ctx)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: run the baseline capture: %w", p.name, err)
	}
	if record.State == CapturePassed {
		return shedengine.Done, shedengine.OutputPointer{}, nil
	}
	// The capture ran past its precondition, so HEAD is on the branch point.
	reason := fmt.Sprintf("%sthe capture ended %s (%s), log %s; way forward: %s", deferrableCaptureHalt, record.State, record.Reason, record.Log, awaitWaysForward(true))
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
}

// checkCapture reads the run's status, HEAD, branch point and dirty paths and returns the main capture's refusal, or nil when it may run.
// A failed read is the error.
func (p *DriftBaseline) checkCapture() (refusal, err error) {
	d := p.deps
	status, err := d.Status()
	if err != nil {
		return nil, err
	}
	head, err := d.Head()
	if err != nil {
		return nil, err
	}
	branchPoint, err := d.BranchPoint()
	if err != nil {
		return nil, err
	}
	dirty, err := d.DirtyPaths()
	if err != nil {
		return nil, err
	}
	return CheckMainCapture(status, head, branchPoint, dirty), nil
}
