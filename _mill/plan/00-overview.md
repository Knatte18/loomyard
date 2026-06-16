# Plan: Rename mhgo to Loomyard (lyx)

```yaml
task: "Rename mhgo to Loomyard (lyx)"
slug: rename-to-loomyard
approved: true
started: "20260616-114829"
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: code-rename
    file: 01-code-rename.md
    depends-on: []
    verify: go build ./... && go test ./...
  - number: 2
    name: docs-and-config
    file: 02-docs-and-config.md
    depends-on: [1]
    verify: null
```

## Shared Decisions

### Decision: naming-map

- **Decision:** apply this canonical mapping everywhere:

  | Anchor | From | To |
  |---|---|---|
  | Module path | `github.com/Knatte18/mhgo` | `github.com/Knatte18/loomyard` |
  | CLI command / `cmd/` dir | `mhgo` / `cmd/mhgo` | `lyx` / `cmd/lyx` |
  | Managed-state dir | `_mhgo/` | `_lyx/` |
  | Local-state dir | `.mhgo/` | `.lyx/` |
  | gitignore markers | `mhgo-managed` | `lyx-managed` |
  | Exported ident | `MhgoDir()` | `LyxDir()` |
  | Local idents | `mhgoDir`/`mhgoPath`/`mhgoFile`/`mhgoIdx` | `lyxDir`/`lyxPath`/`lyxFile`/`lyxIdx` |
  | Env-var prefix | `MHGO_` | `LYX_` |
  | `short_name` | `MHGO` | `LYX` |
  | Product name (prose) | mhgo | Loomyard |
  | Test email domain | `@mhgo.dev` | `@loomyard.dev` |
  | Integration test repo | `mhgo-wiki-test` | `loomyard-test` |
  | Compiled binary (gitignore) | `/mhgo`, `mhgo.exe` | `/lyx`, `lyx.exe` |

- **Rationale:** repo/module takes the product name `loomyard` (matches the
  already-renamed git remote `github.com/Knatte18/loomyard.git`); everything
  operational (command, on-disk dirs, env-vars, identifiers, short_name) takes the
  short command name `lyx`. Mirrors millhouse's `mill` → `_mill` / `MILL_`.
- **Applies to:** all batches.

### Decision: prose-voice

- **Decision:** in documentation and comments, the project/product is written
  **"Loomyard"** (titles, "X is a Go toolkit", "X will replace mill/millhouse");
  the CLI invocation is written **`lyx`** in code font (`lyx board`,
  `usage: lyx <module>`, "concurrent `lyx` processes"); module-path references
  become `github.com/Knatte18/loomyard`. A blind single-token `mhgo`→`lyx` replace
  is FORBIDDEN — it would corrupt prose ("mhgo is a Go toolkit" must become
  "Loomyard is a Go toolkit", not "lyx is a Go toolkit").
- **Rationale:** the task is "Rename mhgo to Loomyard (lyx)" — Loomyard is the
  brand, `lyx` is how you invoke it.
- **Applies to:** all batches (code comments in batch 1, docs in batch 2).

### Decision: import-rewrite-precision

- **Decision:** the module-path rewrite replaces `github.com/Knatte18/mhgo/`
  (with trailing slash) → `github.com/Knatte18/loomyard/`, and the exact go.mod
  line `module github.com/Knatte18/mhgo` → `module github.com/Knatte18/loomyard`.
  It MUST NOT touch `github.com/Knatte18/mhgo-wiki-test` — that is a different
  external repo, renamed separately to `loomyard-test` (note: not
  `loomyard-wiki-test`). The trailing slash protects it (no `/` follows `mhgo` in
  `mhgo-wiki-test`).
- **Rationale:** a greedy replace would produce `loomyard-wiki-test`, the wrong
  repo name, and break the integration test's remote.
- **Applies to:** all batches.

### Decision: green-per-batch

- **Decision:** batch 1 lands the entire code rename atomically — at its boundary
  `go build ./...` and `go test ./...` both pass. Intermediate per-card states are
  not independently verified (verify runs once per batch, after all cards). Batch 2
  changes only `.md` docs and config-display fields, with no runnable code surface.
