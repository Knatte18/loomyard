// preflight.go implements Check and CheckResolved, the orchestrator-agnostic tier-1 and tier-2
// preconditions — worktree geometry, worktree pair cleanliness, and fabric readiness/sync — over
// git and filesystem state, reporting a determined Report rather than erroring on anything short of
// an infra failure.
// See doc.go for the full report-not-error contract.

package preflight

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// Check resolves cwd into a *lyxcwd.Location and validates the tier-1/tier-2 preconditions against
// it.
//
// Returns (Report{OK:true}, l, nil) when every precondition is met.
// Returns (Report{OK:false, Failures}, l, nil) when one or more preconditions are unmet — a normal,
// expected outcome, not an error.
// Returns (Report{}, nil, err) when Check could not determine an answer at all because
// lyxcwd.Resolve itself failed — the caller must escalate, not treat this as "not ready". When
// Resolve succeeds but the downstream CheckResolved(l) call returns an infra error instead, Check
// returns (Report{}, l, err) with a non-nil Location alongside the error.
//
// The resolved *lyxcwd.Location is returned on success (both the OK and the determined-failure
// cases) so the caller never re-resolves: lyxcwd.Resolve spawns `git rev-parse --show-toplevel`,
// and forcing an orchestrator to resolve again to reach its own tier-3 paths would double the git
// spawns of every preflight.
func Check(cwd string) (Report, *lyxcwd.Location, error) {
	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		// ErrNotAGitRepo is a determined verdict (not inside a git repository at
		// all), not an infra failure — short-circuit with a single geometry
		// failure rather than escalating.
		if errors.Is(err, lyxcwd.ErrNotAGitRepo) {
			return Report{
				OK:       false,
				Failures: []Failure{{Check: CheckGeometry, Reason: "not inside a git repository"}},
			}, nil, nil
		}
		// Any other Resolve error is reachable only through lyxcwd's
		// anchor-resolution path — e.g. ErrCwdOutsideAnchor from the cwd gate,
		// ErrStaleAnchorMarker from a board carrying only the pre-rename
		// marker, or an ErrInvalidAnchor-wrapping failure when a recorded
		// anchor exists but fails validation, among other anchor-resolution
		// failures — and is a genuine "couldn't determine" infra failure.
		return Report{}, nil, err
	}

	report, err := CheckResolved(l)
	return report, l, err
}

// CheckResolved runs the tier-1/tier-2 checks against an already-resolved Location, letting a
// caller that already holds one (or a test building one synthetically) skip Check's own
// lyxcwd.Resolve call.
func CheckResolved(l *lyxcwd.Location) (Report, error) {
	// Check 1b: geometry sanity. A PrimeName resolution failure (List found no
	// main-worktree entry, or the git subprocess itself failed) is treated the
	// same as "not a git repo" for this purpose, since there is no coherent
	// worktree to check — never a hard error, preserving the report-not-error
	// contract.
	if _, err := fabricengine.PrimeName(l); err != nil {
		return Report{
			OK:       false,
			Failures: []Failure{{Check: CheckGeometry, Reason: "no main worktree resolved"}},
		}, nil
	}
	// There is deliberately no at-the-anchor check here. lyxcwd.Resolve applies a strict cwd gate,
	// so a successful resolve already proves cwd equals AnchorPath() exactly;
	// a non-"." AnchorRel therefore means "this repo is subpath-anchored", never "the caller stood
	// in a subdirectory", and rejecting it failed every subpath-anchored hub unconditionally.

	var report Report

	// Check 2: worktree pair cleanliness. Collected, not short-circuited — a
	// dirty side does not prevent the remaining checks from also reporting.
	clean, reason, err := fabricengine.Clean(l)
	if err != nil {
		return Report{}, err
	}
	if !clean {
		report.AddFailure(CheckWorktreeClean, reason)
	}

	// Check 3: fabric readiness and sync.
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
			// Healthy's typed Cause replaces a substring match: a branch
			// mismatch classifies as CheckFabricSync, and every other cause
			// classifies as CheckJunction.
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
