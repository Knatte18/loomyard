// report.go re-exposes internal/preflight's CheckID/Failure/Report result types as aliases, and
// declares the four loom-specific check-ID constants — seed-missing, seed-unreadable,
// seed-incoherent, half-finished — that check 4 (loom's own precondition, layered over
// internal/preflight's tier-1/tier-2 checks) reports against.
// Preflight uses these types to communicate its determined verdict — which of the four preconditions
// failed, and why — to a caller, distinct from the error return that signals an undetermined infra
// failure.

package loomengine

import "github.com/Knatte18/loomyard/internal/preflight"

// CheckID names one of the closed set of preconditions Preflight validates.
// It is a type alias for preflight.CheckID, not a new named type, so loomengine.Report and
// preflight.Report stay the identical type across the package boundary.
type CheckID = preflight.CheckID

// Failure is one determined precondition violation: which check failed and why.
// It is a type alias for preflight.Failure — see CheckID's comment for why.
type Failure = preflight.Failure

// Report is Preflight's determined verdict: OK reports whether every precondition passed,
// and Failures lists every violation found.
// The invariant OK == (len(Failures) == 0) always holds for a Report returned with a nil error.
// It is a type alias for preflight.Report — see CheckID's comment for why.
type Report = preflight.Report

// The five tier-1/tier-2 check-ID constants, re-exposed as const aliases of the ones
// internal/preflight declares, so existing callers of these names keep compiling unchanged.
const (
	// CheckGeometry fails when the cwd is not inside a git repository,
	// or the resolved Layout has no Prime (main worktree).
	CheckGeometry = preflight.CheckGeometry
	// CheckWorktreeClean fails when the worktree has any dirty (tracked or untracked) paths.
	CheckWorktreeClean = preflight.CheckWorktreeClean
	// CheckFabricReady fails when fabric is not usable in this worktree.
	CheckFabricReady = preflight.CheckFabricReady
	// CheckFabricSync fails when fabric is out of sync with its expected branch.
	CheckFabricSync = preflight.CheckFabricSync
	// CheckJunction fails when the _lyx junction is missing or points somewhere other than its
	// correct target.
	CheckJunction = preflight.CheckJunction
)

// The four loom-specific checks (check 4), declared here because internal/preflight has no notion
// of _lyx/loom/status.json.
const (
	// CheckSeedMissing fails when _lyx/loom/status.json does not exist and fabric is otherwise ready and
	// healthy.
	CheckSeedMissing CheckID = "seed-missing"
	// CheckSeedUnreadable fails when _lyx/loom/status.json cannot be stat'd or read for a reason other than
	// not-existing,
	// or when a stat failure (including not-exist) is attributable to fabric not being ready or
	// healthy rather than a genuinely missing seed.
	CheckSeedUnreadable CheckID = "seed-unreadable"
	// CheckSeedIncoherent fails when _lyx/loom/status.json exists and decodes but violates the coherence
	// validator's rules (see checkCoherence).
	CheckSeedIncoherent CheckID = "seed-incoherent"
	// CheckHalfFinished fails when _lyx/loom/status.json is otherwise coherent but its fresh-start
	// invariants are violated — the task has already advanced past the point Preflight is meant to
	// gate.
	CheckHalfFinished CheckID = "half-finished"
)
