# Batch: warp-geometry

```yaml
task: 'Harden the Path Invariant: close enforcement hole + fix geometry leaks'
batch: warp-geometry
number: 2
cards: 6
verify: go test ./internal/warpengine/... ./internal/warpcli/...
depends-on: [1]
```

## Batch Scope

Routes every geometry-construction site in `warpengine` and `warpcli` through the batch-1
`paths` API, leaving zero geometry literals in warp. Deletes the local `weftSuffix` /
`boardDirName` / `HubSuffix` constants from `clone.go` and repoints all consumers — including the
exported-`HubSuffix` consumer in `warpcli/clone.go` and the two test references in
`clone_integration_test.go` — so the package still compiles. All conversions are byte-identical
joins, so the existing warp test suites are the parity gate. This batch depends only on batch 1
and shares no edited files with batches 3 or 4.

## Cards

### Card 6: Route prune.go construction and reverse-parse through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warpengine/prune.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Pass 1 (~line 79): replace `filepath.Join(l.Hub, slug+"-weft")` with
  `l.WeftWorktreePath(slug)`. Pass 2 (~lines 121–128): replace the manual suffix guard and slice
  (`len(name) <= len("-weft")` / `name[len(name)-len("-weft"):] != "-weft"` /
  `name[:len(name)-len("-weft")]`) with a call to `paths.WeftHostSlug(name)`; when `ok` is false,
  `continue` (same skip semantics as the old guard); when true, use the returned host slug.
  Remove any now-unused locals. Behaviour must be identical.
- **Commit:** `refactor(warp): route prune weft paths through paths helpers`

### Card 7: Route reconcile.go weft path through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warpengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** At ~line 102 replace `filepath.Join(l.Hub, slug+"-weft")` (where
  `slug = filepath.Base(hostPath)`) with `l.WeftWorktreePath(slug)`. No other change.
- **Commit:** `refactor(warp): route reconcile weft path through WeftWorktreePath`

### Card 8: Route status.go weft path through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warpengine/status.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** At ~line 91 replace
  `filepath.Join(l.Hub, filepath.Base(hostPath)+"-weft")` with
  `l.WeftWorktreePath(filepath.Base(hostPath))`. Leave the `"_lyx"` / `"_codeguide"` git-pathspec
  literals at ~lines 235/260/271 untouched — they are pathspec args and parse comparisons, not
  path construction, and are explicitly allowed.
- **Commit:** `refactor(warp): route status weft path through WeftWorktreePath`

### Card 9: Delete clone.go geometry consts; build paths via helpers

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warpengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `weftSuffix = "-weft"`, `boardDirName = "_board"`, and
  `HubSuffix = "-HUB"` constants from the `const (...)` block (keep any unrelated consts there).
  Repoint the three build sites: `~line 61` `filepath.Join(cwd, name+HubSuffix)` →
  `paths.HubPath(cwd, name)`; `~line 92` `filepath.Join(hubPath, name+weftSuffix)` →
  `paths.WeftSiblingPath(hubPath, name)`; `~line 103` `filepath.Join(hubPath, boardDirName)` →
  `paths.BoardDir(hubPath)`. `paths` is already imported in `clone.go`. Remove the `filepath`
  import only if it becomes unused (it likely stays).
- **Commit:** `refactor(warp): build clone geometry via paths HubPath/WeftSiblingPath/BoardDir`

### Card 10: Repoint warpcli HubSuffix consumer

- **Context:**
  - `internal/paths/paths.go`
  - `internal/warpengine/clone.go`
- **Edits:**
  - `internal/warpcli/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** At ~line 51 replace `filepath.Join(cwd, name+warpengine.HubSuffix)` with
  `paths.HubPath(cwd, name)`. Add the `internal/paths` import if absent; drop the
  `warpengine`/`filepath` imports only if they become unused. This site breaks compilation the
  moment Card 9 deletes the exported `HubSuffix`, so it must land in the same batch.
- **Commit:** `refactor(warpcli): use paths.HubPath instead of deleted warpengine.HubSuffix`

### Card 11: Repoint clone_integration_test references to paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warpengine/clone_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** At lines 95 and 181 replace `filepath.Join(hubPath, boardDirName)` with
  `paths.BoardDir(hubPath)`. Add the `internal/paths` import to the test file if absent. (The
  enforcement scan does not read `*_test.go`, but the deleted `boardDirName` const breaks
  compilation, so the reference must move.) After this card, a tree-wide grep for `HubSuffix`,
  `weftSuffix`, `boardDirName` identifiers must return zero hits.
- **Commit:** `test(warp): repoint clone_integration_test to paths.BoardDir`

## Batch Tests

`verify: go test ./internal/warpengine/... ./internal/warpcli/...` runs both edited packages. The
existing prune/reconcile/status/clone unit and integration suites are the parity gate — they must
stay green with no assertion changes (the conversions are byte-identical joins). Card 11 ensures
`warpengine` (which contains the integration test) compiles after the const deletion. Scope is
limited to the two packages this batch edits.
