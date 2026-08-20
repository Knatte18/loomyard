// doc.go carries the package godoc for pattern: the active check, why it is pure existence, why the
// three roles are what they are, why the injected pointer stays a relative path, and the stencil
// read path Directive uses to produce that directive text.

// Package pattern answers one question for every code-touching lyx agent —
// is PATTERN active in this worktree, and what should the agent be told? —
// and returns the role-appropriate directive text, read from a stencil
// file, to inject into that agent's prompt.
//
// # The active check is pure existence
//
// PATTERN is active iff `_lyx/PATTERN.md` exists, resolved via this
// package's own File applied to the caller-supplied anchor directory and
// nothing else: File is what constructs the path, built from
// lyxdirs.LyxDirName rather than a literal of its own.
// The Cwd Resolution Invariant's enforcement test polices the "_lyx" token
// itself, which belongs to internal/lyxdirs, not to this package;
// TestEnforcement_GeometryLiterals matches whole tokens by exact equality
// and so cannot see "_lyx/PATTERN.md" at all. Keeping File and
// PathspecFile/PathspecDir built from lyxdirs.LyxDirName is therefore a
// review obligation this package accepts, not something any test
// mechanically enforces. Existence alone is the check —
// never a content inspection — because the `_lyx/` directory always exists
// (every worktree has one), so its presence never implies PATTERN is
// active, and a content-inspecting check would turn a
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
// agent that edits code (webster fork/Master, burler review+fix, loom plan) is
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
// Each directive injects a pointer to `_lyx/PATTERN.md`, never the
// constraints inline, so prompt size stays constant however large PATTERN
// grows. The pointer is a literal relative string in the stencil file's own
// body, never an interpolated absolute path built from the caller-supplied anchor path:
// an absolute path would vary per worktree, which would make the fixed
// directive strings unable to be compared for equality (or matched by
// substring) across worktrees the way this package's own tests, and any
// consumer's tests, need to.
//
// # The stencil read path
//
// Directive is told a stencilsDir, reads the role's stencil through
// stencilstore.Read, strips the leading banner with
// stencil.StripLeadingComment because its return value is injected as a
// producer template's marker value and so never passes through
// stencil.Fill, and returns an error rather than an empty string when an
// active PATTERN's stencil cannot be read. The read is lazy: no read is
// attempted on an empty anchor path, an inactive PATTERN, or an unknown role.
//
// # PathspecFile and PathspecDir
//
// PathspecFile and PathspecDir are the git-pathspec spellings of the
// PATTERN entry point and its detail-docs directory, respectively —
// worktree-relative, forward-slashed strings for use in git plumbing
// arguments, not filesystem paths built with filepath.Join.
// internal/fabricengine is their consumer, for the PatternResidue pathspec.
package pattern
