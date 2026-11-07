// verbs.go — the rule of the `lym drift defer` verb: what it admits from the run's status, and the records it writes.
//
// Defer takes the run's status as a VerbStatus the caller read, so the rule runs on a fake.
// A refusal opens "drift defer refused:" or "drift:" and names its way forward; a refused verb writes nothing.

package driftengine

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// verifyRow is the row Drift-Verify runs at, one of the rows a halt can ask the parent to defer or amend at.
const verifyRow = "Drift-Verify"

// VerbStatus is what a drift verb judges: the run status and the facts read beside it.
// The zero VerbStatus is a run with no current row, which every verb refuses.
type VerbStatus struct {
	// Status is the run's status file.
	Status shedengine.Status
	// CaptureHeld is true when the capture lock is held.
	CaptureHeld bool
}

// errNoScope is the refusal of a verb that needs a recorded scope.
var errNoScope = errors.New("drift: no scope is recorded yet; way forward: lym loom status shows the row the run is at")

// deferHaltRows are the rows past Drift-Await that a halt naming `lym drift defer`, or a failed baseline check, blocks:
// Drift-Expect, the writer rows, Drift-Verify and Drift-Recheck.
var deferHaltRows = []string{expectRow, darnWriteRow, websterRow, verifyRow, recheckRow}

// Defer settles the baseline wait with no baseline taken,
// so the run goes on to code changes, and records `awaited_at` and `deferred` in the drift records.
// It is admitted while the run is blocked at Drift-Await, or at a Drift-Baseline capture halt.
// It is also admitted at a blocked halt of Drift-Expect, a writer row, Drift-Verify or Drift-Recheck.
// For those halts, the error must open with the defer halt prefix, optionally after a decision halt prefix.
// A run whose scope is neither off nor deferred is also admitted when its baseline check fails.
// A scope-off run has no baseline directory, so it never admits a deferral that way.
// It refuses while the capture lock is held, which captures and checks take.
func Defer(d Deps, st VerbStatus) error {
	admitted, err := deferAdmitted(d, st)
	if err != nil {
		return err
	}
	if !admitted {
		return fmt.Errorf("drift defer refused: the run is at %s (%s), and defer settles only a blocked %s, a %s capture halt, or a blocked %s halt whose reason names lym drift defer or whose baseline check fails; way forward: lym loom status shows the halt's own ways forward", orNone(st.Status.CurrentProducer), st.Status.State, captureRowAwait, captureRowBaseline, strings.Join(deferHaltRows, ", "))
	}
	if st.CaptureHeld {
		return errors.New("drift defer refused: a capture holds the capture lock; way forward: lym loom resume, whose row waits on a running capture, or, when a lym drift check or the drift gate holds it, wait for that check to finish, then run lym drift defer again")
	}
	records, err := ReadRecords(d)
	if err != nil {
		return err
	}
	if !records.ScopeFound {
		return errNoScope
	}
	return WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.AwaitedAt, scope.Deferred = stamp(d.now()), true
		return nil
	})
}

// deferAdmitted reports whether a deferral settles the halt that st holds.
func deferAdmitted(d Deps, st VerbStatus) (bool, error) {
	row := st.Status.CurrentProducer
	if st.Status.State != shedengine.StateBlocked {
		return false, nil
	}
	switch {
	case row == captureRowAwait:
		return true, nil
	case row == captureRowBaseline:
		return strings.HasPrefix(st.Status.Error, CaptureHaltPrefix), nil
	case !slices.Contains(deferHaltRows, row):
		return false, nil
	case strings.HasPrefix(strings.TrimPrefix(st.Status.Error, shedadapters.DecisionHaltPrefix), DeferHaltPrefix):
		return true, nil
	}
	records, err := ReadRecords(d)
	if err != nil {
		return false, err
	}
	if !records.ScopeFound || records.Scope.Scope == ScopeOff || records.Scope.Deferred {
		return false, nil
	}
	baseline, err := CheckBaseline(d)
	return baseline.Fault != "", err
}
