// lock.go — file-based advisory locks (gofrs/flock).
//
// FileLock backs both an exclusive write lock and a shared read lock.
// Being a real OS file lock it coordinates across processes — the way Loomyard is used, one
// short-lived process per command.

package lock

import (
	"fmt"

	"github.com/gofrs/flock"
)

// FileLock wraps a file-based advisory lock (gofrs/flock) for both exclusive and shared access.
type FileLock struct {
	fl *flock.Flock
}

// AcquireWriteLock acquires an exclusive lock, blocking until available.
func AcquireWriteLock(lockPath string) (*FileLock, error) {
	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		return nil, fmt.Errorf("acquire write lock: %w", err)
	}
	return &FileLock{fl}, nil
}

// TryAcquireWriteLock attempts to acquire an exclusive lock without blocking.
// It reports (lock, true, nil) on success, and (nil, false, nil) when already held (not an error).
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

// AcquireReadLock acquires a shared lock, blocking until available.
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
