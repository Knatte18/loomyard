package preflightshed

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestNewPreflight_CarriesToldName asserts NewPreflight hardcodes nothing: the returned
// shedengine.ShedProducer's concrete type is *preflightProducer, and its name and cwd fields hold
// the told constructor arguments verbatim.
func TestNewPreflight_CarriesToldName(t *testing.T) {
	got := NewPreflight("Some-Row", "/some/cwd")
	if got == nil {
		t.Fatalf("NewPreflight() = nil; want non-nil")
	}

	p, ok := got.(*preflightProducer)
	if !ok {
		t.Fatalf("NewPreflight() concrete type = %T; want *preflightProducer", got)
	}
	if p.name != "Some-Row" {
		t.Errorf("NewPreflight() name = %q; want %q", p.name, "Some-Row")
	}
	if p.cwd != "/some/cwd" {
		t.Errorf("NewPreflight() cwd = %q; want %q", p.cwd, "/some/cwd")
	}
}

// TestCall_CancelledAtEntryReturnsErrorNotStuck calls Call on an already-cancelled context and
// asserts a non-nil error with an outcome that is neither Done nor Stuck -- the same assertion
// shape internal/loomshed/resume_test.go's TestCancellation_RealProducersReturnErrorNotStuck uses.
// This case is Tier-1-legal precisely because entryErr returns before preflight.Check is ever
// reached, so nothing spawns; cwd is t.TempDir() so the test would not accidentally resolve a real
// repository even if that guarantee ever broke.
func TestCall_CancelledAtEntryReturnsErrorNotStuck(t *testing.T) {
	p := NewPreflight("Preflight", t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outcome, _, err := p.Call(ctx)
	if err == nil {
		t.Fatalf("Call(cancelled) error = nil; want non-nil error")
	}
	if outcome == shedengine.Done || outcome == shedengine.Stuck {
		t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
	}
}
