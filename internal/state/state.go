// state.go implements generic locked typed JSON I/O for persistent state.
//
// This package provides WriteJSON and ReadJSON to atomically read and write JSON-serialized values
// to disk with advisory file locking, ensuring concurrent readers and writers are properly
// synchronized.

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

	return writeJSONUnlocked(path, v)
}

// writeJSONUnlocked marshals v as indented JSON and writes it to path atomically, assuming the
// caller already holds whatever lock guards path.
// It exists so WriteJSON and UpdateJSON can share this body without either composing on top of the
// other's own internal lock acquisition.
func writeJSONUnlocked[T any](path string, v T) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return fsx.AtomicWriteBytes(path, data)
}

// ReadJSON reads a JSON value from the given path.
// Returns (zero, false, nil) if the file does not exist.
// Returns (value, true, nil) on success.
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

	return readJSONUnlocked[T](path)
}

// readJSONUnlocked reads and decodes a JSON value from path, assuming the caller already holds
// whatever lock guards path.
// Returns (zero, false, nil) if the file does not exist.
// It exists so ReadJSON and UpdateJSON can share this body without either composing on top of the
// other's own internal lock acquisition.
func readJSONUnlocked[T any](path string) (T, bool, error) {
	var zero T

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

// UpdateJSON performs a locked read-modify-write over the JSON value at path: it holds one
// exclusive lock across the read, the caller-supplied mutate, and the write, so a concurrent
// writer's value can never be clobbered by a payload this call composed from a base that writer
// already superseded.
// A missing file is not an error — mutate receives the zero T with found=false, matching
// ReadJSON's own miss contract.
// A decode failure on an existing file aborts before mutate ever runs, and no write happens;
// UpdateJSON never hands mutate a zero value to paper over a corrupt file, since a corrupt file's
// zero-value "fix" would silently discard whatever was actually on disk.
// UpdateJSON must never be composed from ReadJSON and WriteJSON — both acquire lockPath
// internally, so nesting one inside a held lock on the same path hangs rather than failing.
func UpdateJSON[T any](path, lockPath string, mutate func(cur T, found bool) (T, error)) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	l, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		return fmt.Errorf("acquire write lock: %w", err)
	}
	defer l.Release()

	cur, found, err := readJSONUnlocked[T](path)
	if err != nil {
		return err
	}

	next, err := mutate(cur, found)
	if err != nil {
		return err
	}

	return writeJSONUnlocked(path, next)
}

// ReadJSONStrict reads a JSON value from path, rejecting unknown fields.
// Returns (zero, false, nil) if the file does not exist.
// Unlike ReadJSON, does not create missing parent directories.
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
