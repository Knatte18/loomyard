# Plan: Rename Cobra modules to `<module>cli`, extract kernels as `<module>engine`

```yaml
task: "Rename Cobra modules to `<module>cli`, extract kernels as `<module>engine`"
slug: "cobra-cli-engine-sweep"
approved: false
started: "20260629-115456"
parent: "main"
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
    name: board split
    file: 01-board-split.md
    depends-on: []
    verify: "go build ./... && go test ./... && go test -tags integration ./..."
  - number: 2
    name: weft split
    file: 02-weft-split.md
    depends-on: [1]
    verify: "go build ./... && go test ./... && go test -tags integration ./..."
  - number: 3
    name: warp split
    file: 03-warp-split.md
    depends-on: [2]
    verify: "go build ./... && go test ./... && go test -tags integration ./..."
  - number: 4
    name: ide split
    file: 04-ide-split.md
    depends-on: [3]
    verify: "go build ./... && go test ./... && go test -tags integration ./..."
  - number: 5
    name: ghissues split
    file: 05-ghissues-split.md
    depends-on: [4]
    verify: "go build ./... && go test ./... && go test -tags integration ./..."
  - number: 6
    name: muxpoc rename
    file: 06-muxpoc-rename.md
    depends-on: [5]
    verify: "go build ./... && go test ./... && go test -tags integration ./..."
  - number: 7
    name: update fold to config reconcile
    file: 07-update-fold.md
    depends-on: [6]
    verify: "go build ./... && go test ./... && go test -tags integration ./..."
  - number: 8
    name: constraints docs guards and comment sweep
    file: 08-constraints-docs-guards.md
    depends-on: [7]
    verify: "go build ./... && go test ./... && go test -tags integration ./..."
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: cli/engine package split

- **Decision:** Each split module becomes **two new directories**, `internal/<module>cli`
  (package `<module>cli`) and `internal/<module>engine` (package `<module>engine`); the
  old `internal/<module>` directory is deleted. Directory name == package name. The
  **cli** package owns everything that exists because of the command line —
  `Command() *cobra.Command`, the `RunCLI(out io.Writer, args []string) int` seam, Cobra
  subcommands, flags, `Short`/`Long`, `PersistentPreRunE`, exit-code handling. The
  **engine** package owns the domain kernel — types and operations returning `(T, error)`,
  with no Cobra, no `io.Writer`-for-output, no exit codes.
- **Rationale:** Matches existing precedent (`internal/configengine`, `internal/yamlengine`).
- **Applies to:** board, weft, warp, ide, ghissues split batches (1–5).

### Decision: dependency direction

- **Decision:** **cli imports engine**; **engine → engine is allowed** (e.g. `ideengine`
  imports `boardengine`); **engine must never import a `cli` package or cobra.** When a
  cli-half caller needs an engine symbol, export that symbol (never the reverse).
- **Rationale:** Keeps the import graph acyclic, one-directional, and the kernel
  loom-consumable.
- **Applies to:** all split batches.

### Decision: behaviour-preserving sweep

- **Decision:** No behaviour changes other than the single observable CLI change
  (`lyx update` → `lyx config reconcile`, batch 7). `Use:` command names DO NOT change —
  only Go package and directory names. No backward-compat alias for `update`. No
  opportunistic refactors, dead-code cleanup, or behaviour tweaks beyond what the rename
  mechanically requires. Every existing test assertion is preserved; relocated test files
  are the behaviour-preservation guard.
- **Rationale:** This is a rename/extraction sweep, not a feature change.
- **Applies to:** all batches.

### Decision: build + test green after every batch

- **Decision:** Every batch must leave `go build ./...`, `go test ./...` (Tier 1), and
  `go test -tags integration ./...` (Tier 2) green — not only at the end. Each batch
  therefore retargets every cross-module importer it affects (`cmd/lyx/main.go`,
  `internal/configreg`, and any feature consumer) in the same batch.
- **Rationale:** Many relocated `_test.go` files are integration-tagged; a compile break
  in them is invisible to plain `go test ./...` and must be caught at the introducing
  batch via the `-tags integration` run (Tier 2 ≈ 65 s; Tier 1 ≈ 3.5 s — both cheap
  enough to run per round). See each batch's `## Batch Tests`.
- **Applies to:** all batches.

### Decision: config module names are unchanged

- **Decision:** The config module *identifiers* in `configreg.Modules()` stay the literal
  strings `"board"`, `"warp"`, `"weft"`; only the Go package the `ConfigTemplate` function
  lives in moves (to `*engine`). `lyx config board` and the on-disk `board.yaml` are
  unaffected.
- **Rationale:** Config module names are a user/file contract, not a package name.
- **Applies to:** batches 1–3, 7.

## All Files Touched

