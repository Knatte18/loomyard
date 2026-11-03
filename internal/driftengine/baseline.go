// baseline.go — the Drift-Baseline producer: it has the baseline agent record the task's regression scope and report, then takes the baseline capture the scope calls for.

package driftengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

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

// Call runs one Drift-Baseline pass and commits the drift records on every way out but one: with drift switched off it is Done at once, spawning, starting and committing nothing.
// When the drift config sets scope, it records that scope when none is recorded and writes baseline.md itself when missing, spawning no agent;
// otherwise it has the baseline agent record the scope and write baseline.md when either is missing.
// Then, by the recorded scope, it takes the baseline capture:
// scope off, a set awaited_at and a passed capture take none, and a capture that holds the lock is left running.
// Otherwise the capture is a detached child and the pass returns once the child's record exists;
// when a detached start is refused breakaway, the capture runs in this process.
// A failed or refused capture, or a capture child that ended or stalled before it recorded its start, is a Stuck whose reason opens with CaptureHaltPrefix and names its ways forward, `lym drift defer` among them.
// A child's halt carries the error its log ends on, and for an error the child marked lasting it drops the resume that captures again, naming only `lym drift defer` and a fix of the cause.
// A start that fails for another reason, such as an unresolvable binary, an unwritable child log or a held capture lock, is an error.
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
	)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if outcome, pointer, stop := passWhenSwitchedOff(d.Config); stop {
		return outcome, pointer, nil
	}

	var commitAll bool
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

	cfg, err := d.Config()
	if err != nil {
		return shedengine.Stuck, shedengine.OutputPointer{Reason: fmt.Sprintf("%v; then lym loom resume", err)}, nil
	}
	if cfg.Scope != "" {
		if err := settleConfiguredScope(d.Paths, cfg); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}

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
	if held {
		return shedengine.Done, done, nil
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

// settleConfiguredScope records the scope cfg sets when no scope is recorded, and writes baseline.md from the recorded scope when it is missing, so the pass spawns no baseline agent.
// A scope already recorded, by an earlier pass or the run's parent, is kept.
func settleConfiguredScope(p Paths, cfg Config) error {
	err := UpdateScope(p, func(cur ScopeRecord, found bool) (ScopeRecord, error) {
		if found {
			return cur, errNoRecord
		}
		return ScopeRecord{Scope: cfg.Scope, Skip: cfg.Skip, RecordedAt: stamp(time.Now())}, nil
	})
	if err != nil && !errors.Is(err, errNoRecord) {
		return err
	}
	if _, err := os.Stat(p.BaselineReport); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("drift: read %s: %w", p.BaselineReport, err)
	}
	scope, _, err := ReadScope(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.BaselineReport), 0o755); err != nil {
		return fmt.Errorf("drift: create %s: %w", filepath.Dir(p.BaselineReport), err)
	}
	if err := os.WriteFile(p.BaselineReport, []byte(configuredBaselineReport(scope)), 0o644); err != nil {
		return fmt.Errorf("drift: write %s: %w", p.BaselineReport, err)
	}
	return nil
}

// configuredBaselineReport renders a baseline.md written without a baseline agent, which names the scope and its skipped tests and predicts no drift per test.
func configuredBaselineReport(scope ScopeRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Drift baseline\n\nThis report was generated, so no baseline agent ran and it expects no particular drift per test.\n\nScope: %s\n", scope.Scope)
	if len(scope.Skip) > 0 {
		fmt.Fprintf(&b, "Skipped tests: %s\n", strings.Join(scope.Skip, ", "))
	}
	return b.String()
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

	startErr := d.StartCapture()
	var noRecord childNoRecordError
	switch {
	case startErr == nil:
		return shedengine.Done, done, nil
	case errors.As(startErr, &noRecord) && noRecord.lasting:
		reason := fmt.Sprintf("%sthe baseline capture did not start: %v; lym loom resume cannot change this error, since it starts the capture child the same way; way forward: lym drift defer followed by lym loom resume to go on with regression deferred, or a fix of the error's cause", CaptureHaltPrefix, startErr)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	case errors.As(startErr, &noRecord):
		reason := fmt.Sprintf("%sthe baseline capture did not start: %v; way forward: lym loom resume to capture again, or lym drift defer followed by lym loom resume to go on with regression deferred", CaptureHaltPrefix, startErr)
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
