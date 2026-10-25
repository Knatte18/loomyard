// verbs.go — the rules of the `lyx drift` verbs: what each admits from the run's status, and the scope record each writes.
//
// Every function takes the run's status as a VerbStatus the caller read, so the rules run on a fake.
// A refusal opens "drift <verb> refused:" or "drift:" and names its way forward; a refused verb writes nothing.

package driftengine

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// verifyRow is the row Drift-Verify runs at, where accept, withdraw, compare and the side capture are admitted.
const verifyRow = "Drift-Verify"

// VerbStatus is what a drift verb judges: the run status and the facts read beside it.
// The zero VerbStatus is a run with no current row, which every verb refuses.
type VerbStatus struct {
	// Status is the run's status file.
	Status shedengine.Status
	// AwaitRow is true when the run's recipe has a Drift-Await row.
	AwaitRow bool
	// CaptureHeld is true when the capture lock is held.
	CaptureHeld bool
	// AgentSession is true when the agent-session mark reads open.
	AgentSession bool
}

// AtAgentSession reports whether the caller is the Drift-Verify row's live agent session: the status is running at Drift-Verify and the mark is open.
func (s VerbStatus) AtAgentSession() bool {
	return s.Status.CurrentProducer == verifyRow && s.Status.State == shedengine.StateRunning && s.AgentSession
}

// AtDecisionHalt reports whether the run is blocked on a Drift-Verify decision halt.
func (s VerbStatus) AtDecisionHalt() bool {
	return s.Status.CurrentProducer == verifyRow && s.Status.State == shedengine.StateBlocked && strings.HasPrefix(s.Status.Error, shedadapters.DecisionHaltPrefix)
}

// wrongRow builds the refusal of a verb run at a row it does not admit.
func wrongRow(verb, row string, st VerbStatus) error {
	return fmt.Errorf("drift %s refused: the run is at %s (%s), and the verb runs only at %s; way forward: lyx loom status shows where the run is", verb, orNone(st.Status.CurrentProducer), st.Status.State, row)
}

// errNoScope is the refusal of a verb that needs a recorded scope.
var errNoScope = errors.New("drift: no scope is recorded yet; way forward: lyx loom status shows the row the run is at")

// RecordScope records the task's regression scope, the baseline agent's one write of it.
// It is admitted only at Drift-Baseline, once, for scope off, partial or full, with skip given only under partial and required there.
func RecordScope(p Paths, st VerbStatus, scope string, skip []string) error {
	if st.Status.CurrentProducer != captureRowBaseline {
		return wrongRow("scope", captureRowBaseline, st)
	}
	switch {
	case scope != ScopeOff && scope != ScopePartial && scope != ScopeFull:
		return fmt.Errorf("drift scope refused: %q is not a scope; way forward: pass --scope %s, %s or %s", scope, ScopeOff, ScopePartial, ScopeFull)
	case len(skip) > 0 && scope != ScopePartial:
		return fmt.Errorf("drift scope refused: --skip is given with scope %s; way forward: pass --skip only with --scope %s", scope, ScopePartial)
	case len(skip) == 0 && scope == ScopePartial:
		return fmt.Errorf("drift scope refused: scope %s names no test to skip; way forward: pass --skip <test> for each skipped test, or --scope %s", ScopePartial, ScopeFull)
	}
	return UpdateScope(p, func(cur ScopeRecord, found bool) (ScopeRecord, error) {
		if found {
			return cur, fmt.Errorf("drift: the scope is already recorded as %s; way forward: write baseline.md alone and end your turn", cur.Scope)
		}
		return ScopeRecord{Scope: scope, Skip: skip, RecordedAt: stamp(time.Now())}, nil
	})
}

// Accept records one drifted test as intended, on behalf of the Drift-Verify agent session or of a caller at a halt.
// It is admitted only at Drift-Verify, from a running status with the agent-session mark open or a blocked status;
// a running status with the mark closed is the row's own work, and any other state is an error or a transient stop that only lyx loom resume settles.
// A withdrawn test is accepted again only by a caller at a decision halt, and under scope full so is every test a caller accepts.
func Accept(p Paths, st VerbStatus, test, reason string) error {
	if st.Status.CurrentProducer != verifyRow {
		return wrongRow("accept", verifyRow, st)
	}
	by := AcceptedByCaller
	switch {
	case st.AtAgentSession():
		by = AcceptedByAgent
	case st.Status.State == shedengine.StateBlocked:
	case st.Status.State == shedengine.StateRunning:
		return errors.New("drift accept refused: the row is running its own compare and checks; way forward: wait for the row's halt (lyx loom status shows it), then accept there under the parent's ruling")
	default:
		return fmt.Errorf("drift accept refused: the run is %s at %s, and no acceptance settles that; way forward: lyx loom resume", st.Status.State, verifyRow)
	}
	decisionHalt := st.AtDecisionHalt()

	return UpdateScope(p, func(cur ScopeRecord, found bool) (ScopeRecord, error) {
		if !found {
			return cur, errNoScope
		}
		withdrawn := slices.Contains(cur.Withdrawn, test)
		switch {
		case withdrawn && by == AcceptedByAgent:
			return cur, fmt.Errorf("drift accept refused: the run's parent withdrew %s; way forward: fix its drift or report it in verify-report.md; only the parent re-accepts it at a decision halt", test)
		case by == AcceptedByCaller && !decisionHalt && (withdrawn || cur.Scope == ScopeFull):
			return cur, fmt.Errorf("drift accept refused: %s needs the parent's ruling; way forward: re-accept it at a decision halt, which lyx loom status shows", test)
		}

		acceptance := Acceptance{Test: test, Reason: reason, By: by, At: stamp(time.Now())}
		if cur.Scope == ScopeFull {
			acceptance.Ruling = RulingUnruled
			if by == AcceptedByCaller {
				acceptance.Ruling = RulingRuled
			}
		}
		kept := slices.DeleteFunc(slices.Clone(cur.Accepted), func(existing Acceptance) bool { return existing.Test == test })
		cur.Accepted = append(kept, acceptance)
		cur.Withdrawn = slices.DeleteFunc(slices.Clone(cur.Withdrawn), func(existing string) bool { return existing == test })
		return cur, nil
	})
}

