// Package planparser is the SOLE parser of the on-disk plan format written under
// `_lyx/plan/` (see contracts/specs/loom-plan-spec.md, the pinned spec this package
// implements). No other package may read `_lyx/plan/` directly — every consumer
// (the batcher, webster's master, and fork prompt rendering) goes through
// planparser.ParsePlan and the Plan/Card model it returns, so the on-disk grammar has
// exactly one reader and the rest of webster never re-derives it.
//
// # Type model
//
// A parsed plan is a Plan: the overview's frontmatter (Format, Approved, Root), its
// task-framing paragraph (Framing), the flat ordered list of Cards, and the three
// optional plan-level body sections (SharedDecisions, RenameMechanic, Verify). Each
// Card carries its Card Index fields (Number, Slug, Intent), its own file's Title,
// What prose (the implementer's concrete instruction, verbatim) and HasWhat presence
// bit, its five typed file-op fields (Context/Edits/Creates/Deletes/
// Moves, each with a HasX presence bit), its Depends-on list, and the optional Commit
// and Verify fields. Every field-op path — all five fields, both sides of every
// MovePair — is stored fully normalized (see below); nothing downstream ever sees a
// raw `root:`/`//` form.
//
// # The root:/// resolution rule
//
// The overview's optional `root:` frontmatter key names a worktree-relative directory
// every card path resolves against, unless the path is `//`-prefixed, in which case it
// is always worktree-root-relative regardless of root:. ParsePlan applies this
// resolution exactly once, at parse time (see normalizeCardPath in normalize.go) — a
// malformed result (a still-absolute path, a `..` escape) is left in place rather than
// rejected here; catching it is the validator's job.
//
// # The none sentinel
//
// Plan-format requires every card to carry all five typed file-op fields and
// Depends-on, never omitted: a field with nothing to declare carries the literal
// `none` on its label line. ParsePlan distinguishes three states per field: the
// label was never seen at all (nil slice, HasX false), the label carried `none`
// (empty non-nil slice, HasX true), and the label carried real entries (populated
// non-nil slice, HasX true). This nil-vs-empty distinction is deliberate: an omitted
// field and a present-but-empty field are mechanically different card defects, and
// only the validator (not this package) turns either into an enumerable finding.
//
// # Validation lives in validate.go
//
// This package's parse step is lenient at the card level: a malformed bullet or an
// absent field is recorded into the model (via the HasX bits, MovesRaw, or a nil
// slice) rather than failing the parse, so Validate can enumerate every defect in one
// pass instead of stopping at the first one. ParsePlan fails loud only on
// document-structure errors — a missing or undecodable overview file, an unparseable
// Card Index line, a missing card file, an unparseable card heading, or an inline
// value where a field admits only `none` or a bullet list. The plan format's 14
// validation checks (numbering gaps, path malformation, Moves grammar, on-disk
// existence, Depends-on ordering, and so on) are implemented by Validate in
// validate.go, not by ParsePlan itself.
package planparser
