# Batch: board-paths-owned

```yaml
task: 'Harden the Path Invariant: close enforcement hole + fix geometry leaks'
batch: board-paths-owned
number: 3
cards: 5
verify: go test ./internal/boardengine/... ./internal/boardcli/... ./internal/configsync/...
depends-on: [1]
```

## Batch Scope

Makes the board data dir (`<hub>/_board`) paths-owned instead of config/env-resolved. Removes the
`path:` key from the board template, drops the corresponding resolution from
`boardengine.LoadConfig`, and rewires `boardcli` to resolve the data dir as `--board-path` flag
(transient) > `paths.BoardDir(l.Hub)` while surfacing any `paths.Resolve` error. The non-geometry
template keys (`home`, `sidebar`, `proposal_prefix`) and their `${env:NAME:-default}` form are
left untouched. Because `path:` leaves the template, `yamlengine.Reconcile` now treats a
committed `path:` as a removable extra key, which breaks one `configsync` test assertion — fixed
here so the change is atomic. Depends only on batch 1; shares no edited files with batches 2 or 4.

## Cards

### Card 12: Remove the path: key from the board template

- **Context:**
  - `internal/boardengine/template.go`
- **Edits:**
  - `internal/boardengine/template.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete only the first line, `path: ${env:LYX_BOARD_PATH:-../_board}  # ...`.
  Leave `home:`, `sidebar:`, and `proposal_prefix:` lines exactly as they are (keep their
  `${env:LYX_*:-default}` form — they are non-geometry filenames with optional overrides, not in
  scope). The template now has no geometry key.
- **Commit:** `refactor(board): remove geometry path key from board config template`

### Card 13: Drop path resolution from boardengine.LoadConfig

- **Context:**
  - `internal/boardengine/board.go`
  - `internal/boardengine/template.yaml`
- **Edits:**
  - `internal/boardengine/config.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `Config` struct change the `Path` field's tag to `yaml:"-"` so yaml.v3
  does not map a leftover `path:` key onto it (an untagged `Path` would still match `path:` because
  yaml.v3 lowercases field names — `yaml:"-"` is the real exclusion; the cli-overwrite in Card 14
  is the functional guarantee either way). Keep the field — it is now cli-populated like
  `SkipGit`/`SkipPush`; restate its doc comment to say it is set by the caller, not the config
  file. In `LoadConfig`, delete the relative-path resolution block
  (`if !filepath.IsAbs(cfg.Path) { cfg.Path = filepath.Join(baseDir, cfg.Path) }`) AND update the
  `LoadConfig` function godoc (the "Preserves relative-Path resolution …" sentence, ~lines 53–56)
  to state that `LoadConfig` no longer resolves a data-dir path. Remove the `path/filepath` import
  if it becomes unused after the deletion. `LoadConfig` now returns only
  `home`/`sidebar`/`proposal_prefix`; `Path` stays zero from this function.
- **Commit:** `refactor(board): stop resolving data-dir path in LoadConfig`

### Card 14: Resolve board data dir via paths in boardcli

- **Context:**
  - `internal/boardengine/config.go`
  - `internal/boardengine/board.go`
  - `internal/paths/paths.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/boardcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Command()`'s `PersistentPreRunE`, leave the `--board-path` branch
  unchanged (`cfg = boardengine.Config{Path: *boardPathFlag}`). In the normal branch, after
  `cfg, err = boardengine.LoadConfig(cwd, "board")` succeeds, resolve the layout:
  `layout, rerr := paths.Resolve(cwd)`; on `rerr != nil` emit it through the JSON envelope exactly
  like the `LoadConfig` failure path (`output.Err(cmd.OutOrStdout(), rerr.Error())` +
  `clihelp.Abort(ctx, 1)` + `return nil`) — never discard it. On success set
  `cfg.Path = paths.BoardDir(layout.Hub)`. Then reword the command `Long` (currently
  "Configuration is resolved from the current working directory via _lyx/config/board.yaml.") so
  it states: the config *file* (home/sidebar/proposal_prefix) resolves from the cwd via
  `_lyx/config/board.yaml`; the board *data dir* lives at `<hub>/_board`, derived via `paths`
  (not config- or env-overridable); the hidden `--board-path` flag overrides the data dir for the
  detached sync child. Update the file-header doc comment (lines ~1–8) to match. Keep the bare
  `lyx board` group guard (`if cmd.Name() == "board" { return nil }`) so listing needs no git repo.
- **Commit:** `refactor(boardcli): resolve board data dir via paths.BoardDir`

### Card 15: Update board engine/cli tests for paths-owned data dir

- **Context:**
  - `internal/boardengine/template.go`
  - `internal/boardengine/config.go`
  - `internal/boardcli/cli.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/boardengine/template_test.go`
  - `internal/boardengine/config_test.go`
  - `internal/boardcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `template_test.go`: drop `path` from the expected-required-keys set
  (`TestConfigTemplate_HasRequiredKeys`) and remove the `{"path", "../_board"}` row from
  `TestConfigTemplate_ResolvesToDefaults`. `config_test.go`: **repurpose** `TestLoadConfig_HappyPath`
  (~line 36) — remove only its `path:` seed line and the path-suffix assertion (~lines 51–52), but
  KEEP its `home`/`sidebar`/`proposal_prefix` and env-resolution coverage (do not delete the test).
  **Delete** the now-obsolete pure path-resolution cases — absolute (~line 83), `../custom_board`
  (~line 119), and the `${env:TEST_BOARD_PATH}` data-dir env case (~line 158) — since `LoadConfig`
  no longer resolves a data-dir path. `cli_test.go`: strip the stale `path:` seed line from any
  board-config fixture (e.g. `seedCwd`, ~line 40 `path: board`) and fix any "all template keys"
  comment that referenced it; then add a test that with no `--board-path`, the resolved `cfg.Path`
  equals `paths.BoardDir(hub)` for a fixture worktree, and that `--board-path <abs>` overrides it.
  Fixtures that build `boardengine.Config{Path: ...}` directly (in `boardengine` tests) keep their
  explicit `Path` — that is a struct field, not a config seed.
