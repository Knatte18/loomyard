// moverecord_helper_test.go holds the helpers tests use to stand a move record on a weft by hand and to hold its move lock.
//
// Package fabricengine_test.

package fabricengine_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
)

// writeMoveRecord stands record on the weft worktree at weftPath and removes it when the test ends.
func writeMoveRecord(t *testing.T, weftPath string, record fabricengine.MoveRecord) {
	t.Helper()

	if _, err := os.Stat(fabricengine.RecordsLockDirPath(weftPath)); err != nil {
		if err := os.MkdirAll(fabricengine.RecordsLockDirPath(weftPath), 0o755); err != nil {
			t.Fatalf("create the lock directory of %s: %v", weftPath, err)
		}
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal the move record: %v", err)
	}
	if err := os.WriteFile(fabricengine.MoveRecordPath(weftPath), data, 0o644); err != nil {
		t.Fatalf("write the move record of %s: %v", weftPath, err)
	}
	t.Cleanup(func() {
		if err := fabricengine.RemoveMoveRecord(weftPath); err != nil {
			t.Errorf("remove the move record of %s: %v", weftPath, err)
		}
	})
}

// holdMoveLock holds the move lock of the weft worktree at weftPath until the test ends.
func holdMoveLock(t *testing.T, weftPath string) {
	t.Helper()

	held, acquired, err := lock.TryAcquireWriteLock(fabricengine.MoveLockPathForTest(weftPath))
	if err != nil || !acquired {
		t.Fatalf("hold the move lock of %s = (acquired %v, %v); want it acquired", weftPath, acquired, err)
	}
	t.Cleanup(func() { _ = held.Release() })
}
