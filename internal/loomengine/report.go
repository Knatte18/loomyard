// report.go re-exposes internal/preflight's CheckID/Failure/Report result types as aliases, and
// declares the four loom-specific check-ID constants — seed-missing, seed-unreadable,
// seed-incoherent, half-finished — that CheckSeed reports against.

package loomengine

import "github.com/Knatte18/loomyard/internal/preflight"

// CheckID names one of the closed set of preconditions loomengine validates.
// It is a type alias for preflight.CheckID, not a new named type, so loomengine.Report and
// preflight.Report stay the identical type across the package boundary.
type CheckID = preflight.CheckID

// Failure is one determined precondition violation: which check failed and why.
// It is a type alias for preflight.Failure — see CheckID's comment for why.
type Failure = preflight.Failure

// Report is CheckSeed's determined verdict: OK reports whether every precondition passed,
// and Failures lists every violation found.
// The invariant OK == (len(Failures) == 0) always holds for a Report returned with a nil error.
// It is a type alias for preflight.Report — see CheckID's comment for why.
type Report = preflight.Report

// The four loom-specific checks, declared here because internal/preflight has no notion
// of .lyx/loom/status.json.
const (
	// CheckSeedMissing fails when .lyx/loom/status.json does not exist.
	// Unreachable through Shed: step 1's own read gate already hard-errors on the same not-found
	// verdict — see CheckSeed's doc comment for the pre-emption rule stated in full.
	CheckSeedMissing CheckID = "seed-missing"
	// CheckSeedUnreadable fails when .lyx/loom/status.json cannot be stat'd or read for a reason
	// other than not-existing.
	CheckSeedUnreadable CheckID = "seed-unreadable"
	// CheckSeedIncoherent fails when .lyx/loom/status.json exists and decodes but violates the coherence
	// validator's rules (see checkCoherence).
	// Unreachable through Shed: two of its three producing branches are pre-empted by step 1's own
	// read gate — see CheckSeed's doc comment for the pre-emption rule stated in full.
	CheckSeedIncoherent CheckID = "seed-incoherent"
	// CheckHalfFinished fails when .lyx/loom/status.json is otherwise coherent but its fresh-start
	// invariants are violated — the task has already advanced past the point loom's own seed row is
	// meant to gate.
	CheckHalfFinished CheckID = "half-finished"
)
