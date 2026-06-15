# Discussion: internal/paths: subpath init + mirrored system dirs

```yaml
task: 'internal/paths: subpath init + mirrored system dirs'
slug: paths-subpath-mirroring
status: discussing
parent: main
```

## Problem

`mhgo-portals-launchers` (landed in `main` at commit `9de0c46`) introduced
`internal/paths` as the canonical geometry resolver and added machine-local
container system dirs: portals (`_portals/`) and launchers (`_launchers/`). That
task deliberately scoped OUT two cases:

1. **mhgo initialized in a subdirectory** of a worktree rather than at the
   worktree root. `paths.Resolve` already computes `RelPath` correctly from any
   cwd, and `PortalTarget(slug)` already embeds `RelPath` on the *target* side
   (`<container>/<slug>/<RelPath>/_mhgo`). But the **link/dir side** of portals
   and launchers is still flat-by-slug: portal links land at `_portals/<slug>`
   and launcher dirs at `_launchers/<slug>`, ignoring the subpath entirely.
2. **multiple mhgo instances per worktree** — two `mhgo init`s at different
   subpaths of one repo share one container and collide on `_portals/<slug>` /
   `_launchers/<slug>`.

This follow-up makes the container system dirs **mirror the repo's subpath
structure** (the same way millhouse mirrors a guided repo's structure into its
sibling codeguide repo): if mhgo is initialized at `<mainWT>/<subpath>`, portals
land at `<container>/_portals/<subpath>/<slug>` and launchers at
`<container>/_launchers/<subpath>/<slug>`. Mirroring the subpath also resolves
the multi-instance collision structurally — distinct subpaths produce distinct
container dirs.

**Why now:** mhgo must be initializable from *any* subfolder of a repo (cwd ≠ git
root, `_mhgo/` created wherever `init` runs). The flat-by-slug placement is the
last piece blocking subpath-rooted mhgo from working end-to-end (spawn → portal
→ launcher → ide menu).

## Scope

**In:**

- Generalize `internal/paths` so the **link/dir side** of portals and launchers
  mirrors `RelPath`:
  - Add `PortalLink(slug)` = `<Container>/_portals/<RelPath>/<slug>` — the portal
    junction link (replaces the hand-built `filepath.Join(PortalsDir(), slug)` in
    `portals.go`). No extra `MkdirAll` is needed for the mirrored
    `_portals/<RelPath>/` chain: `createJunction` already `MkdirAll`s
    `filepath.Dir(link)` (`junction_windows.go` line 31 / `junction_other.go`
    line 25), so only the link path changes on the portals side.
  - Change `LauncherDir(slug)` to `<Container>/_launchers/<RelPath>/<slug>`.
  - Add `MenuLauncherPath()` = `<Container>/_launchers/<RelPath>/ide-menu.cmd`
    (replaces the hand-built `filepath.Join(LaunchersDir(), "ide-menu.cmd")` in
    `launchers.go`).
  - Add relative-climb methods so `launchers.go` does **zero** path arithmetic
    (see decision *relative-climb-via-paths*):
    - `LauncherSpawnRel(slug)` — relative cd target from `LauncherDir(slug)` to
      `<Container>/<slug>/<RelPath>`, for `ide.cmd`.
    - `MenuLauncherRel()` — relative cd target from the menu launcher's dir to
      `<MainWorktree>/<RelPath>`, for `ide-menu.cmd`.
  - Keep `PortalsDir()` / `LaunchersDir()` as the **un-mirrored container roots**
    (used as prune stop-boundaries and as `MkdirAll` bases). Note the resulting
    role split that the docs must make explicit: `LauncherDir(slug)` is now the
    **mirrored leaf** (`LaunchersDir()/<RelPath>/<slug>`), while `LaunchersDir()`
    stays the **flat root** used only as a boundary/base — same naming, two
    distinct roles.
  - `PortalTarget(slug)` is unchanged (already embeds `RelPath` correctly).
