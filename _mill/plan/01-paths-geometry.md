# Batch: paths-geometry

```yaml
task: 'internal/paths: subpath init + mirrored system dirs'
batch: paths-geometry
number: 1
cards: 5
verify: go test ./internal/paths/...
depends-on: []
```

## Batch Scope

This batch generalizes `internal/paths` to mirror the repo subpath into the
container system dirs. It adds/changes the geometry methods that
`internal/worktree` (batch 2) consumes, and extends the paths unit tests. It is
one batch because every change lives in the single `internal/paths` package and
shares the same test scaffolding (`helpers_test.go`'s `newTestRepo`). The
external interface batch 2 consumes: `PortalLink(slug)`, the mirrored
`LauncherDir(slug)`, `MenuLauncherPath()`, `LauncherSpawnRel(slug)`,
`MenuLauncherRel()`. Batch-local note: `PortalsDir()`/`LaunchersDir()` and
`PortalTarget(slug)` are left unchanged in signature and meaning (un-mirrored
roots / already-correct target).

## Cards

### Card 1: Add `PortalLink(slug)` mirrored junction-link method

- **Context:**
  - `internal/paths/enforcement_test.go`
- **Edits:**
  - `internal/paths/paths.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add method `func (l *Layout) PortalLink(slug string) string`
  returning `filepath.Join(l.Container, "_portals", l.RelPath, slug)` — the
  mirrored portal junction link. Keep the existing `PortalsDir()` method
  unchanged (it remains the un-mirrored container root used as a prune boundary).
  Keep `PortalTarget(slug)` unchanged. Add a Go doc comment matching the style of
  the surrounding methods, stating the returned path and that at `RelPath == "."`
  it collapses to `<Container>/_portals/<slug>`. Do not use `os.Getwd` or
  `--show-toplevel` (enforcement ban).
- **Commit:** `feat(paths): add PortalLink for subpath-mirrored portal links`

### Card 2: Mirror `LauncherDir(slug)` and add `MenuLauncherPath()`

- **Context:**
  - `internal/paths/enforcement_test.go`
- **Edits:**
  - `internal/paths/paths.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Change `func (l *Layout) LauncherDir(slug string) string` to
  return `filepath.Join(l.Container, "_launchers", l.RelPath, slug)` (was
  `filepath.Join(l.LaunchersDir(), slug)`). Add
  `func (l *Layout) MenuLauncherPath() string` returning
  `filepath.Join(l.Container, "_launchers", l.RelPath, "ide-menu.cmd")` — the
  per-subpath menu launcher path. Keep `LaunchersDir()` unchanged (un-mirrored
  root, prune boundary / MkdirAll base). Update the `LauncherDir` doc comment to
  reflect the mirrored leaf semantics and the `RelPath == "."` collapse; add a doc
  comment for `MenuLauncherPath`. The `LaunchersDir()` vs mirrored-`LauncherDir`
  role split must be clear in the comments.
- **Commit:** `feat(paths): mirror LauncherDir by subpath and add MenuLauncherPath`

### Card 3: Add `LauncherSpawnRel(slug)` and `MenuLauncherRel()` climb methods

- **Context:**
  - `internal/paths/enforcement_test.go`
  - `internal/worktree/launchers.go`
- **Edits:**
  - `internal/paths/paths.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add the two relative-climb methods used by `launchers.go` to
  build `.cmd` cd targets, so no path arithmetic happens outside `paths`:
  - `func (l *Layout) LauncherSpawnRel(slug string) string` returns
    `rel, _ := filepath.Rel(l.LauncherDir(slug), filepath.Join(l.WorktreePath(slug), l.RelPath)); return rel`.
    This yields `(..\)^(2+N)<slug>\<sub>` on Windows (N = `RelPath` segment
    count); at `RelPath == "."` it collapses to `..\..\<slug>`.
  - `func (l *Layout) MenuLauncherRel() string` returns
    `rel, _ := filepath.Rel(filepath.Dir(l.MenuLauncherPath()), filepath.Join(l.MainWorktree, l.RelPath)); return rel`.
    This yields `(..\)^(1+N)<hub>\<sub>`; at `RelPath == "."` it collapses to
    `..\<hub>`. `filepath.Dir(l.MenuLauncherPath())` is the menu's directory
    (`<Container>/_launchers/<RelPath>`).
  Both rely on cross-worktree subpath uniformity (every worktree of a repo has
  the same `RelPath`), so the spawning worktree's `RelPath` validly indexes
  `MainWorktree`'s hub subpath. Add doc comments stating the returned form and the
  `RelPath == "."` collapse. `filepath.Rel` returns OS-native separators — do not
  hand-normalize here (callers handle backslash conversion).
- **Commit:** `feat(paths): add LauncherSpawnRel/MenuLauncherRel climb helpers`

### Card 4: Extend paths_test.go with subpath + new-method cases

- **Context:**
  - `internal/paths/helpers_test.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/paths/paths_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Extend the existing black-box tests (package `paths_test`,
  using `newTestRepo` + `paths.Resolve` from a created subdir as in
  `TestResolve_FromSubdirectory`). Cover, with `filepath.Join`-built expectations
  (never literal separators, so the tests pass cross-platform):
  - `PortalLink(slug)`: at `RelPath == "."` equals `Join(Container, "_portals", slug)`;
    at subpath `services/api` equals `Join(Container, "_portals", "services", "api", slug)`.
  - `LauncherDir(slug)`: at `RelPath == "."` still equals
    `Join(LaunchersDir(), slug)` (backward compat — the existing
    `TestResolve_GeometryMethods` assertion stays valid); at a subpath equals
    `Join(Container, "_launchers", <relpath>, slug)`.
  - `MenuLauncherPath()`: equals `Join(Container, "_launchers", <relpath>, "ide-menu.cmd")`
    (and `Join(Container, "_launchers", "ide-menu.cmd")` at root).
  - `LauncherSpawnRel(slug)` / `MenuLauncherRel()`: assert via
    `filepath.Rel(...)` recomputation of the documented targets (climb depth grows
    with subpath segments; collapses at root). Do not assert literal `..\..\`
    strings.
  - Multi-subpath no-collision: two different `RelPath`s (e.g. `services/api` vs
    `services/web`) produce distinct `PortalLink`/`LauncherDir` for the same slug.
  Use subtests so the matrix is readable. Keep the existing tests intact except
  where the doc/behavior genuinely changed (none should need rewriting at root).
- **Commit:** `test(paths): cover subpath mirroring and climb helpers`

### Card 5: Add `_codeguide` guard test

- **Context:**
  - `internal/paths/enforcement_test.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/paths/codeguide_guard_test.go`
- **Deletes:** none
- **Requirements:** Add a guard test (package `paths`, mirroring
  `enforcement_test.go`'s predicate + scan style using `runtime.Caller(0)` to
  locate the package dir). It must assert that no **non-test** `.go` file in
  `internal/paths` contains the literal substring `_codeguide`. Scan scope:
  `internal/paths/*.go` excluding files whose name ends in `_test.go` (so a future
  `_codeguide` mention inside a test fixture does not trip the guard; only
  production sources are scanned). Include a small predicate sub-test on synthetic
  strings (one containing `_codeguide` → true, one clean → false), as
  `enforcement_test.go` does. This documents that `paths` never enumerates the
  worktree to mirror dirs, so a future nested/ignored `_codeguide` can never be
  treated as a sibling.
- **Commit:** `test(paths): guard that paths never references _codeguide`

## Batch Tests

`verify: go test ./internal/paths/...` runs the whole `internal/paths` package:
the extended `paths_test.go` (cards 1-4), the new `codeguide_guard_test.go`
(card 5), and the unchanged `enforcement_test.go` (confirms cards 1-3 introduced
no banned `os.Getwd`/`--show-toplevel` tokens). Scope is the single package this
batch edits — no cross-cutting suite needed.
