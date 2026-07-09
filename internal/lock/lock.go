// lock.go — file-based advisory locks (gofrs/flock).
//
// FileLock backs both an exclusive write lock and a shared read lock. Being a
// real OS file lock it coordinates across processes — the way Loomyard is used, one
// short-lived process per command.

package lock

import (
	"fmt"

	"github.com/gofrs/flock"
)

// FileLock wraps a file-based advisory lock (gofrs/flock). The same type backs
// both exclusive (write) and shared (read) locks; Release drops whichever was
// taken. Because it is a real OS file lock it coordinates across processes — the
// way Loomyard is actually used, one short-lived process per command.
type FileLock struct {
	fl *flock.Flock
}

// AcquireWriteLock acquires an exclusive lock on lockPath, blocking until it is
// available. While held, no other exclusive or shared lock on the path succeeds.
func AcquireWriteLock(lockPath string) (*FileLock, error) {
	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		return nil, fmt.Errorf("acquire write lock: %w", err)
	}
	return &FileLock{fl}, nil
}

// TryAcquireWriteLock attempts to acquire an exclusive lock on lockPath
// without blocking: it returns (lock, true, nil) on success, and (nil,
// false, nil) — not an error — when the lock is already held by someone
// else, so a caller can fail fast with its own descriptive message ("this
// resource is already in use") instead of hanging until a possibly
// long-running holder releases it. A non-nil error is reserved for an
// actual OS-level failure to even attempt the lock.
func TryAcquireWriteLock(lockPath string) (*FileLock, bool, error) {
	fl := flock.New(lockPath)
	locked, err := fl.TryLock()
	if err != nil {
		return nil, false, fmt.Errorf("try acquire write lock: %w", err)
	}
	if !locked {
		return nil, false, nil
	}
	return &FileLock{fl}, true, nil
}

// AcquireReadLock acquires a shared lock on lockPath, blocking until it is
// available. Multiple readers may hold it at once; it blocks only while a writer
// holds the exclusive lock. Used to fence reads of tasks.json against the brief
// instant a writer is swapping the file in (see store.Save / store.Load).
func AcquireReadLock(lockPath string) (*FileLock, error) {
	fl := flock.New(lockPath)
	if err := fl.RLock(); err != nil {
		return nil, fmt.Errorf("acquire read lock: %w", err)
	}
	return &FileLock{fl}, nil
}

// Release releases the lock.
func (l *FileLock) Release() error {
	if err := l.fl.Unlock(); err != nil {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}