- Update consumer call sites in `internal/worktree`:
  - `portals.go`: `createPortal` / `removePortal` use `PortalLink(slug)`.
  - `launchers.go`: `writeLaunchers` / `removeLaunchers` use the new
    `LauncherDir`, `MenuLauncherPath`, and `*Rel` methods; `ide-menu.cmd` is
    written per-subpath (still never-clobber).
- Teardown: on `remove`, after deleting the per-slug link/dir, **best-effort
  prune now-empty mirrored intermediate dirs** up to (not including)
  `PortalsDir()` / `LaunchersDir()`.
- Extend tests: `paths_test.go` (new/changed methods, subpath cases),
  `portals_test.go`, `launchers_test.go` (including `.cmd` climb at subpath depth
  ≥ 1, and the multi-subpath no-collision case), a teardown-prune test, and a
  guard test asserting `internal/paths` never references `_codeguide`. Keep
  `enforcement_test.go` intact.
- Update docs to match the new API: `CONSTRAINTS.md` "For New Code" method list
  (line 19) and `docs/shared-libs/paths.md` method table.

**Out:**

- **Codeguide of any kind.** There is no `_codeguide` in mhgo yet (zero
  references in the Go tree). It *will* come later, but this task adds no
  codeguide detection or handling — only a doc note + guard test confirming
  `paths` cannot accidentally mirror a future `_codeguide` (see decision
  *codeguide-non-interaction*). The millhouse sibling-codeguide layout is
  reference-only, to keep this design forward-compatible.
- The "active sibling codeguide repo" (proposal mode 4) — explicitly a future,
  standalone effort, not an mhgo-internal concern.
- No collision registry / dedup machinery — the multi-instance case is resolved
  structurally by subpath nesting (see decision *multi-instance-structural*).
- No change to `PortalTarget(slug)` semantics, to `paths.Resolve`, or to the
  enforcement ban itself.
- No mutation logic inside `internal/paths` — it stays geometry-only; pruning
  (a mutation) lives in `internal/worktree`.

## Decisions

### nested-mirroring

- Decision: Combine subpath + slug as **nested directories** —
  `_portals/<subpath>/<slug>`, `_launchers/<subpath>/<slug>` — computed by
  `filepath.Join(Container, "_portals", RelPath, slug)` (and the launcher
  equivalent).
- Rationale: Literal mirror of the repo subpath; matches the millhouse
  sibling-codeguide precedent the proposal cites; browsable on disk; teardown is
  deterministic from the `Layout` (no decode step). At subpath `.`,
  `filepath.Join(..., ".", slug)` collapses to the flat form — **fully
  backward-compatible** with today's single-root-init repos.
- Rejected: An encoded flat path (e.g. `_portals/<subpath-encoded>__<slug>`) —
  avoids deep nesting but is ugly and needs a decode step for teardown.

### menu-mirrored-per-subpath

- Decision: `ide-menu.cmd` is written **per-subpath** at
  `<Container>/_launchers/<RelPath>/ide-menu.cmd`, one per subpath, each cd-ing to
  that subpath's hub. Still never-clobber (per subpath).
- Rationale: Each subpath is an independent mhgo universe — `ide.Menu()` already
  filters discovered worktrees by `<path>/<RelPath>/_mhgo`. A single root menu
  could only ever serve one subpath. At subpath `.` this is identical to today's
  single `_launchers/ide-menu.cmd`.
- Rejected: Keep one root menu — only correct if one subpath per container is
  guaranteed forever; breaks the multi-instance case this task enables.

### teardown-prune-empty

- Decision: On `remove`, after deleting `PortalLink(slug)` / `LauncherDir(slug)`,
  **best-effort prune** now-empty mirrored ancestor dirs upward, stopping at (and
  never removing) `PortalsDir()` / `LaunchersDir()`. Prune failures are masked
  (consistent with existing best-effort teardown).
