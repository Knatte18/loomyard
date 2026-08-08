// preflight.go implements Preflight, the orchestrator that runs loom's four preconditions —
// worktree geometry, worktree cleanliness, fabric readiness/sync, and _lyx/status.json coherence —
// over git and filesystem state, and reports a determined Report rather than erroring on anything
// short of an infra failure.
// See the error-vs-Report contract Shared Decision.

package loomengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/state"
)

// Preflight validates that the current worktree is fit for loom to run: the worktree is resolvable
// and at its root, the worktree is clean, fabric is ready and in sync, and _lyx/status.json
// is coherent and fresh.
//
// Callers MUST NOT invoke Preflight except when the task is at the fresh/preflight stage.
// Invoking it on an already-advanced task (non-empty history, set start_sha) is a caller error that
// will be reported as a half-finished precondition failure.
//
// Returns (Report{OK:true}, nil) when every precondition is met.
// Returns (Report{OK:false, Failures}, nil) when one or more preconditions are unmet — a normal,
// expected outcome, not an error.
// Returns (Report{}, err) when Preflight could not determine an answer at all — the caller must
// escalate, not treat this as "not ready".
func Preflight() (Report, error) {
	// Resolve cwd via lyxcwd.Getwd(), the only permitted raw-cwd read
	// outside cmd/lyx/main.go (per the Cwd Resolution Invariant).
	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return Report{}, err
	}

	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		// ErrNotAGitRepo is a determined verdict (check 1: not inside a git
		// repository at all), not an infra failure — short-circuit with a single
		// geometry failure rather than escalating.
		if errors.Is(err, lyxcwd.ErrNotAGitRepo) {
			return Report{
				OK:       false,
				Failures: []Failure{{Check: CheckGeometry, Reason: "not inside a git repository"}},
			}, nil
		}
		// Any other Resolve error (e.g. the git subprocess itself failed to
		// spawn) is a genuine "couldn't determine" infra failure.
		return Report{}, err
	}

	return checkResolved(l)
}

// checkResolved runs checks 1b–4 against an already-resolved Layout, allowing
// tests to exercise preconditions in isolation.
func checkResolved(l *lyxcwd.Location) (Report, error) {
	// Check 1b: geometry sanity. A PrimeName resolution failure (List found no
	// main-worktree entry, or the git subprocess itself failed) is treated the
	// same as "not a git repo" for Preflight's purposes, since there is no
	// coherent worktree to check — never a hard error, preserving Preflight's
	// report-not-error contract.
	if _, err := fabricengine.PrimeName(l); err != nil {
		return Report{
			OK:       false,
			Failures: []Failure{{Check: CheckGeometry, Reason: "no main worktree resolved"}},
		}, nil
	}
	// At-worktree-root: Preflight only validates a worktree from its own root,
	// since checks 2-4 all read state anchored at WorktreeRoot. A subdirectory
	// invocation is reported distinctly (and short-circuits) rather than
	// silently validating the wrong scope.
	if l.AnchorRel != "." {
		return Report{
			OK: false,
			Failures: []Failure{
				{Check: CheckWorktreeRoot, Reason: fmt.Sprintf("invoked from subdirectory %q, not the worktree root", l.AnchorRel)},
			},
		}, nil
	}

	var report Report

	// Check 2: worktree pair cleanliness. Collected, not short-circuited — a
	// dirty code or state side does not prevent the remaining checks from
	// also reporting.
	clean, reason, err := fabricengine.Clean(l)
	if err != nil {
		return Report{}, err
	}
	if !clean {
		report.addFailure(CheckWorktreeClean, reason)
	}

	// Check 3: fabric readiness and sync. check3BlocksSeed tracks whether
	// check 3 failed in a way that also makes the seed file unreadable
	// through no fault of its own (fabric not ready, or a broken junction) —
	// check 4 gates its classification on this, per strict-read-mechanism.
	check3BlocksSeed := false
	ready, err := fabricengine.Ready(l)
	if err != nil {
		return Report{}, err
	}
	if !ready {
		report.addFailure(CheckFabricReady, "fabric not ready")
		check3BlocksSeed = true
	} else {
		ok, reason, err := fabricengine.Healthy(l)
		if err != nil {
			return Report{}, err
		}
		if !ok {
			// Healthy's typed Cause replaces the old substring match: a
			// branch mismatch classifies as CheckFabricSync, and every other
			// cause classifies as CheckJunction and also blocks the seed
			// read, since each of those causes means the junction that would
			// carry _lyx/status.json is itself broken.
			var check CheckID
			switch reason.Cause {
			case fabricengine.CauseBranchMismatch:
				check = CheckFabricSync
			default:
				check = CheckJunction
				check3BlocksSeed = true
			}
			report.addFailure(check, reason.Detail)
		}
	}

	// Check 4: seed presence, readability, and coherence.
	if _, err := os.Stat(LoomStatusFile(l)); err != nil {
		switch {
		case check3BlocksSeed:
			// The seed is unreadable as a downstream consequence of check 3's
			// failure (fabric not ready, or broken junction), not because it
			// is genuinely absent — report it as unreadable, never missing,
			// so an operator fixes fabric first rather than chasing a
			// phantom missing-seed report.
			report.addFailure(CheckSeedUnreadable, "unreadable, see check 3")
		case os.IsNotExist(err):
			report.addFailure(CheckSeedMissing, "status.json does not exist")
		default:
			report.addFailure(CheckSeedUnreadable, err.Error())
		}
	} else {
		// LoomStatusLock now lives under .lyx rather than beside LoomStatusFile
		// under _lyx, so its parent directory is no longer guaranteed to exist
		// by the time the guarding os.Stat(LoomStatusFile(l)) above succeeds.
		// state.ReadJSONStrict does not create missing parent directories (see
		// its own doc comment), and neither does internal/lock's
		// AcquireReadLock/AcquireWriteLock -- gofrs/flock opens the lock file
		// with O_CREATE but never creates parents, the same gap
		// internal/reedengine/lock.go already works around with its own
		// MkdirAll-before-lock. Without this MkdirAll, Preflight would
		// escalate a missing-.lyx worktree to a hard infra error instead of
		// honouring its report-not-error contract.
		if err := os.MkdirAll(filepath.Dir(LoomStatusLock(l)), 0o755); err != nil {
			return Report{}, err
		}
		s, found, rerr := state.ReadJSONStrict[Status](LoomStatusFile(l), LoomStatusLock(l))
		switch {
		case rerr != nil:
			// A decode failure (malformed JSON or an unknown field) is a
			// determined incoherent-seed verdict; anything else (a raw read
			// failure, a lock-acquire failure) is an infra failure to escalate.
			if errors.Is(rerr, state.ErrDecode) {
				report.addFailure(CheckSeedIncoherent, rerr.Error())
			} else {
				return Report{}, rerr
			}
		case !found:
			// The stat above just succeeded, so ReadJSONStrict returning
			// not-found (its (zero,false,nil) not-exist path) means the file
			// vanished between the two calls — a TOCTOU race, not a determined
			// verdict. Synthesize a non-nil error so this never masquerades as
			// Report{}, nil.
			return Report{}, fmt.Errorf("loomengine: seed vanished between stat and read: %s", LoomStatusFile(l))
		default:
			for _, f := range checkCoherence(s) {
				report.addFailure(f.Check, f.Reason)
			}
		}
	}

	report.OK = len(report.Failures) == 0
	return report, nil
}
