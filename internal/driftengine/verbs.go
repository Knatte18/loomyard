// verbs.go — the rules of the `lym drift` verbs: what each admits from the run's status, and the scope record each writes.
//
// Every function takes the run's status as a VerbStatus the caller read, so the rules run on a fake.
// A refusal opens "drift <verb> refused:" or "drift:" and names its way forward; a refused verb writes nothing.

package driftengine

import (
	"errors"
	"fmt"
	"os"
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
	// CaptureHeld is true when the capture lock is held.
	CaptureHeld bool
	// AgentSession is true when the agent-session mark reads open.
	AgentSession bool
	// OperatorSession is true when the open agent session was spawned with the operator in its pane.
	OperatorSession bool
}

// AtAgentSession reports whether the caller is the Drift-Verify row's live agent session: the status is running at Drift-Verify and the mark is open.
func (s VerbStatus) AtAgentSession() bool {
	return s.Status.CurrentProducer == verifyRow && s.Status.State == shedengine.StateRunning && s.AgentSession
}

// AtOperatorSession reports whether the caller is the Drift-Verify row's live agent session and that session has the operator in its pane.
func (s VerbStatus) AtOperatorSession() bool {
	return s.AtAgentSession() && s.OperatorSession
}

// AtDecisionHalt reports whether the run is blocked on a Drift-Verify decision halt.
func (s VerbStatus) AtDecisionHalt() bool {
	return s.Status.CurrentProducer == verifyRow && s.Status.State == shedengine.StateBlocked && strings.HasPrefix(s.Status.Error, shedadapters.DecisionHaltPrefix)
}

// wrongRow builds the refusal of a verb run at a row it does not admit.
func wrongRow(verb, row string, st VerbStatus) error {
	return fmt.Errorf("drift %s refused: the run is at %s (%s), and the verb runs only at %s; way forward: lym loom status shows where the run is", verb, orNone(st.Status.CurrentProducer), st.Status.State, row)
}

// errNoScope is the refusal of a verb that needs a recorded scope.
var errNoScope = errors.New("drift: no scope is recorded yet; way forward: lym loom status shows the row the run is at")

// RecordScope records the task's regression scope off, partial or full, with skip given only under partial and required there.
// The baseline agent records it once, at a running Drift-Baseline.
// The run's parent records or replaces it at a blocked Drift-Baseline or Drift-Await, while no capture runs and before the baseline wait is settled.
// A replacement keeps the record's other fields, and drops the capture record when the new scope compares a test the capture skipped, since that capture holds no profile of it.
// The capture record is dropped before the scope is written, so a failed drop leaves the old scope and the call can be repeated.
func RecordScope(p Paths, st VerbStatus, scope string, skip []string) error {
	row := st.Status.CurrentProducer
	agent := row == captureRowBaseline && st.Status.State == shedengine.StateRunning
	parent := (row == captureRowBaseline || row == captureRowAwait) && st.Status.State == shedengine.StateBlocked
	if !agent && !parent {
		return fmt.Errorf("drift scope refused: the run is at %s (%s); the baseline agent records the scope at a running %s, and the run's parent records or replaces it at a blocked %s or %s; way forward: lym loom status shows where the run is", orNone(row), st.Status.State, captureRowBaseline, captureRowBaseline, captureRowAwait)
	}
	switch {
	case scope != ScopeOff && scope != ScopePartial && scope != ScopeFull:
		return fmt.Errorf("drift scope refused: %q is not a scope; way forward: pass --scope %s, %s or %s", scope, ScopeOff, ScopePartial, ScopeFull)
	case len(skip) > 0 && scope != ScopePartial:
		return fmt.Errorf("drift scope refused: --skip is given with scope %s; way forward: pass --skip only with --scope %s", scope, ScopePartial)
	case len(skip) == 0 && scope == ScopePartial:
		return fmt.Errorf("drift scope refused: scope %s names no test to skip; way forward: pass --skip <test> for each skipped test, or --scope %s", ScopePartial, ScopeFull)
	case parent && st.CaptureHeld:
		return errors.New("drift scope refused: a capture is running; way forward: wait for it to end (lym drift status shows it), then record the scope again")
	}

	return UpdateScope(p, func(cur ScopeRecord, found bool) (ScopeRecord, error) {
		switch {
		case !found:
			return ScopeRecord{Scope: scope, Skip: skip, RecordedAt: stamp(time.Now())}, nil
		case agent:
			return cur, fmt.Errorf("drift: the scope is already recorded as %s; way forward: write baseline.md alone and end your turn", cur.Scope)
		case cur.AwaitedAt != "":
			return cur, fmt.Errorf("drift scope refused: the baseline wait is already settled, so the scope no longer changes; way forward: settle the drift at the %s halt, whose ways forward lym loom status shows", verifyRow)
		}
		if scope != ScopeOff {
			if err := dropCaptureSkipping(p, skip); err != nil {
				return cur, err
			}
		}
		cur.Scope, cur.Skip, cur.RecordedAt = scope, skip, stamp(time.Now())
		return cur, nil
	})
}

