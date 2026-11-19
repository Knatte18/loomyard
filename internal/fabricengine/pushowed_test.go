// pushowed_test.go pins IsPushOwed, the classification a transition seam retries a failed push by, and IsPushProtected.

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
		name          string
		err           error
		want          bool
		wantProtected bool
	}{
		{"Transport", failedWithClass(fabricengine.PushFailureTransport, errors.New("unreachable")), true, false},
		{"Deferred", failedWithClass(fabricengine.PushFailureDeferred, errors.New("deferred")), true, false},
		{"TeardownLock", failedWithClass(fabricengine.PushFailureTeardownLock, fabricengine.ErrPairPushRunning), true, false},
		{"OperatorUnpushed", failedWithClass(fabricengine.PushFailureOperatorUnpushed, errors.New("operator commits")), true, false},
		{"PushLockBusy", fmt.Errorf("lock held: %w", fabricengine.ErrPushLockBusy), true, false},
		{"OwedFailuresJoined", errors.Join(
			failedWithClass(fabricengine.PushFailureTransport, errors.New("code")),
			failedWithClass(fabricengine.PushFailureDeferred, errors.New("records")),
		), true, false},
		{"NotFastForward", failedWithClass(fabricengine.PushFailureNotFastForward, gitrepo.ErrPushRejected), false, false},
		{"OwedJoinedWithRejection", errors.Join(
			failedWithClass(fabricengine.PushFailureTransport, errors.New("code")),
			failedWithClass(fabricengine.PushFailureNotFastForward, gitrepo.ErrPushRejected),
		), false, false},
		{"BareTeardownRefusal", fmt.Errorf("pair is being removed: %w", fabricengine.ErrPairPushRunning), false, false},
		{"HandedOver", fmt.Errorf("pushing: %w", fabricengine.ErrHandedOver), false, false},
		{"Protected", failedWithClass(fabricengine.PushFailureProtected, errors.New("protected branch")), false, true},
		{"ProtectedJoinedWithTransport", errors.Join(
			failedWithClass(fabricengine.PushFailureProtected, errors.New("protected branch")),
			failedWithClass(fabricengine.PushFailureTransport, errors.New("records")),
		), false, false},
		{"Nil", nil, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fabricengine.IsPushOwed(tt.err); got != tt.want {
				t.Errorf("IsPushOwed(%v) = %v; want %v", tt.err, got, tt.want)
			}
			if got := fabricengine.IsPushProtected(tt.err); got != tt.wantProtected {
				t.Errorf("IsPushProtected(%v) = %v; want %v", tt.err, got, tt.wantProtected)
			}
		})
	}
}
