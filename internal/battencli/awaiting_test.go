// awaiting_test.go asserts a batten status file in StateAwaiting arms without refusal in both
// battenPreRun and battenPreStep, rather than tripping the unrecognised-state default.

package battencli

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestBattenPreRun_AwaitingArmsWithoutRefusal(t *testing.T) {
	c := newFakeReceiver(t, nil)
	writeStatus(t, c, shedengine.Status{
		CurrentProducer: battenrecipe.NameWorktreeTeardown,
		State:           shedengine.StateAwaiting,
	})

	if err := c.battenPreRun(context.Background()); err != nil {
		t.Errorf("battenPreRun() = %v; want nil for an awaiting status", err)
	}
}

func TestBattenPreStep_AwaitingArmsWithoutRefusal(t *testing.T) {
	c := newFakeReceiver(t, nil)
	writeStatus(t, c, shedengine.Status{
		CurrentProducer: battenrecipe.NameWorktreeTeardown,
		State:           shedengine.StateAwaiting,
	})

	kind, err := c.battenPreStep(context.Background())
	if err != nil || kind != "" {
		t.Errorf("battenPreStep() = (%q, %v); want (\"\", nil) for an awaiting status", kind, err)
	}
}
