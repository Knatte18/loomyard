// state.go implements generic locked typed JSON I/O for persistent state.
//
// This package provides WriteJSON and ReadJSON to atomically read and write
// JSON-serialized values to disk with advisory file locking, ensuring concurrent
// readers and writers are properly synchronized.

package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
)

// ErrRead sentinels a read failure other than not-exist.
var ErrRead = errors.New("state: read failed")

// ErrDecode sentinels a decode failure (malformed or unknown field).
var ErrDecode = errors.New("state: decode failed")

// WriteJSON writes a value as indented JSON to path atomically.
func WriteJSON[T any](path, lockPath string, v T) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	l, err := lock.AcquireWriteLock(lockPath)
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

// ReadJSON reads a JSON value from the given path. Returns (zero, false, nil)
// if the file does not exist. Returns (value, true, nil) on success.
func ReadJSON[T any](path, lockPath string) (T, bool, error) {
	var zero T
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return zero, false, fmt.Errorf("mkdir: %w", err)
	}

	l, err := lock.AcquireReadLock(lockPath)
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

// ReadJSONStrict reads a JSON value from path, rejecting unknown fields.
// Returns (zero, false, nil) if the file does not exist. Unlike ReadJSON,
// does not create missing parent directories.
func ReadJSONStrict[T any](path, lockPath string) (T, bool, error) {
	var zero T

	l, err := lock.AcquireReadLock(lockPath)
	if err != nil {
		return zero, false, fmt.Errorf("acquire read lock: %w", err)
	}
	defer l.Release()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return zero, false, nil
		}
		return zero, false, fmt.Errorf("%w: %v", ErrRead, err)
	}

	var v T
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return zero, false, fmt.Errorf("%w: %v", ErrDecode, err)
	}

	return v, true, nil
}
