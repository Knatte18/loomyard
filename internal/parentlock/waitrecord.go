// waitrecord.go — the wait record a run keeps in its scratch directory while it waits for a parent lock.

package parentlock

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
)

const waitRecordName = "parent-lock-wait.json"

// Wait is the record of a run waiting for the lock on Branch, held by the run with slug Holder.
// PID and StartTime name the waiting process, so a record left by a dead step is recognisable.
type Wait struct {
	Branch    string    `json:"branch"`
	Holder    string    `json:"holder"`
	PID       int       `json:"pid"`
	StartTime string    `json:"start_time"`
	Started   time.Time `json:"started"`
}

// NewWait returns a Wait for the calling process, stamped with its pid and start time and the current time.
func NewWait(branch, holder string) Wait {
	pid := os.Getpid()
	startTime, _ := proc.StartTime(pid)
	return Wait{Branch: branch, Holder: holder, PID: pid, StartTime: startTime, Started: time.Now()}
}

// WaitRecordPath returns the wait record's path under the told run directory.
func WaitRecordPath(dir string) string {
	return filepath.Join(dir, waitRecordName)
}

// WriteWait writes w atomically at path, replacing any record already there.
func WriteWait(path string, w Wait) error {
	if err := writeJSONAtomic(path, w); err != nil {
		return fmt.Errorf("write parent lock wait record: %w", err)
	}
	return nil
}

// ReadWait returns the wait record at path.
// found is false for an absent record and for one whose writer is not alive under alive.
func ReadWait(path string, alive func(pid int, startTime string) bool) (Wait, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Wait{}, false, nil
	}
	if err != nil {
		return Wait{}, false, fmt.Errorf("%w: %s: %v", ErrUnreadable, path, err)
	}
	var w Wait
	if err := json.Unmarshal(data, &w); err != nil {
		return Wait{}, false, fmt.Errorf("%w: %s: %v", ErrUnreadable, path, err)
	}
	if !alive(w.PID, w.StartTime) {
		return Wait{}, false, nil
	}
	return w, true, nil
}

// RemoveWait removes the wait record at path; an absent file is success.
func RemoveWait(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove parent lock wait record: %w", err)
	}
	return nil
}
