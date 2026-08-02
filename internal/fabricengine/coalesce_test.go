// coalesce_test.go covers coalescePush's loop-exit contract with scripted
// step closures: no real git is spawned, only a flock on a t.TempDir() path,
// so this file stays untagged per the Test Tier Purity Invariant.

package fabricengine

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
)

// TestCoalescePush_LoopsWhileProgressed asserts the loop-exit contract: a
// step returning progressed=true a fixed number of times before returning
// false runs exactly one more time than the number of true returns (the
// final, no-progress call that ends the loop), and a step that reports
// no-progress on its very first call runs exactly once — proving the loop
// does not spin depending on any external "still unpushed" signal, only on
// step's own return value.
func TestCoalescePush_LoopsWhileProgressed(t *testing.T) {
	tests := []struct {
		name          string
		progressCount int
		wantCalls     int
	}{
		{"NoProgressImmediately", 0, 1},
		{"ProgressesTwiceThenStops", 2, 3},
		{"ProgressesFiveTimesThenStops", 5, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lockPath := filepath.Join(t.TempDir(), "push.lock")

			calls := 0
			step := func() (bool, error) {
				calls++
				return calls <= tt.progressCount, nil
			}

			if err := coalescePush(lockPath, step); err != nil {
				t.Fatalf("coalescePush() error = %v; want nil", err)
			}
			if calls != tt.wantCalls {
				t.Errorf("step call count = %d; want %d", calls, tt.wantCalls)
			}
		})
	}
}

// TestCoalescePush_StepErrorAbortsImmediately asserts a step returning a
// non-nil error stops the loop on that call — no further step calls — and
// coalescePush returns that exact error to the caller.
func TestCoalescePush_StepErrorAbortsImmediately(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "push.lock")
	wantErr := errors.New("boom")

	calls := 0
	step := func() (bool, error) {
		calls++
		return true, wantErr
	}

	err := coalescePush(lockPath, step)
	if !errors.Is(err, wantErr) {
		t.Fatalf("coalescePush() error = %v; want %v", err, wantErr)
	}
	if calls != 1 {
		t.Errorf("step call count = %d; want 1 (loop must abort on the first error)", calls)
	}
}

// TestCoalescePush_ReleasesLockOnReturn asserts coalescePush releases its
// absorbing lock before returning: a second, independent AcquireWriteLock on
// the same path must succeed without blocking once coalescePush has returned.
func TestCoalescePush_ReleasesLockOnReturn(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "push.lock")

	step := func() (bool, error) { return false, nil }
	if err := coalescePush(lockPath, step); err != nil {
		t.Fatalf("coalescePush() error = %v; want nil", err)
	}

	l, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock() after coalescePush returned error = %v; want nil (lock must be released)", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}
