// stuck.go holds the stuck-verdict helpers: the logger reportStuck, and the reconcile and gate mappings built on it.
// The engine persists a producer's Reason as the blocked error when the row has no `OnStuck` target,
// and every landingshed row has none; the logged warning stays in the trace after that error is overwritten.

package landingshed

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// reportStuck logs a structured warning through the shared logger.
// The producer and reason fields come first, followed by the caller's key-value pairs.
func reportStuck(producer, reason string, fields ...any) {
	logFields := append([]any{"producer", producer, "reason", reason}, fields...)
	logger.Warn("landingshed: producer stuck", logFields...)
}

// stuckVerdict returns the cancellation error when ctx is cancelled.
// Otherwise it logs a warning and returns a Stuck verdict carrying reason on the output pointer.
// When halt is true, the run blocks at the producer's own row whatever the row's on_stuck route.
func stuckVerdict(ctx context.Context, producer, reason string, halt bool, fields ...any) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, producer); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	reportStuck(producer, reason, fields...)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason, Halt: halt}, nil
}

// reconcileVerdict runs the run-scope origin reconcile and maps the run's own entry onto the producer's return values.
// The fourth result is false when the run is active, when no reconcile is wired, or when origin was unreachable.
// An unreachable origin logs one Warn; the run is not judged against it, so the row fails on its own remote steps.
// Otherwise the fourth result is true.
// An ended entry gives a halting Stuck naming the landing or the board removal.
// Its way forward is the end of the run from the prime.
// A stopped entry gives a Stuck naming the branch, its state and its way forward, then the row's re-entry.
// A cancellation or a reconcile error is returned as an error.
// Each entry of the report's mutation record is logged at Info, since a row has no envelope to carry it.
// followUp, when non-nil, ends a stopped reason whose way forward names the pair's removal.
func reconcileVerdict(ctx context.Context, producer string, reconcile func() (fabricengine.OriginReport, error), followUp func() string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if reconcile == nil {
		return "", shedengine.OutputPointer{}, nil, false
	}
	report, err := reconcileReport(ctx, producer, reconcile)
	if err != nil {
		return "", shedengine.OutputPointer{}, err, true
	}
	if report.Unreachable {
		logger.Warn("landingshed: origin was unreachable, proceeding without judging the run against it", "producer", producer)
		return "", shedengine.OutputPointer{}, nil, false
	}
	return reportVerdict(ctx, producer, report, followUp)
}

// reconcileReport runs the run-scope origin reconcile and logs each entry of the report's mutation record at Info.
// A cancellation is returned as the cancellation error and any other failure wrapped, with the producer named.
func reconcileReport(ctx context.Context, producer string, reconcile func() (fabricengine.OriginReport, error)) (fabricengine.OriginReport, error) {
	report, err := reconcile()
	for _, mutation := range report.Mutations.Entries() {
		logger.Info("landingshed: origin reconcile mutation", "producer", producer, "kind", string(mutation.Kind), "target", mutation.Target, "detail", mutation.Detail)
	}
	if err != nil {
		if cerr := cancelErr(ctx, producer); cerr != nil {
			return report, cerr
		}
		return report, fmt.Errorf("landingshed: %s: origin reconcile: %w", producer, err)
	}
	return report, nil
}

// reportVerdict maps the ended and stopped entries of a reached origin's report onto the producer's return values.
// The fourth result is false when no entry ended or stopped, including an entry another machine owns.
// A false fourth result lets the producer proceed.
// An ended entry gives a halting Stuck naming the landing or the board removal.
// Its way forward is the end of the run from the prime.
// A stopped entry gives a Stuck naming the branch, its state and its way forward, then the row's re-entry.
// followUp, when non-nil, ends a stopped reason whose way forward names the pair's removal.
func reportVerdict(ctx context.Context, producer string, report fabricengine.OriginReport, followUp func() string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	for _, entry := range report.Entries {
		var reason string
		var halt bool
		switch entry.State {
		case fabricengine.EntryEnded:
			reason, halt = endedReason(entry.How), true
		case fabricengine.EntryStopped:
			reason = stoppedReason(entry, report.Branches, followUp)
		default:
			continue
		}
		outcome, out, err := stuckVerdict(ctx, producer, reason, halt)
		return outcome, out, err, true
	}
	return "", shedengine.OutputPointer{}, nil, false
}

// endedReason words the Stuck reason for a run whose origin ended, given how it ended.
// The way forward is the end of the run from the prime.
func endedReason(how string) string {
	ended := "its board entry was removed from the board"
	if how == "done" {
		ended = "its landing is done and its board entry is done"
	}
	return fmt.Sprintf(`origin ended this run: %s; way forward: pause the run, then run "lym cleanup --apply" from the prime, whose hub reconcile tears this pair down`, ended)
}

// stoppedReason words the Stuck reason for a run whose origin judgment stopped.
// The reason names the stopping branch, its state and its way forward, then the row's re-entry.
// The stopping branch is the one whose way forward equals the entry's; without one, no branch is named.
// A way forward that names the pair's removal ends in followUp instead, when followUp returns non-empty text.
// That removal deletes the task worktree the resume runs in.
func stoppedReason(entry fabricengine.EntryOutcome, branches []fabricengine.BranchOutcome, followUp func() string) string {
	for _, branch := range branches {
		if branch.WayForward != "" && branch.WayForward == entry.WayForward {
			return fmt.Sprintf("origin reconcile stopped this run: branch %s is %s against origin; way forward: %s; %s", branch.Branch, branch.State, branch.WayForward, reentryAfter(branch.WayForward, followUp))
		}
	}
	return fmt.Sprintf("origin reconcile stopped this run; way forward: %s; %s", entry.WayForward, reentryAfter(entry.WayForward, followUp))
}

// resumeReentry is the step that ends a stopped reason: the resume in the task worktree.
const resumeReentry = `then run "lym loom resume" in the task worktree`

// fabricRemoveCommand is the removal a way forward names for a branch that only the removal clears.
const fabricRemoveCommand = "lym fabric remove"

// reentryAfter returns the step that ends a stopped reason, given that reason's way forward.
// It is followUp's text when wayForward names the pair's removal and followUp returns non-empty text;
// otherwise it is the resume.
func reentryAfter(wayForward string, followUp func() string) string {
	if followUp != nil && strings.Contains(wayForward, fabricRemoveCommand) {
		if text := followUp(); text != "" {
			return text
		}
	}
	return resumeReentry
}

// gateStop maps one gate call's result onto the producer's return values.
// The fourth result is false when the gate passed, so the producer proceeds.
// Otherwise the fourth result is true.
// A cancellation or an infrastructure fault is returned as an error;
// a Stuck reason is returned as a Stuck verdict.
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
