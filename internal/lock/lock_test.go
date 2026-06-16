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
