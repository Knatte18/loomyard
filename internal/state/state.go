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

// ErrRead sentinels a failure to read the state file's bytes off disk (I/O
// error other than the file simply not existing, e.g. permissions). Callers
// use errors.Is(err, ErrRead) to distinguish this "couldn't even read it"
// class of failure from a decode failure or an infra failure acquiring the
// lock, since a caller may want to retry or escalate each class differently.
var ErrRead = errors.New("state: read failed")

// ErrDecode sentinels a failure to strictly decode the state file's bytes as
// JSON of the expected shape (malformed JSON or an unknown field). Callers
// use errors.Is(err, ErrDecode) to distinguish "the file exists and was read
// but its contents are not a valid instance of T" from a raw read failure.
var ErrDecode = errors.New("state: decode failed")

// WriteJSON writes a value as indented JSON to the given path atomically.
// It creates missing parent directories, acquires an exclusive write lock on
// the caller-supplied lockPath, marshals the value to indented JSON (2 spaces), and writes
// it atomically via fsx.AtomicWriteBytes. The lock is released via defer.
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

// ReadJSON reads a JSON value from the given path into a value of type T.
// It creates missing parent directories, acquires a shared read lock on
// the caller-supplied lockPath, reads the file, and unmarshals it. Returns (zero, false, nil)
// if the file does not exist. Returns (zero, false, err) on other read errors.
// Returns (zero, false, err) on unmarshal errors (corruption is not swallowed).
// Returns (value, true, nil) on success. The lock is released via defer.
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

// ReadJSONStrict reads a JSON value from the given path into a value of type
// T, rejecting unknown fields instead of silently ignoring them. Unlike
// ReadJSON it does not call os.MkdirAll — a read must not have the
// side effect of creating directories that were never written to. (A
// sidecar .lock file is still taken by lock.AcquireReadLock, so the call is
// not fully side-effect-free.) It acquires a shared read lock on the
// caller-supplied lockPath, reads the file, and decodes it via
// json.Decoder.DisallowUnknownFields so that stale or mistyped fields are
// caught rather than silently dropped. Returns (zero, false, nil) if the
// file does not exist. A raw read failure (I/O error other than
// not-exist) is wrapped so errors.Is(err, ErrRead) is true; a decode failure
// (malformed JSON or an unknown field) is wrapped so errors.Is(err,
// ErrDecode) is true — callers classify the failure via errors.Is rather
// than string-matching. A lock.AcquireReadLock failure is returned wrapped
// as today, carrying neither sentinel: it is a third, infra-level failure
// mode the caller escalates rather than classifies as read-vs-decode.
// Returns (value, true, nil) on success. The lock is released via defer.
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
