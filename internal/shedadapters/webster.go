// webster.go implements WebsterProducer, an adapter over websterengine.Run.
// It maps the run's RunResult and error onto the shedengine.ShedProducer contract.

package shedadapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// websterEngineLabel is the engine label that log lines and error text carry.
const websterEngineLabel = "webster"

// WebsterRunner is the seam WebsterProducer calls to run webster.
// It is a func type rather than an interface, because websterengine.Run is already a free function.
type WebsterRunner func(websterengine.RunDeps, websterengine.RunOptions) (websterengine.RunResult, error)

var _ WebsterRunner = websterengine.Run

// websterengine keeps its outcome values unexported, so these constants copy its literals.
// A rename there surfaces here: the default branch in Call errors on any value other than these two,
// and a test drives that branch, so the rename fails a test rather than mis-mapping a verdict.
const (
	websterOutcomeDone  = "done"
	websterOutcomeStuck = "stuck"
)

// planConflictWayForward is the way-forward clause that ends the Stuck reason for a webster plan conflict.
// It names the steps that clear the conflict.
const planConflictWayForward = "the run's parent records the fix as a decision naming the conflicting pending cards by slug and number with \"lym loom decision add <slug> --by parent … --amends-plan\", then runs \"lym loom goto --to Plan-Burler-Review\" and \"lym loom resume\" in the task worktree"

// WebsterProducer is a shedengine.ShedProducer that runs websterengine.Run once per Call.
// Each Call runs with Fresh false and AutoRebaseline true, and maps the result onto the shedengine contract.
type WebsterProducer struct {
	name string
	run  WebsterRunner
	deps websterengine.RunDeps
}

var _ shedengine.ShedProducer = (*WebsterProducer)(nil)

// NewWebsterProducer returns a WebsterProducer named name that drives deps through run.
// A nil run defaults to websterengine.Run.
func NewWebsterProducer(name string, run WebsterRunner, deps websterengine.RunDeps) *WebsterProducer {
	if run == nil {
		run = websterengine.Run
	}
	if deps.ReentryStep == "" {
		deps.ReentryStep = fmt.Sprintf("re-step the %s row", name)
	}
	return &WebsterProducer{name: name, run: run, deps: deps}
}

// Call runs one iteration of the producer.
// It checks the context on entry.
// It invokes the run seam with Fresh false and AutoRebaseline true.
// It maps the RunResult and error onto the shedengine contract.
//
// RunOptions.Fresh is fixed false and is never configurable here.
// Fresh true is the destructive fingerprint-mismatch escape.
// It archives state, reports, and clears the rendered prompts.
// It must stay an explicit human act through the CLI, never something a Shed resume triggers.
// AutoRebaseline is the non-destructive counterpart this row sets.
// A plan changed between steps is rebaselined on entry.
// Every failure of that path, a refusal or a transient alike, maps to Stuck with its text.
// The way forward in that text re-steps this row.
//
// No mid-run bridge is installed.
// A cancel goes unobserved until Master reaches a terminal outcome or its configured whole-run timeout elapses.
func (p *WebsterProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name, websterEngineLabel); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	result, err := p.run(p.deps, websterengine.RunOptions{Fresh: false, AutoRebaseline: true})
	if err != nil {
		if errors.Is(err, websterengine.ErrPendingAuditFindings) || errors.Is(err, websterengine.ErrAutoRebaseline) || errors.Is(err, websterengine.ErrBatchDeadEnd) {
			// A halt only the operator can clear.
			// The run blocks with the refusal's own text, and its way forward already ends in this row's re-entry step.
			if cerr := cancelErr(ctx, p.name, websterEngineLabel); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return shedengine.Stuck, shedengine.OutputPointer{Reason: err.Error()}, nil
		}

		if cerr := cancelErr(ctx, p.name, websterEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, err
	}

	switch result.Outcome {
	case websterOutcomeDone:
		// A success verdict survives cancellation, the one outcome cancelErr never applies to.
		// The summary is Webster's account of the whole run and is present on this outcome.
		// The webster dir is taken from the producer's geometry, never derived.
		return shedengine.Done, shedengine.OutputPointer{Path: summaryparser.Path(p.deps.Geom.WebsterDir), GateAttempts: gateAttemptsPointer(result.Gate), GateSpans: gateSpansOf(result.Gate)}, nil

	case websterOutcomeStuck:
		if cerr := cancelErr(ctx, p.name, websterEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("shedadapters: webster run is stuck", "producer", p.name, "engine", websterEngineLabel, "stuckReason", result.StuckReason, "batchesDone", result.BatchesDone)
		ptr := shedengine.OutputPointer{GateAttempts: gateAttemptsPointer(result.Gate), GateSpans: gateSpansOf(result.Gate), Reason: result.StuckReason}
		route, claimErr := websterengine.ClaimConflictRoute(p.deps.Geom.ScratchDir)
		if claimErr != nil {
			logger.Warn("shedadapters: webster plan-conflict record not claimed; the stuck outcome halts", "producer", p.name, "engine", websterEngineLabel, "error", claimErr)
			return shedengine.Stuck, ptr, nil
		}
		switch {
		case route.New:
			ptr.Reroute = true
			ptr.Reason = "webster plan conflict, asking a reroute to plan review: " + result.StuckReason + "; if plan review does not clear it: " + planConflictWayForward
		case route.Found:
			ptr.Reason = "the same webster plan conflict recurred after a reroute of it was asked for: " + result.StuckReason + "; way forward: " + planConflictWayForward
		}
		return shedengine.Stuck, ptr, nil

	default:
		if cerr := cancelErr(ctx, p.name, websterEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): unrecognized webster outcome %q", p.name, websterEngineLabel, result.Outcome)
	}
}
