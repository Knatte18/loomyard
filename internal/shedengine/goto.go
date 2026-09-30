// goto.go declares the status-file mutation behind `lyx shed goto`.
// It lives here, beside the other status-file writes, so the generic verb body in
// internal/shedverbs stays a thin caller that derives no path.

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
// It writes paused, never running: the next step resumes through the ordinary resume write,
// whereas running with the lock free would read as a crashed driver.
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
		return Status{}, fmt.Errorf("%w: %q; way forward: \"lyx shed pause\" asks the live driver to stop at its next producer boundary; check the holder with \"lyx shed status\", then retry", ErrShedBusy, req.LockPath)
	}
	defer runLock.Release()

	st, found, err := state.ReadJSONStrict[Status](req.StatusPath, req.StatusLockPath)
	if err != nil {
		return Status{}, fmt.Errorf("shedengine: read status file %q: %w", req.StatusPath, err)
	}
	if !found {
		return Status{}, fmt.Errorf("shedengine: status file %q does not exist; Shed never seeds one", req.StatusPath)
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
		return Status{}, fmt.Errorf("shedengine: goto target %q names no producer in the list; valid targets: %s", req.Target, strings.Join(names, ", "))
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
