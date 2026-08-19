package loomshed

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEntryErr(t *testing.T) {
	t.Run("HealthyContext", func(t *testing.T) {
		if err := entryErr(context.Background(), "Preflight"); err != nil {
			t.Errorf("entryErr(healthy) = %v; want nil", err)
		}
	})

	t.Run("CancelledContext", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := entryErr(ctx, "Preflight")
		if err == nil {
			t.Fatal("entryErr(cancelled) = nil; want non-nil error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("entryErr(cancelled) = %v; want errors.Is match against context.Canceled", err)
		}
		if !strings.Contains(err.Error(), "Preflight") {
			t.Errorf("entryErr(cancelled) = %q; want it to name the producer %q", err.Error(), "Preflight")
		}
	})
}

func TestCancelErr(t *testing.T) {
	t.Run("HealthyContext", func(t *testing.T) {
		if err := cancelErr(context.Background(), "Preflight"); err != nil {
			t.Errorf("cancelErr(healthy) = %v; want nil", err)
		}
	})

	t.Run("CancelledContext", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := cancelErr(ctx, "Preflight")
		if err == nil {
			t.Fatal("cancelErr(cancelled) = nil; want non-nil error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("cancelErr(cancelled) = %v; want errors.Is match against context.Canceled", err)
		}
		if !strings.Contains(err.Error(), "Preflight") {
			t.Errorf("cancelErr(cancelled) = %q; want it to name the producer %q", err.Error(), "Preflight")
		}
	})
}
