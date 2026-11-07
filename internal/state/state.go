// state.go implements locked, atomic reads and writes of persistent state, as typed JSON or as raw bytes.
// Writes take an exclusive advisory file lock, and reads take a shared one.
// So concurrent readers and writers stay synchronized.

package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
)

// ErrRead is the sentinel a read failure other than not-exist wraps, so errors.Is can detect it.
var ErrRead = errors.New("state: read failed")

// ErrDecode is the sentinel a malformed file wraps, so errors.Is can tell it apart from a read failure.
var ErrDecode = errors.New("state: decode failed")

// WriteJSON writes a value as indented JSON to path atomically under the write lock at lockPath.
// It keeps every key of the file it replaces that the value's type does not declare.
// It creates the directories of path and lockPath.
func WriteJSON[T any](path, lockPath string, v T) error {
	if err := mkdirParents(path, lockPath); err != nil {
		return err
	}

	l, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		return fmt.Errorf("acquire write lock: %w", err)
	}
	defer l.Release()

	return writeJSONUnlocked(path, v)
}

// WriteFile writes data verbatim to path atomically under the write lock at lockPath.
// It creates the directories of path and lockPath.
// It does no marshalling and keeps nothing of the file it replaces,
// so it fits a record that is not JSON, such as a patch.
func WriteFile(path, lockPath string, data []byte) error {
	if err := mkdirParents(path, lockPath); err != nil {
		return err
	}

	l, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		return fmt.Errorf("acquire write lock: %w", err)
	}
	defer l.Release()

	return fsx.AtomicWriteBytes(path, data)
}

// mkdirParents creates the directories of a record at path and of its lock at lockPath.
// The lock may sit in another tree.
func mkdirParents(path, lockPath string) error {
	for _, dir := range []string{filepath.Dir(path), filepath.Dir(lockPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
	}
	return nil
}

// writeJSONUnlocked marshals v as indented JSON and writes it to path atomically.
// The caller must already hold the lock that guards path.
// It keeps every key of the file it replaces that T does not declare.
// It takes no lock itself, because acquiring the same lock twice on one path hangs.
func writeJSONUnlocked[T any](path string, v T) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	replaced, err := os.ReadFile(path)
	switch {
	case err == nil:
		// A file that does not parse adds nothing: v is written alone over it.
		if json.Valid(replaced) {
			data = keepUnknown(reflect.TypeFor[T](), replaced, data)
		}
	case !os.IsNotExist(err):
		return fmt.Errorf("read state to keep its unknown keys: %w", err)
	}

	var indented bytes.Buffer
	if err := json.Indent(&indented, data, "", "  "); err != nil {
		return fmt.Errorf("indent state: %w", err)
	}

	return fsx.AtomicWriteBytes(path, indented.Bytes())
}

// ReadJSON reads a JSON value from path under a shared lock at lockPath, accepting fields T does not declare.
// It creates no directory: a file whose directory is absent reads as a missing file.
// When the lock file's directory is absent, it reads without the lock.
// No writer can hold a lock there, and every write replaces the file atomically.
// Returns (zero, false, nil) if the file does not exist and (value, true, nil) on success.
// A read failure other than not-exist wraps ErrRead,
// and a malformed file wraps ErrDecode.
func ReadJSON[T any](path, lockPath string) (T, bool, error) {
	var zero T

	l, err := lock.AcquireReadLock(lockPath)
	switch {
	case err == nil:
		defer l.Release()
	case !errors.Is(err, fs.ErrNotExist):
		return zero, false, fmt.Errorf("acquire read lock: %w", err)
	}

	return readJSONUnlocked[T](path)
}

// readJSONUnlocked reads and decodes a JSON value from path.
// The caller must already hold the lock that guards path.
// Returns (zero, false, nil) if the file does not exist.
// It takes no lock itself, because acquiring the same lock twice on one path hangs.
func readJSONUnlocked[T any](path string) (T, bool, error) {
	var zero T

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return zero, false, nil
		}
		return zero, false, fmt.Errorf("%w: read state: %w", ErrRead, err)
	}

	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		// The error wraps ErrDecode and the json error via %w.
		// errors.Is tells a decode failure from a read or lock failure.
		// loomshed.Seed relies on this to refuse a present but undecodable status file.
		return zero, false, fmt.Errorf("%w: unmarshal state: %w", ErrDecode, err)
	}

	return v, true, nil
}

// UpdateJSON performs a locked read-modify-write over the JSON value at path.
// It holds one exclusive lock across the read, the caller-supplied mutate, and the write.
// This keeps a concurrent writer's value from being overwritten by a payload built on a superseded base.
// A missing file is not an error: mutate receives the zero T with found=false.
// A decode failure on an existing file aborts before mutate runs, and no write happens.
// The function never hands mutate a zero value in its place, since that would silently discard whatever is on disk.
// mutate must not call any function that takes lockPath, since nesting hangs rather than fails.
// It creates the directories of path and lockPath.
func UpdateJSON[T any](path, lockPath string, mutate func(cur T, found bool) (T, error)) error {
	if err := mkdirParents(path, lockPath); err != nil {
		return err
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
