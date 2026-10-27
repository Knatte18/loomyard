// baseline.go — the Drift-Baseline producer: it has the baseline agent record the task's regression scope and report, then takes the baseline capture the scope calls for.

package driftengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// DriftBaseline is the Drift-Baseline ShedProducer.
type DriftBaseline struct {
	name string
	row  Row
	deps Deps
}

var _ shedengine.ShedProducer = (*DriftBaseline)(nil)

// NewDriftBaseline returns the Drift-Baseline producer identified as name.
func NewDriftBaseline(name string, row Row, deps Deps) *DriftBaseline {
	return &DriftBaseline{name: name, row: row, deps: deps}
}

// Call runs one Drift-Baseline pass and commits the drift records on every way out.
// It has the baseline agent record the scope and write baseline.md when either is missing, then, by the recorded scope, takes the baseline capture:
// scope off, a set awaited_at and a passed capture take none, and a capture that holds the lock is waited on under WaitCapture and left running otherwise.
// Without WaitCapture the capture is a detached child and the pass returns once the child's record exists;
// with WaitCapture, and when a detached start is refused breakaway, the capture runs in this process.
// A failed or refused capture is a Stuck whose reason opens with CaptureHaltPrefix and names its ways forward.
// Only the scope record and baseline.md are committed while a detached capture may still write its logs; all of the durable drift directory is committed once a capture has ended in this process.
func (p *DriftBaseline) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	err := requireSeams(p.name,
		seam{"Config", d.Config != nil},
		seam{"Status", d.Status != nil},
		seam{"Head", d.Head != nil},
		seam{"BranchPoint", d.BranchPoint != nil},
		seam{"DirtyPaths", d.DirtyPaths != nil},
		seam{"Commit", d.Commit != nil},
		seam{"Seats", d.Seats != nil},
		seam{"BaselineTable", d.BaselineTable != nil},
		seam{"StartCapture", d.StartCapture != nil},
		seam{"RunCapture", d.RunCapture != nil},
		seam{"Sleep", d.Sleep != nil || !p.row.WaitCapture},
	)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	commitAll := p.row.WaitCapture
	outcome, pointer, err := p.drive(ctx, &commitAll)

	paths := []string{
		path.Join(DirRel(), filepath.Base(d.Paths.Scope)),
		path.Join(DirRel(), filepath.Base(d.Paths.BaselineReport)),
	}
	if commitAll {
		paths = []string{DirRel()}
	}
	return commitOnExit(p.name, d.Commit, paths, outcome, pointer, err)
}

// drive is Call without the commit; it sets commitAll once a capture has ended in this process.
func (p *DriftBaseline) drive(ctx context.Context, commitAll *bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	done := shedengine.OutputPointer{Path: d.Paths.BaselineReport}

	scope, found, err := ReadScope(d.Paths)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if _, statErr := os.Stat(d.Paths.BaselineReport); !found || statErr != nil {
		note := ""
		if found {
			note = fmt.Sprintf("The scope is already recorded as %s; write baseline.md alone against it, and expect `lym drift scope` to refuse a second call.", scope.Scope)
		}
		tables := func() (seatengine.Table, error) { return d.BaselineTable(note) }
		agent := shedadapters.NewMultiLLMProducerSourced(p.name, tables, d.Seats, d.Now, nil)
		outcome, pointer, err := agent.Call(ctx)
		if err != nil || outcome != shedengine.Done {
			return outcome, pointer, err
		}
		if scope, found, err = ReadScope(d.Paths); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if !found {
			return shedengine.Stuck, shedengine.OutputPointer{Reason: "drift: the baseline agent recorded no scope; way forward: lym loom resume, which spawns it again"}, nil
		}
	}

	if scope.Scope == ScopeOff || scope.AwaitedAt != "" {
		return shedengine.Done, done, nil
	}

	held, err := ProbeLock(d.Paths.CaptureLock)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if held && !p.row.WaitCapture {
		return shedengine.Done, done, nil
	}
	if held {
		if err := awaitCaptureEnd(ctx, d.Paths, d.Sleep); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	capture, captureFound, err := ReadCapture(d.Paths)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if captureFound && capture.State == CapturePassed {
		return shedengine.Done, done, nil
	}
	return p.captureAgain(ctx, commitAll)
}

// captureAgain checks the capture's precondition and takes the capture, detached or in this process.
func (p *DriftBaseline) captureAgain(ctx context.Context, commitAll *bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	done := shedengine.OutputPointer{Path: d.Paths.BaselineReport}

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
		return shedengine.Stuck, shedengine.OutputPointer{Reason: CaptureHaltPrefix + refusal.Error()}, nil
	}

	if !p.row.WaitCapture {
		startErr := d.StartCapture()
		switch {
		case startErr == nil:
			return shedengine.Done, done, nil
		case !errors.Is(startErr, ErrBreakawayRefused):
			return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: start the baseline capture: %w", p.name, startErr)
		}
	}

	*commitAll = true
	record, err := d.RunCapture(ctx)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: run the baseline capture: %w", p.name, err)
	}
	if record.State == CapturePassed {
		return shedengine.Done, done, nil
	}
	reason := fmt.Sprintf("%sthe capture ended %s (%s), log %s; way forward: lym drift capture followed by lym loom resume to capture again, or lym drift defer followed by lym loom resume to go on with regression deferred", CaptureHaltPrefix, record.State, record.Reason, record.Log)
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