- **Portals vs launchers asymmetry (important):** The two sides behave
  differently because only launchers carry a never-removed `ide-menu.cmd`.
  - **Portals** have no menu, so pruning is fully effective: after the last slug
    under a subpath is removed, the empty `_portals/<RelPath>/...` chain is pruned
    up to (not including) `PortalsDir()`.
  - **Launchers** keep `ide-menu.cmd` in the leaf `_launchers/<RelPath>/` dir, and
    `removeLaunchers` (per the existing "leave ide-menu.cmd in place" rule) never
    deletes it. So that dir is never empty, and launcher-side pruning in practice
    only ever removes `LauncherDir(slug)` itself — the `<RelPath>` chain is
    **intentionally retained** by the menu and is NOT reclaimed. This is by
    design, not a missed prune: the menu's existence is exactly what keeps the
    subpath universe addressable. The prune-ancestors loop still runs on the
    launcher side for uniformity, but stops immediately at the menu-bearing dir.
- Rationale: Avoids accumulating empty `<subpath>` dirs on the portals side. No
  "discovery" needed — the exact path is recomputed from the `Layout`. Retaining
  the launcher `<RelPath>` chain is consistent with never clobbering/removing the
  per-subpath menu.
- Rejected: (a) Leave empty dirs everywhere — simpler but accumulates portal
  cruft over many spawn/remove cycles. (b) Remove the per-subpath `ide-menu.cmd`
  when the last slug under a subpath is removed (to enable full launcher-side
  prune) — adds "is this the last slug under this subpath?" detection and breaks
  the never-remove-menu invariant; not worth it for empty-dir tidiness.

### multi-instance-structural

- Decision: The "multiple mhgo instances per worktree" collision (left open by
  `mhgo-portals-launchers`) is resolved **structurally by the nested mirroring** —
  distinct subpaths yield distinct `_portals/<subpath>/<slug>` /
  `_launchers/<subpath>/<slug>`. No registry or dedup logic.
- Rationale: The nesting from decision *nested-mirroring* already eliminates the
  collision. Same slug at the *same* subpath is the same worktree — not a
  collision. Adding machinery would be YAGNI.
- Rejected: Explicit collision detection / a portals registry.

### relative-climb-via-paths

- Decision: The `.cmd` relative cd-target is computed **inside `internal/paths`**
  via `filepath.Rel`, exposed as `LauncherSpawnRel(slug)` and `MenuLauncherRel()`.
  `launchers.go` only converts separators to backslash and appends the CRLF
  command tail — it performs no path arithmetic.
