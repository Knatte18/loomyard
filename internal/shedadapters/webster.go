// webster.go implements WebsterProducer, the shedadapters adapter over websterengine's black-box
// multi-spawn run seam: it drives one websterengine.Run call and maps its RunResult/error onto the
// shedengine.ShedProducer contract.

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

// websterEngineLabel is the short engine label WebsterProducer's log lines and error text carry.
const websterEngineLabel = "webster"

// WebsterRunner is the func-typed seam WebsterProducer drives Webster's run verb through. Webster's
// entry point is already a free function, so this seam is a func type rather than an interface.
type WebsterRunner func(websterengine.RunDeps, websterengine.RunOptions) (websterengine.RunResult, error)

// Compile-time proof that websterengine.Run satisfies WebsterRunner.
var _ WebsterRunner = websterengine.Run

// Webster's own outcome values (websterengine's own outcomeDone/outcomeStuck/outcomePaused
// constants) are unexported, so this adapter compares RunResult.Outcome against its own copies of
// the same literals. The duplication is named rather than hidden: the default: branch in Call below
// errors on any value that is not one of these three, and a test row drives that branch, so a
// webster-side rename surfaces as a failing test here instead of a silently mis-mapped verdict.
const (
	websterOutcomeDone   = "done"
	websterOutcomeStuck  = "stuck"
	websterOutcomePaused = "paused"
)

// WebsterProducer is the shedadapters adapter over websterengine's Run seam: it invokes run once per
// Call with Fresh always false and AutoRebaseline always true, and maps the resulting RunResult/error onto shedengine's contract.
type WebsterProducer struct {
	name string
	run  WebsterRunner
	deps websterengine.RunDeps
}

var _ shedengine.ShedProducer = (*WebsterProducer)(nil)

// NewWebsterProducer returns a WebsterProducer identified as name, driving deps through run. A nil
// run defaults to websterengine.Run, the production seam.
func NewWebsterProducer(name string, run WebsterRunner, deps websterengine.RunDeps) *WebsterProducer {
	if run == nil {
		run = websterengine.Run
	}
	if deps.ReentryStep == "" {
		deps.ReentryStep = fmt.Sprintf("re-step the %s row", name)
	}
	return &WebsterProducer{name: name, run: run, deps: deps}
}

// Call runs one WebsterProducer iteration: entry-check the context, invoke the run seam with
// Fresh: false and AutoRebaseline: true, and map its RunResult/error onto shedengine's contract.
//
// RunOptions.Fresh is fixed false and is never configurable here: Fresh: true is the destructive
// fingerprint-mismatch escape (it archives state and reports and clears the rendered prompts), and
// must stay an explicit human act via the CLI, never something a Shed resume triggers.
// AutoRebaseline is the non-destructive counterpart this row sets: a plan changed between steps is rebaselined on entry.
// Every failure of that path, a refusal or a transient alike, maps to Stuck with its text, whose way forward re-steps this row.
//
// No mid-run bridge is installed: Webster's pause is an operator-owned flag file the batch loop
// polls, and writing it from a context watcher would conflate the two pause channels, race the
// run's own clear calls, and risk leaving the next invocation permanently paused. The accepted
// consequence is that a cancel is not observed until Master reaches a terminal outcome or its own
// configured whole-run timeout elapses.
func (p *WebsterProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name, websterEngineLabel); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	result, err := p.run(p.deps, websterengine.RunOptions{Fresh: false, AutoRebaseline: true})
	if err != nil {
		if errors.Is(err, websterengine.ErrPendingAuditFindings) || errors.Is(err, websterengine.ErrAutoRebaseline) {
			// A halt only the operator can clear: the run blocks with the refusal's own text, whose way forward already ends in this row's re-entry step.
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
		// A genuine success verdict survives cancellation -- the one exception cancelErr never
		// applies to. The summary is Webster's human-readable account of the whole run and is
		// guaranteed present on this outcome; the webster dir is already told, never derived.
		return shedengine.Done, shedengine.OutputPointer{Path: summaryparser.Path(p.deps.Geom.WebsterDir)}, nil

	case websterOutcomeStuck:
		if cerr := cancelErr(ctx, p.name, websterEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("shedadapters: webster run is stuck", "producer", p.name, "engine", websterEngineLabel, "stuckReason", result.StuckReason, "batchesDone", result.BatchesDone)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: result.StuckReason}, nil

	case websterOutcomePaused:
		if cerr := cancelErr(ctx, p.name, websterEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): webster run paused out of band; way forward: re-step the %s row, since lyx webster run clears the pause and resumes", p.name, websterEngineLabel, p.name)

	default:
		if cerr := cancelErr(ctx, p.name, websterEngineLabel); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("shedadapters: %s (%s): unrecognized webster outcome %q", p.name, websterEngineLabel, result.Outcome)
	}
}
