// doc.go carries the package godoc for pattern:
// the active check, why the overview is inlined while background files stay pointers, why the roles differ,
// and the stencil read path Directive uses to produce that directive text.

// Package pattern answers one question for every code-touching lyx agent —
// is PATTERN active in this worktree, and what should the agent be told? —
// and returns the role-appropriate directive text, read from a stencil
// file and carrying the PATTERN overview, to inject into that agent's prompt.
//
// # The active check
//
// PATTERN lives at the repository's worktree root: `PATTERN.md` is the overview,
// and background files sit under `pattern/` beside it.
// Directive and File take that root, never the anchor path,
// because the overview sits next to `go.mod` even when the anchor is a subdirectory.
// PATTERN is active iff the file at File(worktreeRoot) holds anything but whitespace.
// An absent file, a directory in its place and a whitespace-only file are all inactive.
// Any other content, however malformed, is active and inlined verbatim:
// format violations are the format checker's job, not this package's.
// A stat error that is not "not exist", or a read error on an existing file,
// is an error rather than an inactive PATTERN,
// since silently disabling the constraints is worse than a visible failure.
//
// # Why the roles differ
//
// Directive's Role parameter selects the directive-text variant for one agent shape,
// because each shape needs its constraint text worded for what that agent actually does.
// RoleImplementer is worded as a pre-edit checklist, because its agents edit code.
// RoleReviewFix covers both phases of the one review+fix round in the round's own order,
// since a pure reviewer variant would have no user: the burler template that reviews also fixes.
// RoleOrchestrator is worded for forking rather than editing,
// because an implementer-worded instruction would ask webster's Master to do something its own prompt says it never does.
// RoleDesigner is worded for checking decisions against the entries before any plan exists,
// because its agent settles a design rather than changing code.
// RoleJudge is worded for weighing findings the review already made,
// because the Bouncer judge never opens the artifacts and must not downgrade a finding that cites an entry.
//
// # Why the overview is inlined and the background files are not
//
// Each directive stencil carries one required `{{.pattern_overview}}` marker,
// which Directive fills with the overview's content.
// Inlining the overview means every agent is guaranteed to see the rules,
// saves it a read turn, and keeps the prompt prefix cache-stable.
// The background files stay a fixed relative `pattern/` pointer in the stencil's own body,
// never an interpolated absolute path built from the caller-supplied root:
// an absolute path would vary per worktree,
// which would make the fixed directive strings unable to be compared for equality (or matched by substring) across worktrees
// the way this package's own tests, and any consumer's tests, need to.
//
// # The stencil read path
//
// Directive is told a stencilsDir, reads the role's stencil through stencilstore.Read,
// strips the leading banner with stencil.StripLeadingComment, fills the overview marker with stencil.Fill,
// and returns an error rather than an empty string when an active PATTERN's stencil cannot be read or filled.
// The read is lazy: no stencil read is attempted on an empty root, an inactive PATTERN, or an unknown role.
//
// # The PATTERN format
//
// Another repository starts a PATTERN by writing `PATTERN.md` at its root; there is no init verb.
// The overview is a thin list of entry lines, grouped under `##` topic headings.
// An entry line is one line:
//
//	- `PATTERN-<name>` — <when it applies>: <what it requires>. (test) — [background](pattern/PATTERN-<name>.md)
//
// The name matches `PATTERN-[a-z0-9-]+` and is unique in the overview.
// `(test)` marks a rule a test enforces, and is left out for a rule held by review alone.
// The background link is optional.
// When present it points at `pattern/<the entry's own name>.md`, and that file exists.
// Every file under `pattern/` is named by exactly one entry.
//
// Check enforces the shape on an fs.FS holding the root, and the real file is checked under `go test`.
// It caps the overview at MaxOverviewBytes and an entry line at MaxEntryLineChars,
// and reports an entry that continues onto a second line, an empty overview and an overview with no entry.
// PATTERN text states rules for the code only:
// it never says how an agent, a run, a stencil or a workflow uses it.
// Nothing in the render path calls Check, so Directive inlines a malformed overview verbatim.
package pattern
