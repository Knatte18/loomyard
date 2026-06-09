# Batch: board migration — adopt all three packages

```yaml
task: "Extract shared infrastructure (config, git, lock)"
batch: "board migration — adopt all three packages"
number: 4
cards: 8
verify: go test ./...
depends-on: [1, 2, 3]
```

## Batch Scope

This batch wires board to the three new packages and deletes the board-internal code that has been lifted out. Cards apply in order: lock import-site updates (9–11), RunGit removal and git.RunGit adoption (12), hideProcWindow removal from spawn files (13–14), deletion of board/lock.go and board/lock_test.go (15), and config rewrite (16). By the time card 15 deletes `lock.go`, all call sites have already been updated to `lock.*` prefix, so the board package compiles throughout.

The external batch-4 interface produced for other modules is unchanged: `board.LoadConfig`, `board.RunCLI`, `board.RunInit`, `board.Config`, and all other exported board symbols remain identical in signature and behaviour.

## Cards

### Card 9: Update internal/board/board.go — lock import

- **Context:**
  - `internal/lock/lock.go`
- **Edits:**
  - `internal/board/board.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `flock "github.com/Knatte18/mhgo/internal/lock"` to the import block of `internal/board/board.go` (use the alias `flock` to avoid shadowing the local `lock` variable used in `writeOp`). Grep for `AcquireWriteLock` in this file and replace each unqualified call with `flock.AcquireWriteLock(...)`. Replace any `FileLock` type references with `flock.FileLock`. Note: `Release` is a method on `*flock.FileLock` — existing `defer lock.Release()` call sites require no transformation. No other changes.
- **Commit:** `refactor(board): use internal/lock in board.go`

### Card 10: Update internal/board/store.go — lock imports

- **Context:**
  - `internal/lock/lock.go`
- **Edits:**
  - `internal/board/store.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `flock "github.com/Knatte18/mhgo/internal/lock"` to the import block of `internal/board/store.go` (alias `flock` to avoid shadowing any local `lock` variable). Grep for `AcquireReadLock`, `AcquireWriteLock`, and `FileLock` in this file. Replace each unqualified occurrence: `AcquireReadLock(...)` → `flock.AcquireReadLock(...)`, `AcquireWriteLock(...)` → `flock.AcquireWriteLock(...)`, `FileLock` → `flock.FileLock`. Note: `Release` is a method on `*flock.FileLock` — existing `.Release()` method call sites need no transformation. No other changes.
- **Commit:** `refactor(board): use internal/lock in store.go`

### Card 11: Update internal/board/sync.go — git and lock imports

- **Context:**
  - `internal/git/git.go`
  - `internal/lock/lock.go`
- **Edits:**
  - `internal/board/sync.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `"github.com/Knatte18/mhgo/internal/git"` and `flock "github.com/Knatte18/mhgo/internal/lock"` to the import block of `internal/board/sync.go` (alias `flock` to avoid shadowing local `lock` variables). Grep for `RunGit`, `AcquireWriteLock`, and `FileLock` in this file. Replace each: `RunGit(...)` → `git.RunGit(...)`, `AcquireWriteLock(...)` → `flock.AcquireWriteLock(...)`, `FileLock` → `flock.FileLock`. Note: `Release` is a method on `*flock.FileLock` — existing `.Release()` method call sites need no transformation. No other changes.
- **Commit:** `refactor(board): use internal/git and internal/lock in sync.go`

### Card 12: Update internal/board/git.go — remove RunGit, adopt git.RunGit

- **Context:**
  - `internal/git/git.go`
- **Edits:**
  - `internal/board/git.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `"github.com/Knatte18/mhgo/internal/git"` to the import block of `internal/board/git.go`. Remove the `RunGit` function definition from this file (it is now in `internal/git`). Grep for `RunGit(` calls within `Pull` and `CommitPush` and replace each with `git.RunGit(`. All other functions (`PathGuard`, `AtomicWrite`, `BoardPathError`, `BoardPushError`, and their methods) are unchanged. After removing `RunGit`, check whether any imports become unused (e.g. `bytes`, `os/exec`) and remove them if so — `git.go` no longer needs those directly.
- **Commit:** `refactor(board): remove RunGit from board/git.go, use internal/git`

### Card 13: Update internal/board/spawn_windows.go — remove hideProcWindow

- **Context:** none
- **Edits:**
  - `internal/board/spawn_windows.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Remove the `hideProcWindow(cmd *exec.Cmd)` function definition from `spawn_windows.go`. Keep `spawnSync`, `createNoWindow`, and `createNewProcessGroup` — `spawnSync` uses both constants and must remain unchanged. Do NOT add a `//go:build windows` tag; the filename suffix is the build constraint. After removing `hideProcWindow`, verify no imports become unused. If `os/exec` was shared with `hideProcWindow` but `spawnSync` also uses it, the import stays; if any import is now unused, remove it.
- **Commit:** `refactor(board): remove hideProcWindow from spawn_windows.go`

### Card 14: Update internal/board/spawn_other.go — remove hideProcWindow no-op

- **Context:** none
- **Edits:**
  - `internal/board/spawn_other.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Remove the no-op `hideProcWindow(cmd *exec.Cmd) {}` function from `spawn_other.go`. Keep the `//go:build !windows` tag at the top of the file. Keep `spawnSync` unchanged. After removing `hideProcWindow`, verify no imports become unused; remove any that are.
