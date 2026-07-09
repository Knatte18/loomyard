// lock_test.go — unit tests for file locking (lock.go).
//
// Acquire/release and exclusive-lock contention between holders.

package lock_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
)

func TestAcquireWriteLock(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("acquires lock and creates lock file", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "test.lock")

		lock, err := lock.AcquireWriteLock(lockPath)
		if err != nil {
			t.Fatalf("AcquireWriteLock failed: %v", err)
		}
		defer lock.Release()

		// Check that lock file was created
		_, err = os.Stat(lockPath)
		if err != nil {
			t.Fatalf("lock file not created: %v", err)
		}
	})

	t.Run("release succeeds after acquire", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "test2.lock")

		lock, err := lock.AcquireWriteLock(lockPath)
		if err != nil {
			t.Fatalf("AcquireWriteLock failed: %v", err)
		}

		if err := lock.Release(); err != nil {
			t.Fatalf("Release failed: %v", err)
		}
	})

	t.Run("acquire after release succeeds", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "test3.lock")

		// First acquire and release
		lock1, err := lock.AcquireWriteLock(lockPath)
		if err != nil {
			t.Fatalf("first AcquireWriteLock failed: %v", err)
		}
		if err := lock1.Release(); err != nil {
			t.Fatalf("first Release failed: %v", err)
		}

		// Second acquire should succeed
		lock2, err := lock.AcquireWriteLock(lockPath)
		if err != nil {
			t.Fatalf("second AcquireWriteLock failed: %v", err)
		}
		defer lock2.Release()
	})
}

// TestTryAcquireWriteLock proves the non-blocking contention contract:
// success reports (lock, true, nil); a second attempt against an
// already-held path reports (nil, false, nil) — never an error — so a
// caller can fail fast with its own message rather than being told the
// underlying flock error; and a released lock is immediately re-acquirable.
func TestTryAcquireWriteLock(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("succeeds and creates the lock file", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "try1.lock")

		l, ok, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil {
			t.Fatalf("TryAcquireWriteLock() error = %v; want nil", err)
		}
		if !ok {
			t.Fatalf("TryAcquireWriteLock() ok = false; want true on an unheld path")
		}
		defer l.Release()

		if _, err := os.Stat(lockPath); err != nil {
			t.Fatalf("lock file not created: %v", err)
		}
	})

	t.Run("fails fast, without an error, against an already-held path", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "try2.lock")

		first, ok, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil || !ok {
			t.Fatalf("first TryAcquireWriteLock() = (%v, %v, %v); want (lock, true, nil)", first, ok, err)
		}
		defer first.Release()

		second, ok, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil {
			t.Errorf("second TryAcquireWriteLock() error = %v; want nil (contention is not an error)", err)
		}
		if ok {
			t.Errorf("second TryAcquireWriteLock() ok = true; want false while the first lock is held")
		}
		if second != nil {
			t.Errorf("second TryAcquireWriteLock() lock = %v; want nil", second)
		}
	})

	t.Run("re-acquirable after release", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "try3.lock")

		first, ok, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil || !ok {
			t.Fatalf("first TryAcquireWriteLock() = (%v, %v, %v); want (lock, true, nil)", first, ok, err)
		}
		if err := first.Release(); err != nil {
			t.Fatalf("Release() error = %v; want nil", err)
		}

		second, ok, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil || !ok {
			t.Fatalf("second TryAcquireWriteLock() = (%v, %v, %v); want (lock, true, nil) after release", second, ok, err)
		}
		defer second.Release()
	})
}

func TestAcquireReadLock(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("acquires read lock and creates lock file", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "read.lock")

		lock, err := lock.AcquireReadLock(lockPath)
		if err != nil {
			t.Fatalf("AcquireReadLock failed: %v", err)
		}
		defer lock.Release()

		// Check that lock file was created
		_, err = os.Stat(lockPath)
		if err != nil {
			t.Fatalf("lock file not created: %v", err)
		}
	})

	t.Run("release succeeds after acquire read lock", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "read2.lock")

		lock, err := lock.AcquireReadLock(lockPath)
		if err != nil {
			t.Fatalf("AcquireReadLock failed: %v", err)
		}

		if err := lock.Release(); err != nil {
			t.Fatalf("Release failed: %v", err)
		}
	})
}

// NOTE: gofrs/flock's OS-level implementation (flock on Unix, LockFileEx on Windows)
// guarantees that locks are automatically released on process death. No subprocess
// test is required to verify this property.