_Full union of every `Creates:` / `Edits:` across every batch, sorted
alphabetically. mill-go reads this to warn if two parallel batches touch the same file._

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/main_test.go`
- `cmd/lyx/unknown_subcommand_test.go`
- `cmd/testtiming/main.go`
- `docs/benchmarks/board-performance.md`
- `docs/benchmarks/running-tests.md`
- `docs/modules/README.md`
- `docs/modules/mux.md`
- `docs/overview.md`
- `docs/roadmap.md`
- `docs/sandbox-hub.md`
- `docs/shared-libs/configengine.md`
- `internal/boardcli/cli.go`
- `internal/boardcli/cli_test.go`
- `internal/boardcli/help_test.go`
- `internal/boardcli/skipenv_internal_test.go`
- `internal/boardengine/board.go`
- `internal/boardengine/board_test.go`
- `internal/boardengine/boardtest/bench_test.go`
- `internal/boardengine/boardtest/concurrency_test.go`
- `internal/boardengine/boardtest/doc.go`
- `internal/boardengine/boardtest/git_test.go`
- `internal/boardengine/boardtest/sync_test.go`
- `internal/boardengine/config.go`
- `internal/boardengine/config_test.go`
- `internal/boardengine/git.go`
- `internal/boardengine/layer.go`
- `internal/boardengine/layer_test.go`
- `internal/boardengine/render.go`
- `internal/boardengine/render_test.go`
- `internal/boardengine/spawn.go`
- `internal/boardengine/store.go`
- `internal/boardengine/store_test.go`
- `internal/boardengine/sync.go`
- `internal/boardengine/task.go`
- `internal/boardengine/task_test.go`
- `internal/boardengine/template.go`
- `internal/boardengine/template.yaml`
- `internal/boardengine/template_test.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configcli/reconcile_test.go`
- `internal/configengine/config.go`
- `internal/configengine/config_test.go`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/ghissuescli/cli.go`
- `internal/ghissuescli/cli_test.go`
- `internal/ghissuesengine/ghissues.go`
- `internal/ide/menu.go`
- `internal/idecli/cli.go`
- `internal/idecli/cli_test.go`
- `internal/ideengine/menu.go`
- `internal/ideengine/menu_test.go`
- `internal/ideengine/spawn.go`
- `internal/ideengine/spawn_test.go`
- `internal/initcli/initcli.go`
- `internal/initcli/initcli_test.go`
- `internal/lyxtest/doc.go`
- `internal/lyxtest/leaf_enforcement_test.go`
- `internal/muxpoccli/attach.go`
- `internal/muxpoccli/cli.go`
- `internal/muxpoccli/cli_test.go`
- `internal/muxpoccli/cmd.go`
- `internal/muxpoccli/cmd_test.go`
- `internal/muxpoccli/daemon.go`
- `internal/muxpoccli/down.go`
- `internal/muxpoccli/muxpoc_smoke_test.go`
- `internal/muxpoccli/review.go`
- `internal/muxpoccli/spawnattach_other.go`
- `internal/muxpoccli/spawnattach_windows.go`
- `internal/muxpoccli/state.go`
- `internal/muxpoccli/state_test.go`
- `internal/muxpoccli/status.go`
- `internal/muxpoccli/up.go`
- `internal/paths/paths.go`
- `internal/warpcli/clone_cli_test.go`
- `internal/warpcli/clone.go`
- `internal/warpcli/warp.go`
- `internal/warpcli/warp_test.go`
- `internal/warpengine/add.go`
- `internal/warpengine/add_test.go`
- `internal/warpengine/ancestors.go`
- `internal/warpengine/ancestors_test.go`
- `internal/warpengine/checkout.go`
- `internal/warpengine/checkout_test.go`
- `internal/warpengine/cleanup.go`
- `internal/warpengine/cleanup_test.go`
- `internal/warpengine/clone.go`
- `internal/warpengine/clone_integration_test.go`
- `internal/warpengine/clone_test.go`
- `internal/warpengine/config.go`
- `internal/warpengine/config_test.go`
- `internal/warpengine/drift.go`
- `internal/warpengine/drift_test.go`
- `internal/warpengine/hook.go`
- `internal/warpengine/hook_test.go`
- `internal/warpengine/junction.go`
- `internal/warpengine/launchers.go`
- `internal/warpengine/launchers_test.go`
- `internal/warpengine/list.go`
- `internal/warpengine/list_test.go`
- `internal/warpengine/portals.go`
- `internal/warpengine/portals_test.go`
- `internal/warpengine/post-checkout.sh`
- `internal/warpengine/prune.go`
- `internal/warpengine/prune_test.go`
- `internal/warpengine/reconcile.go`
- `internal/warpengine/reconcile_test.go`
- `internal/warpengine/remove.go`
- `internal/warpengine/remove_test.go`
- `internal/warpengine/status.go`
- `internal/warpengine/status_test.go`
- `internal/warpengine/template.go`
- `internal/warpengine/template.yaml`
- `internal/warpengine/template_test.go`
- `internal/warpengine/weftwiring.go`
- `internal/warpengine/weftwiring_test.go`
- `internal/warpengine/worktreelifecycle.go`
- `internal/weftcli/cli.go`
- `internal/weftcli/cli_test.go`
- `internal/weftcli/spawn.go`
- `internal/weftengine/config.go`
- `internal/weftengine/config_test.go`
- `internal/weftengine/status.go`
- `internal/weftengine/status_test.go`
- `internal/weftengine/sync.go`
- `internal/weftengine/sync_test.go`
- `internal/weftengine/template.go`
- `internal/weftengine/template.yaml`
- `internal/weftengine/template_test.go`
- `internal/weftengine/weft.go`
- `internal/weftengine/weft_integration_test.go`
- `tools/sandbox/main.go`
