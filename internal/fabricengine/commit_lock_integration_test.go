// commit_lock_integration_test.go holds the path helper for the combined write lock on .weft/weft.write.lock.
// Fabric.Commit takes that lock.
// The file is in package fabricengine because it reaches the unexported lock directory and lock file name.

package fabricengine

import (
	"path/filepath"
	"testing"
)

// weftWriteLockPath returns the path of f's combined commit write lock file.
// It creates the .weft lock directory if it does not exist.
// Every commit on f contends on this file.
func weftWriteLockPath(t *testing.T, f *Fabric) string {
	t.Helper()

	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		t.Fatalf("ensureWeftLockDir() error = %v", err)
	}
	return filepath.Join(lockDir, weftWriteLockFile)
}

// WeftWriteLockPathForTest returns the path of f's combined commit write lock file.
// It creates the .weft lock directory if needed, so the package's external tests can hold the lock.
func WeftWriteLockPathForTest(t *testing.T, f *Fabric) string {
	return weftWriteLockPath(t, f)
}
