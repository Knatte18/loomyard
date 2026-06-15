# Batch: worktree-consumers

```yaml
task: 'internal/paths: subpath init + mirrored system dirs'
batch: worktree-consumers
number: 2
cards: 5
verify: go test ./internal/worktree/... ./internal/paths/...
depends-on: [1]
```

## Batch Scope

This batch updates the `internal/worktree` consumers to use the batch-1
geometry: portal links and launcher dirs/menus now mirror the subpath, and
teardown prunes empty mirrored ancestor dirs. It adds a shared
`pruneEmptyAncestors` helper used by both portal and launcher removal. It is one
batch because all changes live in `internal/worktree` and share the package's
white-box test scaffolding (`testhelpers_test.go`). It depends on batch 1 for
`PortalLink`, the mirrored `LauncherDir`, `MenuLauncherPath`, `LauncherSpawnRel`,
and `MenuLauncherRel`. Batch-local decision: the prune helper is a void,
best-effort function (errors swallowed) consistent with the existing
best-effort teardown in `remove.go`/`rollbackAdd`.

Note: the existing `internal/worktree/add_test.go` and `remove_test.go` call
`LauncherDir(slug)` / `PortalsDir()`+slug and resolve only from the hub root
(`RelPath == "."`), where the mirror collapses to the flat layout (byte-identical
paths). They are therefore **unaffected by this batch and must stay green
unedited** — an implementer should not modify them and should not treat their
passing-unchanged as a problem.

## Cards

### Card 6: Add `pruneEmptyAncestors` best-effort helper

- **Context:**
  - `internal/worktree/links.go`
  - `internal/worktree/remove.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/prune.go`
  - `internal/worktree/prune_test.go`
- **Deletes:** none
- **Requirements:** Create `prune.go` (package `worktree`) with
  `func pruneEmptyAncestors(start, stop string)` — a void, best-effort helper that
  walks upward from `start`. Each loop iteration first evaluates a **boundary
  guard at the top, before any `os.Remove`**, so `stop` is never a removal
  candidate: compute `rel, err := filepath.Rel(stop, cur)` and `return` if
  `err != nil`, `rel == "."` (this is the single `cur == stop` / filesystem-root
  condition — do not also write a separate string-equality check), or `rel`
  starts with `..` (`cur` not strictly under `stop`). Only after passing the guard
  does it `os.Remove(cur)` (which succeeds only on an empty dir); on success set
  `cur = filepath.Dir(cur)` and continue, on any error (non-empty / already gone)
  `return`. No return value; all errors swallowed. Create `prune_test.go` (package `worktree`) building a temp dir tree
  (use `t.TempDir()`) that asserts: empty mirrored ancestors are removed up to but
  not including the stop dir; a non-empty intermediate dir halts the walk (dirs
  above it survive); calling with `start == stop` is a no-op; the helper is
  idempotent on an already-pruned tree.
- **Commit:** `feat(worktree): add pruneEmptyAncestors teardown helper`

### Card 7: Point portals at mirrored `PortalLink` and prune on remove

- **Context:**
  - `internal/paths/paths.go`
  - `internal/worktree/junction_windows.go`
  - `internal/worktree/junction_other.go`
