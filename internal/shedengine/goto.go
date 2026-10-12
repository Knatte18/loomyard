// goto.go declares the status-file mutation behind `lyx shed goto`.
// It lives here, beside the other status-file writes, so the generic verb body in internal/shedverbs stays a thin caller that derives no path.

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

// GotoRequest is everything Goto needs.
// All three paths are told.
// Producers needs only Name, Segment, OnDone and OnStuck, so a Routing.Producers projection works.
type GotoRequest struct {
	StatusPath     string
	LockPath       string
	StatusLockPath string
	Producers      []ProducerDef
	Target         string
	// RunID is the addressed run-id the goto texts name; see Shed.RunID.
	RunID string
	// MissingStatusWayForward is the told clause for a missing status file; see Shed.MissingStatusWayForward.
	MissingStatusWayForward string
	// PauseWayForward is the told command the busy and running-run refusals name to stop the run at a boundary; see Shed.PauseWayForward.
	PauseWayForward string
}

// Goto moves a halted run onto the Target row and leaves it paused.
// It writes paused, never running: the next step resumes through the ordinary resume write, whereas running with the lock free would read as a crashed driver.
// The appended history entry carries the OutcomeGoto outcome, which ends the target segment's bounce episode.
// It calls no CommitStatus and sets no transient mark;
// the next step's persist commits the file, as pause's write is.
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

	st, found, err := state.ReadJSONNoMkdir[Status](req.StatusPath, req.StatusLockPath)
	if err != nil {
		return Status{}, fmt.Errorf("shedengine: read status file %q: %w", req.StatusPath, err)
	}
	if !found {
		return Status{}, fmt.Errorf("shedengine: status file %q does not exist; Shed never seeds one; %s", req.StatusPath, missingStatusClause(req.MissingStatusWayForward))
	}
	if !st.State.valid() {
		return Status{}, fmt.Errorf("shedengine: status file %q carries an invalid state %q", req.StatusPath, st.State)
	}
	if st.State == StateDone {
		return Status{}, fmt.Errorf("shedengine: status file %q is done; goto never re-opens a finished run; way forward: seed a new run", req.StatusPath)
	}
	if _, ok := findProducer(req.Producers, req.Target); !ok {
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

	var written Status
	err = state.UpdateJSON(req.StatusPath, req.StatusLockPath, func(cur Status, found bool) (Status, error) {
		if !found {
			return Status{}, fmt.Errorf("shedengine: status file %q vanished mid-goto; Shed refuses to create one", req.StatusPath)
		}
		cur.CurrentProducer = req.Target
		cur.State = StatePaused
		cur.Error = ""
		cur.Transient = ""
		cur.PauseRequested = false
		cur.PauseBefore = ""
		cur.PauseAfter = ""
		cur.History = append(cur.History, HistoryEntry{Producer: req.Target, Outcome: OutcomeGoto, At: nowRFC3339()})
		cur.Activity = composeActivity(req.Target, cur.History, StatePaused, "", "")
		written = cur
		return cur, nil
	})
	if err != nil {
		return Status{}, err
	}
	return written, nil
}

// CheckGotoTarget returns nil when target names a producer of producers and the status st admits it as a goto target, and otherwise the refusal Goto returns for that case:
// the unknown-target refusal naming the producers, the awaiting refusal naming the admitted rows, or the past-the-current-row refusal.
// It reads and writes no file; runID is the addressed run-id the refusal texts name.
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

// RelaunchRequest is everything Relaunch needs.
// All three paths are told.
// Producers needs only Name, Segment, OnDone and OnStuck, so a Routing.Producers projection works.
type RelaunchRequest struct {
	StatusPath     string
	LockPath       string
	StatusLockPath string
	Producers      []ProducerDef
	// RunID is the addressed run-id the refusal texts name; see Shed.RunID.
	RunID string
	// MissingStatusWayForward is the told clause for a missing status file; see Shed.MissingStatusWayForward.
	MissingStatusWayForward string
	// Target is the row to move the run onto; empty leaves the run where it is.
	Target string
	// Admit is called with the status the run holds once every check has passed, under the status lock, before anything is written.
	// An error it returns is returned unchanged from Relaunch and nothing is written.
	Admit func(cur Status) error
}

// Relaunch is the status-file mutation behind a driver's re-launch of a halted run.
// Under the run lock and one hold of the status lock, it refuses a missing file, an invalid state, a done run and a running run, checks a non-empty Target with CheckGotoTarget, then calls Admit.
// Only after Admit passes does it write: with a Target, it moves the run as Goto does, except that pause_requested, pause_before and pause_after stay as they stand; without one, the status is written unchanged.
// The checks run before Admit so a refused target never lets Admit write its side effect.
// A held run lock returns ErrShedBusy and Admit is never called.
// It calls no CommitStatus, as Goto does not.
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
			return Status{}, fmt.Errorf("shedengine: status file %q does not exist; Shed never seeds one; %s", req.StatusPath, missingStatusClause(req.MissingStatusWayForward))
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
		}
		if err := req.Admit(cur); err != nil {
			return Status{}, err
		}
		if req.Target != "" {
			cur.CurrentProducer = req.Target
			cur.State = StatePaused
			cur.Error = ""
			cur.Transient = ""
			cur.History = append(cur.History, HistoryEntry{Producer: req.Target, Outcome: OutcomeGoto, At: nowRFC3339()})
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

// gotoAdmitted returns the reference row's name and every admitted goto target in list order.
// The reference row is current_producer, or, when that names no row, the producer of the latest history entry that does;
// that entry's routed row is admitted too.
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
