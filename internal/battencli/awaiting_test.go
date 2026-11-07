// awaiting_test.go asserts that a batten status file in StateAwaiting arms without refusal.
// It holds in both battenPreRun and battenPreStep, neither of which trips the unrecognised-state default.

package battencli

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestAwaitingStatusArmsWithoutRefusal(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	c := newFakeReceiver(t, nil)
	writeStatus(t, c, shedengine.Status{
		CurrentProducer: battenrecipe.NameWorktreeTeardown,
		State:           shedengine.StateAwaiting,
	})

	if err := c.battenPreRun(context.Background()); err != nil {
		t.Errorf("battenPreRun() = %v; want nil for an awaiting status", err)
	}

	kind, err := c.battenPreStep(context.Background())
	if err != nil || kind != "" {
		t.Errorf("battenPreStep() = (%q, %v); want (\"\", nil) for an awaiting status", kind, err)
	}
}
