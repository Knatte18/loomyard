//go:build tier0

// cancellation_test.go carries the one test whose subject is this package's own constructors under
// an already-cancelled context, plus the reduced fixture it drives from. It stays in
// internal/loomshed rather than moving to internal/loomrecipe alongside the rest of the sequence
// suite because it calls neither New nor Run: it constructs NewWebsterProducer directly and calls Call on it.
//
// This is the only test anywhere proving their real Call wiring
// returns an error rather than a verdict under an already-cancelled context -- the shared
// cancellation helpers' own tests exercise those helpers in isolation and never through a producer.

package loomshed

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// cancellationFixture carries the one told value TestCancellation_RealProducersReturnErrorNotStuck reads.
type cancellationFixture struct {
	AnchorPath string
}

// buildCancellationFixture seeds only what the producers this test drives read: NewWebsterProducer reads AnchorPath.
func buildCancellationFixture(t *testing.T) cancellationFixture {
	t.Helper()

	return cancellationFixture{AnchorPath: t.TempDir()}
}

// TestCancellation_RealProducersReturnErrorNotStuck asserts the one obligation shedengine cannot enforce for itself:
// every real producer this test drives -- the Webster wrapper -- returns a non-nil error rather than shedengine.Stuck when called under an already-cancelled context.
// A Stuck under a cancelled context is indistinguishable to Shed from a genuine verdict and would silently consume bounce budget for what was actually an operator stop.
func TestCancellation_RealProducersReturnErrorNotStuck(t *testing.T) {
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
