// weaver_test.go pins how WeaverProducer maps Weaver's run onto the shed producer contract.

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// weaverDeps returns RunDeps whose anchor and run directories are fresh temporary ones.
func weaverDeps(t *testing.T) weaverengine.RunDeps {
	t.Helper()
	anchor := t.TempDir()
	return weaverengine.RunDeps{Geom: weaverengine.Geometry{AnchorRoot: anchor, RunDir: weaverengine.Dir(anchor)}}
}

// weaverRunner returns a WeaverRunner answering result and err, and counting its calls.
func weaverRunner(result weaverengine.RunResult, err error, calls *int) WeaverRunner {
	return func(weaverengine.RunDeps) (weaverengine.RunResult, error) {
		*calls++
		return result, err
	}
}

func TestWeaverProducer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	at := func(minute int) time.Time { return time.Date(2026, 11, 28, 10, minute, 0, 0, time.UTC) }
	verifySpans := []shuttleengine.GateSpan{
		{Kind: shuttleengine.GateSpanGate, Start: at(0), End: at(1)},
		{Kind: shuttleengine.GateSpanRework, Start: at(1), End: at(5)},
	}
	wantSpans := []shedengine.GateSpan{
		{Gate: "verify", Kind: "gate", Start: "2026-11-28T10:00:00Z", End: "2026-11-28T10:01:00Z"},
		{Gate: "verify", Kind: "rework", Start: "2026-11-28T10:01:00Z", End: "2026-11-28T10:05:00Z"},
	}

	t.Run("DonePointsAtTheSummaryAndCarriesTheVerifySpans", func(t *testing.T) {
		t.Parallel()
		deps := weaverDeps(t)
		var calls int
		p := NewWeaverProducer("loom", weaverRunner(weaverengine.RunResult{Outcome: "done", BatchesDone: 2, VerifySpans: verifySpans}, nil, &calls), deps)

		ptr := shedfake.RequireOutcome(t, p, shedengine.Done)

		if want := summaryparser.Path(deps.Geom.RunDir); ptr.Path != want {
			t.Errorf("Call() Path = %q; want Weaver's summary %q", ptr.Path, want)
		}
		if !reflect.DeepEqual(ptr.GateSpans, wantSpans) {
			t.Errorf("Call() GateSpans = %+v; want %+v", ptr.GateSpans, wantSpans)
		}
	})

	t.Run("StuckReasonPassesThroughWithTheVerifySpansWhenNoConflictRecordWasLeft", func(t *testing.T) {
		t.Parallel()
		var calls int
		p := NewWeaverProducer("loom", weaverRunner(weaverengine.RunResult{Outcome: "stuck", StuckReason: "batch 01-a stopped", VerifySpans: verifySpans}, nil, &calls), weaverDeps(t))

		ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)

		if ptr.Reason != "batch 01-a stopped" || ptr.Reroute || !reflect.DeepEqual(ptr.GateSpans, wantSpans) {
			t.Errorf("Call() pointer = %+v; want the stuck reason as it is, no reroute and the gate spans %+v", ptr, wantSpans)
		}
	})

	t.Run("RunWithNoVerifyCarriesNoGateSpans", func(t *testing.T) {
		t.Parallel()
		var calls int
		p := NewWeaverProducer("loom", weaverRunner(weaverengine.RunResult{Outcome: "stuck", StuckReason: "batch 01-a stopped"}, nil, &calls), weaverDeps(t))

		if ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck); ptr.GateSpans != nil {
			t.Errorf("Call() GateSpans = %+v; want nil for a run that ran no verify", ptr.GateSpans)
		}
	})

	t.Run("NewConflictRecordReroutesAndARepeatDoesNot", func(t *testing.T) {
		t.Parallel()
		deps := weaverDeps(t)
		conflict := &runplan.PlanConflictError{Batch: 1, Cause: runplan.ConflictCausePlanDefect, Findings: []string{"finding-a"}}
		if err := runplan.WriteConflictRecord(runplan.ConflictDir(deps.Geom.AnchorRoot), conflict); err != nil {
			t.Fatal(err)
		}
		var calls int
		p := NewWeaverProducer("loom", weaverRunner(weaverengine.RunResult{Outcome: "stuck", StuckReason: "batch 01-a refused"}, nil, &calls), deps)

		first := shedfake.RequireOutcome(t, p, shedengine.Stuck)
		if !first.Reroute || !strings.Contains(first.Reason, "weaver plan conflict, asking a reroute") || !strings.Contains(first.Reason, "batch 01-a refused") {
			t.Errorf("first Call() pointer = %+v; want a reroute naming the stuck reason", first)
		}

		second := shedfake.RequireOutcome(t, p, shedengine.Stuck)
		if second.Reroute || !strings.Contains(second.Reason, "the same weaver plan conflict recurred") {
			t.Errorf("second Call() pointer = %+v; want a repeat with no reroute", second)
		}
	})

	faults := []struct {
		name string
		err  error
	}{
		{"RunBusy", fmt.Errorf("%w: %q; way forward: wait for the running row to stop, then re-step the loom row", weaverengine.ErrRunBusy, "/scratch")},
		{"ActiveRefused", fmt.Errorf("%w: repair the state file; way forward: re-step the loom row", weaverengine.ErrActiveRefused)},
		{"StateFault", errors.New("weaver: save state /run/state.json: disk full")},
	}
	for _, tt := range faults {
		t.Run("Stuck_"+tt.name, func(t *testing.T) {
			t.Parallel()
			var calls int
			p := NewWeaverProducer("loom", weaverRunner(weaverengine.RunResult{}, tt.err, &calls), weaverDeps(t))

			ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)

			if ptr.Reason != tt.err.Error() || ptr.Reroute {
				t.Errorf("Call() pointer = %+v; want the error's text as the reason and no reroute", ptr)
			}
		})
	}

	t.Run("UnrecognizedOutcomeIsAnError", func(t *testing.T) {
		t.Parallel()
		var calls int
		p := NewWeaverProducer("loom", weaverRunner(weaverengine.RunResult{Outcome: "WEIRD"}, nil, &calls), weaverDeps(t))

		if _, _, err := p.Call(context.Background()); err == nil || !strings.Contains(err.Error(), "WEIRD") {
			t.Errorf("Call() error = %v; want one quoting the unrecognized value", err)
		}
	})

	t.Run("CancelledContextReturnsTheContextError", func(t *testing.T) {
		t.Parallel()
		var calls int
		p := NewWeaverProducer("loom", weaverRunner(weaverengine.RunResult{Outcome: "stuck", StuckReason: "x"}, nil, &calls), weaverDeps(t))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, _, err := p.Call(ctx); !errors.Is(err, context.Canceled) || calls != 0 {
			t.Errorf("Call() error = %v after %d run(s); want the context error before any run", err, calls)
		}
	})

	t.Run("ReentryStepNamesTheRow", func(t *testing.T) {
		t.Parallel()
		var got string
		p := NewWeaverProducer("loom", func(deps weaverengine.RunDeps) (weaverengine.RunResult, error) {
			got = deps.ReentryStep
			return weaverengine.RunResult{Outcome: "done"}, nil
		}, weaverDeps(t))

		shedfake.RequireOutcome(t, p, shedengine.Done)

		if got != "re-step the loom row" {
			t.Errorf("ReentryStep = %q; want it to name the row", got)
		}
	})
}
