// looppid.go holds the files a detached loop leaves in the run's steps directory: its pid record and its envelope file.
// It declares their shapes and their atomic reads and writes, and the check on a loop id that names a file.

package shedverbs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/proc"
)

// TeardownLoopID is the loop id of the mark a pair's session end puts in place of a loop's pid record.
// No loop runs while the mark stands.
const TeardownLoopID = "teardown"

// LoopPIDRecord is the content of a loop's pid file: who the loop is and what step child it has in flight.
// A record whose Child is the zero value names no child, as before the loop's first child and as the teardown's mark.
type LoopPIDRecord struct {
	// LoopID names the loop; it keys the loop's delivered record.
	LoopID string `json:"loop_id"`
	// Loop is the loop process's own pid and start time.
	Loop proc.TreeRecord `json:"loop"`
	// Child is the in-flight step child's process tree.
	Child proc.TreeRecord `json:"child"`
	// CurrentProducer is the status file's current_producer when the child started.
	CurrentProducer string `json:"current_producer"`
	// HistoryLength is the status file's history length when the child started.
	HistoryLength int `json:"history_length"`
}

// hasChild reports whether the record names an in-flight child.
func (r LoopPIDRecord) hasChild() bool {
	return r.Child.PID != 0
}

// ReadLoopPIDRecord reads the pid file at path.
// It reports false for a file that does not exist, and an error for one that cannot be read or decoded.
func ReadLoopPIDRecord(path string) (LoopPIDRecord, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return LoopPIDRecord{}, false, nil
	}
	if err != nil {
		return LoopPIDRecord{}, false, fmt.Errorf("shedverbs: read the loop pid file %q: %w", path, err)
	}
	var rec LoopPIDRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return LoopPIDRecord{}, false, fmt.Errorf("shedverbs: decode the loop pid file %q: %w", path, err)
	}
	return rec, true, nil
}

// WriteLoopPIDRecord writes rec to the pid file at path atomically, creating the directory when it is missing.
func WriteLoopPIDRecord(path string, rec LoopPIDRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("shedverbs: encode the loop pid record: %w", err)
	}
	return writeFileAtomically(path, data)
}

// loopEnvelopeFile is the content of the loop envelope file and of a delivered record: the loop's id beside its envelope.
type loopEnvelopeFile struct {
	LoopID   string          `json:"loop_id"`
	Envelope json.RawMessage `json:"envelope"`
}

// readLoopEnvelopeFile reads the loop envelope file or delivered record at path.
// It reports false for a file that is missing, partial or names no loop.
func readLoopEnvelopeFile(path string) (loopEnvelopeFile, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return loopEnvelopeFile{}, false
	}
	var file loopEnvelopeFile
	if json.Unmarshal(data, &file) != nil || file.LoopID == "" || len(file.Envelope) == 0 {
		return loopEnvelopeFile{}, false
	}
	return file, true
}

// writeLoopEnvelopeFile writes envelope, the bytes of a printed loop envelope, to path under loopID, atomically.
func writeLoopEnvelopeFile(path, loopID string, envelope []byte) error {
	data, err := json.Marshal(loopEnvelopeFile{LoopID: loopID, Envelope: envelope})
	if err != nil {
		return fmt.Errorf("shedverbs: encode the loop envelope: %w", err)
	}
	return writeFileAtomically(path, data)
}

// writeFileAtomically writes data to path through a temporary file in the same directory and a rename, creating the directory when it is missing.
func writeFileAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("shedverbs: create %q: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("shedverbs: create a temporary file for %q: %w", path, err)
	}
	_, writeErr := temp.Write(data)
	closeErr := temp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(temp.Name())
		return fmt.Errorf("shedverbs: write %q: %w", path, err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		_ = os.Remove(temp.Name())
		return fmt.Errorf("shedverbs: replace %q: %w", path, err)
	}
	return nil
}

// validLoopID reports whether id is safe to join into a file name: non-empty, lowercase letters, digits and hyphens only.
func validLoopID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
