// doc.go carries the package godoc for pattern: the active check, why it is
// pure existence, why the three roles are what they are, and why the
// injected pointer stays a relative path.

// Package pattern answers one question for every code-touching lyx agent —
// is PATTERN active in this worktree, and what should the agent be told? —
// and returns the role-appropriate directive text to inject into that
// agent's prompt.
//
// # The active check is pure existence
//
// PATTERN is active iff `_pattern/PATTERN.md` exists, resolved via
// hubgeometry.Layout.PatternFileHere() and nothing else: this package never
// constructs the path itself (the Hub Geometry Invariant's enforcement test
// makes that impossible from batch 2 onward). Existence alone is the check —
// never a content inspection — because the `_pattern/` directory itself may
// exist without PATTERN.md (the normal inactive state; `lyx init` always
// creates the directory), and a content-inspecting check would turn a
// benign empty file into a runtime error in every one of the five agent
// paths that call Directive. Three edge cases follow from that same
// existence-only design and are each pinned by a test: an empty PATTERN.md
// is active (degenerate but harmless); PATTERN.md present as a directory is
// inactive (it is not a readable index); and a stat error that is not
// "not exist" — a permission or I/O failure — is treated as active, since
// resolving that ambiguity by silently disabling five agents' constraints is
// worse than resolving it by handing the agent the directive anyway, so it
// reads the file itself and reports a real, visible failure if it genuinely
// cannot.
//
// # Why three roles, not one
//
// Directive's Role parameter selects one of three directive-text variants,
// one per agent shape in the stack, because each shape needs its constraint
// text worded for what that agent actually does: RoleImplementer for any
// agent that edits code (builder implementer, webster fork, loom plan) is
// worded as a pre-edit checklist; RoleReviewFix for the one review+fix round
// (burler) covers both of that round's phases in the round's own order,
// since a pure reviewer variant would have no user — burler is the only
// reviewing template in the set and it also fixes; RoleOrchestrator for the
// one role that never edits code itself (webster Master) is worded for
// forking rather than editing, because an implementer-worded instruction
// would ask Master to do something its own prompt says it never does.
//
// # Why the pointer stays relative
//
// Each directive injects a pointer to `_pattern/PATTERN.md`, never the
// constraints inline, so prompt size stays constant however large PATTERN
// grows. The pointer is a literal relative string baked into the directive
// constant, never an interpolated absolute path built from a Layout field:
// an absolute path would vary per worktree, which would make the fixed
// directive strings unable to be compared for equality (or matched by
// substring) across worktrees the way this package's own tests, and any
// consumer's tests, need to.
package pattern
