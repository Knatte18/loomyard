// stuck.go holds the ending every landing step returns and the helpers that build it.
// It also holds the reconcile mapping built on them.
// The engine persists a producer's Reason as the blocked error when the row has no `OnStuck` target,
// and every landingshed row has none; the logged warning stays in the trace after that error is overwritten.

package landingshed

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// ending is how a step ends a producer's call: a verdict with its output pointer, or an error.
// A step that lets the call go on returns a nil ending.
type ending struct {
	outcome shedengine.Outcome
	out     shedengine.OutputPointer
	err     error
}

// result returns the ending as a producer's Call returns it.
func (e *ending) result() (shedengine.Outcome, shedengine.OutputPointer, error) {
	return e.outcome, e.out, e.err
}

// done ends a producer's call with the Done verdict.
func done() *ending {
	return &ending{outcome: shedengine.Done}
}

// fail ends producer's call with err, or with the cancellation error when ctx is cancelled.
func fail(ctx context.Context, producer string, err error) *ending {
	if cerr := cancelErr(ctx, producer); cerr != nil {
		return &ending{err: cerr}
	}
	return &ending{err: err}
}

// stuck ends producer's call with a Stuck verdict carrying reason, after a warning that logs reason and fields.
// A cancelled ctx ends the call with the cancellation error instead.
func stuck(ctx context.Context, producer, reason string, fields ...any) *ending {
	if cerr := cancelErr(ctx, producer); cerr != nil {
		return &ending{err: cerr}
	}
	logger.Warn("landingshed: producer stuck", append([]any{"producer", producer, "reason", reason}, fields...)...)
	return &ending{outcome: shedengine.Stuck, out: shedengine.OutputPointer{Reason: reason}}
}

// halt ends producer's call like stuck, and blocks the run at the producer's own row whatever the row's on_stuck route.
func halt(ctx context.Context, producer, reason string) *ending {
	end := stuck(ctx, producer, reason)
	end.out.Halt = end.err == nil
	return end
}

// gateEnding ends producer's call on one gate step's result: err as an error, a non-empty reason as a Stuck.
// It returns nil when the step passed, so the producer proceeds.
func gateEnding(ctx context.Context, producer, reason string, err error) *ending {
	switch {
	case err != nil:
		return fail(ctx, producer, err)
	case reason != "":
		return stuck(ctx, producer, reason)
	}
	return nil
}

// reconcileVerdict runs the run-scope origin reconcile and maps the run's own entry onto the producer's ending.
// It returns nil when the run is active, when no reconcile is wired, or when origin was unreachable.
// An unreachable origin logs one Warn; the run is not judged against it, so the row fails on its own remote steps.
// An ended entry gives a halting Stuck naming the landing or the board removal.
// Its way forward is the end of the run from the prime.
// A stopped entry gives a Stuck naming the branch, its state and its way forward, then the row's re-entry.
// A cancellation or a reconcile error ends the call as an error.
// Each entry of the report's mutation record is logged at Info, since a row has no envelope to carry it.
// followUp, when non-nil, ends a stopped reason whose way forward names the pair's removal.
func reconcileVerdict(ctx context.Context, producer string, reconcile func() (fabriclayout.OriginReport, error), followUp func() string) *ending {
	if reconcile == nil {
		return nil
	}
	report, err := reconcileReport(ctx, producer, reconcile)
	if err != nil {
		return &ending{err: err}
	}
	if report.Unreachable {
		logger.Warn("landingshed: origin was unreachable, proceeding without judging the run against it", "producer", producer)
		return nil
	}
	return reportVerdict(ctx, producer, report, followUp)
}

// reconcileReport runs the run-scope origin reconcile and logs each entry of the report's mutation record at Info.
// A cancellation is returned as the cancellation error and any other failure wrapped, with the producer named.
func reconcileReport(ctx context.Context, producer string, reconcile func() (fabriclayout.OriginReport, error)) (fabriclayout.OriginReport, error) {
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

// reportVerdict maps the ended and stopped entries of a reached origin's report onto the producer's ending.
// It returns nil when no entry ended or stopped, including an entry another machine owns, so the producer proceeds.
// An ended entry gives a halting Stuck naming the landing or the board removal.
// Its way forward is the end of the run from the prime.
// A stopped entry gives a Stuck naming the branch, its state and its way forward, then the row's re-entry.
// followUp, when non-nil, ends a stopped reason whose way forward names the pair's removal.
func reportVerdict(ctx context.Context, producer string, report fabriclayout.OriginReport, followUp func() string) *ending {
	for _, entry := range report.Entries {
		switch entry.State {
		case fabriclayout.EntryEnded:
			return halt(ctx, producer, endedReason(entry.How))
		case fabriclayout.EntryStopped:
			return stuck(ctx, producer, stoppedReason(entry, report.Branches, followUp))
		}
	}
	return nil
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
func stoppedReason(entry fabriclayout.EntryOutcome, branches []fabriclayout.BranchOutcome, followUp func() string) string {
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
