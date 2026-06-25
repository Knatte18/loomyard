# Plan: Introduce warp: the host↔weft-coordinated git module

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
slug: warp-module
approved: false
started: 20260625-120650
parent: main
root: ""
verify: go build ./...
```

## Batch Index

```yaml
batches:
  - number: 1
    name: gitexec-rename
    file: 01-gitexec-rename.md
    depends-on: []
    verify: go build ./... && go test ./internal/gitexec/ ./internal/board/ ./internal/weft/ ./internal/paths/
  - number: 2
    name: warp-clone-fold
    file: 02-warp-clone-fold.md
    depends-on: [1]
    verify: go build ./... && go test -tags integration ./internal/warp/ ./cmd/lyx/
  - number: 3
    name: warp-worktree-absorb
    file: 03-warp-worktree-absorb.md
    depends-on: [2]
    verify: go build ./... && go test -tags integration ./internal/warp/ ./internal/configreg/ ./internal/configcli/ ./internal/initcli/ ./internal/lyxtest/ ./cmd/lyx/
  - number: 4
    name: topology-primitives-activation
    file: 04-topology-primitives-activation.md
    depends-on: [3]
    verify: go build ./... && go test -tags integration ./internal/warp/ ./internal/initcli/ ./internal/configcli/
  - number: 5
    name: coordinated-checkout
    file: 05-coordinated-checkout.md
    depends-on: [4]
    verify: go build ./... && go test -tags integration ./internal/warp/
  - number: 6
    name: status-reconcile-pollution
    file: 06-status-reconcile-pollution.md
    depends-on: [5]
    verify: go build ./... && go test -tags integration ./internal/warp/ ./internal/weft/
  - number: 7
    name: prune-cleanup
    file: 07-prune-cleanup.md
    depends-on: [6]
    verify: go build ./... && go test -tags integration ./internal/warp/
  - number: 8
    name: hook-and-launcher
    file: 08-hook-and-launcher.md
    depends-on: [7]
    verify: go build ./... && go test -tags integration ./internal/warp/
  - number: 9
    name: docs-finalize
    file: 09-docs-finalize.md
    depends-on: [8]
    verify: go vet -tags integration ./... && go test -tags integration ./...