- **Commit:** `test(board): update template/config/cli tests for paths-owned data dir`

### Card 16: Fix configsync test for stripped path key

- **Context:**
  - `internal/configsync/configsync.go`
  - `internal/boardengine/template.yaml`
  - `internal/yamlengine/reconcile.go`
- **Edits:**
  - `internal/configsync/configsync_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** With `path:` gone from the board template, `yamlengine.Reconcile` now reports a
  seeded `path: board` as a *removed* (extra) key. In `TestReconcileAll_ApplyCreatesFiles` the
  assertion at ~lines 127–129 ("board.yaml missing user value 'path: board' after apply; should be
  preserved") is now false — replace it: either assert `path` is removed after apply, or change the
  seed's "preserved user value" probe to a key that still exists in the template (e.g. a custom
  `home:` value). In `TestReconcileAll_DryRunDetectsChanges` confirm the `Added`/`Removed`-non-empty
  and stale_key-still-present assertions still hold (they do — `home`/`sidebar`/`proposal_prefix`
  are still added, `stale_key`/`path` are removed). Keep the test's intent (reconcile adds missing
  template keys and removes extras) intact.
- **Commit:** `test(configsync): adjust board reconcile assertions for removed path key`

## Batch Tests

`verify: go test ./internal/boardengine/... ./internal/boardcli/... ./internal/configsync/...`
covers all three packages whose behaviour or tests this batch changes. `boardengine` includes the
`boardtest` fixtures (`concurrency_test.go`, `bench_test.go`, `board_test.go`) that build
`Config{Path: ...}` directly — they must stay green unchanged. `boardcli` covers the new
resolution test (Card 14/15). `configsync` covers the reconcile assertion fix (Card 16). `initcli`
is intentionally excluded: its idempotency test reads the template content twice and compares, so
removing `path:` does not break it (verified during planning). The cross-package backstop is
batch 5's repo-wide `go test ./...`.
