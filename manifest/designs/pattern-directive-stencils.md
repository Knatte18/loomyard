# PATTERN directives — move from Go constants to stencil files

> **Status: Design — not built.**

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
   `stencilstore.Read` instead of returning a Go constant. On a read error (should not happen for a
   shipped default — treat as exceptional), fail safe: log a warning, return `""`, exactly this
   package's existing default-on-failure posture (see `isActive`'s own non-`IsNotExist`-error handling
   for the sibling precedent).
4. Update all four call sites to pass `stencilsDir` — already in scope at every one of them
   (`websterengine`'s functions already take it as a parameter; `burlerengine.Engine` already stores
   `e.stencilsDir`; `loomengine.PlanSpec` already has it), so this is plumbing-free.
5. Producer templates are **unchanged** — each keeps its existing flat, optional `pattern_directive`
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
must instead compare against the corresponding stencil file's (embedded-default or seeded) content.
The "compared by fixed-string equality" property the original Go-constant comment cites as the reason
for keeping them as plain literals is preserved — the string is still a fixed literal, just sourced
from a `.md` file instead of a `const` block.

## Related

- [shed.md](shed.md), [loom.md](loom.md) — the flat-producer-list work this rides alongside; independent of it, no build-order dependency either way.
- `docs/shared-libs/stencil.md` — the `Fill`/`FillOptional` contract these stencil files render through, unchanged by this task.
- `internal/pattern` package documentation — where `Directive`'s new `stencilsDir` parameter and read-path land.
