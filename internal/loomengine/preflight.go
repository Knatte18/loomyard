// preflight.go implements Preflight, the orchestrator that runs loom's four preconditions —
// worktree geometry, host cleanliness, weft pairing/sync, and _lyx/status.json coherence — over git
// and filesystem state, and reports a determined Report rather than erroring on anything short of
// an infra failure.
// See the error-vs-Report contract Shared Decision.

package loomengine

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/state"
)

// Preflight validates that the current worktree is fit for loom to run: the worktree is resolvable
// and at its root, the host is clean, the weft pairing is present and in sync, and _lyx/status.json
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
	// dirty host or weft does not prevent the remaining checks from also
	// reporting.
	clean, reason, err := fabricengine.Clean(l)
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
	if _, err := os.Stat(fabricengine.WeftWorktree(l)); err != nil {
		if !os.IsNotExist(err) {
			return Report{}, err
		}
		report.addFailure(CheckWeftPairing, "weft not paired")
		check3BlocksSeed = true
	} else {
		ok, reason, err := fabricengine.Healthy(l)
		if err != nil {
			return Report{}, err
		}
		if !ok {
			// Healthy's junction reasons are a consumed string format: all
			// three now read "host <name> junction …" or "host <name> is not a
			// junction" (fabricengine's junction-name parameterisation), so a
			// prefix match on "junction" no longer catches any of them — only
			// a substring match does. Any future reword of those reasons must
			// keep the substring "junction" in them, or this classification
			// silently reverts to CheckWeftSync.
			//
			// Order matters: the "host on " case is checked first so the
			// branch-mismatch reason ("host on <a>, weft on <b> (want <c>)")
			// is classified before the broader Contains check runs — relying
			// on that ordering is the safer arrangement, even though the
			// branch-mismatch reason's content alone never contains
			// "junction" either way.
			var check CheckID
			switch {
			case strings.HasPrefix(reason, "host on "):
				check = CheckWeftSync
			case strings.Contains(reason, "junction"):
				check = CheckJunction
				check3BlocksSeed = true
			default:
				check = CheckWeftSync
			}
			report.addFailure(check, reason)
		}
	}

	// Check 4: seed presence, readability, and coherence.
	if _, err := os.Stat(LoomStatusFile(l)); err != nil {
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
