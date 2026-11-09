// exitrecord.go declares the record a coverage map refresh writes as it exits.
// It reads and writes the record through internal/state.

package testcover

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// ExitRecord is what one coverage map refresh leaves as it exits.
// Its process ID tells a reader the refresh it waited on from any other.
type ExitRecord struct {
	PID int `json:"pid"`
	// Unbuilt counts the stale packages the refresh's package cap left unbuilt.
	Unbuilt int `json:"unbuilt"`
}

// ExitRecordPath returns the path of the exit record under the maps directory dir.
func ExitRecordPath(dir string) string {
	return filepath.Join(dir, lymdirs.TestMapExitFileName)
}

// WriteExitRecord replaces the exit record under the maps directory dir with r, atomically.
// It keeps each key of the replaced record that ExitRecord does not declare.
func WriteExitRecord(dir string, r ExitRecord) error {
	path := ExitRecordPath(dir)
	return state.WriteJSON(path, lock.TrackedFileLockPath(path, ".lock"), r)
}

// ReadExitRecord reads the exit record under the maps directory dir, and reports whether one exists.
// A key the record does not declare is ignored, never refused.
func ReadExitRecord(dir string) (ExitRecord, bool, error) {
	path := ExitRecordPath(dir)
	return state.ReadJSON[ExitRecord](path, lock.TrackedFileLockPath(path, ".lock"))
}
