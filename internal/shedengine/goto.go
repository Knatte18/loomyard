// goto.go declares the status-file mutation behind `lyx shed goto`.
// It lives here, beside the other status-file writes, so the generic verb body in internal/shedverbs stays a thin caller that derives no path.

package shedengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// GotoRequest is everything Goto needs.
// All three paths are told.
// Producers needs only Name and Segment, so a Routing.Producers projection works.
type GotoRequest struct {
	StatusPath     string
	LockPath       string
	StatusLockPath string
	Producers      []ProducerDef
	Target         string
}

// Goto moves a halted run onto the Target row and leaves it paused.
// It writes paused, never running: the next step resumes through the ordinary resume write, whereas running with the lock free would read as a crashed driver.
// The appended history entry carries the OutcomeGoto outcome, which ends the target segment's bounce episode.
// It calls no CommitStatus and sets no transient mark; the next step's persist commits the file, as pause's write is.
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
		return Status{}, fmt.Errorf("%w: %q; %s", ErrShedBusy, req.LockPath, busyWayForward)
	}
	defer runLock.Release()

	st, found, err := state.ReadJSONStrict[Status](req.StatusPath, req.StatusLockPath)
	if err != nil {
		return Status{}, fmt.Errorf("shedengine: read status file %q: %w", req.StatusPath, err)
	}
	if !found {
		return Status{}, fmt.Errorf("shedengine: status file %q does not exist; Shed never seeds one; %s", req.StatusPath, missingStatusWayForward)
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
		return Status{}, fmt.Errorf("shedengine: goto moves only a halted run, and this run is running; way forward: \"lyx shed pause\" then \"lyx shed step\" leaves the run paused at its next producer boundary, then re-run goto")
	}
	reference, admitted := gotoAdmitted(req.Producers, st)
	if !containsName(admitted, req.Target) {
		if st.State == StateAwaiting {
			return Status{}, fmt.Errorf("shedengine: goto target %q is not before the row %q the run is awaiting at; goto only moves a run back, so it never skips a row's own work or a review or approval row; \"lyx shed status\" shows the hand-off the run waits on; way forward: re-run goto with --to naming one of: %s", req.Target, reference, strings.Join(admitted, ", "))
		}
		return Status{}, fmt.Errorf("shedengine: goto target %q lies past the run's current row %q; goto only moves a run back, so it never skips a row's own work or a review or approval row; way forward: re-run goto with --to naming one of: %s", req.Target, reference, strings.Join(admitted, ", "))
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

// gotoAdmitted returns the reference row's name and every admitted goto target in list order.
// The reference row is current_producer, or, when that names no row, the producer of the latest history entry that does;
// that entry's routed row is admitted too.
// An awaiting run admits only rows strictly before the reference row; paused, blocked and failed admit the reference row itself.
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

func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}
