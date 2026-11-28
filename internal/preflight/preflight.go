// preflight.go implements Check and CheckResolved, the orchestrator-agnostic tier-1 and tier-2 preconditions:
// worktree geometry, worktree pair cleanliness, and fabric readiness and sync, read from git and the filesystem.
// A failed precondition is reported in the Report; only an infra failure is returned as an error.

package preflight

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// runStateExemptRels returns the anchor-relative pathspecs under `_lym` that the worktree-clean check ignores.
// Their uncommitted content is lym's own state.
// The run-records root holds what lym writes while a run handles a stop:
// settle records, tackler and arbiter attempts, and stop markers.
// The friction directory holds the halt and crash-resume notes lym writes and the notes agents write.
// A reflection moves the notes it covered into a timestamped archive sibling.
// Those writes, deletions and moves stay uncommitted until a later records commit or the run's removal.
// The check ignores them because they are lym's own bookkeeping in flight, never work a run could lose.
// Counting them would let one halt's note cause the next halt.
// It is the one place that decides which lym state the check skips.
func runStateExemptRels() []string {
	return []string{shedrun.RunsRootRel(), loomengine.LoomFrictionDirRel(), loomengine.LoomFrictionArchivesRelPattern()}
}

// Check resolves cwd into a *lymcwd.Location and validates the tier-1 and tier-2 preconditions against it.
//
// Returns (Report{OK:true}, l, nil) when every precondition is met.
// Returns (Report{OK:false, Failures}, l, nil) when one or more preconditions are unmet.
// That is a normal outcome, not an error.
// Returns (Report{}, nil, err) when lymcwd.Resolve fails for any reason but a cwd outside a git repository.
// No answer was determined, so the caller must escalate rather than treat the result as not ready.
// When lymcwd.Resolve succeeds but CheckResolved returns an infra error:
// Check returns (Report{}, l, err), with the non-nil Location alongside the error.
//
// The Location is returned whenever Resolve succeeds, so the caller never resolves again:
// lymcwd.Resolve spawns `git rev-parse --show-toplevel`,
// and a second resolve would double the git spawns of every preflight.
func Check(cwd string) (Report, *lymcwd.Location, error) {
	l, err := lymcwd.Resolve(cwd)
	if err != nil {
		// Not being in a git repository is a determined verdict, not an infra failure,
		// so it short-circuits with a single geometry failure instead of escalating.
		if errors.Is(err, lymcwd.ErrNotAGitRepo) {
			return Report{
				OK:       false,
				Failures: []Failure{{Check: CheckGeometry, Reason: "not inside a git repository"}},
			}, nil, nil
		}
		// Any other Resolve error comes only from lymcwd's anchor resolution, and it is a genuine infra failure.
		return Report{}, nil, err
	}

	report, err := CheckResolved(l)
	return report, l, err
}

// CheckResolved runs the tier-1 and tier-2 checks against an already-resolved Location.
// A caller that already holds a Location, or a test that builds one, uses it to skip resolving again.
func CheckResolved(l *lymcwd.Location) (Report, error) {
	// A PrimeName failure, whether no main worktree is listed or the git subprocess fails, is a geometry failure.
	// There is no coherent worktree to check, so it is reported, never returned as an error.
	if _, err := fabricengine.PrimeName(l); err != nil {
		return Report{
			OK:       false,
			Failures: []Failure{{Check: CheckGeometry, Reason: "no main worktree resolved"}},
		}, nil
	}
	// No at-the-anchor check is needed:
	// the strict cwd gate in lymcwd.Resolve already proves cwd equals the anchor path.
	// A non-"." AnchorRel therefore means the repo is subpath-anchored, never that the caller stood in a subdirectory.
	// Rejecting it would fail every subpath-anchored hub unconditionally.

	var report Report

	// Worktree pair cleanliness: failures are collected, not short-circuited,
	// so a dirty side does not stop the remaining checks from reporting.
	clean, reason, err := fabricengine.Clean(l, runStateExemptRels()...)
	if err != nil {
		return Report{}, err
	}
	if !clean {
		report.AddFailure(CheckWorktreeClean, reason)
	}

	ready, err := fabricengine.Ready(l)
	if err != nil {
		return Report{}, err
	}
	if !ready {
		report.AddFailure(CheckFabricReady, "fabric not ready")
	} else {
		ok, reason, err := fabricengine.Healthy(l)
		if err != nil {
			return Report{}, err
		}
		if !ok {
			// Healthy's typed Cause picks the check: a branch mismatch is CheckFabricSync,
			// and every other cause is CheckJunction.
			var check CheckID
			switch reason.Cause {
			case fabricengine.CauseBranchMismatch:
				check = CheckFabricSync
			default:
				check = CheckJunction
			}
			report.AddFailure(check, reason.Detail)
		}
	}

	report.OK = len(report.Failures) == 0
	return report, nil
}
