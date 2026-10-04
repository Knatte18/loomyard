# PATTERN-told-geometry

An engine is handed the absolute paths it operates on and derives none of its own, so it never imports `internal/lyxcwd` directly.

## Tiers

- Three tiers: `lyxcwd.Resolve`, then `preflight.Check` (fabric wired, synced, clean), then `loomengine.CheckSeed`.
- A producer needs none of the tiers.
- An orchestrator needs tier 3.
- A standalone CLI probes tier 1 through `preflight.ResolveMode` only.

## Constructors

`internal/hubgeom` and `internal/standalonegeom` are the only `Geometry`-struct constructors.

## Bound packages

`internal/tokenvocab`, `pattern`, `buildinfo`, `standalonestate`, `shedengine`, `treadleengine`, `loomshed`, `landingshed`, `mergeresolve`, `shedrecipe`, `shedbuild`, `loomrecipe`, `planparser`, `planglyph`, `configengine`, `shuttleengine`, `reedengine`, `burlerengine`, `websterengine`, `verifytree`, `cliwire`, `battenshed`, `battenrecipe`, `orchengine`, `parentreview`.

## Detached runners

A `shuttleengine` runner whose anchor is deliberately outside its worktree root is constructed only through `shuttleengine.NewDetachedRunner`, and only from a standalone CLI's own wiring.
`NewRunner`'s containment assertion is never relaxed to accommodate it.
