# Batch: configsync-fabric-repowide

```yaml
task: 'fabric: clone-does-everything + subpath-in-weft + init dissolution'
batch: configsync-fabric-repowide
number: 3
cards: 3
verify: go test ./internal/configsync/...
depends-on: []
```

## Batch Scope

This batch makes fabric's config repo-wide inside `configsync`. Today `configsync.ReconcileAll` materializes a **per-worktree** `fabric.yaml` (via `ConfigFile`) and runs the one-shot legacy `warp.yaml`/`weft.yaml`→`fabric.yaml` migration (`legacyFabricConfig`). Since fabric's config becomes a single repo-wide fact on `weft:main`, this batch: (1) drops fabric from the per-worktree `ReconcileAll` iteration set so a worktree no longer carries its own `fabric.yaml`, and (2) adds a targeted `ReconcileFabricAt(boardDir, apply)` that materializes the repo-wide `fabric.yaml` at `<boardDir>/_lyx/config/` carrying the `legacyFabricConfig` migration, which clone (batch 4) calls once against `BoardDir`.

External interface batch 4 consumes: `configsync.ReconcileFabricAt`. This batch is independent of batches 1/2 — it is pure config-materialization logic keyed on a `baseDir` path. Config ownership stays in `configsync`/`configengine`; clone just invokes the new function.

Batch-local decision: fabric is skipped in the general `ReconcileAll` loop by name (`m.Name == "fabric"`), keeping the alphabetical `configreg.Modules()` registration untouched (fabric stays a registered module for `configcli`/coverage — only its per-worktree materialization is removed).

## Cards

### Card 11: Drop fabric from per-worktree `ReconcileAll`

- **Context:**
  - `internal/configreg/configreg.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/configsync/configsync.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `ReconcileAll(baseDir string, apply bool) ([]Result, error)` (configsync.go:116), skip fabric in the per-module loop over `configreg.Modules()` (configsync.go:119): at the top of the loop body, `if m.Name == "fabric" { continue }` — a worktree base no longer materializes its own `fabric.yaml`, because `pathspec`/`branch_prefix` are now the repo-wide facts materialized by `ReconcileFabricAt` (card 12). Remove the now-dead fabric special-case at configsync.go:138-140 (`if fileAbsent && m.Name == "fabric" { existing, migratedFrom = legacyFabricConfig(baseDir) }`) since fabric never reaches that code after the skip — but KEEP `legacyFabricConfig` itself (card 12 relocates the migration call, not the function). Add a comment at the skip explaining fabric's config is repo-wide (materialized once at clone via `ReconcileFabricAt`), not per-worktree. (The initengine-referencing doc comment at configsync.go:99 describes the seed-only "created" heuristic, not fabric — leave it to batch 6 card 28's init-reference sweep, which rewords it after the init packages are deleted.)
- **Commit:** `refactor(configsync): drop fabric from per-worktree ReconcileAll`

### Card 12: Add repo-wide `ReconcileFabricAt` carrying the legacy migration

- **Context:**
  - `internal/configreg/configreg.go`
  - `internal/configengine/config.go`
  - `internal/fabricengine/template.yaml`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/configsync/configsync.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `configsync.go`, add `func ReconcileFabricAt(boardDir string, apply bool) (Result, error)` that materializes the single repo-wide `fabric.yaml` at `hubgeometry.ConfigFile(boardDir, "fabric")`:
  - resolve the fabric template via `configreg.Template("fabric")` ONLY — do NOT import `fabricengine.ConfigTemplate` directly. `configsync` must not import `fabricengine`: batch 4 (card 16) establishes `fabricengine -> configsync` (clone calls `configsync.ReconcileAll`/`ReconcileFabricAt`), so a `configsync -> fabricengine` edge would close an import cycle. `configreg.Template("fabric")` is the already-imported, cycle-safe accessor (`configreg` is the neutral registry `ReconcileAll` already uses);
  - read the existing file at `hubgeometry.ConfigFile(boardDir, "fabric")` (`fileAbsent` on `os.IsNotExist`);
  - when absent, seed migration input via `existing, migratedFrom = legacyFabricConfig(boardDir)` (the same one-shot warp/weft→fabric migration, now keyed on `boardDir`);
  - `merged := yamlengine.Reconcile([]byte(template), existing)` and, when `apply && (fileAbsent || hasChanges)`, `os.MkdirAll(hubgeometry.ConfigDir(boardDir), 0o755)` then `fsx.AtomicWriteBytes(cfgPath, merged)`, and prune migrated legacy files with `os.Remove(hubgeometry.ConfigFile(boardDir, legacy))` for each `legacy` in `migratedFrom` (mirroring configsync.go:199-204);
  - return a `Result` with `MigratedFrom: migratedFrom` and the applied/changes flags, matching `ReconcileAll`'s `Result` shape.
  Reuse existing helpers (`legacyFabricConfig`, `yamlengine.Reconcile`, `fsx.AtomicWriteBytes`) — do not reimplement them. This is the function clone calls once against `BoardDir`.
- **Commit:** `feat(configsync): add repo-wide ReconcileFabricAt with legacy migration`

### Card 13: Update configsync tests for the per-worktree→repo-wide split

- **Context:**
  - `internal/configsync/configsync.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/template.yaml`
- **Edits:**
  - `internal/configsync/configsync_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `internal/configsync/configsync_test.go`:
  - In `TestReconcileAll_ApplyCreatesFiles` (configsync_test.go:67), remove the assertions that `ReconcileAll` creates a per-worktree `fabric.yaml` and reports a fabric result (configsync_test.go:100-118) — assert instead that `ReconcileAll` returns NO fabric result and writes NO `fabric.yaml` under the per-worktree base (fabric is now skipped).
  - Move `TestReconcileAll_MigratesLegacyFabricConfig` (configsync_test.go:399) to target `ReconcileFabricAt(boardDir, apply)` instead of `ReconcileAll`: rename it to `TestReconcileFabricAt_MigratesLegacyFabricConfig` and route each of its five subtests through `ReconcileFabricAt`. The subtests keep their assertions (both-legacy-migrate-and-prune; warp-only-with-template-default-pathspec; dry-run-writes-nothing; pre-existing-fabric.yaml-untouched; unparseable-legacy-skipped) but now write `warp.yaml`/`weft.yaml` under the board base and assert the repo-wide `fabric.yaml` lands at `hubgeometry.ConfigFile(boardDir, "fabric")`.
  - Keep the other `ReconcileAll` subtests (`TestReconcileAll_DryRun`, `TestReconcileAll_DropsStaleReedClaudeKey`, `TestReconcileAll_Idempotent`, `TestReconcileAll_SeedOnly`) green — adjust only their fabric expectations if any assert a per-worktree fabric result.
- **Commit:** `test(configsync): cover fabric per-worktree drop and repo-wide ReconcileFabricAt`

## Batch Tests

`verify: go test ./internal/configsync/...` runs the `configsync` package (white-box `package configsync`, untagged — no `//go:build integration`, so no `-tags` needed). It covers the edited `configsync_test.go` in full: the flipped `TestReconcileAll_ApplyCreatesFiles`, the relocated `TestReconcileFabricAt_MigratesLegacyFabricConfig`, and the unchanged reconcile subtests. Scope is the single package this batch touches; the repo-wide `done_gate` catches any downstream caller (there are none until clone in batch 4).
