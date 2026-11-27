// recordscommit_test.go covers the retry over a held index.lock.
// It is an internal-package test because retryHeldIndexLock is unexported and its sleep is faked.
// The tests rely on the package TestMain in testmain_test.go.

package fabricengine

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRetryHeldIndexLock(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	held := errors.New("git commit: fatal: Unable to create '/r/.git/index.lock': File exists.")
	other := errors.New("git commit: nothing to commit")
	tests := []struct {
		name       string
		bound      time.Duration
		results    []error
		wantErr    error
		wantSleeps []time.Duration
		wantCalls  int
	}{
		{"ReleasedWithinTheBoundLands", time.Second, []error{held, held, nil}, nil, []time.Duration{50 * time.Millisecond, 100 * time.Millisecond}, 3},
		{"HeldPastTheBoundFails", 350 * time.Millisecond, []error{held}, held, []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}, 4},
		{"OtherErrorIsNotRetried", time.Second, []error{other}, other, nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var sleeps []time.Duration
			calls := 0
			err := retryHeldIndexLock(tt.bound, func(d time.Duration) { sleeps = append(sleeps, d) }, func() error {
				// The last listed result repeats, so a single held error stands for a lock that is never released.
				result := tt.results[min(calls, len(tt.results)-1)]
				calls++
				return result
			})
			if err != tt.wantErr {
				t.Errorf("retryHeldIndexLock() = %v; want %v", err, tt.wantErr)
			}
			if !slices.Equal(sleeps, tt.wantSleeps) {
				t.Errorf("sleeps = %v; want %v", sleeps, tt.wantSleeps)
			}
			if calls != tt.wantCalls {
				t.Errorf("commit ran %d times; want %d", calls, tt.wantCalls)
			}
		})
	}
}
