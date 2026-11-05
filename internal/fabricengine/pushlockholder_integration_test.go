// pushlockholder_integration_test.go covers the holder record at the lock sites that need a git-backed weft:
// the origin reconcile's try-acquire of a records branch's locks and the weft pull's replay lock.

package fabricengine

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// holderRecordPID reads the holder record in lockDir and returns its pid, or fails the test when none is readable.
func holderRecordPID(t *testing.T, lockDir string) int {
	t.Helper()
	content, err := os.ReadFile(pushLockHolderPath(lockDir))
	if err != nil {
		t.Fatalf("holder record: %v; want it written while the lock is held", err)
	}
	var holder pushLockHolder
	if err := json.Unmarshal(content, &holder); err != nil {
		t.Fatalf("holder record %q: %v", content, err)
	}
	return holder.PID
}

// TestTryRecordsLocks_HolderRecord covers the record a records branch's try-acquire writes:
// on a weft it names this process while the locks are held and is gone after the release, and on Bolt none is written.
func TestTryRecordsLocks_HolderRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	tests := []struct {
		name       string
		bolt       bool
		wantRecord bool
	}{
		{"Weft", false, true},
		{"Bolt", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktree := t.TempDir()
			if tt.bolt {
				// Bolt's push lock sits in the checkout's git dir, so the checkout must be a repository.
				gitkit.Git(t, worktree, "init", "-q", "-b", "main")
			} else {
				worktree = newPlainWeftRepo(t)
			}
			release, busy, _, err := tryRecordsLocks(worktree, tt.bolt)
			if err != nil || busy != "" {
				t.Fatalf("tryRecordsLocks() = (busy %q, %v); want the locks held", busy, err)
			}
			lockDir := RecordsLockDirPath(worktree)
			if tt.wantRecord {
				if got := holderRecordPID(t, lockDir); got != os.Getpid() {
					t.Errorf("holder record pid = %d; want %d", got, os.Getpid())
				}
			} else {
				// Bolt's locks sit at the worktree root, so a record could appear there or in the weft lock directory.
				for _, dir := range []string{worktree, lockDir} {
					if _, err := os.Stat(pushLockHolderPath(dir)); !os.IsNotExist(err) {
						t.Errorf("holder record in %s while held on Bolt: %v; want none", dir, err)
					}
				}
			}
			release()
			if _, err := os.Stat(pushLockHolderPath(lockDir)); !os.IsNotExist(err) {
				t.Errorf("holder record after the release: %v; want it gone", err)
			}
		})
	}
}

// TestLockWeftForReplay_HolderRecord covers the record the weft replay lock writes:
// it names this process while the locks are held and is gone after the release.
func TestLockWeftForReplay_HolderRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	weft := newPlainWeftRepo(t)
	f := &Fabric{weftPath: weft}
	release, err := f.lockWeftForReplay()
	if err != nil {
		t.Fatalf("lockWeftForReplay() = %v; want nil", err)
	}
	lockDir := RecordsLockDirPath(weft)
	if got := holderRecordPID(t, lockDir); got != os.Getpid() {
		t.Errorf("holder record pid = %d; want %d", got, os.Getpid())
	}
	release()
	if _, err := os.Stat(pushLockHolderPath(lockDir)); !os.IsNotExist(err) {
		t.Errorf("holder record after the release: %v; want it gone", err)
	}
}
