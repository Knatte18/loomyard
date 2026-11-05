// wake.go holds a run's wake record: the stamp a row writes after each completed wake, which the Go driver reads to tell a loop that lived through a wake from one that kept dying.

package shedrun

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/state"
)

// wakeStamp is the content of the wake record: the row that completed a wake and when.
type wakeStamp struct {
	Row string    `json:"row"`
	At  time.Time `json:"at"`
}

// WakeRecord returns the path to the ephemeral, never-tracked wake record of the run, wake.json under ScratchDir(l, runID).
func WakeRecord(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "wake.json")
}

// WriteWakeStamp writes the stamp of a wake the row completed at at to the wake record at path, atomically, creating its directory and replacing the stamp already there.
func WriteWakeStamp(path, row string, at time.Time) error {
	if err := state.WriteJSON(path, path+".lock", wakeStamp{Row: row, At: at}); err != nil {
		return fmt.Errorf("shedrun: write the wake record: %w", err)
	}
	return nil
}

// ReadWakeStamp reads the time of the last wake the row completed from the wake record at path.
// An absent record, and one that names another row, read as not found.
func ReadWakeStamp(path, row string) (time.Time, bool, error) {
	stamp, found, err := state.ReadJSON[wakeStamp](path, path+".lock")
	if err != nil {
		return time.Time{}, false, fmt.Errorf("shedrun: read the wake record: %w", err)
	}
	if !found || stamp.Row != row {
		return time.Time{}, false, nil
	}
	return stamp.At, true, nil
}
