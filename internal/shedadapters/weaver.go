// weaver.go implements WeaverProducer, an adapter over weaverengine.Run.
// It maps the run's RunResult and error onto the shedengine.ShedProducer contract.
// It also holds claimConflict, the plan-conflict claim a stuck implementer row makes.

package shedadapters

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// weaverEngineLabel is the engine label that log lines and error text carry.
const weaverEngineLabel = "weaver"

// WeaverRunner is the seam WeaverProducer calls to run Weaver.
// It is a func type rather than an interface, because weaverengine.Run is already a free function.
type WeaverRunner func(weaverengine.RunDeps) (weaverengine.RunResult, error)

var _ WeaverRunner = weaverengine.Run

// weaverengine keeps its outcome values unexported, so these constants copy its literals.
// A rename there surfaces here: the default branch in Call errors on any value other than these two,
// and a test drives that branch.
const (
	weaverOutcomeDone  = "done"
	weaverOutcomeStuck = "stuck"
)

// WeaverProducer is a shedengine.ShedProducer that runs weaverengine.Run once per Call.
// One Call runs every remaining batch of the plan and the plan-level verify.
// It maps the result onto the shedengine contract.
type WeaverProducer struct {
	name string
	run  WeaverRunner
	deps weaverengine.RunDeps
}

var _ shedengine.ShedProducer = (*WeaverProducer)(nil)

// NewWeaverProducer returns a WeaverProducer named name that drives deps through run.
// A nil run defaults to weaverengine.Run.
// An empty deps.ReentryStep becomes "re-step the <name> row", so every way forward in a stop reason names this row.
func NewWeaverProducer(name string, run WeaverRunner, deps weaverengine.RunDeps) *WeaverProducer {
	if run == nil {
		run = weaverengine.Run
	}
	if deps.ReentryStep == "" {
		deps.ReentryStep = fmt.Sprintf("re-step the %s row", name)
	}
	return &WeaverProducer{name: name, run: run, deps: deps}
}

// Call runs one iteration of the producer.
// It checks the context on entry and maps the run's result.
// Done points at Weaver's summary, which survives a cancellation that arrives during the run.
// A stuck result consults the plan-conflict record the run left, through claimConflict,
// so a new conflict asks for a reroute to plan review and a repeat does not.
// A busy run and a fault in Weaver's config or state map to Stuck with the error's text, which names its way forward.
// A cancellation replaces every other non-success verdict with the context error.
func (p *WeaverProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name, weaverEngineLabel); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	result, err := p.run(p.deps)
	if err != nil {
		if cerr := cancelErr(ctx, p.name, weaverEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return shedengine.Stuck, shedengine.OutputPointer{Reason: err.Error()}, nil
	}

	switch result.Outcome {
	case weaverOutcomeDone:
		return shedengine.Done, shedengine.OutputPointer{Path: summaryparser.Path(p.deps.Geom.RunDir)}, nil

	case weaverOutcomeStuck:
		if cerr := cancelErr(ctx, p.name, weaverEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("shedadapters: weaver run is stuck", "producer", p.name, "engine", weaverEngineLabel, "stuckReason", result.StuckReason, "batchesDone", result.BatchesDone)
		ptr := claimConflict(p.name, weaverEngineLabel, result.StuckReason, runplan.ConflictDir(p.deps.Geom.AnchorRoot), shedengine.OutputPointer{})
		return shedengine.Stuck, ptr, nil

	default:
		if cerr := cancelErr(ctx, p.name, weaverEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): unrecognized weaver outcome %q", p.name, weaverEngineLabel, result.Outcome)
	}
}

// claimConflict claims the plan-conflict record in conflictDir for a stuck implementer row named name over engine.
// It returns ptr with its Reason set.
// A new conflict sets Reroute and asks plan review to clear it.
// A repeat of the last routed conflict names the hand edit that clears it.
// No record, or a claim that fails, keeps reason as it is.
// A failed claim is logged.
func claimConflict(name, engine, reason, conflictDir string, ptr shedengine.OutputPointer) shedengine.OutputPointer {
	ptr.Reason = reason
	route, err := runplan.ClaimConflictRoute(conflictDir)
	if err != nil {
		logger.Warn("shedadapters: plan-conflict record not claimed; the stuck outcome halts", "producer", name, "engine", engine, "error", err)
		return ptr
	}
	switch {
	case route.New:
		ptr.Reroute = true
		ptr.Reason = engine + " plan conflict, asking a reroute to plan review: " + reason + "; if plan review does not clear it: " + planConflictWayForward
	case route.Found:
		ptr.Reason = "the same " + engine + " plan conflict recurred after a reroute of it was asked for: " + reason + "; way forward: " + planConflictWayForward
	}
	return ptr
}