- **Edits:**
  - `internal/worktree/portals.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `createPortal`, replace
  `link := filepath.Join(l.PortalsDir(), slug)` with `link := l.PortalLink(slug)`;
  keep `target := l.PortalTarget(slug)` and the `createJunction(link, target)`
  call. No extra `MkdirAll` is needed — `createJunction` already
  `MkdirAll`s `filepath.Dir(link)`, creating the mirrored `_portals/<RelPath>/`
  chain. In `removePortal`, replace the link computation with
  `link := l.PortalLink(slug)`, keep the idempotent `os.Remove(link)` handling,
  and after a successful/idempotent removal call
  `pruneEmptyAncestors(filepath.Dir(link), l.PortalsDir())` so emptied
  `_portals/<RelPath>/` dirs are reclaimed (full prune — portals carry no menu).
- **Commit:** `feat(worktree): mirror portal links by subpath and prune on remove`

### Card 8: Mirror launcher dir/menu and derive climbs from paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/worktree/launchers.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rework `writeLaunchers` (still Windows-gated via
  `runtime.GOOS != "windows"`):
  - `launcherDir := l.LauncherDir(slug)` (now mirrored); keep the `MkdirAll`.
  - Build `ide.cmd` content from `l.LauncherSpawnRel(slug)` instead of the
    hand-built `relPathPart`/`..\..\` logic:
    `fmt.Sprintf("@cd /d \"%%~dp0%s\" && mhgo ide spawn %s\r\n", spawnRelBackslash, slug)`
    where `spawnRelBackslash = strings.ReplaceAll(l.LauncherSpawnRel(slug), "/", "\\")`.
  - Menu: `menuCmdPath := l.MenuLauncherPath()`; keep the never-clobber `os.Stat`
    early-return (if the menu already exists, return without rewriting). Only in
    the menu-**absent** branch (after that early return) call
    `MkdirAll(filepath.Dir(menuCmdPath), 0o755)` (the per-subpath launchers dir),
    so MkdirAll does not run on every `writeLaunchers` call; then write. Build menu
    content from `l.MenuLauncherRel()`:
    `fmt.Sprintf("@cd /d \"%%~dp0%s\" && mhgo ide menu\r\n", menuRelBackslash)` where
    `menuRelBackslash = strings.ReplaceAll(l.MenuLauncherRel(), "/", "\\")`. Remove
    the now-dead `hubName`/`relPathPartMenu` hand-building.
  - Preserve file modes (`0o644` for `.cmd`, `0o755` for dirs) and CRLF endings.
  In `removeLaunchers`, keep `launcherDir := l.LauncherDir(slug)` +
  `os.RemoveAll(launcherDir)`, then call
  `pruneEmptyAncestors(filepath.Dir(launcherDir), l.LaunchersDir())`. The leaf
  `_launchers/<RelPath>/` dir retains `ide-menu.cmd`, so the prune stops there —
  in practice removing only `LauncherDir(slug)` itself (intended asymmetry).
- **Commit:** `feat(worktree): mirror launcher dir/menu by subpath via paths climbs`

### Card 9: Update portals_test.go for mirrored links + prune

- **Context:**
  - `internal/paths/paths.go`
  - `internal/worktree/portals.go`
  - `internal/worktree/testhelpers_test.go`
- **Edits:**
  - `internal/worktree/portals_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update the portal tests (keep the existing platform gating the
  file already uses for junction creation). Assert: `createPortal` creates the
  junction at `l.PortalLink(slug)` (mirrored under `_portals/<RelPath>/`) pointing
  at `l.PortalTarget(slug)`, resolving from a subdir so `RelPath` is non-trivial
  (follow `paths` `TestResolve_FromSubdirectory` for building the subpath cwd);
  two distinct subpaths do not collide for the same slug; after `removePortal` the
  emptied `_portals/<RelPath>/` ancestor dirs are pruned up to but not including
  `PortalsDir()`, and `removePortal` remains idempotent. Keep the root-level
  (`RelPath == "."`) behavior asserted as backward-compatible with the flat
  layout.
- **Commit:** `test(worktree): cover mirrored portal links and prune`

### Card 10: Rewrite launchers_test.go for deeper climb + per-subpath menu

- **Context:**
  - `internal/paths/paths.go`
  - `internal/worktree/launchers.go`
  - `internal/worktree/testhelpers_test.go`
- **Edits:**
  - `internal/worktree/launchers_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** The existing `NonEmptyRelPath` case asserts
  `..\..\task-b\subdir\nested` from a flat `_launchers/<slug>/` dir; that
  expectation is now wrong because the launcher dir moves to
  `_launchers/subdir/nested/<slug>/`. Rewrite it to the deeper climb
  `..\..\..\..\task-b\subdir\nested` (2 base + 2 subpath segments). Keep
  `EmptyRelPath`/`DotRelPath` asserting the collapsed `..\..\<slug>`. Add
  assertions that `ide-menu.cmd` is written at `l.MenuLauncherPath()`
  (`_launchers/<RelPath>/ide-menu.cmd`) with the `1+N` climb produced by
  `MenuLauncherRel()`, and that never-clobber holds per-subpath (a second slug at
  the same subpath does not rewrite that subpath's menu). Read the `ide.cmd`/menu
  via `l.LauncherDir(slug)`/`l.MenuLauncherPath()` rather than hand-built paths.
  Add/adjust the `removeLaunchers` test so the per-slug dir is removed while the
  subpath's `ide-menu.cmd` (and thus its dir) survives. Preserve the existing
  Windows-only `t.Skip` gating.
- **Commit:** `test(worktree): rewrite launcher climb + per-subpath menu tests`

## Batch Tests

`verify: go test ./internal/worktree/... ./internal/paths/...`. The
`internal/worktree` portion runs the new `prune_test.go` (card 6) and the updated
`portals_test.go`/`launchers_test.go` (cards 9-10); launcher/junction assertions
self-skip off Windows via the existing `t.Skip`. The `internal/paths` portion is
included deliberately (not per-batch scope creep): `enforcement_test.go` scans
the whole repo tree and is the guard that the new `prune.go` and consumer edits
introduced no banned `os.Getwd`/`--show-toplevel` tokens — it lives in the
`paths` package, so it only runs when that package is tested.