- **Rationale:** the `_mhgo`→`_lyx` literal and the module path are coupled across
  packages (e.g. `config.FindBaseDir` ↔ every `_lyx` creator); only a single
  atomic batch keeps the suite green. The `//go:build integration` files
  (`integration_test.go`, `bench_git_test.go`) are skipped by `go test ./...`, so
  the `mhgo-wiki-test`→`loomyard-test` URL change does not affect the verify gate.
- **Applies to:** all batches.

## All Files Touched

- `.gitignore`
- `CONSTRAINTS.md`
- `cmd/lyx/main.go`
- `cmd/lyx/main_test.go`
- `cmd/mhgo/main.go`
- `cmd/mhgo/main_test.go`
- `docs/benchmarks/board-performance.md`
- `docs/benchmarks/test-suite-timing.md`
- `docs/modules/board.md`
- `docs/modules/ide.md`
- `docs/modules/mux-exploration.md`
- `docs/modules/mux-hooks-exploration.md`
- `docs/modules/mux-proposal.md`
- `docs/modules/mux.md`
- `docs/modules/muxpoc.md`
- `docs/modules/worktree.md`
- `docs/overview.md`
- `docs/psmux-tui-behavior.md`
- `docs/roadmap.md`
- `docs/shared-libs/README.md`
- `docs/shared-libs/config.md`
- `docs/shared-libs/gitignore.md`
- `docs/shared-libs/lock.md`
- `docs/shared-libs/paths.md`
- `docs/shared-libs/state.md`
- `go.mod`
- `internal/board/board.go`
- `internal/board/board_test.go`
- `internal/board/boardtest/bench_git_test.go`
- `internal/board/boardtest/bench_test.go`
- `internal/board/boardtest/concurrency_test.go`
- `internal/board/boardtest/doc.go`
- `internal/board/boardtest/integration_test.go`
- `internal/board/cli.go`
- `internal/board/cli_test.go`
- `internal/board/config.go`
- `internal/board/config_test.go`
- `internal/board/git.go`
- `internal/board/git_test.go`
- `internal/board/init.go`
- `internal/board/init_test.go`
- `internal/board/layer_test.go`
- `internal/board/render_test.go`
- `internal/board/spawn_other.go`
- `internal/board/spawn_windows.go`
- `internal/board/store.go`
- `internal/board/store_test.go`
- `internal/board/sync.go`
- `internal/board/sync_test.go`
- `internal/board/task_test.go`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/git/git_test.go`
- `internal/gitignore/gitignore.go`
- `internal/gitignore/gitignore_test.go`
- `internal/ide/cli.go`
- `internal/ide/color.go`
- `internal/ide/color_test.go`
- `internal/ide/menu.go`
- `internal/ide/menu_test.go`
- `internal/ide/spawn.go`
- `internal/ide/spawn_test.go`
- `internal/ide/vscode.go`
- `internal/lock/lock.go`
- `internal/lock/lock_test.go`
- `internal/muxpoc/attach.go`
- `internal/muxpoc/cli.go`
- `internal/muxpoc/daemon.go`
- `internal/muxpoc/down.go`
- `internal/muxpoc/muxpoc_smoke_test.go`
- `internal/muxpoc/review.go`
- `internal/muxpoc/state.go`
- `internal/muxpoc/state_test.go`
- `internal/muxpoc/status.go`
- `internal/muxpoc/up.go`
- `internal/output/output_test.go`
- `internal/paths/enforcement_test.go`
- `internal/paths/paths.go`
- `internal/paths/paths_test.go`
- `internal/paths/worktreelist.go`
- `internal/paths/worktreelist_test.go`
- `internal/worktree/add.go`
- `internal/worktree/add_test.go`
- `internal/worktree/cli.go`
- `internal/worktree/cli_test.go`
- `internal/worktree/config.go`
- `internal/worktree/config_test.go`
- `internal/worktree/launchers.go`
- `internal/worktree/launchers_test.go`
- `internal/worktree/list.go`
- `internal/worktree/list_test.go`
- `internal/worktree/portals.go`
- `internal/worktree/portals_test.go`
- `internal/worktree/remove.go`
- `internal/worktree/remove_test.go`
- `internal/worktree/worktree.go`
- `mill-config.yaml`
