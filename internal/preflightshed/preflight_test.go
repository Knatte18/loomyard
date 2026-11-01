package preflightshed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestNewPreflight_CarriesToldName asserts NewPreflight hardcodes nothing: the returned shedengine.ShedProducer's concrete type is *preflightProducer, and its name and cwd fields hold the told constructor arguments verbatim.
//
//testtiming:keep pins the told name and cwd landing on the producer's fields, which the cancelled-call test covering its blocks never reads
func TestNewPreflight_CarriesToldName(t *testing.T) {
	tiergate.Require(t, 0)
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
// shape internal/loomshed/cancellation_test.go's TestCancellation_RealProducersReturnErrorNotStuck uses.
// This case is Tier-1-legal precisely because entryErr returns before preflight.Check is ever
// reached, so nothing spawns; cwd is t.TempDir() so the test would not accidentally resolve a real
// repository even if that guarantee ever broke.
func TestCall_CancelledAtEntryReturnsErrorNotStuck(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestPreflight_BatcherCheck covers the optional batcher check of NewPreflightWithBatcher over a temporary non-repository cwd.
// A failing check is Stuck with the batchifier reason and its way forward, and is logged with the row's name and the error.
// A retired-key error closes with the reconcile hand-off instead.
// A check that cancels the context before failing yields the cancellation error and no verdict.
// A nil or passing check never yields the batchifier reason.
// It does not call t.Parallel, because logcapture.Capture redirects the process-global logger output.
func TestPreflight_BatcherCheck(t *testing.T) {
	tiergate.Require(t, 0)
	retiredErr := fmt.Errorf("batcher.yaml: key master_base is retired; run lym config reconcile --apply: %w", batcher.ErrRetiredKey)
	plainErr := errors.New("unknown batchifier bogus")

	tests := []struct {
		name string
		// check builds the seam under test; cancel lets a seam cancel the call's context.
		check func(cancel context.CancelFunc) func() error
		// failure is the error a failing seam returns, logged by the row.
		failure    error
		wantReason string
		wantCancel bool
		wantNone   bool
	}{
		{
			name:       "FailingCheckIsStuckWithTheBatchifierReason",
			check:      func(context.CancelFunc) func() error { return func() error { return plainErr } },
			failure:    plainErr,
			wantReason: batchifierReasonPrefix + plainErr.Error() + batchifierWayForward,
		},
		{
			name:       "RetiredKeyClosesWithTheReconcileHandOff",
			check:      func(context.CancelFunc) func() error { return func() error { return retiredErr } },
			failure:    retiredErr,
			wantReason: batchifierReasonPrefix + retiredErr.Error() + ", then re-step",
		},
		{
			name: "CancelledContextWinsOverTheFailure",
			check: func(cancel context.CancelFunc) func() error {
				return func() error {
					cancel()
					return plainErr
				}
			},
			wantCancel: true,
		},
		{
			name:     "NilCheckRunsNoCheck",
			check:    func(context.CancelFunc) func() error { return nil },
			wantNone: true,
		},
		{
			name:     "PassingCheckYieldsNoBatchifierReason",
			check:    func(context.CancelFunc) func() error { return func() error { return nil } },
			wantNone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			buf := logcapture.Capture(t)
			p := NewPreflightWithBatcher("Pre-Row", t.TempDir(), tt.check(cancel))

			outcome, pointer, err := p.Call(ctx)

			switch {
			case tt.wantCancel:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("Call() error = %v; want the cancellation error", err)
				}
				if outcome == shedengine.Stuck || outcome == shedengine.Done {
					t.Errorf("Call() outcome = %q; want no verdict alongside a cancellation error", outcome)
				}
			case tt.wantNone:
				if strings.Contains(pointer.Reason, batchifierReasonPrefix) {
					t.Errorf("Call() reason = %q; want no batchifier reason", pointer.Reason)
				}
			default:
				if err != nil {
					t.Fatalf("Call() error = %v; want nil", err)
				}
				if outcome != shedengine.Stuck || pointer.Reason != tt.wantReason {
					t.Errorf("Call() = (%q, %q); want (%q, %q)", outcome, pointer.Reason, shedengine.Stuck, tt.wantReason)
				}
				logged := buf.String()
				for _, want := range []string{"Pre-Row", tt.failure.Error()} {
					if !strings.Contains(logged, want) {
						t.Errorf("log = %q; want it to contain %q", logged, want)
					}
				}
			}
		})
	}
}
