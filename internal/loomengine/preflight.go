// preflight.go implements Preflight, the orchestrator that composes internal/preflight's
// orchestrator-agnostic tier-1/tier-2 checks — worktree geometry, worktree cleanliness, fabric
// readiness/sync — with loom's own check 4, _lyx/loom/status.json coherence, over git and
// filesystem state, reporting a determined Report rather than erroring on anything short of an
// infra failure.
// See internal/preflight/doc.go for the report-not-error contract this file preserves rather than
// restates.

package loomengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// Preflight validates that the worktree at cwd is fit for loom to run: the worktree is resolvable
// and at its root, the worktree is clean, fabric is ready and in sync, and _lyx/loom/status.json
// is coherent and fresh.
//
// cwd is the caller-resolved seam cwd — Preflight itself reads no process or context cwd, so a
// caller under RunCLIIn passes the injected cwd through unchanged.
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
func Preflight(cwd string) (Report, error) {
	report, l, err := preflight.Check(cwd)
	if err != nil {
		return Report{}, err
	}
	if report.Has(CheckGeometry) {
		// preflight.Check already short-circuited with a single geometry
		// failure (not a git repository, or no main worktree resolved) —
		// there is no coherent worktree to run check 4 against.
		return report, nil
	}

	// Proceed to check 4 against the Location preflight.Check handed back,
	// never re-resolving and never re-running the tier-1/tier-2 checks
	// preflight.Check already ran.
	return runCheck4(report, l)
}

// checkResolved runs checks 1b–4 against an already-resolved Location, allowing tests to exercise
// preconditions in isolation.
// It keeps its unexported name and signature so internal/loomengine/export_test.go's
// CheckResolvedForTest = checkResolved re-export needs no edit.
func checkResolved(l *lyxcwd.Location) (Report, error) {
	report, err := preflight.CheckResolved(l)
	if err != nil {
		return Report{}, err
	}
	if report.Has(CheckGeometry) {
		// Same short-circuit as Preflight, for callers driving checkResolved
		// directly.
		return report, nil
	}

	return runCheck4(report, l)
}

// runCheck4 appends check 4 (seed presence, readability, and coherence) onto report, which already
// carries checks 1b–3's verdict from either preflight.Check or preflight.CheckResolved.
func runCheck4(report Report, l *lyxcwd.Location) (Report, error) {
	// check3BlocksSeed derives from the tier-1/tier-2 report rather than being
	// tracked as a local alongside the tier-1/tier-2 checks themselves: fabric
	// not ready (CheckFabricReady) and every non-branch-mismatch Healthy cause
	// (CheckJunction) are exactly the two conditions that make the seed file
	// unreadable through no fault of its own, since each means the junction
	// that would carry _lyx/loom/status.json is itself unusable or broken.
	check3BlocksSeed := report.Has(CheckFabricReady) || report.Has(CheckJunction)

	// Check 4: seed presence, readability, and coherence.
	if _, err := os.Stat(LoomStatusFile(l)); err != nil {
		switch {
		case check3BlocksSeed:
			// The seed is unreadable as a downstream consequence of check 3's
			// failure (fabric not ready, or broken junction), not because it
			// is genuinely absent — report it as unreadable, never missing,
			// so an operator fixes fabric first rather than chasing a
			// phantom missing-seed report.
			report.AddFailure(CheckSeedUnreadable, "unreadable, see check 3")
		case os.IsNotExist(err):
			report.AddFailure(CheckSeedMissing, "status.json does not exist")
		default:
			report.AddFailure(CheckSeedUnreadable, err.Error())
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
		shed, found, rerr := state.ReadJSONStrict[shedengine.Status](LoomStatusFile(l), LoomStatusLock(l))
		switch {
		case rerr != nil:
			// A decode failure (malformed JSON or an unknown field) is a
			// determined incoherent-seed verdict; anything else (a raw read
			// failure, a lock-acquire failure) is an infra failure to escalate.
			if errors.Is(rerr, state.ErrDecode) {
				report.AddFailure(CheckSeedIncoherent, rerr.Error())
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
			// An absent or null Product decodes to the zero Status, whose empty
			// Slug/Parent the mandatory-field rules below then report -- no
			// special-casing needed. A Product that fails to unmarshal is
			// itself a determined CheckSeedIncoherent verdict, not an infra
			// error: an external writer wrote something loom cannot read as
			// its own product shape.
			var product Status
			var uerr error
			if len(shed.Product) > 0 {
				uerr = json.Unmarshal(shed.Product, &product)
			}
			if uerr != nil {
				report.AddFailure(CheckSeedIncoherent, fmt.Sprintf("product does not decode as loom's status shape: %s", uerr.Error()))
			} else {
				// These two literals are transitional: batch 4 deletes this function
				// outright, and internal/loomshed is what tells the real names to
				// CheckSeed.
				for _, f := range checkCoherence(shed, product, "Preflight", []string{"Preflight"}) {
					report.AddFailure(f.Check, f.Reason)
				}
			}
		}
	}

	report.OK = len(report.Failures) == 0
	return report, nil
}
