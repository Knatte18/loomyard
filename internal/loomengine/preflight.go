// preflight.go implements Preflight, the orchestrator that runs loom's four
// preconditions — worktree geometry, host cleanliness, weft pairing/sync, and
// _lyx/status.json coherence — over git and filesystem state, and reports a
// determined Report rather than erroring on anything short of an infra
// failure. See the error-vs-Report contract Shared Decision.

package loomengine

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/warpengine"
)

// Preflight validates that the current worktree is fit for loom to begin
// running a task: the worktree is resolvable and at its root, the host is
// clean, the weft pairing is present and in sync, and _lyx/status.json is a
// coherent, fresh seed. It owns cwd resolution end-to-end (Getwd + Resolve)
// so it can be called with no arguments from anywhere in a worktree; a
// caller outside this package that already holds a resolved Layout (e.g. for
// isolated testing) cannot reach checkResolved directly, since that helper is
// unexported by design — Preflight is the only entry point from outside the
// package.
//
// Callers MUST NOT invoke Preflight except when the task is at the
// fresh/preflight stage. Invoking it on an already-advanced task (non-empty
// history, set start_sha, …) is a caller error that will be reported as a
// half-finished precondition failure, not diagnosed as misuse, because
// Preflight is a stateless validator.
//
// Returns (Report{OK:true}, nil) when every precondition is met.
// Returns (Report{OK:false, Failures}, nil) when one or more preconditions
// are determined to be unmet — a normal, expected outcome, not an error.
// Returns (Report{}, err) when Preflight could not determine an answer at
// all (a git spawn failure, an unexpected I/O error, or similar infra
// failure) — the caller must escalate, not treat this as "not ready".
func Preflight() (Report, error) {
	// Resolve cwd via hubgeometry.Getwd(), the only permitted raw-cwd read
	// outside cmd/lyx/main.go (per the Hub Geometry Invariant).
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return Report{}, err
	}

	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		// ErrNotAGitRepo is a determined verdict (check 1: not inside a git
		// repository at all), not an infra failure — short-circuit with a single
		// geometry failure rather than escalating.
		if errors.Is(err, hubgeometry.ErrNotAGitRepo) {
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

// checkResolved runs checks 1b–4 against an already-resolved Layout. It is
// unexported and takes an injected Layout so integration tests can drive
// every precondition scenario in isolation, without going through Preflight's
// process-cwd resolution.
func checkResolved(l *hubgeometry.Layout) (Report, error) {
	// Check 1b: geometry sanity. Resolve can succeed with no Prime when List
	// found no main-worktree entry — treat that the same as "not a git repo"
	// for Preflight's purposes, since there is no coherent worktree to check.
	if l.Prime == "" {
		return Report{
			OK:       false,
			Failures: []Failure{{Check: CheckGeometry, Reason: "no main worktree resolved"}},
		}, nil
	}
	// At-worktree-root: Preflight only validates a worktree from its own root,
	// since checks 2-4 all read state anchored at WorktreeRoot. A subdirectory
	// invocation is reported distinctly (and short-circuits) rather than
	// silently validating the wrong scope.
	if l.RelPath != "." {
		return Report{
			OK: false,
			Failures: []Failure{
				{Check: CheckWorktreeRoot, Reason: fmt.Sprintf("invoked from subdirectory %q, not the worktree root", l.RelPath)},
			},
		}, nil
	}

	var report Report

	// Check 2: host worktree cleanliness. Collected, not short-circuited — a
	// dirty host does not prevent the remaining checks from also reporting.
	clean, reason, err := warpengine.HostClean(l)
	if err != nil {
		return Report{}, err
	}
	if !clean {
		report.addFailure(CheckWorktreeClean, reason)
	}

	// Check 3: weft pairing and sync. check3BlocksSeed tracks whether check 3
	// failed in a way that also makes the seed file unreadable through no
	// fault of its own (missing weft worktree, or a broken junction) — check 4
	// gates its classification on this, per strict-read-mechanism.
	check3BlocksSeed := false
	if _, err := os.Stat(l.WeftWorktree()); err != nil {
		if !os.IsNotExist(err) {
			return Report{}, err
		}
		report.addFailure(CheckWeftPairing, "weft not paired")
		check3BlocksSeed = true
	} else {
		ok, reason, err := warpengine.PairInSync(l)
		if err != nil {
			return Report{}, err
		}
		if !ok {
			var check CheckID
			switch {
			case strings.HasPrefix(reason, "host on "):
				check = CheckWeftSync
			case strings.HasPrefix(reason, "junction"):
				check = CheckJunction
				check3BlocksSeed = true
			default:
				check = CheckWeftSync
			}
			report.addFailure(check, reason)
		}
	}

	// Check 4: seed presence, readability, and coherence.
	if _, err := os.Stat(l.LoomStatusFile()); err != nil {
		switch {
		case check3BlocksSeed:
			// The seed is unreadable as a downstream consequence of check 3's
			// failure (missing weft, or broken junction), not because it is
			// genuinely absent — report it as unreadable, never missing, so an
			// operator fixes the pairing first rather than chasing a phantom
			// missing-seed report.
			report.addFailure(CheckSeedUnreadable, "unreadable, see check 3")
		case os.IsNotExist(err):
			report.addFailure(CheckSeedMissing, "status.json does not exist")
		default:
			report.addFailure(CheckSeedUnreadable, err.Error())
		}
	} else {
		s, found, rerr := state.ReadJSONStrict[Status](l.LoomStatusFile(), l.LoomStatusLock())
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
			return Report{}, fmt.Errorf("loomengine: seed vanished between stat and read: %s", l.LoomStatusFile())
		default:
			for _, f := range checkCoherence(s) {
				report.addFailure(f.Check, f.Reason)
			}
		}
	}

	report.OK = len(report.Failures) == 0
	return report, nil
}
