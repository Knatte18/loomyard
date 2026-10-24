// teardown_helper_test.go holds the helpers that tests of more than one tier share.

package pairteardown

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

// mustHoldLock takes the exclusive lock at lockPath and fails the test when it cannot.
func mustHoldLock(t *testing.T, lockPath string) *lock.FileLock {
	t.Helper()
	held, ok, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil || !ok {
		t.Fatalf("TryAcquireWriteLock(%s) = %v, %v; want the lock", lockPath, ok, err)
	}
	return held
}

// writeInflight writes a step's in-flight record into stepsDir, and its envelope beside it when finished.
func writeInflight(t *testing.T, stepsDir, traceID string, pid int, started time.Time, finished bool) {
	t.Helper()
	record := fmt.Sprintf(`{"trace_id":%q,"pid":%d,"started_at":%q}`, traceID, pid, started.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(stepsDir, traceID+".inflight.json"), []byte(record), 0o644); err != nil {
		t.Fatalf("write the in-flight record: %v", err)
	}
	if finished {
		if err := os.WriteFile(filepath.Join(stepsDir, traceID+".json"), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write the envelope: %v", err)
		}
	}
}
