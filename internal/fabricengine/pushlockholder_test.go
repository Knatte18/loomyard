// pushlockholder_test.go covers the push lock's holder record over a temp lock directory:
// what a busy report says of the holder,
// and that the coalescing holder loop writes the record while it holds the lock and removes it on release.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDescribePushLockHolder covers each state the holder record can be in when a bounded push gives up on the lock.
func TestDescribePushLockHolder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	since := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	now := since.Add(95 * time.Second)
	tests := []struct {
		name string
		// record is the holder record's content, nil for no record on disk.
		record []byte
		alive  bool
		want   string
	}{
		{"NoRecord", nil, true, "no holder record names the process that holds it"},
		{"LiveHolder", fmt.Appendf(nil, `{"pid":4242,"since":%q}`, since.Format(time.RFC3339)), true, "held by pid 4242 for 1m35s"},
		{"DeadHolder", fmt.Appendf(nil, `{"pid":4242,"since":%q,"unknown":1}`, since.Format(time.RFC3339)), false, "its holder record names pid 4242 from 1m35s ago, no running process by that pid was found, and the record may be stale"},
		{"Corrupt", []byte("{not json"), true, "its holder record is unreadable: invalid character 'n' looking for beginning of object key string"},
		{"EmptyObject", []byte("{}"), true, "its holder record is unreadable: it names no pid or no time the lock was taken"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lockDir := t.TempDir()
			if tt.record != nil {
				if err := os.WriteFile(pushLockHolderPath(lockDir), tt.record, 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}
			alive := func(int) bool { return tt.alive }
			if got := describePushLockHolder(lockDir, now, alive); got != tt.want {
				t.Errorf("describePushLockHolder() = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestHoldPushLock_ReleasesTheLockWhenTheRecordFails covers a holder record that cannot be written or removed:
// the release still returns nil and frees the lock.
func TestHoldPushLock_ReleasesTheLockWhenTheRecordFails(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// prepare returns the lock directory whose holder record fails.
		prepare func(t *testing.T) string
	}{
		{"RecordPathIsNonEmptyDirectory", func(t *testing.T) string {
			dir := t.TempDir()
			occupied := pushLockHolderPath(dir)
			if err := os.MkdirAll(occupied, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(filepath.Join(occupied, "entry"), []byte("x"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			return dir
		}},
		{"LockDirIsAFile", func(t *testing.T) string {
			file := filepath.Join(t.TempDir(), "file")
			if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			return filepath.Join(file, "locks")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lockDir := tt.prepare(t)
			lockPath := filepath.Join(t.TempDir(), "push.lock")
			held, acquired, err := lock.TryAcquireWriteLock(lockPath)
			if err != nil || !acquired {
				t.Fatalf("TryAcquireWriteLock() = (%v, %v); want it to succeed", acquired, err)
			}
			release := holdPushLock(held, lockDir, time.Now())
			if err := release(); err != nil {
				t.Fatalf("release() = %v; want nil", err)
			}
			again, acquired, err := lock.TryAcquireWriteLock(lockPath)
			if err != nil || !acquired {
				t.Fatalf("TryAcquireWriteLock() after release = (%v, %v); want it to succeed", acquired, err)
			}
			_ = again.Release()
		})
	}
}

// TestCoalesceFlagged_HolderRecordLivesWhileTheLockIsHeld covers the holder loop's record:
// during the step it names this process.
// The busy error of a waiter names that process as live.
// After the loop the record is gone and the lock free.
func TestCoalesceFlagged_HolderRecordLivesWhileTheLockIsHeld(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	f := newPendingFlagFixture(t)
	wantClause := fmt.Sprintf("held by pid %d for", os.Getpid())

	steps := 0
	step := func() (bool, error) {
		steps++
		busy := pushLockBusyError(f.lockDir, time.Second)
		if !errors.Is(busy, ErrPushLockBusy) || !strings.Contains(busy.Error(), wantClause) {
			t.Errorf("pushLockBusyError() during the step = %v; want it to wrap ErrPushLockBusy and name %q", busy, wantClause)
		}
		return false, nil
	}
	acquired, err := coalesceFlagged(f.lockDir, map[string]PushSpawn{pushSideRecords: {WeftPath: f.records}}, lock.TryAcquireWriteLock, step)
	if !acquired || err != nil {
		t.Fatalf("coalesceFlagged() = (%v, %v); want (true, nil)", acquired, err)
	}
	if steps != 1 {
		t.Errorf("step ran %d times; want 1", steps)
	}
	if _, err := os.Stat(pushLockHolderPath(f.lockDir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("holder record after the loop: %v; want it gone", err)
	}
	again, ok, err := lock.TryAcquireWriteLock(f.lockPath)
	if err != nil || !ok {
		t.Fatalf("try-acquire after the loop = (%v, %v); want it to succeed", ok, err)
	}
	_ = again.Release()
}
