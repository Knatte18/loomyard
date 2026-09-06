# Review: Adopt quarry's glyph alphabet as the plan alphabet

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [BLOCKING:design] `internal/planglyph`'s Told-Geometry tier and root-path source are undecided
**Section:** `Constraints` (Told-Geometry Invariant) / `package-ownership` / `cli-verb-surface`
**Issue:** The Constraints section states `internal/planglyph` "is bound by this and must be added to the invariant's bound-packages list **if it takes a geometry**" — but `package-ownership` already commits `planglyph` to being "sole owner of every `quarry.Repo` call," and `quarry.Repo` is only obtainable via `quarry.Open(root)` (confirmed: `quarry`'s facade, "Package `quarry` (the facade): `Open(root)`..."). Opening a repo unavoidably needs an absolute root path — a geometry-shaped input by construction — so the "if" is already answered by the discussion's own text; leaving it conditional pushes a settled question into planning.
Worse, *how* that root reaches `planglyph` is never stated, and `planglyph` has three distinct call shapes with potentially different answers: (a) from the three `drift-boundaries` producer rows (`begin-batch`, `record-batch`, `Plan-Revalidate`) — the Told-Geometry Invariant says "a producer needs none of the tiers," implying the root must arrive already-resolved from the orchestrator, same as every other bound package; (b) from `planglyph`'s resolve-backed validation pass, called by both `Plan-Validate`/`validate-plan` (a producer+CLI pair per Gate Self-Check Parity) and directly by the golden-test/parity CLI, which per the invariant needs tier 1 only, via `preflight.ResolveMode` (confirmed present: `internal/preflight/predicates.go:110`, `func ResolveMode(cwd string) (*lyxcwd.Location, Mode, error)`) — one of the two existing standalone-CLI patterns; (c) from the new standalone `lyx quarry toc|glyphs|resolve|expand` verb group, which is never discussed at all — does it probe tier 1 the same way, or does it default to `cwd`, and does it even need a plan in scope, or can it be pointed at an arbitrary repo?
Verified: `planparser.Validate`/`ValidateFormat` already take `worktreeRoot string` as a caller-supplied parameter today (`internal/planparser/validate.go:58,67`), i.e. the existing pattern this task must extend is fully told — nothing in the discussion says `planglyph`'s equivalent entry point follows the same shape, or names which of `internal/hubgeom`/`internal/standalonegeom` (the invariant's only two `Geometry`-struct constructors) supplies it.
**Suggested fix:** Add a decision (or extend `package-ownership`) stating: `internal/planglyph` is unconditionally added to the Told-Geometry Invariant's bound-packages list in the same commit; its `quarry.Open`-calling entry points take an already-resolved root/`*lyxcwd.Location`-shaped value from their caller, never resolving it themselves; and the standalone `lyx quarry` verb group's root-resolution path is named explicitly (tier-1-via-`preflight.ResolveMode`, or cwd-default, or requires an explicit flag) rather than left to the planner to invent.

## Verdict

REQUEST_CHANGES
One undecided architectural point — how `internal/planglyph` and the new `lyx quarry` CLI verbs source their repository root under the Told-Geometry Invariant — should be settled in discussion, not left for the plan to improvise across three different call sites.
