// report.go defines the CheckID/Failure/Report result types Preflight uses to
// communicate its determined verdict — which of the four preconditions failed,
// and why — to a caller, distinct from the error return that signals an
// undetermined infra failure.

package loomengine

// CheckID names one of the closed set of preconditions Preflight validates.
// It is a machine-consumable classification a future phase machine can switch
// on, paired with a human-readable Reason string per Failure.
type CheckID string

// The closed set of checks Preflight can report a failure against, per
// report-shape. Each corresponds to one of the four checks described in
// Preflight's godoc (geometry/worktree-root fold into check 1, worktree-clean
// is check 2, weft-pairing/weft-sync/junction are check 3, and
// seed-missing/seed-unreadable/seed-incoherent/half-finished are check 4).
const (
	// CheckGeometry fails when the cwd is not inside a git repository, or the
	// resolved Layout has no Prime (main worktree).
	CheckGeometry CheckID = "geometry"
	// CheckWorktreeRoot fails when Preflight is invoked from a subdirectory of
	// the worktree rather than its root.
	CheckWorktreeRoot CheckID = "worktree-root"
	// CheckWorktreeClean fails when the host worktree has any dirty (tracked or
	// untracked) paths.
	CheckWorktreeClean CheckID = "worktree-clean"
	// CheckWeftPairing fails when the paired weft worktree does not exist.
	CheckWeftPairing CheckID = "weft-pairing"
	// CheckWeftSync fails when the host and weft worktrees are on different
	// branches.
	CheckWeftSync CheckID = "weft-sync"
	// CheckJunction fails when the host _lyx junction is missing or points
	// somewhere other than the paired weft's _lyx directory.
	CheckJunction CheckID = "junction"
	// CheckSeedMissing fails when _lyx/status.json does not exist and the weft
	// pairing/junction are otherwise healthy.
	CheckSeedMissing CheckID = "seed-missing"
	// CheckSeedUnreadable fails when _lyx/status.json cannot be stat'd or read
	// for a reason other than not-existing, or when a stat failure (including
	// not-exist) is attributable to an unhealthy weft pairing/junction rather
	// than a genuinely missing seed.
	CheckSeedUnreadable CheckID = "seed-unreadable"
	// CheckSeedIncoherent fails when _lyx/status.json exists and decodes but
	// violates the coherence validator's rules (see checkCoherence).
	CheckSeedIncoherent CheckID = "seed-incoherent"
	// CheckHalfFinished fails when _lyx/status.json is otherwise coherent but
	// its fresh-start invariants are violated — the task has already advanced
	// past the point Preflight is meant to gate.
	CheckHalfFinished CheckID = "half-finished"
)

// Failure is one determined precondition violation: which check failed
// (Check) and a human-readable explanation (Reason) an operator can read
// directly, distinct from the machine-classifiable Check.
type Failure struct {
	Check  CheckID
	Reason string
}

// Report is Preflight's determined verdict: OK reports whether every
// precondition passed, and Failures lists every violation found. Preflight
// collects rather than short-circuits (per check-ordering-and-collection),
// so Failures may contain more than one entry when multiple preconditions
// are unmet simultaneously. The invariant OK == (len(Failures) == 0) always
// holds for a Report returned with a nil error.
type Report struct {
	OK       bool
	Failures []Failure
}

// addFailure appends a Failure built from check and reason to r.Failures and
// keeps r.OK consistent (false, since a non-empty Failures list can never be
// OK). It exists so Preflight's checkResolved can build up a Report across
// several non-short-circuiting checks without hand-rolling the append+OK-flip
// pair at each call site.
func (r *Report) addFailure(check CheckID, reason string) {
	r.Failures = append(r.Failures, Failure{Check: check, Reason: reason})
	r.OK = false
}