```

## Shared Decisions

### Decision: behaviour-preserving move first, features after

- **Decision:** Batches 1–3 are a pure consolidation (rename `internal/git`→`internal/gitexec`; fold `internal/gitclone` and `internal/worktree` into a new `internal/warp` package; rename the config module `worktree`→`warp`). They change **no behaviour** — the existing test suite, moved verbatim, is the guardrail. New verbs (checkout, reconcile, cleanup, prune), the junction relocation, drift detection, the host-pollution guard, the post-checkout hook, and the launcher shortcut land in batches 4–8 on top of the consolidated package.
- **Rationale:** Principle 5 (incremental, behaviour-preserving refactors with the test suite as guardrail). Keeps each batch reviewable and the build green at every batch boundary.
- **Applies to:** all batches

### Decision: one internal/warp package, files split by verb/concern

- **Decision:** All warp code lives in one package `internal/warp` (Go convention: a module's domain logic + tests in one package, like `board`/`weft`). Files are split by concern: `warp.go` (facade + `RunCLI` string-switch dispatch), `clone.go`, `add.go`, `remove.go`, `list.go`, `checkout.go`, `reconcile.go`, `cleanup.go`, `status.go`, `prune.go` (the verb), `junction.go`, `drift.go`, `hook.go`, `launchers.go`, `portals.go`, `weftwiring.go`, `worktreelifecycle.go`, `ancestors.go` (the empty-dir sweeper helper, renamed from `worktree/prune.go` to avoid colliding with the `prune` verb), `config.go`, `template.go`, `template.yaml`.
- **Rationale:** Matches existing module structure; `worktree`+`gitclone` collapse cleanly.
- **Applies to:** all batches

### Decision: hand-rolled RunCLI dispatch (no cobra)

- **Decision:** `warp.RunCLI(out io.Writer, args []string) int` routes its subcommand with an internal `switch` exactly like every other module. Output is JSON via `internal/output.Ok(w, map[string]any) int` / `output.Err(w, msg) int`; exit 1 on error. Every feature batch that adds a verb edits the single `RunCLI` switch in `warp.go` — this is why batches 4–8 are a serial chain (they share `warp.go`).
- **Rationale:** Repo-wide CLI convention; there is no cobra anywhere.
- **Applies to:** all batches

### Decision: gitexec leaf signature unchanged

- **Decision:** `gitexec.RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)` keeps the exact 4-tuple of `internal/git.RunGit` — non-zero git exit is returned in `exitCode` with `err == nil`; only a spawn failure sets `err` + `exitCode == -1`. Only the package name and import path change.
- **Rationale:** Behaviour-preserving rename; every caller relies on this convention.
- **Applies to:** gitexec-rename, all batches that call git

### Decision: dependency direction — warp never imports the config layer

- **Decision:** `internal/warp` must NOT import `internal/initcli` or `internal/configsync`. Activation is the reverse: `internal/initcli` imports `internal/warp` and calls warp's junction primitive, then `configsync.ReconcileAll`. `internal/configreg` imports `internal/warp` for the config template (like it imports `board`/`weft`); `warp` does not import `configreg`.
- **Rationale:** Content-vs-topology layering from the design; prevents an import cycle and keeps topology below config.
- **Applies to:** topology-primitives-activation, all batches

### Decision: coordinated operations are all-or-nothing

- **Decision:** `warp checkout` (and `warp add`) precondition-check first and roll back the host side if the weft side fails — the pair is always consistent or untouched, never half-switched. Reuse the existing `rollbackAdd` discipline (junction removed before weft teardown for the Windows junction-lock hazard).
- **Rationale:** The correctness gap that motivated the module.
- **Applies to:** topology-primitives-activation, coordinated-checkout

### Decision: Go test verify, module-wide build gate

- **Decision:** Per-batch `verify:` uses the native Go runner scoped to the touched packages (no `PYTHONPATH=` prefix — that is a Python-only rule). The overview-level `verify: go build ./...` is the module-wide compile gate run at each batch boundary to catch cross-package regressions from the rename/move.
- **Rationale:** Go project; `go build ./...` is the cheap whole-tree compile check.
- **Applies to:** all batches

## All Files Touched

- `cmd/lyx/main.go`
- `cmd/lyx/main_test.go`
- `docs/modules/README.md`
- `docs/modules/loom.md`
- `docs/overview.md`
- `docs/shared-libs/README.md`
- `docs/shared-libs/paths.md`
- `internal/board/git.go`
- `internal/board/sync.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configcli/configcli_test.go`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/gitclone/clone.go`
- `internal/gitexec/gitexec.go`
- `internal/gitexec/gitexec_test.go`
- `internal/initcli/initcli.go`
- `internal/initcli/initcli_test.go`
- `internal/lyxtest/leaf_enforcement_test.go`
- `internal/paths/paths.go`
- `internal/paths/worktreelist.go`
- `internal/update/update_test.go`
- `internal/warp/add.go`
- `internal/warp/add_test.go`
- `internal/warp/ancestors.go`
- `internal/warp/ancestors_test.go`
- `internal/warp/cleanup.go`
- `internal/warp/cleanup_test.go`
- `internal/warp/clone.go`
- `internal/warp/clone_integration_test.go`
- `internal/warp/clone_test.go`
- `internal/warp/checkout.go`
- `internal/warp/checkout_test.go`
- `internal/warp/config.go`
- `internal/warp/config_test.go`
- `internal/warp/drift.go`
- `internal/warp/drift_test.go`
- `internal/warp/hook.go`
- `internal/warp/hook_test.go`
- `internal/warp/junction.go`
- `internal/warp/launchers.go`
- `internal/warp/launchers_test.go`
- `internal/warp/list.go`
- `internal/warp/list_test.go`
- `internal/warp/portals.go`
- `internal/warp/portals_test.go`
- `internal/warp/post-checkout.sh`
- `internal/warp/prune.go`
- `internal/warp/prune_test.go`
- `internal/warp/reconcile.go`
- `internal/warp/reconcile_test.go`
- `internal/warp/remove.go`
- `internal/warp/remove_test.go`
- `internal/warp/status.go`
- `internal/warp/status_test.go`
- `internal/warp/template.go`
- `internal/warp/template.yaml`
- `internal/warp/template_test.go`
- `internal/warp/warp.go`
- `internal/warp/warp_test.go`
- `internal/warp/weftwiring.go`
- `internal/warp/weftwiring_test.go`
- `internal/warp/worktreelifecycle.go`
- `internal/weft/cli.go`
- `internal/weft/status.go`
- `internal/weft/status_test.go`
- `internal/weft/sync.go`
- `internal/worktree/add.go`
- `internal/worktree/add_test.go`
- `internal/worktree/remove.go`
- `internal/worktree/weft.go`
- `internal/worktree/weft_test.go`
