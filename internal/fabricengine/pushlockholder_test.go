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

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDescribePushLockHolder covers how the holder file reads: absent, or holding bytes that do not decode.
func TestDescribePushLockHolder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// record is the holder record's content, nil for no record on disk.
		record []byte
		want   string
	}{
		{"NoRecord", nil, "no holder record names the process that holds it"},
		{"Corrupt", []byte("{not json"), "its holder record is unreadable: invalid character 'n' looking for beginning of object key string"},
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
			alive := func(int) bool { return true }
			if got := describePushLockHolder(lockDir, time.Now(), alive); got != tt.want {
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
	acquired, err := coalesceFlagged(f.lockDir, map[string]fabriclayout.PushSpawn{pushSideRecords: {WeftPath: f.records}}, lock.TryAcquireWriteLock, step)
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