// dropCaptureSkipping removes the capture record when the capture left out a test that skip does not, since that capture holds no profile of the test.
// An absent record is no error.
func dropCaptureSkipping(p Paths, skip []string) error {
	capture, found, err := ReadCapture(p)
	if err != nil || !found {
		return err
	}
	if !slices.ContainsFunc(capture.Skip, func(test string) bool { return !slices.Contains(skip, test) }) {
		return nil
	}
	if err := os.Remove(p.Capture); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("drift: drop the capture taken under the old scope: %w", err)
	}
	return nil
}

// Accept records one drifted test as intended, on behalf of the Drift-Verify agent session or of a caller at a halt.
// It is admitted only at Drift-Verify, from a running status with the agent-session mark open or a blocked status;
// a running status with the mark closed is the row's own work, and any other state is an error or a transient stop that only lym loom resume settles.
// A session with the operator in its pane records the operator's decision, which is ruled under scope full and may accept a withdrawn test again.
// Otherwise a withdrawn test is accepted again only by a caller at a decision halt, and under scope full so is every test a caller accepts.
// A test with no profile file under the baseline directory is refused, naming the test and the directory.
// The test's profile files that RunAcceptanceCapture left in the accepting directory are kept as the output the acceptance saw, replacing any an earlier acceptance kept, and an acceptance with none there is refused.
func Accept(p Paths, st VerbStatus, test, reason string) error {
	by, decisionHalt, err := acceptingCaller(st)
	if err != nil {
		return err
	}
	return UpdateScope(p, func(cur ScopeRecord, found bool) (ScopeRecord, error) {
		if err := admitAcceptance(p, cur, found, by, decisionHalt, test); err != nil {
			return cur, err
		}
		if err := keepAcceptedOutput(p, test); err != nil {
			return cur, err
		}

		acceptance := Acceptance{Test: test, Reason: reason, By: by, At: stamp(time.Now())}
		if cur.Scope == ScopeFull {
			acceptance.Ruling = RulingRuled
			if by == AcceptedByAgent {
				acceptance.Ruling = RulingUnruled
			}
		}
		kept := slices.DeleteFunc(slices.Clone(cur.Accepted), func(existing Acceptance) bool { return existing.Test == test })
		cur.Accepted = append(kept, acceptance)
		cur.Withdrawn = slices.DeleteFunc(slices.Clone(cur.Withdrawn), func(existing string) bool { return existing == test })
		cur.AcceptedProfileHead = ""
		return cur, nil
	})
}

// CheckAccept runs every check Accept runs before it keeps the test's output, and writes nothing, so a caller refuses an acceptance before it captures that output.
func CheckAccept(p Paths, st VerbStatus, test string) error {
	by, decisionHalt, err := acceptingCaller(st)
	if err != nil {
		return err
	}
	cur, found, err := ReadScope(p)
	if err != nil {
		return err
	}
	return admitAcceptance(p, cur, found, by, decisionHalt, test)
}

// acceptingCaller returns who an acceptance at st is by and whether st is a decision halt, or the refusal of an acceptance at st.
func acceptingCaller(st VerbStatus) (by string, decisionHalt bool, err error) {
	if st.Status.CurrentProducer != verifyRow {
		return "", false, wrongRow("accept", verifyRow, st)
	}
	switch {
	case st.AtOperatorSession():
		return AcceptedByOperator, false, nil
	case st.AtAgentSession():
		return AcceptedByAgent, false, nil
	case st.Status.State == shedengine.StateBlocked:
		return AcceptedByCaller, st.AtDecisionHalt(), nil
	case st.Status.State == shedengine.StateRunning:
		return "", false, errors.New("drift accept refused: the row is running its own compare and checks; way forward: wait for the row's halt (lym loom status shows it), then accept there under the parent's ruling")
	}
	return "", false, fmt.Errorf("drift accept refused: the run is %s at %s, and no acceptance settles that; way forward: lym loom resume", st.Status.State, verifyRow)
}

