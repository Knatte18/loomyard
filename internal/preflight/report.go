// report.go defines the CheckID/Failure/Report result types a composing orchestrator uses to
// communicate a determined verdict — which precondition failed, and why — to a caller, distinct
// from the error return that signals an undetermined infra failure.

package preflight

// CheckID names one of the closed set of preconditions a composing orchestrator validates.
type CheckID string

// The tier-1/tier-2 checks this package can report a failure against, per report-shape.
// Geometry is tier 1 (is there a resolvable worktree with a main worktree); worktree-clean is
// tier 2's cleanliness check; fabric-ready/fabric-sync/junction are tier 2's fabric readiness and
// sync checks.
// There is deliberately no at-the-anchor check: the cwd gate in lyxcwd.Resolve already guarantees
// the property, so a repo anchored at a subpath is a legal geometry rather than a misinvocation.
const (
	// CheckGeometry fails when the cwd is not inside a git repository,
	// or the resolved Location has no main worktree.
	CheckGeometry CheckID = "geometry"
	// CheckWorktreeClean fails when the worktree has any dirty (tracked or untracked) paths.
	CheckWorktreeClean CheckID = "worktree-clean"
	// CheckFabricReady fails when fabric is not usable in this worktree.
	CheckFabricReady CheckID = "fabric-ready"
	// CheckFabricSync fails when fabric is out of sync with its expected branch.
	CheckFabricSync CheckID = "fabric-sync"
	// CheckJunction fails when a wired junction is missing or points somewhere other than its
	// correct target.
	CheckJunction CheckID = "junction"
)

// Failure is one determined precondition violation: which check failed and why.
type Failure struct {
	Check  CheckID
	Reason string
}

// Report is a composing orchestrator's determined verdict: OK reports whether every precondition
// passed, and Failures lists every violation found.
// The invariant OK == (len(Failures) == 0) always holds for a Report returned with a nil error.
type Report struct {
	OK       bool
	Failures []Failure
}

// AddFailure appends a Failure to r.Failures and keeps r.OK consistent with the Failures list.
// It is exported so a composing orchestrator (e.g. loomengine, which appends its own check-4
// failures onto a Report this package started) can append from another package.
func (r *Report) AddFailure(check CheckID, reason string) {
	r.Failures = append(r.Failures, Failure{Check: check, Reason: reason})
	r.OK = false
}

// Has reports whether any entry in r.Failures carries check.
// This is how a composing orchestrator derives its own control-flow signals — e.g. whether to
// short-circuit before running a later check — from the shared Report, rather than from bespoke
// fields riding along on the type.
func (r Report) Has(check CheckID) bool {
	for _, f := range r.Failures {
		if f.Check == check {
			return true
		}
	}
	return false
}
