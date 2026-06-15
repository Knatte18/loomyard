# Batch: docs

```yaml
task: 'internal/paths: subpath init + mirrored system dirs'
batch: docs
number: 3
cards: 2
verify: null
depends-on: [1]
```

## Batch Scope

This batch syncs the two authoritative docs with the batch-1 `paths` API so the
documented method surface matches the code: `CONSTRAINTS.md`'s sanctioned-method
list and `docs/shared-libs/paths.md`'s method table. It is one batch (pure
prose, no runnable surface) and depends only on batch 1 because the docs describe
the `paths` API, not the worktree consumer internals. No batch-local decisions
beyond `## Shared Decisions`.

## Cards

### Card 11: Update CONSTRAINTS.md sanctioned-method list

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In the "For New Code" section's `Layout` method bullet (the
  line listing `MhgoDir()`, `WorktreePath(slug)`, `PortalsDir()`,
  `PortalTarget(slug)`, `LaunchersDir()`, `LauncherDir(slug)`, `HubName()`), add
  the new methods `PortalLink(slug)`, `MenuLauncherPath()`, `LauncherSpawnRel(slug)`,
  and `MenuLauncherRel()`. Note the mirrored-leaf vs un-mirrored-root role split:
  `PortalLink`/`LauncherDir` are subpath-mirrored leaves, while
  `PortalsDir`/`LaunchersDir` remain the flat container roots (used as
  prune boundaries). Keep the prose consistent with the existing section's tone.
- **Commit:** `docs(constraints): list new subpath-mirrored paths methods`

### Card 12: Update paths.md method table + codeguide note

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `docs/shared-libs/paths.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In the `### Layout methods` table, change the `LauncherDir`
  entry from `filepath.Join(LaunchersDir(), slug)` to the mirrored
  `filepath.Join(Container, "_launchers", RelPath, slug)`, and add entries for
  `PortalLink(slug)` (`filepath.Join(Container, "_portals", RelPath, slug)` — the
  portal junction link), `MenuLauncherPath()`
  (`filepath.Join(Container, "_launchers", RelPath, "ide-menu.cmd")`),
  `LauncherSpawnRel(slug)` and `MenuLauncherRel()` (the `filepath.Rel`-derived
  `.cmd` cd targets). Make the mirrored-leaf (`LauncherDir`, `PortalLink`) vs
  un-mirrored-root (`LaunchersDir`, `PortalsDir`) role split explicit so the
  changed `LauncherDir` semantics are unambiguous, and note the `RelPath == "."`
  collapse to the flat layout. Add a short note (e.g. under "Design principles")
  that `paths` only derives mhgo's own dirs (`_mhgo`, `_portals`, `_launchers`)
  from `RelPath` and never enumerates the worktree, so a nested or git-ignored
  `_codeguide` is never mirrored as a sibling.
- **Commit:** `docs(paths): document subpath-mirrored methods and codeguide non-interaction`

## Batch Tests

`verify: null` — this batch edits only Markdown docs (`CONSTRAINTS.md`,
`docs/shared-libs/paths.md`) with no runnable/test surface. Correctness is
confirmed by review against `internal/paths/paths.go`.
