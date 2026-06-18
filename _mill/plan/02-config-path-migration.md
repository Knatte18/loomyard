# Batch: config-path-migration

```yaml
task: "Weft repo — companion-repo overlay for lyx"
batch: config-path-migration
number: 2
cards: 7
verify: go test ./internal/config/... ./internal/board/... ./internal/worktree/... ./cmd/...
depends-on: []
```

## Batch Scope

Migrate the config file path from `_lyx/<module>.yaml` to `_lyx/config/<module>.yaml`. The change has two production sites (`internal/config/config.go` and `internal/board/init.go`) and seven fixture-bearing test files that must be updated in lock-step. This is a hard cut — no fallback. `FindBaseDir` in `config.go` is unchanged (still probes for `_lyx/` existence). One new regression test is added to `config_test.go` to assert the old flat path is NOT picked up. The batch is self-contained and independent of batch 1 (rename) and batch 3 (docs).

## Cards

### Card 6: Migrate path in internal/config/config.go

- **Context:** none
- **Edits:**
  - `internal/config/config.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/config/config.go`, locate the `Load` function (or equivalent). The path construction that currently produces `filepath.Join(baseDir, "_lyx", module+".yaml")` must become `filepath.Join(baseDir, "_lyx", "config", module+".yaml")`. The `FindBaseDir` function checks for `_lyx/` directory existence — leave it unchanged. No other logic changes.
- **Commit:** `config: read YAML from _lyx/config/<module>.yaml`

### Card 7: Add _lyx/config/ mkdir and retarget WriteFile in board/init.go

- **Context:**
  - `internal/config/config.go`
- **Edits:**
  - `internal/board/init.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/init.go`, `RunInit` currently: (a) creates `_lyx/`; (b) writes `_lyx/board.yaml` (line ~61); (c) writes `_lyx/worktree.yaml` (line ~82). Change: (1) after the existing `_lyx/` mkdir step, add `os.MkdirAll(filepath.Join(lyxDir, "config"), 0755)` (where `lyxDir` is `init.go`'s existing variable for the `_lyx/` path — equivalently `filepath.Join(cwd, "_lyx", "config")`; use whichever local variable is in scope, with error handling matching the surrounding code); (2) retarget the first `os.WriteFile` from `_lyx/board.yaml` → `_lyx/config/board.yaml`; (3) retarget the second `os.WriteFile` from `_lyx/worktree.yaml` → `_lyx/config/worktree.yaml`; (4) update the file or package comment if it mentions `_lyx/board.yaml`. No other logic changes.
- **Commit:** `board: init scaffolds _lyx/config/ and writes configs there`

### Card 8: Update config_test.go fixtures and add regression test

- **Context:**
  - `internal/config/config.go`
- **Edits:**
  - `internal/config/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/config/config_test.go`: (1) update every test fixture that creates `_lyx/board.yaml` or `_lyx/worktree.yaml` to instead create `_lyx/config/board.yaml` or `_lyx/config/worktree.yaml` respectively — also ensure the `_lyx/config/` directory is created before the file write (use `os.MkdirAll` in each fixture helper or test setup); (2) add one new test case named `TestLoad_OldFlatPathNotPickedUp` (or equivalent) that: seeds a temp dir with `_lyx/` and `_lyx/board.yaml` (old path) but NOT `_lyx/config/board.yaml`, calls `config.Load(baseDir, "board", defaultCfg)`, and asserts that the returned config does NOT contain the values from the old flat-path file (i.e. the old path is silently ignored and defaults are returned).
- **Commit:** `config: update tests for _lyx/config/ path; add old-path regression guard`

### Card 9: Update board/init_test.go assertions

- **Context:**
  - `internal/board/init.go`
- **Edits:**
  - `internal/board/init_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/init_test.go`: (1) update all assertions that check for `_lyx/board.yaml` or `_lyx/worktree.yaml` file existence or content to instead check `_lyx/config/board.yaml` and `_lyx/config/worktree.yaml`; (2) add or update an assertion that the `_lyx/config/` directory exists after `RunInit` completes; (3) if the test seeds the directory tree, update the seed paths accordingly.
- **Commit:** `board: update init tests for _lyx/config/ paths`

### Card 10: Update board/config_test.go and board/cli_test.go fixtures

- **Context:**
  - `internal/config/config.go`
- **Edits:**
  - `internal/board/config_test.go`
  - `internal/board/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/config_test.go`: update every site (≥4) that writes `_lyx/board.yaml` → `_lyx/config/board.yaml`; ensure `_lyx/config/` directory is created in fixture setup. In `internal/board/cli_test.go`: the `seedCwd` helper (or equivalent) writes `_lyx/board.yaml` — change to `_lyx/config/board.yaml` and add `_lyx/config/` mkdir. No logic or assertion changes beyond the path update.
- **Commit:** `board: update config_test and cli_test fixtures for _lyx/config/ path`

### Card 11: Update boardtest benchmark and concurrency fixtures

- **Context:**
  - `internal/config/config.go`
- **Edits:**
  - `internal/board/boardtest/bench_test.go`
  - `internal/board/boardtest/concurrency_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/boardtest/bench_test.go`: find the `seedWiki` function fixture that constructs the `_lyx/board.yaml` path and change the path to `_lyx/config/board.yaml`; add `_lyx/config/` mkdir before the file write — this single change covers all callers. In `internal/board/boardtest/concurrency_test.go`: `seedWiki` is defined in `bench_test.go` and called 3 times from this file; there are no independent board.yaml path strings in this file — updating `bench_test.go` covers all 3 call sites automatically. No logic changes.
- **Commit:** `board: update boardtest fixtures for _lyx/config/ path`

### Card 12: Update worktree/config_test.go, cmd/lyx/main_test.go, and cmd/lyx/main.go comment

- **Context:**
  - `internal/config/config.go`
- **Edits:**
  - `internal/worktree/config_test.go`
  - `cmd/lyx/main_test.go`
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/worktree/config_test.go`: update fixture(s) that write `_lyx/worktree.yaml` → `_lyx/config/worktree.yaml`; add `_lyx/config/` mkdir in fixture setup. In `cmd/lyx/main_test.go`: update lines 43 and 73 (approximately) where `_lyx/board.yaml` is created → `_lyx/config/board.yaml`; add `_lyx/config/` mkdir before each write. In `cmd/lyx/main.go`: find the package-level comment at approximately line 13 that references `_lyx/board.yaml` and update the path to `_lyx/config/board.yaml`. No other changes to `cmd/lyx/main.go`.
- **Commit:** `worktree,cmd: update config fixtures and main.go comment for _lyx/config/ path`

## Batch Tests

`verify: go test ./internal/config/... ./internal/board/... ./internal/worktree/... ./cmd/...` covers all packages touched by this batch. The scope is justified because the config path change is not cross-cutting — only these four package subtrees contain fixture code that seeds the config YAML. `internal/paths` and `internal/ide` are untouched by this batch.
