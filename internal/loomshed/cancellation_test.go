// cancellation_test.go tests the real producers' Call under an already-cancelled context.
// It also holds the reduced fixture the test drives from.
// Each producer must return an error rather than a verdict in that state,
// and the helpers' own tests cannot show that because they never run through a producer.

package loomshed

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// cancellationFixture holds the one value TestCancellation_RealProducersReturnErrorNotStuck reads.
type cancellationFixture struct {
	AnchorPath string
}

// buildCancellationFixture seeds only AnchorPath, the one field the producers under test read.
func buildCancellationFixture(t *testing.T) cancellationFixture {
	t.Helper()

	return cancellationFixture{AnchorPath: t.TempDir()}
}

// TestCancellation_RealProducersReturnErrorNotStuck asserts that each producer under test returns a non-nil error.
// It must return that error, not shedengine.Stuck, when called under an already-cancelled context.
// A Stuck under a cancelled context looks to Shed like a genuine verdict:
// it would silently consume bounce budget for an operator stop.
func TestCancellation_RealProducersReturnErrorNotStuck(t *testing.T) {
	tiergate.Require(t, 0)
	fx := buildCancellationFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	producers := []struct {
		name string
		p    interface {
			Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)
		}
	}{
		{NameWebster, NewWebsterProducer(NameWebster, configengine.Dirs{Anchor: fx.AnchorPath}, (&fakeWebsterRun{}).run, websterengine.RunDeps{}, func() error { return nil })},
	}

	for _, tt := range producers {
		t.Run(tt.name, func(t *testing.T) {
			outcome, _, err := tt.p.Call(ctx)
			if err == nil {
				t.Fatalf("Call(cancelled) error = nil; want non-nil error")
			}
			if outcome == shedengine.Done || outcome == shedengine.Stuck {
				t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
			}
		})
	}
}
