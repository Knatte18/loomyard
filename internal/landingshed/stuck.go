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

// holdParent takes the parent lock for producer's row and then tests whether the parent moved since Merge-In merged it.
// stop is false when the row holds the lock and the parent did not move, so Call proceeds.
// Otherwise the other values are what Call returns at once:
// a paused wait or an unreadable lock record as a halting Stuck, a moved parent as a reroute Stuck, a cancellation or an infrastructure fault as an error.
func holdParent(ctx context.Context, guard parentGuard, producer string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	reason, err := guard.ensure(ctx, producer)
	if err != nil {
		if cerr := cancelErr(ctx, producer); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr, true
		}
		return "", shedengine.OutputPointer{}, err, true
	}
	if reason != "" {
		outcome, out, err := stuckVerdict(ctx, producer, reason, true)
		return outcome, out, err, true
	}
	return rerouteIfMoved(ctx, guard, producer)
}

// rerouteIfMoved returns a reroute Stuck for producer's row when the parent moved since Merge-In merged it, which spends no bounce budget.
// stop is false when it did not move.
// A guard built without a tip read, as a struct-literal test builds, never reroutes.
func rerouteIfMoved(ctx context.Context, guard parentGuard, producer string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if guard.readParentTips == nil {
		return "", shedengine.OutputPointer{}, nil, false
	}
	reason, err := guard.moved(producer)
	if err != nil {
		if cerr := cancelErr(ctx, producer); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr, true
		}
		return "", shedengine.OutputPointer{}, err, true
	}
	if reason == "" {
		return "", shedengine.OutputPointer{}, nil, false
	}
	if cerr := cancelErr(ctx, producer); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr, true
	}
	reportStuck(producer, reason, "reroute", true)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason, Reroute: true}, nil, true
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
