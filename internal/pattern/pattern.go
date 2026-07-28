// pattern.go implements the PATTERN active check and the three role-specific
// directive constants Directive selects between. See doc.go for the
// package-level rationale.

package pattern

import (
	"os"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// Role identifies which agent-facing directive variant Directive should
// render. The zero Role is deliberately not one of the three named
// constants below; Directive documents its behaviour for that case (the
// empty string) rather than leaving it to fall through undocumented.
type Role int

// The three directive variants Directive knows how to render, one per agent
// shape in the stack. See doc.go's "Why three roles, not one" section for
// why each variant is worded the way it is.
const (
	// RoleImplementer selects the pre-edit checklist used by any agent that
	// edits code: builder's implementer, webster's fork, and loom's plan
	// producer.
	RoleImplementer Role = iota + 1
	// RoleReviewFix selects the combined review+fix variant used by the
	// burler round, the one reviewing template in the set — it also fixes,
	// so a pure reviewer-only variant would have no user.
	RoleReviewFix
	// RoleOrchestrator selects the forking-only variant used by webster's
	// Master session, which never edits code itself.
	RoleOrchestrator
)

// implementerDirective is RoleImplementer's directive text. Every directive
// constant in this file carries its own "##" heading inline so an inactive
// render leaves no orphan heading behind, is phrased as an imperative
// checklist rather than a single sentence, and names the literal relative
// pointer "_pattern/PATTERN.md" rather than an interpolated absolute path.
const implementerDirective = `## Constraints — do this before you write any code

- **STOP.** Read _pattern/PATTERN.md in full before editing a single file.
- Read every detail doc under _pattern/ that PATTERN.md points to and that touches what you are about to change.
- These constraints are BINDING: a change that violates one is wrong even if the verify command passes.
- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
`

// reviewFixDirective is RoleReviewFix's directive text, covering both of the
// burler round's phases (A-review, B-fix) in the round's own order.
const reviewFixDirective = `## Constraints — do this before you judge or change anything

- Read _pattern/PATTERN.md in full before forming any judgment.
- Read every detail doc under _pattern/ that PATTERN.md points to and that touches what you are about to judge or change.
- In part A, every violation of a listed constraint is a BLOCKING finding: record it no matter how small it looks, and never wave it through because the code works or the tests pass.
- In part B, the fix must not introduce a violation of its own: a fix that trades one finding for a constraint breach is not a fix.
- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
`

// orchestratorDirective is RoleOrchestrator's directive text, worded for
// forking rather than editing since webster's Master never edits code
// itself.
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

// Directive reports whether PATTERN is active for the worktree l resolves
// to and, if so, returns the role's directive text to inject into that
// agent's prompt.
//
// It returns the empty string in three documented cases, each a deliberate
// no-op rather than a panic or an error — because a slip in any one of them
// would otherwise take down prompt assembly in every one of the five agent
// paths that call Directive:
//
//   - l is nil (several Deps structs are assembled field-by-field by CLI
//     callers that could leave Layout unset).
//   - PATTERN is inactive for l (see isActive).
//   - role is not one of RoleImplementer, RoleReviewFix, or
//     RoleOrchestrator — including the zero Role.
//
// See doc.go for the full active-check rule and the rationale behind the
// three role variants.
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

// isActive reports whether PATTERN is active for the worktree l resolves
// to. The rule is: an absent PatternFileHere() means inactive, and every
// other stat outcome means active — with one exception, a directory in
// PATTERN.md's place, which is also inactive because it is not a readable
// index.
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
