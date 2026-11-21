// doc.go carries the package godoc for pattern:
// the active check, the directive roles, the inlined overview and the PATTERN format.

// Package pattern answers one question for every code-touching lym agent:
// is PATTERN active in this worktree, and what should the agent be told?
// It returns the role-appropriate directive text for that agent's prompt.
// The text is read from a stencil file and carries the PATTERN overview.
//
// # The active check
//
// PATTERN lives at the repository's worktree root: `PATTERN.md` is the overview,
// and background files sit under `pattern/` beside it.
// Directive and File take that root, never the anchor path.
// PATTERN is active when the file at File(worktreeRoot) holds anything but whitespace.
// An absent file, a directory in its place and a whitespace-only file are all inactive.
// Any other content, however malformed, is active and inlined verbatim:
// format violations are the format checker's job, not this package's.
// Any other stat or read error is an error rather than an inactive PATTERN,
// since silently disabling the constraints is worse than a visible failure.
//
// # Why the roles differ
//
// Directive's Role parameter selects the directive-text variant worded for what that agent shape does.
// The fix half of the burler review receives no PATTERN directive.
// RoleOrchestrator is worded for forking rather than editing,
// because an implementer-worded instruction would ask webster's Master to do what its prompt says it never does.
// RoleJudge is worded for weighing findings the review already made,
// because the Bouncer judge never opens the artifacts and must not downgrade a finding that cites an entry.
//
// # Why the overview is inlined and the background files are not
//
// Each directive stencil carries one required `{{.pattern_overview}}` marker, which Directive fills with the overview.
// It also carries one optional `{{.pattern_changes}}` marker, which Directive fills with the task's declared changes.
// A listed entry is followed and judged by its listed new text, never by the overview's current text.
// Inlining the overview guarantees every agent sees the rules and keeps the prompt prefix cache-stable.
// The background files stay a fixed relative `pattern/` pointer in the stencil's own body.
// An interpolated absolute path would vary per worktree and defeat equality checks across worktrees.
//
// # The PATTERN format
//
// Another repository starts a PATTERN by writing `PATTERN.md` at its root; there is no init verb.
// The overview is a thin list of entry lines, grouped under `##` topic headings.
// An entry line is one line:
//
//   - `PATTERN-<name>` — <when it applies>: <what it requires>. (test) — [background](pattern/PATTERN-<name>.md)
//
// The name matches `PATTERN-[a-z0-9-]+` and is unique in the overview.
// `(test)` marks a rule a test enforces, and is left out for a rule held by review alone.
// On a cluster entry it means at least one member rule is test-enforced,
// and the cluster's background file marks each member rule that is review-only.
// The background link is optional.
// When present it points at `pattern/<the entry's own name>.md`, and that file exists.
// Every file under `pattern/` is named by exactly one entry.
//
// A format checker in the package's tests enforces the shape; the real file is checked under `go test`.
// MaxOverviewBytes is a hard cap on any tree.
// Headroom binds a branch's own growth to the HeadroomBytes line; a merged tree answers only to the cap.
// PATTERN text states rules for the code only:
// it never says how an agent, a run, a stencil or a workflow uses it.
package pattern
