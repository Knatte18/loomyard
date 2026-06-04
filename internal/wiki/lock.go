package wiki

import (
	"fmt"

	"github.com/gofrs/flock"
)

// WriteLock wraps a file-based lock
type WriteLock struct {
	fl *flock.Flock
}

// AcquireWriteLock acquires an exclusive lock on the given path
func AcquireWriteLock(lockPath string) (*WriteLock, error) {
	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	return &WriteLock{fl}, nil
}

// Release releases the lock
func (l *WriteLock) Release() error {
	if err := l.fl.Unlock(); err != nil {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}
