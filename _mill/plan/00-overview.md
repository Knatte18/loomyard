# Plan: internal/paths: subpath init + mirrored system dirs

```yaml
task: 'internal/paths: subpath init + mirrored system dirs'
slug: paths-subpath-mirroring
approved: true
started: 20260615-103745
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: paths-geometry
    file: 01-paths-geometry.md
    depends-on: []
    verify: go test ./internal/paths/...
  - number: 2
    name: worktree-consumers
    file: 02-worktree-consumers.md
    depends-on: [1]
    verify: go test ./internal/worktree/... ./internal/paths/...
  - number: 3
    name: docs
    file: 03-docs.md
    depends-on: [1]
    verify: null
```

## Shared Decisions

### Decision: all-geometry-through-paths

- **Decision:** Every worktree/container/sibling-dir path and every relative
  `.cmd` climb is produced by an `internal/paths.Layout` method. No domain
  module hand-rolls geometry with `filepath.Join`/`filepath.Rel` on
  container-relative paths, and none uses raw `os.Getwd` or
  `git rev-parse --show-toplevel`.
- **Rationale:** CONSTRAINTS.md's Path Invariant, enforced at build time by
  `internal/paths/enforcement_test.go`. The new relative-climb logic is geometry
  (depth depends on subpath segment count), so it lives in `paths`.
- **Applies to:** all batches

### Decision: subpath-mirroring-via-relpath

- **Decision:** Container system dirs mirror the repo subpath by joining
  `l.RelPath` into the path: portal links at `<Container>/_portals/<RelPath>/<slug>`,
  launcher dirs at `<Container>/_launchers/<RelPath>/<slug>`, per-subpath menu at
  `<Container>/_launchers/<RelPath>/ide-menu.cmd`. At `RelPath == "."`
  `filepath.Join(x, ".", y)` collapses to `filepath.Join(x, y)`, so the layout is
  byte-identical to today's flat-by-slug placement — fully backward compatible.
- **Rationale:** Mirrors the repo structure (millhouse sibling-codeguide
  precedent) and resolves the multi-instance-per-worktree collision structurally
  (distinct subpaths → distinct dirs).
- **Applies to:** paths-geometry, worktree-consumers

### Decision: go-test-no-pythonpath

- **Decision:** This is a Go module (`go.mod`); `verify:` uses `go test` directly
  with no `PYTHONPATH=` prefix (that prefix is Python/mill-only).
- **Rationale:** `verify-not-isolated` validator applies the prefix only to
  Python projects; Go uses the native runner.
- **Applies to:** all batches

### Decision: prune-asymmetry

- **Decision:** Teardown best-effort prunes empty mirrored ancestor dirs up to
  (not including) `PortalsDir()`/`LaunchersDir()`. Portals prune fully (no menu).
  Launchers keep the never-removed `ide-menu.cmd` in the leaf `_launchers/<RelPath>/`
  dir, so launcher-side pruning in practice only ever removes `LauncherDir(slug)`
  itself — the `<RelPath>` chain is intentionally retained.
- **Rationale:** Consistent with the existing "leave ide-menu.cmd in place" rule;
  see discussion decision `teardown-prune-empty`.
- **Applies to:** worktree-consumers

## All Files Touched

- `CONSTRAINTS.md`
- `docs/shared-libs/paths.md`
- `internal/paths/codeguide_guard_test.go`
- `internal/paths/paths.go`
- `internal/paths/paths_test.go`
- `internal/worktree/launchers.go`
- `internal/worktree/launchers_test.go`
- `internal/worktree/portals.go`
- `internal/worktree/portals_test.go`
- `internal/worktree/prune.go`
- `internal/worktree/prune_test.go`
