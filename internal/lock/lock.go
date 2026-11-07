// lock.go holds the advisory file locks, exclusive and shared, that lym processes take to coordinate with each other.
// Every acquire refuses a lock path inside a tracked lym directory or a tracked directory of the Bolt checkout.
// That keeps every lock file out of the changes a commit stages.

package lock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofrs/flock"
)

// boundedWaitRetryDelay is the pause between acquisition attempts while a bounded wait polls a held lock.
const boundedWaitRetryDelay = 10 * time.Millisecond

// ProbeGrace is how long an acquirer waits on a held lock before it refuses.
// A read-only probe takes the lock only briefly, to learn whether it is held.
// The wait keeps the acquirer from refusing over that probe.
const ProbeGrace = 250 * time.Millisecond

// FileLock wraps a file-based advisory lock (gofrs/flock) for both exclusive and shared access.
// Being an OS file lock, it coordinates across processes, since each lym command runs as one short-lived process.
type FileLock struct {
	fl *flock.Flock
}

// AcquireWriteLock acquires an exclusive lock, blocking until available.
func AcquireWriteLock(lockPath string) (*FileLock, error) {
	if err := refuseTrackedTree(lockPath); err != nil {
		return nil, err
	}
	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		return nil, fmt.Errorf("acquire write lock: %w", err)
	}
	return &FileLock{fl}, nil
}

// TryAcquireWriteLock attempts to acquire an exclusive lock without blocking.
// It reports (lock, true, nil) on success, and (nil, false, nil) when already held (not an error).
func TryAcquireWriteLock(lockPath string) (*FileLock, bool, error) {
	if err := refuseTrackedTree(lockPath); err != nil {
		return nil, false, err
	}
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

// AcquireWriteLockWithin acquires an exclusive lock, waiting at most wait for it.
// It reports (lock, true, nil) on success.
// It reports (nil, false, nil) when the lock is still held once wait runs out, and that is not an error.
func AcquireWriteLockWithin(lockPath string, wait time.Duration) (*FileLock, bool, error) {
	if err := refuseTrackedTree(lockPath); err != nil {
		return nil, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	fl := flock.New(lockPath)
	_, err := fl.TryLockContext(ctx, boundedWaitRetryDelay)
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("acquire write lock within %s: %w", wait, err)
	}
	return &FileLock{fl}, true, nil
}

// AcquireReadLock acquires a shared lock, blocking until available.
func AcquireReadLock(lockPath string) (*FileLock, error) {
	if err := refuseTrackedTree(lockPath); err != nil {
		return nil, err
	}
	fl := flock.New(lockPath)
	if err := fl.RLock(); err != nil {
		return nil, fmt.Errorf("acquire read lock: %w", err)
	}
	return &FileLock{fl}, nil
}

// TryAcquireReadLock attempts to acquire a shared lock without blocking.
// It reports (lock, true, nil) on success, and (nil, false, nil) when an exclusive holder has it (not an error).
func TryAcquireReadLock(lockPath string) (*FileLock, bool, error) {
	if err := refuseTrackedTree(lockPath); err != nil {
		return nil, false, err
	}
	fl := flock.New(lockPath)
	locked, err := fl.TryRLock()
	if err != nil {
		return nil, false, fmt.Errorf("try acquire read lock: %w", err)
	}
	if !locked {
		return nil, false, nil
	}
	return &FileLock{fl}, true, nil
}

// Release gives up the lock, so that another holder can acquire it.
func (l *FileLock) Release() error {
	if err := l.fl.Unlock(); err != nil {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}
