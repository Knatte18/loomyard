// stuck.go declares reportStuck, the one helper every producer routes its stuck verdicts through, and the verdict helpers built on it.
//
// The producer seam (shedengine.ShedProducer) carries a producer's reason on
// OutputPointer.Reason, which the engine persists as the blocked error when the row has no OnStuck
// target -- every landingshed row has none. This helper is the log-line carrier beside it: a
// structured warning that stays in the trace after the persisted error is overwritten.

package landingshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// reportStuck emits a structured warning through the shared logger carrying at minimum a producer
// field and a reason field alongside fields' own key-value pairs.
func reportStuck(producer, reason string, fields ...any) {
	logFields := append([]any{"producer", producer, "reason", reason}, fields...)
	logger.Warn("landingshed: producer stuck", logFields...)
}

// stuckVerdict consults cancelErr first, like every non-success exit, and otherwise logs reason via reportStuck and returns Stuck with reason on the output pointer.
// halt blocks the run at the producer's own row whatever the row's on_stuck route.
func stuckVerdict(ctx context.Context, producer, reason string, halt bool, fields ...any) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, producer); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	reportStuck(producer, reason, fields...)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason, Halt: halt}, nil
}

// gateStop maps one gate call's result onto a producer's Call return.
// stop is false when the gate passed, so Call proceeds.
// Otherwise the other values are what Call returns at once:
// a cancellation or an infrastructure fault as an error, a Stuck reason as a Stuck verdict.
func gateStop(ctx context.Context, producer, reason string, err error) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if err != nil {
		if cerr := cancelErr(ctx, producer); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr, true
		}
		return "", shedengine.OutputPointer{}, err, true
	}
	if reason != "" {
		outcome, out, err := stuckVerdict(ctx, producer, reason, false)
		return outcome, out, err, true
	}
	return "", shedengine.OutputPointer{}, nil, false
}
