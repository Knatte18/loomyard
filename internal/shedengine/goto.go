// goto.go declares the status-file mutations behind `lym shed goto` and the re-launch of a halted run.
// The verb bodies in internal/shedverbs call them and derive no status path.

package shedengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// GotoGuard is a recipe's check of a goto target, run after the engine's own checks.
// A non-empty refusal refuses the target and carries the recipe's own way forward;
// a non-nil err means the guard could not decide, which refuses too.
// The guard only decides: it moves no run and writes no file.
// Once it admits, a guard may end processes the move would leave running, but it still writes no file and moves no run.
type GotoGuard func(target string) (refusal string, err error)

// checkGuard runs guard for target and returns the refusal when it refuses or cannot decide.
// A nil guard admits every target.
// runID is the run-id the way-forward text names.
func checkGuard(guard GotoGuard, target, runID string) error {
	if guard == nil {
		return nil
	}
	refusal, err := guard(target)
	if err != nil {
		return fmt.Errorf("shedengine: goto target %q could not be checked: %w; way forward: re-run goto as %s", target, err, gotoCommand(runID, target))
	}
	if refusal != "" {
		return fmt.Errorf("shedengine: goto target %q refused: %s", target, refusal)
	}
	return nil
}

// GotoRequest carries everything a goto needs.
// Producers reads only Name, Segment, OnDone, OnStuck, SkipTo and Block.
// A projection that zeroes the other fields therefore works.
type GotoRequest struct {
	StatusPath     string
	LockPath       string
	StatusLockPath string
	Producers      []ProducerDef
	Target         string
	// RunID is the run-id the goto texts name.
	RunID string
	// MissingStatusWayForward is the way-forward text a missing-status error names.
	MissingStatusWayForward string
	// PauseWayForward is the command the busy and running-run refusals name to pause the run at a boundary.
	PauseWayForward string
	// By is the caller label recorded on the goto entry; empty records none.
	By string
	// Guard is the recipe's check of a target, run after the engine's own checks and before anything is written.
	// A nil Guard admits every target.
	Guard GotoGuard
}

