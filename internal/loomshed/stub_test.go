package loomshed

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestStubProducer_Call(t *testing.T) {
	t.Run("HealthyContext", func(t *testing.T) {
		p := newStub("Discussion-Write")
		outcome, pointer, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call(healthy) error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call(healthy) outcome = %q; want %q", outcome, shedengine.Done)
		}
		if pointer != (shedengine.OutputPointer{}) {
			t.Errorf("Call(healthy) pointer = %+v; want empty", pointer)
		}
	})

	t.Run("CancelledContext", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		p := newStub("Discussion-Write")
		outcome, _, err := p.Call(ctx)
		if err == nil {
			t.Fatalf("Call(cancelled) error = nil; want non-nil error rather than Done")
		}
		if outcome == shedengine.Done {
			t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
		}
	})
}
