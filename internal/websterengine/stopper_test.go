// stopper_test.go holds the recording StrandStopper double the reclaim tests share, and pins RemoveRecoveryStrands against it (Tier 1: no git, no reed).

package websterengine_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

// recordingStopper is a websterengine.StrandStopper that records every guid it is asked to stop, in order, and answers Err.
type recordingStopper struct {
	Stopped []string
	Err     error
}

func (s *recordingStopper) StopStrand(guid string) error {
	s.Stopped = append(s.Stopped, guid)
	return s.Err
}

// TestRemoveRecoveryStrands pins which recorded strands are stopped, in batch-number order, and that the first stop failure is reported with its guid.
func TestRemoveRecoveryStrands(t *testing.T) {
	t.Parallel()

	st := &websterengine.State{
		MasterStrand: "master",
		Batches: map[int]*websterengine.BatchState{
			3: {Kind: "recovery", StrandGUID: "recovery-3"},
			1: {Kind: "recovery", StrandGUID: "recovery-1"},
			2: {Kind: "fork"},
			4: {Kind: "recovery"},
		},
	}

	t.Run("stops every recorded recovery strand in batch order", func(t *testing.T) {
		t.Parallel()
		stopper := &recordingStopper{}
		if err := websterengine.RemoveRecoveryStrands(stopper, st); err != nil {
			t.Fatalf("RemoveRecoveryStrands() error = %v; want nil", err)
		}
		if want := []string{"recovery-1", "recovery-3"}; !reflect.DeepEqual(stopper.Stopped, want) {
			t.Errorf("stopped = %v; want %v", stopper.Stopped, want)
		}
	})

	t.Run("a nil state stops nothing", func(t *testing.T) {
		t.Parallel()
		stopper := &recordingStopper{}
		if err := websterengine.RemoveRecoveryStrands(stopper, nil); err != nil || len(stopper.Stopped) != 0 {
			t.Errorf("RemoveRecoveryStrands(nil) = %v with stopped %v; want nil and none", err, stopper.Stopped)
		}
	})

	t.Run("the first stop failure is reported with its guid", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("stop failed")
		stopper := &recordingStopper{Err: boom}
		err := websterengine.RemoveRecoveryStrands(stopper, st)
		var removeErr *websterengine.RecoveryStrandRemoveError
		if !errors.As(err, &removeErr) || removeErr.GUID != "recovery-1" || !errors.Is(err, boom) {
			t.Fatalf("RemoveRecoveryStrands() = %v; want a *RecoveryStrandRemoveError for recovery-1 wrapping %v", err, boom)
		}
		if want := []string{"recovery-1"}; !reflect.DeepEqual(stopper.Stopped, want) {
			t.Errorf("stopped = %v; want %v (the first failure ends the walk)", stopper.Stopped, want)
		}
	})
}
