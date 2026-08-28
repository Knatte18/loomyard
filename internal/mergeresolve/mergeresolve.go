// mergeresolve.go implements Resolve, the full merge-and-conflict decision table this package
// exists for.

package mergeresolve

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// maxConflictAttempts is the number of conflict-resolution sessions Resolve spends on one Resolve
// call before giving up and reporting stuck: one first attempt, one retry.
const maxConflictAttempts = 2

// Resolve merges source into the current pair and, if the merge produces conflicts, drives at most
// two conflict-resolution sessions to clear them.
//
// The mechanical marker re-scan (markers.go), not the session's own verdict, is what decides a
// conflict is resolved, because the conclude step is irreversible at the Fabric layer and the abort
// call is the only checkpoint covering the attempt window: once MergeContinue lands, there is no
// undo, so Resolve never trusts a session's own claim of success and always re-verifies by content
// before staging and concluding.
func (r *Resolver) Resolve(ctx context.Context, source string) (Result, error) {
	if err := entryErr(ctx); err != nil {
		return Result{}, err
	}

	// Crash recovery: a half-resolved worktree left by a killed session is not a state the caller
	// (which re-invokes Resolve verbatim after a crash) should resume into, so any in-progress merge
	// is aborted unconditionally before a clean attempt starts.
	inProgress, err := r.deps.Fabric.MergeInProgress()
	if err != nil {
		return Result{}, fmt.Errorf("mergeresolve: check merge in progress: %w", err)
	}
	if inProgress {
		if _, err := r.deps.Fabric.MergeAbort(); err != nil {
			return Result{}, fmt.Errorf("mergeresolve: abort in-progress merge before a clean attempt: %w", err)
		}
	}

	mergeRes, err := r.deps.Fabric.MergeIn(source)
	if err != nil {
		return r.mergeInErrorResult(ctx, err)
	}

	if mergeRes.AlreadyUpToDate {
		return Result{Outcome: OutcomeResolved, AlreadyUpToDate: true}, nil
	}
	if len(mergeRes.Conflicts) == 0 {
		// The merge concluded itself: nothing further to do.
		return Result{Outcome: OutcomeResolved}, nil
	}

	return r.resolveConflicts(ctx, mergeRes.Conflicts)
}

// mergeInErrorResult classifies a MergeIn error into its own disposition. Every branch is a stuck
// verdict with its own reason text, and none of them calls MergeAbort: a foreign merge state is real
// state a human left behind and is never touched; an unmergeable-state error has already self-
// aborted the whole attempt inside MergeIn itself, so a second abort would report no-merge-in-
// progress and turn a clear diagnosis into a confusing one; and the guard errors behind every other
// case refuse before mutating anything, so there is nothing to unwind.
func (r *Resolver) mergeInErrorResult(ctx context.Context, mergeErr error) (Result, error) {
	var foreign *fabricengine.ErrForeignMergeState
	var unmergeable *fabricengine.ErrUnmergeableState
	switch {
	case errors.As(mergeErr, &foreign):
		return r.stuck(ctx, fmt.Sprintf("merge-in refused: foreign merge state left behind by a human: %s", mergeErr))
	case errors.As(mergeErr, &unmergeable):
		return r.stuck(ctx, fmt.Sprintf("merge-in aborted: conflicts landed outside the fabric-managed tree: %s", mergeErr))
	default:
		return r.stuck(ctx, fmt.Sprintf("merge-in failed: %s", mergeErr))
	}
}

// resolveConflicts drives the resolution loop over conflicts: at most maxConflictAttempts sessions,
// each verified by a mechanical marker re-scan rather than trusted at its own word.
func (r *Resolver) resolveConflicts(ctx context.Context, conflicts []string) (Result, error) {
	for attempt := 1; attempt <= maxConflictAttempts; attempt++ {
		spec, err := buildConflictSpec(r.deps, conflicts, attempt)
		if err != nil {
			return Result{}, fmt.Errorf("mergeresolve: build conflict spec for attempt %d: %w", attempt, err)
		}

		runResult, err := r.deps.Shuttle.Run(spec)
		if err != nil {
			if cerr := cancelErr(ctx); cerr != nil {
				return Result{}, cerr
			}
			return Result{}, fmt.Errorf("mergeresolve: conflict session run (attempt %d): %w", attempt, err)
		}

		if runResult.Outcome != shuttleengine.OutcomeDone {
			// asking, died, timeout, or any outcome this package does not recognize: consult
			// cancelErr first, then abort and report stuck; the conclude call is never reached.
			// The Warn line lands before abortAndStuck so the diagnostic survives even when
			// MergeAbort itself fails.
			logger.Warn("mergeresolve: non-done conflict session outcome", "outcome", runResult.Outcome, "attempt", attempt, "sessionID", runResult.SessionID, "runDir", runResult.RunDir)
			return r.abortAndStuck(ctx, fmt.Sprintf("conflict session outcome %q (attempt %d)", runResult.Outcome, attempt))
		}

		unresolved, err := scanUnresolved(r.deps.WorktreeRoot, conflicts)
		if err != nil {
			return Result{}, fmt.Errorf("mergeresolve: scan conflict resolution (attempt %d): %w", attempt, err)
		}

		if len(unresolved) == 0 {
			// Clean scan: stage exactly the conflicted paths, then conclude, in that order — the
			// ordering is the entire reason the staging verb exists.
			if _, err := r.deps.Fabric.MergeStageResolved(conflicts); err != nil {
				return Result{}, fmt.Errorf("mergeresolve: stage resolved conflict paths (attempt %d): %w", attempt, err)
			}
			if _, err := r.deps.Fabric.MergeContinue(""); err != nil {
				return Result{}, fmt.Errorf("mergeresolve: conclude resolved merge (attempt %d): %w", attempt, err)
			}
			return Result{Outcome: OutcomeResolved}, nil
		}

		if attempt == maxConflictAttempts {
			return r.abortAndStuck(ctx, fmt.Sprintf("conflict markers still present after %d attempt(s) in: %s", attempt, strings.Join(unresolved, ", ")))
		}
		// Fewer than maxConflictAttempts spent: retry with a fresh attempt number, whose report
		// path is distinct from this attempt's, so the spec validator's already-exists rejection
		// cannot fire on the retry.
	}

	// Unreachable: the loop above always returns by its second iteration at the latest.
	return Result{}, fmt.Errorf("mergeresolve: resolveConflicts: exhausted attempts without a terminal result")
}

// stuck consults cancelErr and returns either the context-cancellation error (when ctx was
// cancelled during the run) or a stuck Result carrying reason — the point-7 obligation every
// stuck-producing exit discharges: a stuck verdict returned under a cancelled context is
// indistinguishable from a genuine verdict to the caller.
func (r *Resolver) stuck(ctx context.Context, reason string) (Result, error) {
	if err := cancelErr(ctx); err != nil {
		return Result{}, err
	}
	return Result{Outcome: OutcomeStuck, Reason: reason}, nil
}

// abortAndStuck aborts the in-progress merge unconditionally, then reports the stuck verdict
// (or the context-cancellation error in its place, via stuck) naming reason.
func (r *Resolver) abortAndStuck(ctx context.Context, reason string) (Result, error) {
	if _, err := r.deps.Fabric.MergeAbort(); err != nil {
		return Result{}, fmt.Errorf("mergeresolve: abort merge after %s: %w", reason, err)
	}
	return r.stuck(ctx, reason)
}
