// await.go — the Drift-Await producer: the mechanical row that holds the run until the baseline capture exists, or the parent deferred it.

package driftengine

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// DriftAwait is the Drift-Await ShedProducer, which holds the run until the baseline capture has finished.
type DriftAwait struct {
	name string
	deps Deps
}

var _ shedengine.ShedProducer = (*DriftAwait)(nil)

// NewDriftAwait returns the Drift-Await producer identified as name.
func NewDriftAwait(name string, deps Deps) *DriftAwait {
	return &DriftAwait{name: name, deps: deps}
}

// Call runs one Drift-Await pass.
// It commits the durable drift directory, the capture log included, on every way out but one.
// With drift switched off it is Done at once, committing nothing.
// It is Done with no scope record, with scope off, or with awaited_at set whatever HEAD is.
// It is also Done when a passed capture was taken at HEAD, which it records as awaited_at.
// A capture holding the lock is waited on without a bound of its own,
// and the capture record is read only after the lock reads free.
// A missing, failed or unfinished capture is a Stuck naming its ways forward.
// So is a passed capture taken at another commit than HEAD.
func (p *DriftAwait) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	err := requireSeams(p.name,
		seam{"Config", d.Config != nil},
		seam{"Head", d.Head != nil},
		seam{"BranchPoint", d.BranchPoint != nil},
		seam{"Commit", d.Commit != nil},
		seam{"Sleep", d.Sleep != nil},
	)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if outcome, pointer, stop := passWhenSwitchedOff(d.Config); stop {
		return outcome, pointer, nil
	}
	outcome, pointer, err := p.await(ctx)
	return commitOnExit(p.name, d.Commit, []string{DirRel()}, outcome, pointer, err)
}

// await runs one Drift-Await pass and returns its outcome without committing the drift directory.
func (p *DriftAwait) await(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	records, err := ReadRecords(d)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if !records.ScopeFound || records.Scope.Scope == ScopeOff || records.Scope.AwaitedAt != "" {
		return shedengine.Done, shedengine.OutputPointer{}, nil
	}

	if err := awaitCaptureEnd(ctx, d.Paths, d.Sleep); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	capture, captureFound, err := ReadCapture(d.Paths)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	head, err := d.Head()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	branchPoint, err := d.BranchPoint()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	stuck := func(what string) (shedengine.Outcome, shedengine.OutputPointer, error) {
		return shedengine.Stuck, shedengine.OutputPointer{Reason: fmt.Sprintf("drift: %s; way forward: %s", what, awaitWaysForward(head == branchPoint))}, nil
	}
	switch {
	case !captureFound:
		return stuck("no baseline capture exists")
	case capture.State == CaptureRunning:
		return stuck(fmt.Sprintf("the baseline capture record reads running, but no capture holds the lock%s", logClause(capture.Log)))
	case capture.State != CapturePassed:
		return stuck(fmt.Sprintf("the baseline capture ended %s (%s)%s", capture.State, capture.Reason, logClause(capture.Log)))
	case capture.Head != head:
		return stuck(fmt.Sprintf("HEAD %s moved off the commit %s the baseline was captured at, and no capture may run on changed code", head, capture.Head))
	}

	digest, err := BaselineDigest(d.Paths.BaselineDir)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	now := d.Now
	if now == nil {
		now = time.Now
	}
	err = WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.AwaitedAt = stamp(now())
		return nil
	}, fabricengine.Trailer{Key: BaselineTrailerKey, Value: digest})
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// deferWay is the way forward that goes on with regression deferred, offered by every capture Stuck.
const deferWay = "lym drift defer followed by lym loom resume, which goes on with regression deferred"

// awaitWaysForward names the ways forward of a Drift-Await Stuck.
// A capture is one of them only while HEAD is on the branch point.
func awaitWaysForward(onBranchPoint bool) string {
	if onBranchPoint {
		return "lym drift capture followed by lym loom resume, which captures again on the branch point and re-enters this row, or " + deferWay
	}
	return deferWay
}

// logClause returns ", log <path>", or nothing when log is empty.
func logClause(log string) string {
	if log == "" {
		return ""
	}
	return ", log " + log
}
