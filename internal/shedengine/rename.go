// rename.go declares the status-file migration that carries a run across a row rename.
// It lives here, beside the other status-file writes, so a product that renames a row keeps an in-flight run resumable without writing the Shed-owned fields itself.

package shedengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// RenameRequest is everything RenameProducers needs.
// All three paths are told.
type RenameRequest struct {
	StatusPath     string
	LockPath       string
	StatusLockPath string
	// Renames maps each retired row name to the name its row carries now.
	Renames map[string]string
	// RunID is the addressed run-id the busy refusal names; see Shed.RunID.
	RunID string
}

// RenameProducers rewrites every retired row name the status file carries onto its successor: current_producer, pause_before, pause_after and each history entry's producer.
// It reports whether it wrote the file.
// An absent status file, and one that names no retired row, are left untouched and take no lock, so a caller may run it before every verb.
// A run lock held by a live driver refuses with ErrShedBusy and leaves the file unchanged.
// Like Goto, it calls no CommitStatus: the next step's persist commits the file.
func RenameProducers(req RenameRequest) (bool, error) {
	st, found, err := state.ReadJSONNoMkdir[Status](req.StatusPath, req.StatusLockPath)
	// A run directory not yet created fails the status lock open with not-exist; that is an absent status file too.
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("shedengine: read status file %q: %w", req.StatusPath, err)
	}
	if err != nil || !found || !namesRetiredRow(st, req.Renames) {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(req.LockPath), 0o755); err != nil {
		return false, fmt.Errorf("shedengine: create run lock parent dir: %w", err)
	}
	runLock, locked, err := lock.TryAcquireWriteLock(req.LockPath)
	if err != nil {
		return false, fmt.Errorf("shedengine: acquire run lock %q: %w", req.LockPath, err)
	}
	if !locked {
		return false, fmt.Errorf("%w: %q; %s", ErrShedBusy, req.LockPath, busyWayForward(req.RunID))
	}
	defer runLock.Release()

	err = state.UpdateJSON(req.StatusPath, req.StatusLockPath, func(cur Status, found bool) (Status, error) {
		if !found {
			return Status{}, fmt.Errorf("shedengine: status file %q vanished mid-rename; Shed refuses to create one", req.StatusPath)
		}
		cur.CurrentProducer = renamed(cur.CurrentProducer, req.Renames)
		cur.PauseBefore = renamed(cur.PauseBefore, req.Renames)
		cur.PauseAfter = renamed(cur.PauseAfter, req.Renames)
		for i := range cur.History {
			cur.History[i].Producer = renamed(cur.History[i].Producer, req.Renames)
		}
		return cur, nil
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

// namesRetiredRow reports whether any field RenameProducers rewrites holds a key of renames.
func namesRetiredRow(st Status, renames map[string]string) bool {
	names := []string{st.CurrentProducer, st.PauseBefore, st.PauseAfter}
	for _, entry := range st.History {
		names = append(names, entry.Producer)
	}
	for _, name := range names {
		if _, retired := renames[name]; retired {
			return true
		}
	}

	return false
}

// renamed returns the successor renames maps name to, or name itself when it is not retired.
func renamed(name string, renames map[string]string) string {
	if successor, retired := renames[name]; retired {
		return successor
	}

	return name
}
