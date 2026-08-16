# PATTERN directives — move from Go constants to stencil files

> **Status: Built.**

## Why

`internal/pattern.Directive(l, role)` returns one of three hardcoded Go string constants
(`implementerDirective`/`reviewFixDirective`/`orchestratorDirective`) — static prose, no per-call
interpolation. Four call sites (`internal/websterengine/render.go`×2, `internal/burlerengine/engine.go`,
`internal/loomengine/plan.go`) each compose the returned string into one producer template's
`pattern_directive`-style optional marker.

This is the single source of truth for PATTERN's directive prose today — genuinely not duplicated —
but it is Go source, not markdown: editing it means touching Go and rebuilding, exactly the friction
the `stencils/` reorg exists to remove for every other producer-facing prompt in the tree.

## What needs to happen

1. Move the three constants' content, verbatim, into three new stencil files, following the existing
   `<family>-<type>-<role>.md` convention:
   - `stencils/pattern/pattern-directive-implementer.md`
   - `stencils/pattern/pattern-directive-review-fix.md`
   - `stencils/pattern/pattern-directive-orchestrator.md`
2. Register all three in `stencils/stencils.go` (a `//go:embed` var + a registry entry each), same
   mechanism the other 15 stencils already use.
3. `internal/pattern.Directive` gains a `stencilsDir string` parameter and reads the matching file via
   `stencilstore.Read` instead of returning a Go constant. `Directive` returns `(string, error)` and
   fails loud: an active PATTERN whose directive stencil cannot be read is an error, wrapping
   `stencilstore.Read`'s own error, propagated to the caller. No `internal/logger` import is needed as
   a result.
4. Update all four call sites to pass `stencilsDir`.
   Two are plumbing-free — `burlerengine.Engine` already stores `e.stencilsDir`, and
   `loomengine.PlanSpec` already takes `stencilsDir` as a parameter — so each is a simple assignment
   plus an error check.
   The other two are not: `internal/websterengine/render.go`'s `RenderRecoveryPrompt` and
   `RenderMasterPrompt` each derive `fabricengine.StencilsDir(l.HubPath)` internally rather than
   taking it as a parameter, and each embeds the `Directive` call inline in a `values` map literal —
   a two-return-value call cannot sit inline as a map value in Go, so both need the call hoisted
   above the map with its own error check.
   No webster signature changes.
5. `stencilstore.Read` is a plain read that strips nothing; `stencilstore.Reconcile` stamps a
   `lyx-stencil:` line into every seeded file's leading banner. `Directive` therefore calls
   `stencil.StripLeadingComment` on what it reads, so the stamp banner never reaches a producer
   prompt.
6. Producer templates are **unchanged** — each keeps its existing flat, optional `pattern_directive`
   marker exactly as today. No `{{if}}` block is needed in any producer template for this: "empty
   when inactive" already gives the opt-in behavior, and the zero-duplication property comes from the
   stencil file being the one source, not from template-level conditionals.

## What this does not do

Does not touch the three directive constants' actual prose, `pattern.isActive`'s activation check, or
any producer template's marker set. Purely relocates where the directive text is authored, from a Go
string literal to a stencil file.

## Test migration

`internal/pattern`'s own tests and every consumer-template test currently doing fixed-string
equality/substring matching against `implementerDirective`/`reviewFixDirective`/`orchestratorDirective`
must instead compare against the corresponding stencil file's **stripped body** — the bytes remaining
after `stencil.StripLeadingComment` — never whole-file bytes, because the on-disk file carries a
banner and a stamp the return value never does.
The "compared by fixed-string equality" property the original Go-constant comment cites as the reason
for keeping them as plain literals is preserved — the string is still a fixed literal, just sourced
from a `.md` file instead of a `const` block.

## Related

- [shed.md](shed.md), [loom.md](loom.md) — the flat-producer-list work this rides alongside; independent of it, no build-order dependency either way.
- `docs/shared-libs/stencil.md` — the `Fill`/`FillOptional` contract most stencils render through.
  These three are the first stencils that never pass through `Fill`: `Directive` injects each one's
  stripped body as a producer template's `pattern_directive` values-map string instead.
- `internal/pattern` package documentation — where `Directive`'s new `stencilsDir` parameter and read-path land.
