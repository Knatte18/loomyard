// pattern.go implements the PATTERN active check and the three role-specific
// directive constants Directive selects between. See doc.go for the
// package-level rationale.

package pattern

import (
	"os"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// Role identifies which agent-facing directive variant Directive should render.
type Role int

// The three directive variants Directive knows how to render, one per agent shape.
const (
	// RoleImplementer selects the pre-edit checklist for any agent that edits code.
	RoleImplementer Role = iota + 1
	// RoleReviewFix selects the combined review+fix variant for the burler round.
	RoleReviewFix
	// RoleOrchestrator selects the forking-only variant for webster's Master session.
	RoleOrchestrator
)

// implementerDirective is RoleImplementer's directive text, phrased as an imperative checklist with "_pattern/PATTERN.md" as a literal relative pointer (not interpolated).
const implementerDirective = `## Constraints — do this before you write any code

- **STOP.** Read _pattern/PATTERN.md in full before editing a single file.
- Read every detail doc under _pattern/ that PATTERN.md points to and that touches what you are about to change.
- These constraints are BINDING: a change that violates one is wrong even if the verify command passes.
- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
`

// reviewFixDirective is RoleReviewFix's directive text, covering both of the burler round's phases.
const reviewFixDirective = `## Constraints — do this before you judge or change anything

- Read _pattern/PATTERN.md in full before forming any judgment.
- Read every detail doc under _pattern/ that PATTERN.md points to and that touches what you are about to judge or change.
- In part A, every violation of a listed constraint is a BLOCKING finding: record it no matter how small it looks, and never wave it through because the code works or the tests pass.
- In part B, the fix must not introduce a violation of its own: a fix that trades one finding for a constraint breach is not a fix.
- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
`

// orchestratorDirective is RoleOrchestrator's directive text, worded for forking rather than editing.
const orchestratorDirective = `## Constraints — do this before you fork anything

- Read _pattern/PATTERN.md in full before forking a single implementer.
- Read every detail doc under _pattern/ that PATTERN.md points to and that touches what the forks you are about to spawn will do.
- Every fork inherits its context, so reading this once here is what puts the constraints in front of all of them; it must not be skipped on the grounds of not editing code.
- The constraints are BINDING on the forks it spawns: a batch report trading a constraint for a passing verify is a failed batch, not a success.
`

// statFile is the stat implementation isActive calls. It is a package-level
// variable — rather than a hardcoded os.Stat call — purely so this
// package's own test suite can simulate a non-"not exist" stat error (a
// permission or I/O failure) portably across platforms, without depending
// on process privilege or a POSIX-only permission trick. Production code
// never reassigns it.
var statFile = os.Stat

// Directive reports whether PATTERN is active and returns the role's directive text to inject into the agent's prompt, or empty string if inactive or role is unknown.
func Directive(l *hubgeometry.Layout, role Role) string {
	if l == nil {
		return ""
	}
	if !isActive(l) {
		return ""
	}
	switch role {
	case RoleImplementer:
		return implementerDirective
	case RoleReviewFix:
		return reviewFixDirective
	case RoleOrchestrator:
		return orchestratorDirective
	default:
		// An unknown or zero Role renders no directive; this default case
		// is what makes that behaviour defined and documented rather than
		// an unhandled fall-through.
		return ""
	}
}

// isActive reports whether PATTERN is active: an absent PatternFileHere() means inactive; a directory in its place is also inactive; otherwise active.
func isActive(l *hubgeometry.Layout) bool {
	info, err := statFile(l.PatternFileHere())
	if err != nil {
		// os.IsNotExist is the normal, common inactive case: PATTERN.md was
		// never created. Any other error — permission denied, I/O failure —
		// is treated as active per Directive's doc comment: the ambiguity
		// resolves loud, in the agent's own read of the file, rather than
		// silently disabling the constraints.
		return !os.IsNotExist(err)
	}
	// A directory named PATTERN.md is not a readable index; treat it the
	// same as absent rather than reading something that isn't the file.
	return !info.IsDir()
}