// admitAcceptance returns the refusal of an acceptance of test by by against the scope record cur, or nil when it is admitted.
func admitAcceptance(p Paths, cur ScopeRecord, found bool, by string, decisionHalt bool, test string) error {
	if !found {
		return errNoScope
	}
	withdrawn := slices.Contains(cur.Withdrawn, test)
	switch {
	case withdrawn && by == AcceptedByAgent:
		return fmt.Errorf("drift accept refused: the run's parent withdrew %s; way forward: fix its drift or report it in verify-report.md; only the parent re-accepts it at a decision halt", test)
	case by == AcceptedByCaller && !decisionHalt && (withdrawn || cur.Scope == ScopeFull):
		return fmt.Errorf("drift accept refused: %s needs the parent's ruling; way forward: re-accept it at a decision halt, which lym loom status shows", test)
	}
	return requireBaselineTest(p, test)
}

// Withdraw moves an accepted test back to the compare, so its drift is judged again, and drops the output its acceptance kept.
// It is admitted only at Drift-Verify and refuses a test that is not accepted.
// The output is dropped once the scope record is written, so a failed drop leaves a record that already compares the test again.
func Withdraw(p Paths, st VerbStatus, test string) error {
	if st.Status.CurrentProducer != verifyRow {
		return wrongRow("withdraw", verifyRow, st)
	}
	err := UpdateScope(p, func(cur ScopeRecord, found bool) (ScopeRecord, error) {
		if !found {
			return cur, errNoScope
		}
		accepted := slices.IndexFunc(cur.Accepted, func(a Acceptance) bool { return a.Test == test })
		if accepted < 0 {
			return cur, fmt.Errorf("drift withdraw refused: %s is not accepted; way forward: lym drift status lists the accepted tests", test)
		}
		cur.Accepted = slices.Delete(slices.Clone(cur.Accepted), accepted, accepted+1)
		cur.Withdrawn = append(slices.DeleteFunc(slices.Clone(cur.Withdrawn), func(existing string) bool { return existing == test }), test)
		cur.AcceptedProfileHead = ""
		return cur, nil
	})
	if err != nil {
		return err
	}
	return dropAcceptedOutput(p, test, nil)
}

// Defer settles the baseline wait with no baseline taken, so the run goes on to code changes.
// It is admitted only while blocked at Drift-Await or at a Drift-Baseline capture halt, and refuses while a capture runs.
func Defer(p Paths, st VerbStatus) error {
	atAwaitHalt := st.Status.CurrentProducer == captureRowAwait && st.Status.State == shedengine.StateBlocked
	atCaptureHalt := st.Status.CurrentProducer == captureRowBaseline && st.Status.State == shedengine.StateBlocked && strings.HasPrefix(st.Status.Error, CaptureHaltPrefix)
	if !atAwaitHalt && !atCaptureHalt {
		return fmt.Errorf("drift defer refused: the run is at %s (%s), and defer settles only a %s halt or a %s capture halt; way forward: lym loom status shows the halt's own ways forward", orNone(st.Status.CurrentProducer), st.Status.State, captureRowAwait, captureRowBaseline)
	}
	if st.CaptureHeld {
		return errors.New("drift defer refused: a capture is running; way forward: lym loom resume, whose row waits on the running capture")
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
		return fmt.Errorf("drift waive refused: the run is at %s (%s), and a waiver is admitted only at a blocked %s; way forward: lym loom status shows where the run is", orNone(st.Status.CurrentProducer), st.Status.State, verifyRow)
	}
	capture, captureFound, err := ReadCapture(p)
	if err != nil {
		return err
	}
	return UpdateScope(p, func(cur ScopeRecord, found bool) (ScopeRecord, error) {
		if !found {
			return cur, errNoScope
		}
		if CountedCapture(cur, capture, captureFound) {
			return cur, fmt.Errorf("drift waive refused: a counted capture exists, so this %s halt is no missing baseline; way forward: lym loom resume, or the halt's own ways forward in lym loom status", verifyRow)
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

// CheckVerifyRow checks the row `lym drift <verb>` runs at: Drift-Verify, running or blocked.
// verb is the command as the caller typed it, and each refusal names it.
func CheckVerifyRow(st VerbStatus, verb string) error {
	if st.Status.CurrentProducer != verifyRow {
		return wrongRow(verb, verifyRow, st)
	}
	return nil
}