- **Current behavior being replaced (corrected):** The existing `ide.cmd` is NOT
  a bare `..\..\` climb — `launchers.go` (lines 42-49) **already appends the
  RelPath tail today**, producing `..\..\<slug>\<relpath>` from a *flat*
  `_launchers/<slug>/` dir (asserted by the `NonEmptyRelPath` case in
  `launchers_test.go` line 68: `..\..\task-b\subdir\nested`). The existing
  `ide-menu.cmd` is likewise `..\<hub>\<relpath>` from the flat
  `_launchers/` root. What changes is twofold: the launcher dir **moves deeper**
  by N subpath segments (so the climb grows from 2 to `2+N` for spawn, 1 to `1+N`
  for menu), AND the climb is now derived in `paths` instead of hand-built.
  `LauncherSpawnRel` must therefore reproduce **both** the deeper climb and the
  existing `<slug>\<sub>` tail.
- Rationale: CONSTRAINTS.md's Path Invariant mandates that **all** worktree/
  container geometry flow through `internal/paths`; the climb depth is geometry
  (it depends on subpath segment count). `filepath.Rel(LauncherDir(slug),
  Join(WorktreePath(slug), RelPath))` yields exactly the `(..\)^(2+N)<slug>\<sub>`
  string (N = subpath segments); at subpath `.` it collapses to `..\..\<slug>`,
  matching today. Same approach for the menu: `filepath.Rel` from the menu dir to
  `Join(MainWorktree, RelPath)` gives `(..\)^(1+N)<hub>\<sub>`, collapsing to
  `..\<hub>` at subpath `.`.
- **Cross-worktree subpath uniformity (assumption made explicit):**
  `MenuLauncherRel()` cd-targets `Join(MainWorktree, RelPath)` using the *resolving
  worktree's* `RelPath` (`Rel(WorktreeRoot, Cwd)`, possibly a non-main worktree).
  This is valid because **all worktrees of a repo share the same subpath
  structure** — mhgo lives at the same relative location in every worktree
  (`_mhgo/` travels with the tree). So the spawning worktree's `RelPath` correctly
  indexes the main worktree's hub subpath. The same uniformity underpins
  `PortalTarget` and the `ide.Menu()` `<path>/<RelPath>/_mhgo` filter.
- Rejected: Compute the climb count in `launchers.go` from `len(strings.Split(
  RelPath, ...))` — violates the Path Invariant by hand-rolling geometry outside
  `paths`.

### codeguide-non-interaction

- Decision: **No codeguide code.** Add (a) a note in `docs/shared-libs/paths.md`
  stating `paths` only derives mhgo's own dirs (`_mhgo`, `_portals`,
  `_launchers`) from `RelPath` and never enumerates the worktree, so a nested or
  git-ignored `_codeguide` can never be mistaken for a sibling to mirror; and (b)
  a guard test asserting no `internal/paths` source file references `_codeguide`.
- Rationale: There is no `_codeguide` in mhgo today (verified: zero references in
  the Go tree). Codeguide will arrive later, but the mirroring here applies only
  to mhgo's own fixed set of dirs — paths never scans for dirs to mirror, so the
  failure mode the proposal worried about (mirroring `_codeguide` as a sibling)
  is structurally impossible. The proposal's own scope conclusion is "mode-aware
  enough to leave codeguide alone."
- Rejected: Add explicit codeguide-mode detection in `paths` — contradicts the
  proposal's scope conclusion and would be speculative.

### docs-in-sync

- Decision: Update `CONSTRAINTS.md` (the "For New Code" method list, line 19) and
  `docs/shared-libs/paths.md` (the method table, line 76) in this same task to
  reflect the new `PortalLink`, `MenuLauncherPath`, `LauncherSpawnRel`,
  `MenuLauncherRel` methods and the changed `LauncherDir` semantics. The paths.md
  table currently reads `LauncherDir = Join(LaunchersDir(), slug)`; it must be
  rewritten to show the **mirrored-leaf vs un-mirrored-root** split (see
  *nested-mirroring* Scope note) so the new `LauncherDir` /
  `PortalLink` (mirrored) vs `LaunchersDir` / `PortalsDir` (flat boundary)
  semantics are unambiguous. Extend `CONSTRAINTS.md`
  naturally where the new methods warrant it.
- Rationale: CONSTRAINTS.md is the authoritative statement of the Path Invariant
  and enumerates the sanctioned methods; leaving it stale would mislead future
  code and weaken the enforcement story. The operator confirmed extending it is
  expected when the surface grows.
- Rejected: Defer doc updates — leaves the canonical constraint doc inconsistent
  with the code.

## Technical context

Key files (all paths relative to worktree root):

- `internal/paths/paths.go` — the `Layout` struct and geometry methods. Add the
  new methods here. `Resolve` already populates `Cwd`, `WorktreeRoot`,
  `Container`, `RelPath`, `MainWorktree`. `RelPath` is `.` at the worktree root.
  Existing methods: `MhgoDir`, `WorktreePath`, `PortalsDir`, `PortalTarget`,
  `LaunchersDir`, `LauncherDir`, `HubName`.
- `internal/paths/paths_test.go` — table-driven tests for the methods; uses
  `newTestRepo(t)` (defined in `helpers_test.go`) to build a real git repo, and
  resolves from a created subdir for subpath cases (see
  `TestResolve_FromSubdirectory`).
- `internal/paths/enforcement_test.go` — repo-wide guard banning raw `os.Getwd` /
  `--show-toplevel` outside `internal/paths` and `cmd/mhgo/main.go`. Must stay
  green; the new guard test (no `_codeguide` in paths) can live alongside it or
  in `paths_test.go`.
- `internal/worktree/portals.go` — `createPortal` / `removePortal`. `createPortal`
  delegates to `createJunction(link, target)`; switch `link` to `PortalLink(slug)`
  and `target` stays `PortalTarget(slug)`. `removePortal` removes the link then
  (new) prunes empty ancestors up to `PortalsDir()`.
- `internal/worktree/launchers.go` — `writeLaunchers` / `removeLaunchers`,
  Windows-only (no-op elsewhere via `runtime.GOOS != "windows"`). Currently
  hand-builds the `..\..\` strings and the menu path; replace with `*Rel` methods
  and `MenuLauncherPath()`. The `ide.cmd` content format is
  `@cd /d "%~dp0<rel>" && mhgo ide spawn <slug>\r\n`; the menu format is
  `@cd /d "%~dp0<rel>" && mhgo ide menu\r\n`. Preserve CRLF and `0o644`/`0o755`
  modes. `MkdirAll` the mirrored launcher dir and the menu's parent subpath dir.
- `internal/worktree/add.go` — orchestrates create→portal→launchers→push with
  full rollback (`rollbackAdd` calls `removePortal` + `removeLaunchers`). No
  signature changes needed; it already passes `l` + `slug`. Rollback benefits
  from the same teardown prune.
- `internal/worktree/remove.go` — early best-effort `removePortal` +
  `removeLaunchers` before the exists check; leaves the menu in place. The prune
  is inside the `remove*` helpers, so `remove.go` needs no change.
- `internal/ide/menu.go`, `internal/ide/spawn.go` — consumers. `Menu()` discovers
  worktrees via `paths.List(l.Cwd)` and filters by `<path>/<RelPath>/_mhgo`;
  `Spawn()` opens `Join(WorktreePath(slug), RelPath)`. Both already RelPath-aware
  — no change expected, but verify the per-subpath menu cd target lines up with
  what `Menu()` expects.

Gotchas:

- `filepath.Join(x, ".", y)` == `filepath.Join(x, y)` — this is what makes the
  mirror a no-op at subpath `.` and preserves backward compatibility. Tests must
  assert the `.`/empty case collapses to today's flat layout.
- `filepath.Rel` returns OS-native separators (backslash on Windows). Launcher
  content is Windows-only, so backslash is correct there; the `*Rel` methods are
  still callable cross-platform for unit tests (assert with `filepath`-built
  expectations, not literal backslashes, on non-Windows).
- `RelPath` is never absolute and never contains `..` (it's `filepath.Rel(root,
  cwd)` for a cwd inside root) — no path-escape validation needed.
- The launcher subpath dir keeps `ide-menu.cmd`, so launcher-side pruning of the
  `<subpath>` dir will (correctly) stop there while a menu exists.

## Constraints

From `CONSTRAINTS.md` (Path Invariant — enforced at build time):

- All cwd / worktree-root / sibling-dir geometry MUST go through
  `internal/paths` (`Getwd`, `Resolve`, and `Layout` methods). This task's new
  relative-climb logic is therefore a `paths` method, never hand-rolled.
- Raw `os.Getwd` and `git rev-parse --show-toplevel` remain banned outside
  `internal/paths` and `cmd/mhgo/main.go`; `enforcement_test.go` enforces it.
- `internal/paths` stays geometry-only — it computes *where* things are and never
  mutates. Pruning (a mutation) lives in `internal/worktree`.
- `internal/paths` imports only `internal/git` + stdlib; never a domain module.
- Extending `CONSTRAINTS.md`'s sanctioned-method list is expected and required
  when the geometry surface grows (operator-confirmed).

## Testing

TDD candidates (pure geometry, no I/O — write tests first):

- `internal/paths` new/changed methods: `PortalLink`, `LauncherDir` (mirrored),
  `MenuLauncherPath`, `LauncherSpawnRel`, `MenuLauncherRel`. Cover:
  - subpath `.`/empty → collapses to today's flat layout (backward-compat
    assertions, mirroring the existing `TestResolve_GeometryMethods` style);
  - single-segment subpath (e.g. `services`);
  - multi-segment subpath (e.g. `services/api`) → confirm nesting depth and the
    `*Rel` climb count (`2+N` for spawn, `1+N` for menu);
  - multi-subpath no-collision: two different `RelPath`s yield distinct
    `PortalLink`/`LauncherDir` for the same slug.
- Guard test: assert no `internal/paths` **non-test** source file contains the
  literal `_codeguide`. Scan scope must be `internal/paths/*.go` excluding
  `*_test.go` (the existing `enforcement_test.go` walker explicitly skips
  `_test.go` at line 48, so a `_codeguide` mention inside a test file — e.g. a
  future fixture — must not trip the guard; only production sources are scanned).
  Predicate + scan style mirrors `enforcement_test.go`.

Windows-gated (existing pattern — `t.Skip` off Windows):

- `internal/worktree/launchers_test.go`: **rewrite** the existing
  `NonEmptyRelPath` assertion — it currently expects `..\..\task-b\subdir\nested`
  from a *flat* `_launchers/<slug>/` dir, but the launcher dir now moves to
  `_launchers/subdir/nested/<slug>/`, so the correct content becomes the deeper
  climb `..\..\..\..\task-b\subdir\nested` (2 base + 2 subpath segments). This is
  a changed expectation, not merely an added row. Then extend the table with at
  least one more subpath case asserting the `ide.cmd` deeper climb and that
  `ide-menu.cmd` is written at the mirrored per-subpath location
  (`_launchers/<RelPath>/ide-menu.cmd`) with its `1+N` climb; keep the
  never-clobber assertion (now per-subpath). The `EmptyRelPath`/`DotRelPath`
  cases must still collapse to today's `..\..\<slug>`.
- `internal/worktree/portals_test.go`: assert `createPortal` links at
  `_portals/<subpath>/<slug>` pointing to `PortalTarget`, and that two subpaths
  don't collide.
- Teardown-prune test (portals and launchers): after removing the last slug under
  a subpath, the empty `<subpath>` dir is pruned up to (not including)
  `_portals`/`_launchers`; the launcher `<subpath>` dir is NOT pruned while its
  `ide-menu.cmd` remains; pruning is idempotent / best-effort.

Whole-suite: `go test ./...` must stay green, including `enforcement_test.go`.

## Q&A log

- **Q:** Placement scheme for subpath + slug — nested dirs or encoded flat path?
  **A:** Nested dirs (`_portals/<subpath>/<slug>`).
- **Q:** With multiple mhgo subpaths in one container, does `ide-menu.cmd` split
  per-subpath or stay single at root? **A:** Mirror per-subpath
  (`_launchers/<subpath>/ide-menu.cmd`). (Operator first asked what "multiple
  subpaths sharing a container" meant — clarified as running `mhgo init` at >1
  subdirectory of the same repo, e.g. a monorepo's `services/api` and
  `services/web`.)
- **Q:** Teardown of now-empty mirrored intermediate dirs on remove — prune or
  leave? **A:** Prune empty ancestors, best-effort, stopping at the system-dir
  root.
- **Q:** The multi-instance collision left open by the prior task — resolve
  structurally or add a registry? **A:** Resolved structurally by the nesting; no
  registry.
- **Q:** Codeguide mode-awareness — write detection code now or document
  non-interaction? **A:** No code; document non-interaction + guard test.
  Codeguide does not exist in mhgo yet but will come later; millhouse's codeguide
  is reference-only so this design stays forward-compatible.
- **Q:** Where does the `.cmd` relative-climb computation live? **A:** Inside
  `internal/paths` — all path setup must go through the module (CONSTRAINTS.md
  Path Invariant).
- **Q:** Must `CONSTRAINTS.md` / paths docs be updated? **A:** Yes — follow the
  documented constraint, and extend `CONSTRAINTS.md` naturally when the method
  surface grows.
- **Q:** (review r1 gap) Launcher-side prune is inert because `ide-menu.cmd`
  keeps the `<RelPath>` dir non-empty — clarify or remove menu on last slug?
  **A:** Clarify only — launcher pruning removes just `LauncherDir(slug)`; the
  `<RelPath>` chain is intentionally retained by the menu (portals prune fully).
- **Q:** (review r1 gap) The `relative-climb-via-paths` decision misstated the
  current `ide.cmd` format and the testing section understated the test change —
  apply the correction? **A:** Yes — current `ide.cmd` already appends the
  RelPath tail; `LauncherSpawnRel` must reproduce climb + tail; the existing
  `NonEmptyRelPath` assertion must be rewritten to the deeper climb.