// Goto moves a halted run onto the Target row and leaves it paused.
// It writes paused and never running, since a running run with the lock free would read as a crashed driver.
// The next step resumes through the ordinary resume write.
// The appended history entry carries the OutcomeGoto outcome, which leaves every row's spent bounce budget as it was.
// It names the target as the entered row, so the next step runs the target without evaluating its entry gate.
// The recipe's Guard runs after the engine's target check and before the write, under the run lock,
// so every refusal leaves the status file unchanged.
// It sets no transient mark and commits nothing;
// the next step's persist commits the file, as a pause write does.
func Goto(req GotoRequest) (Status, error) {
	if err := os.MkdirAll(filepath.Dir(req.LockPath), 0o755); err != nil {
		return Status{}, fmt.Errorf("shedengine: create run lock parent dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(req.StatusLockPath), 0o755); err != nil {
		return Status{}, fmt.Errorf("shedengine: create status lock parent dir: %w", err)
	}

	runLock, locked, err := lock.TryAcquireWriteLock(req.LockPath)
	if err != nil {
		return Status{}, fmt.Errorf("shedengine: acquire run lock %q: %w", req.LockPath, err)
	}
	if !locked {
		return Status{}, fmt.Errorf("%w: %q; %s", ErrShedBusy, req.LockPath, busyWayForward(req.RunID, pauseCommandFor(req.PauseWayForward, req.RunID, "")))
	}
	defer runLock.Release()

	st, found, err := state.ReadJSON[Status](req.StatusPath, req.StatusLockPath)
	if err != nil {
		return Status{}, fmt.Errorf("shedengine: read status file %q: %w", req.StatusPath, err)
	}
	if !found {
		return Status{}, fmt.Errorf("shedengine: status file %q does not exist; Shed never seeds one; %s", req.StatusPath, MissingStatusClause(req.MissingStatusWayForward))
	}
	if !st.State.valid() {
		return Status{}, fmt.Errorf("shedengine: status file %q carries an invalid state %q", req.StatusPath, st.State)
	}
	if st.State == StateDone {
		return Status{}, fmt.Errorf("shedengine: status file %q is done; goto never re-opens a finished run; way forward: seed a new run", req.StatusPath)
	}
	target, ok := findProducer(req.Producers, req.Target)
	if !ok {
		names := make([]string, len(req.Producers))
		for i, def := range req.Producers {
			names[i] = def.Name
		}
		return Status{}, fmt.Errorf("shedengine: goto target %q names no producer in the list; way forward: re-run goto with --to naming one of: %s", req.Target, strings.Join(names, ", "))
	}

	if st.State == StateRunning {
		return Status{}, fmt.Errorf("shedengine: goto moves only a halted run, and this run is running; way forward: %q then %s leaves the run paused at its next producer boundary, then re-run goto", pauseCommandFor(req.PauseWayForward, req.RunID, st.CurrentProducer), runVerb("step", req.RunID))
	}
	if err := CheckGotoTarget(req.Producers, st, req.Target, req.RunID); err != nil {
		return Status{}, err
	}
	if err := checkGuard(req.Guard, req.Target, req.RunID); err != nil {
		return Status{}, err
	}

	var written Status
	err = state.UpdateJSON(req.StatusPath, req.StatusLockPath, func(cur Status, found bool) (Status, error) {
		if !found {
			return Status{}, fmt.Errorf("shedengine: status file %q vanished mid-goto; Shed refuses to create one", req.StatusPath)
		}
		cur.CurrentProducer = req.Target
		// Naming the target as entered runs it without evaluating its entry gate.
		cur.EnteredRow = req.Target
		cur.State = StatePaused
		cur.Error = ""
		cur.Transient = ""
		cur.PauseRequested = false
		cur.PauseRequestedBy = ""
		cur.PauseBefore = ""
		cur.PauseAfter = ""
		cur.PauseBeforeBy = ""
		cur.PauseAfterBy = ""
		cur.PauseReason = labelled("goto to "+req.Target, req.By)
		cur.PendingBy = ""
		cur.History = append(cur.History, HistoryEntry{Producer: req.Target, Outcome: OutcomeGoto, At: nowRFC3339(), By: req.By, Block: target.Block})
		cur.Activity = composeActivity(req.Target, cur.History, StatePaused, "", "")
		written = cur
		return cur, nil
	})
	if err != nil {
		return Status{}, err
	}
	return written, nil
}

// CheckGotoTarget returns nil when st admits target as a goto target among producers.
// Otherwise it returns the refusal for the case:
// an unknown target names the producers, an awaiting run names the admitted rows,
// and a target past the current row is refused.
// A goto only moves a run back, so it never skips a row's own work or a review or approval row.
// It reads and writes no file; runID is the run-id the refusal texts name.
func CheckGotoTarget(producers []ProducerDef, st Status, target, runID string) error {
	if _, ok := findProducer(producers, target); !ok {
		names := make([]string, len(producers))
		for i, def := range producers {
			names[i] = def.Name
		}
		return fmt.Errorf("shedengine: goto target %q names no producer in the list; way forward: re-run goto with --to naming one of: %s", target, strings.Join(names, ", "))
	}
	reference, admitted := gotoAdmitted(producers, st)
	if slices.Contains(admitted, target) {
		return nil
	}
	if st.State == StateAwaiting {
		return fmt.Errorf("shedengine: goto target %q is not before the row %q the run is awaiting at; goto only moves a run back, so it never skips a row's own work or a review or approval row; %s shows the hand-off the run waits on; way forward: re-run goto with --to naming one of: %s", target, reference, runVerb("status", runID), strings.Join(admitted, ", "))
	}
	return fmt.Errorf("shedengine: goto target %q lies past the run's current row %q; goto only moves a run back, so it never skips a row's own work or a review or approval row; way forward: re-run goto with --to naming one of: %s", target, reference, strings.Join(admitted, ", "))
}

// RelaunchRequest carries everything a re-launch of a halted run needs.
// Producers reads only Name, Segment, OnDone, OnStuck, SkipTo and Block.
// A projection that zeroes the other fields therefore works.
type RelaunchRequest struct {
	StatusPath     string
	LockPath       string
	StatusLockPath string
	Producers      []ProducerDef
	// RunID is the run-id the refusal texts name.
	RunID string
	// MissingStatusWayForward is the way-forward text a missing-status error names.
	MissingStatusWayForward string
	// Target is the row to move the run onto; empty leaves the run where it is.
	Target string
	// Admit is called with the status the run holds once every check has passed, under the status lock.
	// It runs before anything is written; an error it returns comes back unchanged and nothing is written.
	// inEffect reports that the run already stands where Target puts it, so no goto runs and the status is unchanged.
	Admit func(cur Status) (inEffect bool, err error)
	// Guard is the recipe's check of a non-empty Target, run after the engine's own checks and before Admit.
	// A nil Guard admits every target.
	Guard GotoGuard
}

// Relaunch is the status-file mutation behind a driver's re-launch of a halted run.
// Under the run lock and one hold of the status lock, it refuses an absent, invalid, done or running status file.
// A non-empty Target is then checked as a goto target and by the recipe's Guard, and then Admit is called.
// Only after Admit passes does it write.
// With a Target, it moves the run as a goto does, but keeps pause_requested, pause_before and pause_after as they stand.
// It clears the entered row, so the target's entry gate is evaluated.
// Without a Target, the status is written unchanged.
// The checks run before Admit so a refused target never lets Admit write its side effect.
// When Admit reports the target already in effect, no goto is performed and the status is written unchanged, so the history gains no entry.
// A held run lock returns ErrShedBusy and Admit is never called.
// It commits nothing, as a goto does not.
func Relaunch(req RelaunchRequest) (Status, error) {
	if err := os.MkdirAll(filepath.Dir(req.LockPath), 0o755); err != nil {
		return Status{}, fmt.Errorf("shedengine: create run lock parent dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(req.StatusLockPath), 0o755); err != nil {
		return Status{}, fmt.Errorf("shedengine: create status lock parent dir: %w", err)
	}

	runLock, locked, err := lock.TryAcquireWriteLock(req.LockPath)
	if err != nil {
		return Status{}, fmt.Errorf("shedengine: acquire run lock %q: %w", req.LockPath, err)
	}
	if !locked {
		return Status{}, fmt.Errorf("%w: %q; %s", ErrShedBusy, req.LockPath, busyWayForward(req.RunID, pauseCommandFor("", req.RunID, "")))
	}
	defer runLock.Release()

	var written Status
	err = state.UpdateJSON(req.StatusPath, req.StatusLockPath, func(cur Status, found bool) (Status, error) {
		if !found {
			return Status{}, fmt.Errorf("shedengine: status file %q does not exist; Shed never seeds one; %s", req.StatusPath, MissingStatusClause(req.MissingStatusWayForward))
		}
		if !cur.State.valid() {
			return Status{}, fmt.Errorf("shedengine: status file %q carries an invalid state %q", req.StatusPath, cur.State)
		}
		if cur.State == StateDone {
			return Status{}, fmt.Errorf("shedengine: status file %q is done; relaunch never re-opens a finished run; way forward: seed a new run", req.StatusPath)
		}
		if cur.State == StateRunning {
			return Status{}, fmt.Errorf("shedengine: status file %q is running; another loop owns this run; way forward: %s shows the stop that loop reaches, and the re-launch runs only from a halted stop", req.StatusPath, runVerb("status", req.RunID))
		}
		if req.Target != "" {
			if err := CheckGotoTarget(req.Producers, cur, req.Target, req.RunID); err != nil {
				return Status{}, err
			}
			if err := checkGuard(req.Guard, req.Target, req.RunID); err != nil {
				return Status{}, err
			}
		}
		inEffect, err := req.Admit(cur)
		if err != nil {
			return Status{}, err
		}
		if req.Target != "" && !inEffect {
			// CheckGotoTarget has admitted the target, so the lookup finds it.
			target, _ := findProducer(req.Producers, req.Target)
			cur.CurrentProducer = req.Target
			// Clearing the entered row makes the next step evaluate the target's entry gate.
			cur.EnteredRow = ""
			cur.State = StatePaused
			cur.Error = ""
			cur.Transient = ""
			cur.PauseReason = "re-launched onto " + req.Target
			cur.PendingBy = ""
			cur.History = append(cur.History, HistoryEntry{Producer: req.Target, Outcome: OutcomeGoto, At: nowRFC3339(), Block: target.Block})
			cur.Activity = composeActivity(req.Target, cur.History, StatePaused, "", "")
		}
		written = cur
		return cur, nil
	})
	if err != nil {
		return Status{}, err
	}
	return written, nil
}

// resumableHalt reports whether s is a halt a step resumes from with a record of who resumed it: blocked, failed or awaiting.
func resumableHalt(s State) bool {
	return s == StateBlocked || s == StateFailed || s == StateAwaiting
}

// RecordResumer records by as the pending caller label of the status file at statusPath, under the status lock.
// It marks only a blocked, failed or awaiting run with no label pending already,
// so the first label of a resumed run stands while its loop children step it on.
// A non-empty by is the only mark; an empty one writes nothing, and neither does any other state.
// An absent status file returns nil, because the step that follows refuses it.
// A status file that does not decode returns the read error.
// It takes no run lock and the label is for reports only.
func RecordResumer(statusPath, statusLockPath, by string) error {
	if by == "" {
		return nil
	}
	cur, found, err := state.ReadJSON[Status](statusPath, statusLockPath)
	if err != nil {
		return fmt.Errorf("shedengine: read status file %q: %w", statusPath, err)
	}
	if !found || !resumableHalt(cur.State) || cur.PendingBy != "" {
		return nil
	}
	return state.UpdateJSON(statusPath, statusLockPath, func(cur Status, found bool) (Status, error) {
		if !found {
			return Status{}, fmt.Errorf("shedengine: status file %q vanished mid-record; Shed refuses to create one", statusPath)
		}
		if resumableHalt(cur.State) && cur.PendingBy == "" {
			cur.PendingBy = by
		}
		return cur, nil
	})
}

// gotoAdmitted returns the reference row's name and every admitted goto target in list order.
// The reference row is current_producer, or, when that names no row, the producer of the latest history entry that does;
// that entry's routed row is admitted too, found from its outcome:
// OnDone after a Done, the skip target after a skip, OnStuck after a Stuck and OnReroute after a reroute.
// An awaiting run admits only rows strictly before the reference row;
// paused, blocked and failed admit the reference row itself.
func gotoAdmitted(producers []ProducerDef, st Status) (reference string, admitted []string) {
	refIdx := -1
	routed := ""
	for i, def := range producers {
		if def.Name == st.CurrentProducer {
			refIdx = i
			break
		}
	}
	if refIdx < 0 {
		for h := len(st.History) - 1; h >= 0 && refIdx < 0; h-- {
			entry := st.History[h]
			for i, def := range producers {
				if def.Name != entry.Producer {
					continue
				}
				refIdx = i
				switch entry.Outcome {
				case Done:
					routed = def.OnDone
				case OutcomeSkipped:
					routed = def.skipTarget()
				case Stuck:
					routed = def.OnStuck
				case OutcomeReroute:
					routed = def.OnReroute
				}
				break
			}
		}
	}
	if refIdx < 0 {
		if len(producers) == 0 {
			return st.CurrentProducer, nil
		}
		return producers[0].Name, []string{producers[0].Name}
	}
	limit := refIdx
	if st.State != StateAwaiting {
		limit = refIdx + 1
	}
	for i, def := range producers {
		if i < limit || (routed != "" && def.Name == routed) {
			admitted = append(admitted, def.Name)
		}
	}
	return producers[refIdx].Name, admitted
}
