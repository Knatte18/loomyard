// pushowed_test.go pins IsPushOwed, the classification a transition seam retries a failed push by.

package fabricengine_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// failedWithClass returns a push failure recorded with class.
func failedWithClass(class fabricengine.PushFailureClass, cause error) error {
	return &fabricengine.PushFailedError{Record: fabricengine.FailedPush{Class: class}, Err: cause}
}

func TestIsPushOwed(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"Transport", failedWithClass(fabricengine.PushFailureTransport, errors.New("unreachable")), true},
		{"Deferred", failedWithClass(fabricengine.PushFailureDeferred, errors.New("deferred")), true},
		{"TeardownLock", failedWithClass(fabricengine.PushFailureTeardownLock, fabricengine.ErrPairPushRunning), true},
		{"PushLockBusy", fmt.Errorf("lock held: %w", fabricengine.ErrPushLockBusy), true},
		{"OwedFailuresJoined", errors.Join(
			failedWithClass(fabricengine.PushFailureTransport, errors.New("code")),
			failedWithClass(fabricengine.PushFailureDeferred, errors.New("records")),
		), true},
		{"NotFastForward", failedWithClass(fabricengine.PushFailureNotFastForward, gitrepo.ErrPushRejected), false},
		{"OwedJoinedWithRejection", errors.Join(
			failedWithClass(fabricengine.PushFailureTransport, errors.New("code")),
			failedWithClass(fabricengine.PushFailureNotFastForward, gitrepo.ErrPushRejected),
		), false},
		{"BareTeardownRefusal", fmt.Errorf("pair is being removed: %w", fabricengine.ErrPairPushRunning), false},
		{"HandedOver", fmt.Errorf("pushing: %w", fabricengine.ErrHandedOver), false},
		{"Nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fabricengine.IsPushOwed(tt.err); got != tt.want {
				t.Errorf("IsPushOwed(%v) = %v; want %v", tt.err, got, tt.want)
			}
		})
	}
}
