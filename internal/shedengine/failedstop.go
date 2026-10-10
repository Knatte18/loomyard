// failedstop.go declares the status-file write that records a failure stop for a run no step is driving.
// It lives here, beside the other status-file writes, so State and Error stay written by Shed alone.

package shedengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// FailedStopRequest is everything WriteFailedStop needs.
// All three paths are told.
type FailedStopRequest struct {
	StatusPath     string
	LockPath       string
	StatusLockPath string
	// Error is the failure text the status file's error field records.
	Error string
	// Transient is the class of the failure, empty when it is not transient.
	Transient TransientClass
	// WayForward is the clause the recorded error ends in, joined as "; way forward: <clause>"; empty adds none.
	WayForward string
}

// FailedStopResult reports what WriteFailedStop did; exactly one field is true.
type FailedStopResult struct {
	// Written is true when the file read running and now reads failed.
	Written bool
	// Busy is true when another holder has the run lock; nothing was written.
	Busy bool
	// NotRunning is true when the file read any state but running; it was left untouched.
	NotRunning bool
}

// WriteFailedStop writes state failed, the error text and the transient class into a status file that still reads running, leaving history, pause_requested and both pause conditions as they are.
// It takes the run lock without waiting and reports Busy when another holder has it.
// It runs no CommitStatus: the next step's persist commits the file.
// A missing or unreadable status file is an error.
func WriteFailedStop(req FailedStopRequest) (FailedStopResult, error) {
	if err := os.MkdirAll(filepath.Dir(req.LockPath), 0o755); err != nil {
		return FailedStopResult{}, fmt.Errorf("shedengine: create run lock parent dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(req.StatusLockPath), 0o755); err != nil {
		return FailedStopResult{}, fmt.Errorf("shedengine: create status lock parent dir: %w", err)
	}
	runLock, locked, err := lock.TryAcquireWriteLock(req.LockPath)
	if err != nil {
		return FailedStopResult{}, fmt.Errorf("shedengine: acquire run lock %q: %w", req.LockPath, err)
	}
	if !locked {
		return FailedStopResult{Busy: true}, nil
	}
	defer runLock.Release()

	errorText := req.Error
	if req.WayForward != "" {
		errorText += "; way forward: " + req.WayForward
	}

	// The strict read refuses a malformed file and leaves a file in any other state byte-for-byte untouched.
	current, found, err := state.ReadJSONStrict[Status](req.StatusPath, req.StatusLockPath)
	if err != nil {
		return FailedStopResult{}, fmt.Errorf("shedengine: read status file %q: %w", req.StatusPath, err)
	}
	if !found {
		return FailedStopResult{}, fmt.Errorf("shedengine: status file %q does not exist; Shed never seeds one", req.StatusPath)
	}
	if current.State != StateRunning {
		return FailedStopResult{NotRunning: true}, nil
	}

	result := FailedStopResult{}
	err = state.UpdateJSON(req.StatusPath, req.StatusLockPath, func(cur Status, found bool) (Status, error) {
		if !found {
			return Status{}, fmt.Errorf("shedengine: status file %q does not exist; Shed never seeds one", req.StatusPath)
		}
		if cur.State != StateRunning {
			result.NotRunning = true
			return cur, nil
		}
		cur.State = StateFailed
		cur.Error = errorText
		cur.Transient = string(req.Transient)
		cur.ParentNotice = ""
		prevLast := cur.Activity.Last
		cur.Activity = composeActivity(cur.CurrentProducer, cur.History, StateFailed, errorText, "")
		cur.Activity.Last = prevLast
		result.Written = true
		return cur, nil
	})
	if err != nil {
		return FailedStopResult{}, err
	}
	return result, nil
}