// Withdraw moves an accepted test back to the compare, so its drift is judged again.
// It is admitted only at Drift-Verify and refuses a test that is not accepted.
func Withdraw(p Paths, st VerbStatus, test string) error {
	if st.Status.CurrentProducer != verifyRow {
		return wrongRow("withdraw", verifyRow, st)
	}
	return UpdateScope(p, func(cur ScopeRecord, found bool) (ScopeRecord, error) {
		if !found {
			return cur, errNoScope
		}
		accepted := slices.IndexFunc(cur.Accepted, func(a Acceptance) bool { return a.Test == test })
		if accepted < 0 {
			return cur, fmt.Errorf("drift withdraw refused: %s is not accepted; way forward: lyx drift status lists the accepted tests", test)
		}
		cur.Accepted = slices.Delete(slices.Clone(cur.Accepted), accepted, accepted+1)
		cur.Withdrawn = append(slices.DeleteFunc(slices.Clone(cur.Withdrawn), func(existing string) bool { return existing == test }), test)
		return cur, nil
	})
}

// Defer settles the baseline wait with no baseline taken, so the run goes on to code changes.
// It is admitted only while blocked at Drift-Await or at a Drift-Baseline capture halt, and refuses while a capture runs.
func Defer(p Paths, st VerbStatus) error {
	atAwaitHalt := st.Status.CurrentProducer == captureRowAwait && st.Status.State == shedengine.StateBlocked
	atCaptureHalt := st.Status.CurrentProducer == captureRowBaseline && st.Status.State == shedengine.StateBlocked && strings.HasPrefix(st.Status.Error, CaptureHaltPrefix)
	if !atAwaitHalt && !atCaptureHalt {
		return fmt.Errorf("drift defer refused: the run is at %s (%s), and defer settles only a %s halt or a %s capture halt; way forward: lyx loom status shows the halt's own ways forward", orNone(st.Status.CurrentProducer), st.Status.State, captureRowAwait, captureRowBaseline)
	}
	if st.CaptureHeld {
		return errors.New("drift defer refused: a capture is running; way forward: lyx loom resume, whose row waits on the running capture")
	}
	return UpdateScope(p, func(cur ScopeRecord, found bool) (ScopeRecord, error) {
		if !found {
			return cur, errNoScope
		}
		cur.AwaitedAt, cur.Deferred = stamp(time.Now()), true
		return cur, nil
	})
}

// Waive records the parent's waiver of the compare when no counted capture exists.
// It is admitted only while blocked at Drift-Verify with no counted capture, which is the halt that names no baseline.
func Waive(p Paths, st VerbStatus, reason string) error {
	if st.Status.CurrentProducer != verifyRow || st.Status.State != shedengine.StateBlocked {
		return fmt.Errorf("drift waive refused: the run is at %s (%s), and a waiver is admitted only at a blocked %s; way forward: lyx loom status shows where the run is", orNone(st.Status.CurrentProducer), st.Status.State, verifyRow)
	}
	capture, captureFound, err := ReadCapture(p)
	if err != nil {
		return err
	}
	return UpdateScope(p, func(cur ScopeRecord, found bool) (ScopeRecord, error) {
		if !found {
			return cur, errNoScope
		}
		if CountedCapture(cur, capture, captureFound, st.AwaitRow) {
			return cur, fmt.Errorf("drift waive refused: a counted capture exists, so this %s halt is no missing baseline; way forward: lyx loom resume, or the halt's own ways forward in lyx loom status", verifyRow)
		}
		cur.WaivedReason, cur.WaivedAt = reason, stamp(time.Now())
		return cur, nil
	})
}

// RunCount returns how many compare or side-capture runs the current agent session took, and 0 when no attempt was recorded.
func RunCount(p Paths) (int, error) {
	rec, _, err := readRuns(p)
	return rec.Count, err
}

// CheckVerifyRow checks the row `lyx drift <verb>` runs at: Drift-Verify, running or blocked.
// verb is the command as the caller typed it, and each refusal names it.
func CheckVerifyRow(st VerbStatus, verb string) error {
	if st.Status.CurrentProducer != verifyRow {
		return wrongRow(verb, verifyRow, st)
	}
	return nil
}
