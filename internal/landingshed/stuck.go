// stuck.go declares reportStuck, the one helper every producer routes its stuck verdicts through, and the verdict helpers built on it.
//
// The producer seam (shedengine.ShedProducer) carries a producer's reason on
// OutputPointer.Reason, which the engine persists as the blocked error when the row has no OnStuck
// target -- every landingshed row has none. This helper is the log-line carrier beside it: a
// structured warning that stays in the trace after the persisted error is overwritten.

package landingshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
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

// reconcileVerdict runs the run-scope origin reconcile and maps the verdict on the run's own entry onto a producer's Call return.
// stop is false when the run is active, when origin was unreachable, which logs one Warn so the row fails on its own remote steps as before, and when no reconcile is wired.
// Otherwise the other values are what Call returns at once:
// an ended entry as a halting Stuck naming the landing or the board removal, with the end of the run from the prime as the way forward;
// a stopped entry as a Stuck naming the branch, its state and its way forward, then the row's re-entry;
// a cancellation or a reconcile error as an error.
// Each entry of the report's mutation record is logged at Info, since a row has no envelope to carry it.
func reconcileVerdict(ctx context.Context, producer string, reconcile func() (fabricengine.OriginReport, error)) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if reconcile == nil {
		return "", shedengine.OutputPointer{}, nil, false
	}
	report, err := reconcile()
	for _, mutation := range report.Mutations.Entries() {
		logger.Info("landingshed: origin reconcile mutation", "producer", producer, "kind", string(mutation.Kind), "target", mutation.Target, "detail", mutation.Detail)
	}
	if err != nil {
		if cerr := cancelErr(ctx, producer); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr, true
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: origin reconcile: %w", producer, err), true
	}
	if report.Unreachable {
		logger.Warn("landingshed: origin was unreachable, proceeding without judging the run against it", "producer", producer)
		return "", shedengine.OutputPointer{}, nil, false
	}
	for _, entry := range report.Entries {
		var reason string
		var halt bool
		switch entry.State {
		case fabricengine.EntryEnded:
			reason, halt = endedReason(entry.How), true
		case fabricengine.EntryStopped:
			reason = stoppedReason(entry, report.Branches)
		default:
			continue
		}
		outcome, out, err := stuckVerdict(ctx, producer, reason, halt)
		return outcome, out, err, true
	}
	return "", shedengine.OutputPointer{}, nil, false
}

// endedReason words the Stuck reason of a run origin ended: how it ended, and ending the run here from the prime as the way forward.
func endedReason(how string) string {
	ended := "its board entry was removed from the board"
	if how == "done" {
		ended = "its landing is done and its board entry is done"
	}
	return fmt.Sprintf(`origin ended this run: %s; way forward: pause the run, then run "lyx cleanup --apply" from the prime, whose hub reconcile tears this pair down`, ended)
}

// stoppedReason words the Stuck reason of a run origin's judgment stopped: the branch that stopped it, its state and its way forward, then the row's re-entry.
// The branch is the one whose way forward the entry carries; an entry with no such branch names none.
func stoppedReason(entry fabricengine.EntryOutcome, branches []fabricengine.BranchOutcome) string {
	const reentry = `then run "lyx loom resume" in the task worktree`
	for _, branch := range branches {
		if branch.WayForward != "" && branch.WayForward == entry.WayForward {
			return fmt.Sprintf("origin reconcile stopped this run: branch %s is %s against origin; way forward: %s; %s", branch.Branch, branch.State, branch.WayForward, reentry)
		}
	}
	return fmt.Sprintf("origin reconcile stopped this run; way forward: %s; %s", entry.WayForward, reentry)
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
