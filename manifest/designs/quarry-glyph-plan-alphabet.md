# Adopt quarry's glyph alphabet as the plan alphabet

> **Status: Done.** See each package's own documentation for as-built detail — `manifest/roadmap.md`'s Done section is cleared regularly, not a durable record.
> Full text lives in [GitHub issue #226](https://github.com/Knatte18/loomyard/issues/226) — this doc is the short pointer `manifest/roadmap.md`'s Maintenance section asks for, not a restatement.

## The alphabet

Plan-format symbol targets are spelled as quarry glyphs (`unit#member`, e.g. `internal/shedrecipe#Lookup`) rather than bare package-qualified names.
A self glyph (`unit#`, e.g. `internal/planglyph#`) names a whole file or directory rather than one symbol.
Plain file paths keep validating and packing exactly as before — glyph and path are per-target, never a wholesale replacement.
`plan.Language` (frontmatter `language:`) selects the alphabet: `"go"` (the default) or `"none"`, which opts a plan out of the glyph alphabet entirely and keeps every glyph-aware check a no-op.

## The package-ownership seam

`internal/planparser` imports only the pure `github.com/Knatte18/quarry/glyph` package and stays a tier1-pure leaf: it owns the plan format's on-disk grammar and every check that needs no repository read (format/shape/field checks, the syntactic tier of cross-granularity containment).
`internal/planglyph` owns every `quarry.Repo` call (`Open`, `Resolve`, `DeltaGit`) and the package-level `quarry.Name`, plus the resolve-backed validation pass layered on top.
`planglyph.ValidateFormat`/`Validate` **call** `planparser.ValidateFormat`/`Validate` and append only resolve findings — no check is implemented twice, so the parity pair (`ValidateFormat` for the pre-approval gate, `Validate` adding the `plan-unapproved` check) cannot drift.
Both packages derive no path of their own: `planparser.Validate(plan, worktreeRoot)` and `planglyph.Validate(plan, worktreeRoot)` share the same told-geometry shape, per the Told-Geometry Invariant (`CONSTRAINTS.md`).

## The handle lifecycle: draft, canonicalize, bind

A symbol a plan creates doesn't exist yet, so its glyph is unknowable up front.
A `Create` sub-bullet declares a `plan:<draft-handle>` -> `<declaration head>` pair; a `Rename` group's to-side is required to be a `plan:` handle too, so its real spelling is computed rather than trusted.
`planglyph.CanonicalizeHandles` turns every draft handle into its canonical `plan:<expected-glyph>` form via one batched `quarry.Name` call — covering both a `Create` declaration and a `Rename` pair's derived to-side declaration in the same call — and rewrites every occurrence across the plan on disk via `planparser.RewriteRefs`.
This is the one place `quarry.Name` is ever called: never from a planning-loop caller, never exposed through the CLI, because `name` in an agent's hands is a glyph-spelling machine.
The DAG runs on handles until the creating card completes; binding a handle to its real glyph after the card merges is a later batch's work.

## The resolve status policy and the Create inversion

Every glyph the plan references is resolved in one batched `(*quarry.Repo).Resolve` call.
`found` and `multipart` both pass with no finding — `multipart` marks one symbol the language lets be declared in several places (e.g. Go's several `func init()`), not a defect.
`ambiguous` is blocking (`glyph-ambiguous`), listing every candidate.
`not_found` is blocking (`glyph-not-found`), with the detail branching on `ResolveResult.Unit`: `found` names a misspelled member, `not_found` names a misspelled unit.
A pre-resolution rejection (`ResolveResult.Error`/`Reason`, no `Status`) is blocking (`glyph-rejected`).

A `Create` group's own targets invert this policy: `found`/`multipart` is the blocking finding (`create-already-exists`) — a target that already exists contradicts the card creating it — while `not_found` with `unit: found` passes with no finding, because creating a package is creating its first symbol, so the package's own directory already existing is the ordinary case, not a defect.
`not_found` with `unit: not_found` produces an informational `create-new-unit` finding naming the new unit explicitly, so a misspelled unit cannot silently create a package nobody intended.

## The two containment tiers

A card targeting a member glyph and a card targeting the file self glyph of the file that member lives in overlap physically with no string equality between them: no DAG edge, blind parallel dispatch, merge conflict.
`planparser`'s syntactic tier (`containment-unit-overlap`) catches the unit-level half by comparing parsed `Glyph.Unit` values.
`planglyph`'s resolve-backed tier (`containment-file-overlap`) catches the half string prefixes cannot see: it reads a member's owning file from `ResolveResult.Symbols[].File` (filled by `Resolve` because its entries span files) and matches it against every other card's file self glyph, whose own path comes from `Glyph.UnitPath()`.

## Glyph-conversion discipline

`planglyph` performs no glyph↔path conversion of its own beyond the calls the Glyph Conversion Chokepoint Invariant (`CONSTRAINTS.md`) names — see that invariant for the closed list of allowed calls; it is not restated here.

## The infrastructure-error disposition

A non-nil `error` from `quarry.Open` or `(*quarry.Repo).Resolve` is a category distinct from every per-target verdict, wrapped in `ErrQuarryUnavailable` so a caller distinguishes it with `errors.Is` rather than by string matching.
`planglyph.ValidateFormat`/`Validate` return `([]Finding, error)`: the pure findings already collected are returned alongside a non-nil error, because the gate cannot certify a plan as valid against code it failed to read, and the error must report as a gate/infrastructure failure rather than as a plan finding.
Conflating an infrastructure error with a `not_found` payload would let a quarry outage silently mark every `Create` card done, under the Create inversion above — the specific disaster this disposition exists to prevent.

## Mechanical uses, no LLM involved

Execution DAG at symbol granularity, the resolve status policy and Create inversion above, both containment tiers, and handle canonicalization — none of it touched by an LLM. The Planner's toolset stays `toc` plus the validator's own report.

## Deliberately out of scope

No LSP-shaped tools in an agent's own hands; semantics enter only the mechanical layer.
Done-checks, drift detection (`internal/planglyph/drift.go`), and the `lyx quarry` CLI verb group (`internal/quarrycli`) landed in later batches of this same now-Done task, not this doc's own scope.

## Related

- [GitHub issue #226](https://github.com/Knatte18/loomyard/issues/226) — full proposal text.
- [webster-parallel-execution.md](webster-parallel-execution.md) — the DAG-scheduler consumer waiting on symbol-derived edges.
- `internal/planglyph/doc.go` — the canonical, enumerated list of every resolve-backed `Finding.Check` ID this package can raise, the parallel this package owes `planparser`'s own numbered checks list in [loom-plan-spec.md](../../contracts/specs/loom-plan-spec.md#validation-checks-as-implemented-by-internalplanparser).
