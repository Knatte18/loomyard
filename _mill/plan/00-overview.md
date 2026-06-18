# Plan: Weft repo — companion-repo overlay for lyx

```yaml
task: "Weft repo — companion-repo overlay for lyx"
slug: weft-repo
approved: true
started: 20260618-183018
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: terminology-rename
    file: 01-terminology-rename.md
    depends-on: []
    verify: go test ./internal/paths/... ./internal/ide/... ./internal/worktree/...
  - number: 2
    name: config-path-migration
    file: 02-config-path-migration.md
    depends-on: []
    verify: go test ./internal/config/... ./internal/board/... ./internal/worktree/... ./cmd/...
  - number: 3
    name: docs-and-roadmap
    file: 03-docs-and-roadmap.md
    depends-on: [1, 2]
    verify: null
```

## Shared Decisions

### Decision: behavior-preserving rename

- **Decision:** All code changes in batches 1 and 2 are strictly behavior-preserving. No logic is added or removed — only identifier names and path strings change. If a test fails after a batch, the cause is a missed rename site, not an intentional behavior change.
- **Rationale:** The discussion explicitly forbids logic changes in this task; the builds must stay green throughout.
- **Applies to:** terminology-rename, config-path-migration

### Decision: hard-cut config path

- **Decision:** `internal/config.Load` reads only from `_lyx/config/<module>.yaml`. There is no fallback to the old `_lyx/<module>.yaml` location. `FindBaseDir` still checks for `_lyx/` existence (unchanged).
- **Rationale:** Discussed and confirmed; single-user project, no migration script needed.
- **Applies to:** config-path-migration

### Decision: portal methods stay

- **Decision:** `PortalsDir()`, `PortalLink()`, `PortalTarget()` remain in `internal/paths/paths.go` and `docs/shared-libs/paths.md`. They are documented as deprecated but not removed. Removal is task 006's scope.
- **Rationale:** Discussed and confirmed.
- **Applies to:** terminology-rename, docs-and-roadmap

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/main.go`
- `cmd/lyx/main_test.go`
- `docs/benchmarks/board-performance.md`
- `docs/modules/board.md`
- `docs/modules/worktree.md`
- `docs/overview.md`
- `docs/roadmap.md`
- `docs/shared-libs/README.md`
- `docs/shared-libs/config.md`
- `docs/shared-libs/paths.md`
- `internal/board/boardtest/bench_test.go`
- `internal/board/boardtest/concurrency_test.go`
- `internal/board/cli_test.go`
- `internal/board/config_test.go`
- `internal/board/init.go`
- `internal/board/init_test.go`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/ide/color.go`
- `internal/ide/color_test.go`
- `internal/ide/menu_test.go`
- `internal/ide/spawn_test.go`
- `internal/paths/paths.go`
- `internal/paths/paths_test.go`
- `internal/worktree/config_test.go`
- `internal/worktree/launchers_test.go`
- `internal/worktree/portals_test.go`
- `internal/worktree/remove_test.go`
