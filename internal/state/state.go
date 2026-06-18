// state.go implements generic locked typed JSON I/O for persistent state.
//
// This package provides WriteJSON and ReadJSON to atomically read and write
// JSON-serialized values to disk with advisory file locking, ensuring concurrent
// readers and writers are properly synchronized.

package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
)

// WriteJSON writes a value as indented JSON to the given path atomically.
// It creates missing parent directories, acquires an exclusive write lock on
// path + ".lock", marshals the value to indented JSON (2 spaces), and writes
// it atomically via fsx.AtomicWriteBytes. The lock is released via defer.
func WriteJSON[T any](path string, v T) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	l, err := lock.AcquireWriteLock(path + ".lock")
	if err != nil {
		return fmt.Errorf("acquire write lock: %w", err)
	}
	defer l.Release()

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return fsx.AtomicWriteBytes(path, data)
}

// ReadJSON reads a JSON value from the given path into a value of type T.
// It creates missing parent directories, acquires a shared read lock on
// path + ".lock", reads the file, and unmarshals it. Returns (zero, false, nil)
// if the file does not exist. Returns (zero, false, err) on other read errors.
// Returns (zero, false, err) on unmarshal errors (corruption is not swallowed).
// Returns (value, true, nil) on success. The lock is released via defer.
func ReadJSON[T any](path string) (T, bool, error) {
	var zero T
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return zero, false, fmt.Errorf("mkdir: %w", err)
	}

	l, err := lock.AcquireReadLock(path + ".lock")
	if err != nil {
		return zero, false, fmt.Errorf("acquire read lock: %w", err)
	}
	defer l.Release()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return zero, false, nil
		}
		return zero, false, fmt.Errorf("read state: %w", err)
	}

	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return zero, false, fmt.Errorf("unmarshal state: %w", err)
	}

	return v, true, nil
}
