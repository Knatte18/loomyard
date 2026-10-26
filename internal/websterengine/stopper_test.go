//go:build tier0

// stopper_test.go pins RemoveRecoveryStrands against the recording StrandStopper double (no git, no reed).

package websterengine_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

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
		VerifyFixers: []*websterengine.VerifyFixerState{{StrandGUID: "fixer-1", Status: websterengine.DigestStatusDead}, {StrandGUID: "fixer-2"}},
		PlanFixers:   []*websterengine.PlanFixerState{{StrandGUID: "plan-fixer-1"}},
	}

	t.Run("stops every recorded recovery strand in batch order, then every cold fixer and plan fixer strand", func(t *testing.T) {
		t.Parallel()
		stopper := &recordingStopper{}
		if err := websterengine.RemoveRecoveryStrands(stopper, st); err != nil {
			t.Fatalf("RemoveRecoveryStrands() error = %v; want nil", err)
		}
		if want := []string{"recovery-1", "recovery-3", "fixer-1", "fixer-2", "plan-fixer-1"}; !reflect.DeepEqual(stopper.Stopped, want) {
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