- **Commit:** `refactor(board): remove hideProcWindow no-op from spawn_other.go`

### Card 15: Delete internal/board/lock.go and internal/board/lock_test.go

- **Context:** none
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/board/lock.go`
  - `internal/board/lock_test.go`
- **Requirements:** Delete both files. By this point cards 9–11 have updated every call site in board to use `lock.*`-qualified names from `internal/lock`, so removing `lock.go` leaves the board package in a compilable state. `lock_test.go` is also deleted because its tests have been ported to `internal/lock/lock_test.go` in batch 1. Run `go build ./internal/board/...` after deletion to confirm the package still compiles before proceeding.
- **Commit:** `refactor(board): delete board/lock.go (lifted to internal/lock)`

### Card 16: Rewrite board/config.go, trim board/config_test.go, update board/init.go

- **Context:**
  - `internal/config/config.go`
- **Edits:**
  - `internal/board/config.go`
  - `internal/board/config_test.go`
  - `internal/board/init.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**

  **`internal/board/config.go`:**
  - Add import `"github.com/Knatte18/mhgo/internal/config"`.
  - Remove the `expandEnv` function, the `envTokenRe` package-level var, and all `.mhgo/` layer loading logic.
  - Rewrite `LoadConfig(baseDir, module string) (Config, error)` as a thin wrapper:
    1. Build `defaults := map[string]string{"path": DefaultConfig().Path, "home": DefaultConfig().Home, "sidebar": DefaultConfig().Sidebar, "proposal_prefix": DefaultConfig().ProposalPrefix}`.
    2. Call `raw, err := config.Load(baseDir, module, defaults)`. Return on error.
    3. Map to typed struct: `cfg := Config{Path: raw["path"], Home: raw["home"], Sidebar: raw["sidebar"], ProposalPrefix: raw["proposal_prefix"]}`.
    4. Resolve relative path: `if !filepath.IsAbs(cfg.Path) { cfg.Path = filepath.Join(baseDir, cfg.Path) }`.
    5. Return `cfg, nil`.
  - `DefaultConfig()`, `Config`, `Outputs`, `DefaultOutputs()`, and any other unexported helpers remain unchanged.
  - Remove any imports that become unused after the cleanup.

  **`internal/board/config_test.go`:**
  - Delete the following test functions: `TestDeepMergeMultipleLayers`, and any `TestEnvExpansion*` functions (`TestEnvExpansionWholeValue`, `TestEnvExpansionEmbedded`, `TestEnvExpansionUnsetError`, or similar names). These are now covered by `internal/config/config_test.go`.
  - Keep all remaining tests: `TestDefaultsReturned`, `TestErrorNotInitialized`, `TestRelativePathResolution`, `TestAbsolutePathPassthrough`, `TestMalformedYAMLError`, `TestOutputsFromConfig`, `TestDefaultOutputs`.
  - Add new test `TestLoadConfig_FallbackPathResolution`: create `<tmpDir>/_mhgo/board.yaml` with content `path: $env:NONEXISTENT_MHGO_TEST_VAR_XYZ ? ../_board`. Use an env var name that cannot realistically be set in CI (`NONEXISTENT_MHGO_TEST_VAR_XYZ` is sufficient; do not use `os.Unsetenv` — that mutates global state). Call `board.LoadConfig(tmpDir, "board")`. Assert no error and that the returned `Config.Path` equals `filepath.Join(tmpDir, "../_board")` (which filepath.Join will clean to a sibling directory named `_board`).

  **`internal/board/init.go`:**
  - Change the signature of `generateCommentedBoardYAML` from `generateCommentedBoardYAML(defaults Config) string` to `generateCommentedBoardYAML() string` (drop the parameter).
  - Replace the function body's per-key comment lines with the four static literals (exact strings from discussion):
    ```
    # path: $env:MHGO_BOARD_PATH ? ../_board   # board dir (tasks.json + rendered output); relative to cwd or absolute
    # home: $env:MHGO_HOME ? Home.md           # home page file name; relative to board dir
    # sidebar: $env:MHGO_SIDEBAR ? _Sidebar.md   # sidebar file name; relative to board dir
    # proposal_prefix: $env:MHGO_PROPOSAL_PREFIX ? proposal-   # prefix for proposal files
    ```
  - Update the caller in `RunInit`: find `generateCommentedBoardYAML(defaults)` (or `generateCommentedBoardYAML(DefaultConfig())`) and change to `generateCommentedBoardYAML()`. If `defaults` was only used as an argument to this function, remove the `defaults` variable entirely from `RunInit`.
- **Commit:** `refactor(board): migrate config loading to internal/config`

## Batch Tests

`verify: go test ./...` runs the complete test suite: `internal/lock`, `internal/git`, `internal/config`, `internal/board` (including `boardtest`). This is the behaviour-preserving guardrail for the whole refactor. Board's test suite (`config_test.go`, `git_test.go`, `store_test.go` if any, `boardtest/bench_test.go`) must pass green. The bench test sets up `_mhgo/board.yaml` and calls `board.LoadConfig` via the CLI path — if the config rewrite in card 16 has any regression, it will surface here.
