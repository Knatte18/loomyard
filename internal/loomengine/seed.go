// seed.go implements CheckSeed, loom's own seed-coherence check (check 4), retargeted onto told
// absolute paths so it can run as an exported, directly-testable function rather than a private
// half of Preflight's composite call.

package loomengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// CheckSeed validates that the status file at statusPath is a coherent, fresh seed for the row
// named expectedProducer, tolerating a history entry naming any producer in toleratedProducers.
//
// statusPath and statusLockPath are told absolute paths -- CheckSeed resolves no geometry of its
// own, unlike Preflight, which composes tiers 1-2's worktree/fabric checks ahead of this one.
// expectedProducer and toleratedProducers are likewise told by the caller, never derived here,
// because they are the caller's own durable row identities: see checkCoherence's own doc comment
// for what each parameter means.
//
// Returns (Report{OK:true}, nil) when the seed is coherent and fresh.
// Returns (Report{OK:false, Failures}, nil) when one or more of the seed-coherence preconditions
// is unmet -- a normal, expected outcome, not an error.
// Returns (Report{}, err) when CheckSeed could not determine an answer at all -- the caller must
// escalate, not treat this as "not ready". See internal/preflight/doc.go's "report-not-error
// contract" section for the same three-case shape stated in full.
//
// The step-1 pre-emption rule, stated once and generally: every verdict shedengine.Run's step-1
// read gate already hard-errors or short-circuits on is unreachable when CheckSeed is driven as a
// producer row, because step 1 reads the same told path through the same
// state.ReadJSONStrict[shedengine.Status] decoder before any producer is looked up.
// Four of the verdicts below are covered by that rule, and are kept anyway:
//
//   - CheckSeedMissing: step 1's !found hard-errors, "status file %q does not exist; Shed never
//     seeds one".
//   - CheckSeedIncoherent via the state.ErrDecode branch: step 1's identical strict read errors
//     first.
//   - CheckSeedIncoherent via checkCoherence's invalid-state rule: step 1's !st.State.valid()
//     hard-errors.
//   - CheckSeedIncoherent via checkCoherence's StateDone rule: not an error but a short-circuit --
//     step 1 returns RunDone without calling any producer.
//
// All four are kept because CheckSeed is an exported function over told paths, not a private
// helper of one row -- it has a Tier-1 suite that calls it directly, and deleting verdicts from
// the closed set contracts/specs/loom-status-spec.md pins would trade a documented contract for
// nothing.
//
// The genuine stat-error branch and the TOCTOU guard below carry no unreachability claim: both
// turn on a filesystem state change between step 1's read and this function's own stat, which no
// gate can pre-empt.
func CheckSeed(statusPath, statusLockPath, expectedProducer string, toleratedProducers []string) (Report, error) {
	var report Report

	if _, err := os.Stat(statusPath); err != nil {
		switch {
		case os.IsNotExist(err):
			report.AddFailure(CheckSeedMissing, "status.json does not exist")
		default:
			report.AddFailure(CheckSeedUnreadable, err.Error())
		}
	} else {
		// state.ReadJSONStrict does not create missing parent directories (see its own doc
		// comment), and neither does internal/lock's AcquireReadLock/AcquireWriteLock --
		// gofrs/flock opens the lock file with O_CREATE but never creates parents, the same
		// gap internal/reedengine/lock.go already works around with its own
		// MkdirAll-before-lock. Without this MkdirAll, CheckSeed would escalate a
		// missing-parent worktree to a hard infra error instead of honouring its
		// report-not-error contract.
		if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
			return Report{}, err
		}
		shed, found, rerr := state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
		switch {
		case rerr != nil:
			// A decode failure (malformed JSON or an unknown field) is a determined
			// incoherent-seed verdict; anything else (a raw read failure, a
			// lock-acquire failure) is an infra failure to escalate.
			if errors.Is(rerr, state.ErrDecode) {
				report.AddFailure(CheckSeedIncoherent, rerr.Error())
			} else {
				return Report{}, rerr
			}
		case !found:
			// The stat above just succeeded, so ReadJSONStrict returning not-found (its
			// (zero,false,nil) not-exist path) means the file vanished between the two
			// calls -- a TOCTOU race, not a determined verdict. Synthesize a non-nil
			// error so this never masquerades as Report{}, nil.
			return Report{}, fmt.Errorf("loomengine: seed vanished between stat and read: %s", statusPath)
		default:
			// An absent or null Product decodes to the zero Status, whose empty
			// Slug/Parent the mandatory-field rules below then report -- no
			// special-casing needed. A Product that fails to unmarshal is itself a
			// determined CheckSeedIncoherent verdict, not an infra error: an external
			// writer wrote something loom cannot read as its own product shape.
			var product Status
			var uerr error
			if len(shed.Product) > 0 {
				uerr = json.Unmarshal(shed.Product, &product)
			}
			if uerr != nil {
				report.AddFailure(CheckSeedIncoherent, fmt.Sprintf("product does not decode as loom's status shape: %s", uerr.Error()))
			} else {
				for _, f := range checkCoherence(shed, product, expectedProducer, toleratedProducers) {
					report.AddFailure(f.Check, f.Reason)
				}
			}
		}
	}

	report.OK = len(report.Failures) == 0
	return report, nil
}
