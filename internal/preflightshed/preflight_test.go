package preflightshed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// NewPreflight returns the Preflight producer for name and cwd with no optional checks.
func NewPreflight(name, cwd string) shedengine.ShedProducer {
	return NewPreflightWithChecks(name, cwd, Checks{})
}

// newPreflightWith returns the producer for name and cwd.
// The opts configure the hub config reconcile that runs ahead of the check.
func newPreflightWith(name, cwd string, opts hubreconcile.Options) shedengine.ShedProducer {
	return &preflightProducer{name: name, cwd: cwd, opts: opts}
}

// TestNewPreflightWithChecks_CarriesToldName asserts NewPreflightWithChecks hardcodes nothing.
// The returned producer is a *preflightProducer whose name and cwd fields hold the arguments verbatim.
//
//testtiming:keep pins the told name and cwd landing on the producer's fields by NewPreflightWithChecks, which the cancelled-call test covering its blocks never reads
func TestNewPreflightWithChecks_CarriesToldName(t *testing.T) {
	tiergate.Require(t, 0)
	got := NewPreflightWithChecks("Some-Row", "/some/cwd", Checks{})
	if got == nil {
		t.Fatalf("NewPreflightWithChecks() = nil; want non-nil")
	}

	p, ok := got.(*preflightProducer)
	if !ok {
		t.Fatalf("NewPreflightWithChecks() concrete type = %T; want *preflightProducer", got)
	}
	if p.name != "Some-Row" {
		t.Errorf("NewPreflightWithChecks() name = %q; want %q", p.name, "Some-Row")
	}
	if p.cwd != "/some/cwd" {
		t.Errorf("NewPreflightWithChecks() cwd = %q; want %q", p.cwd, "/some/cwd")
	}
}

// TestCall_CancelledAtEntryReturnsErrorNotStuck calls Call on an already-cancelled context.
// It asserts a non-nil error and an outcome that is neither Done nor Stuck.
// The case is legal at Tier 1 because entryErr returns before preflight.Check runs, so nothing spawns.
// The working directory is t.TempDir(), so the test would not resolve a real repository even if that early return broke.
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

// TestPreflight_BatcherCheck covers the optional batcher check of NewPreflightWithChecks.
// It runs over a temporary cwd that is not a repository.
// A failing check is Stuck with the batchifier reason and its way forward.
// The failure is logged with the row's name and the error.
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
		// check builds the seam under test. Its cancel argument cancels the call's context.
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
			p := NewPreflightWithChecks("Pre-Row", t.TempDir(), Checks{Batcher: tt.check(cancel)})

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

// TestPreflight_HardenCheck covers the optional hardener check of NewPreflightWithChecks.
// A failing check is Stuck with its reason followed by the error's own text.
// A nil or passing check yields no such reason.
// The test does not call t.Parallel: logcapture.Capture redirects the process-global logger output.
func TestPreflight_HardenCheck(t *testing.T) {
	tiergate.Require(t, 0)
	hardenErr := errors.New("hardener campaign: the hub's gate.yaml sets no gates.harden-live key")

	tests := []struct {
		name       string
		harden     func() error
		wantReason string
	}{
		{name: "FailingCheckIsStuckWithTheHardenReason", harden: func() error { return hardenErr }, wantReason: hardenReasonPrefix + hardenErr.Error()},
		{name: "NilCheckIsSkipped"},
		{name: "PassingCheckYieldsNoHardenReason", harden: func() error { return nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.Capture(t)
			p := NewPreflightWithChecks("Pre-Row", t.TempDir(), Checks{Harden: tt.harden})

			outcome, pointer, err := p.Call(context.Background())

			if tt.wantReason == "" {
				if strings.Contains(pointer.Reason, hardenReasonPrefix) {
					t.Errorf("Call() reason = %q; want no hardener reason", pointer.Reason)
				}
				return
			}
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != shedengine.Stuck || pointer.Reason != tt.wantReason {
				t.Errorf("Call() = (%q, %q); want (%q, %q)", outcome, pointer.Reason, shedengine.Stuck, tt.wantReason)
			}
			if logged := buf.String(); !strings.Contains(logged, "Pre-Row") || !strings.Contains(logged, hardenErr.Error()) {
				t.Errorf("log = %q; want it to hold the row name and the error", logged)
			}
		})
	}
}

// TestPreflight_StencilsCheck covers the optional stencils and drift checks of NewPreflightWithChecks.
// It runs over a temporary cwd that is not a repository.
// A failing check is Stuck with its reason followed by the error's own text.
// It is logged with the row's name and the error.
// A failing batcher check runs first and wins over a failing stencils check, which wins over a failing drift check.
// A check that cancels the context before failing yields the cancellation error and no verdict.
// A passing check never yields the stencils reason.
// The test does not call t.Parallel: logcapture.Capture redirects the process-global logger output.
func TestPreflight_StencilsCheck(t *testing.T) {
	tiergate.Require(t, 0)
	stencilsErr := errors.New("stencil darn-template-write has marker(s) subagent_model that no value fills")
	batcherErr := errors.New("unknown batchifier bogus")
	driftErr := errors.New("drift: the drift capture command holds {skip_file}")
	failWith := func(err error) func() error { return func() error { return err } }

	tests := []struct {
		name string
		// checks builds the checks under test; a check that calls cancel cancels the call's context.
		checks     func(cancel context.CancelFunc) Checks
		failure    error
		wantReason string
		wantCancel bool
		wantNone   bool
	}{
		{
			name:       "FailingCheckIsStuckWithTheStencilsReason",
			checks:     func(context.CancelFunc) Checks { return Checks{Stencils: failWith(stencilsErr)} },
			failure:    stencilsErr,
			wantReason: stencilsReasonPrefix + stencilsErr.Error(),
		},
		{
			name: "FailingBatcherCheckWinsOverTheStencilsCheck",
			checks: func(context.CancelFunc) Checks {
				return Checks{Batcher: failWith(batcherErr), Stencils: failWith(stencilsErr)}
			},
			failure:    batcherErr,
			wantReason: batchifierReasonPrefix + batcherErr.Error() + batchifierWayForward,
		},
		{
			name:       "FailingDriftCheckIsStuckWithTheDriftReason",
			checks:     func(context.CancelFunc) Checks { return Checks{Drift: failWith(driftErr)} },
			failure:    driftErr,
			wantReason: driftReasonPrefix + driftErr.Error(),
		},
		{
			name: "FailingStencilsCheckWinsOverTheDriftCheck",
			checks: func(context.CancelFunc) Checks {
				return Checks{Stencils: failWith(stencilsErr), Drift: failWith(driftErr)}
			},
			failure:    stencilsErr,
			wantReason: stencilsReasonPrefix + stencilsErr.Error(),
		},
		{
			name: "CancelledContextWinsOverTheFailure",
			checks: func(cancel context.CancelFunc) Checks {
				return Checks{Stencils: func() error {
					cancel()
					return stencilsErr
				}}
			},
			wantCancel: true,
		},
		{
			name:     "PassingCheckYieldsNoStencilsReason",
			checks:   func(context.CancelFunc) Checks { return Checks{Stencils: failWith(nil)} },
			wantNone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			buf := logcapture.Capture(t)
			p := NewPreflightWithChecks("Pre-Row", t.TempDir(), tt.checks(cancel))

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
				if strings.Contains(pointer.Reason, stencilsReasonPrefix) {
					t.Errorf("Call() reason = %q; want no stencils reason", pointer.Reason)
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
