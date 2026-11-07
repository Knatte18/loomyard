// report.go defines the CheckID, Failure and Report types for a determined verdict.
// A composing orchestrator uses them to tell its caller which precondition failed, and why.
// They are distinct from the error return, which signals an undetermined infra failure.

package preflight

// CheckID names one of the closed set of preconditions a composing orchestrator validates.
type CheckID string

// The tier-1 and tier-2 checks this package can report a failure against.
// Geometry is tier 1: a resolvable worktree with a main worktree.
// Worktree-clean, fabric-ready, fabric-sync and junction are tier 2.
// There is no at-the-anchor check:
// the cwd gate in lymcwd.Resolve already guarantees that cwd is at the anchor,
// so a repo anchored at a subpath is legal geometry, not a misinvocation.
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
	// CheckJunction fails when a wired junction is missing or points somewhere other than its correct target.
	CheckJunction CheckID = "junction"
)

// Failure is one determined precondition violation: which check failed and why.
type Failure struct {
	Check  CheckID
	Reason string
}

// Report is a composing orchestrator's determined verdict:
// OK reports whether every precondition passed, and Failures lists every violation found.
// The invariant OK == (len(Failures) == 0) always holds for a Report returned with a nil error.
type Report struct {
	OK       bool
	Failures []Failure
}

// AddFailure appends a Failure to r.Failures and keeps r.OK consistent with the Failures list.
// It is exported so a composing orchestrator in another package can append failures to a Report this package started.
func (r *Report) AddFailure(check CheckID, reason string) {
	r.Failures = append(r.Failures, Failure{Check: check, Reason: reason})
	r.OK = false
}

// Has reports whether any entry in r.Failures carries check.
// A composing orchestrator uses it for control-flow signals, such as whether to short-circuit a later check.
// Deriving them from the Report keeps the type free of bespoke fields.
func (r Report) Has(check CheckID) bool {
	for _, f := range r.Failures {
		if f.Check == check {
			return true
		}
	}
	return false
}
