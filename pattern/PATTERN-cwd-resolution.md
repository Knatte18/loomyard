# PATTERN-cwd-resolution

`internal/lyxcwd` owns cwd resolution alone — never weft, a junction path, or any per-module subdirectory.

- `root` is the worktree or repo root and `cwd` is the current working directory; the two are never conflated.
- `Resolve` requires cwd to be a git worktree root and to equal `Join(worktreeRoot, AnchorRel)`.
- It exposes only `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel`, `WorktreePath()` and `AnchorPath()`.
- Every cwd or worktree-root query goes through `lyxcwd.Getwd()` or `Resolve()`; raw `os.Getwd` and `git rev-parse --show-toplevel` are banned elsewhere.
- A module's own durable subdirectory is its own constant joined onto `AnchorPath()`, never a `lyxcwd` call.
- `lyxcwd` imports the standard library and `internal/dotgit` only.

## Told geometry

An engine is handed the absolute paths it operates on and derives none of its own, so it never imports `internal/lyxcwd` directly.

### Tiers

- Three tiers: `lyxcwd.Resolve`, then `preflight.Check` (fabric wired, synced, clean), then `loomengine.CheckSeed`.
- A producer needs none of the tiers.
- An orchestrator needs tier 3.
- A standalone CLI probes tier 1 through `preflight.ResolveMode` only.

### Constructors

`internal/hubgeom` and `internal/standalonegeom` are the only `Geometry`-struct constructors.

### Bound packages

`internal/pattern`, `buildinfo`, `standalonestate`, `shedengine`, `sheddriver`, `treadleengine`, `loomshed`, `landingshed`, `mergeresolve`, `shedrecipe`, `shedbuild`, `loomrecipe`, `planparser`, `planglyph`, `configengine`, `shuttleengine`, `shuttleengines`, `reedengine`, `burlerengine`, `websterengine`, `verifytree`, `cliwire`, `battenshed`, `battenrecipe`, `orchengine`, `parentlock`, `parentreview`, `hubreconcile`, `reportengine`, `creelengine`.

### Detached runners

A `shuttleengine` runner whose anchor is deliberately outside its worktree root is constructed only through `shuttleengine.NewDetachedRunner`, and only from a standalone CLI's own wiring.
`NewRunner`'s containment assertion is never relaxed to accommodate it.

## Hub containment

No hub-level container is ever junctioned into a worktree.
`_board` and `_launchers` are reachable from the hub only.

- `_launchers` links point hub-inward only; a per-worktree link to it is banned.
- `_portals` is retired and stays reserved; `lyx fabric reconcile` sweeps what a hub still holds.
- The hub workspace file `<RepoName>.code-workspace` and each beam's file `<RepoName>-<beam>.code-workspace` sit directly in the hub dir, reference hub-inward folders only, and are never written into a worktree.
