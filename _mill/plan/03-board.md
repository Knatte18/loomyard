# Batch: board

```yaml
task: "Fix failing TestRunCLI in internal/worktree"
batch: "board"
number: 3
cards: 3
verify: go test -tags integration ./internal/board/...
depends-on: []
```

## Batch Scope

The consistency sweep across the `internal/board` package and its `boardtest` subpackage.
Three test fixtures hardcode `_lyx/config/board.yaml`; each is routed through the
`internal/paths` helpers per the Shared Decision recipe. Behaviour-preserving; independent of
all other batches.

## Cards

### Card 10: board/cli_test.go — route _lyx/config/board.yaml literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/board/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `seedCwd`, replace `lyxDir := filepath.Join(cwd, "_lyx")` with
  `filepath.Join(cwd, paths.LyxDirName)`, `configDir := filepath.Join(lyxDir, "config")` with
  `paths.ConfigDir(cwd)`, and `configPath := filepath.Join(configDir, "board.yaml")` with
  `paths.ConfigFile(cwd, "board")`. Keep the `os.Mkdir` sequence and assertions unchanged. Add
  `"github.com/Knatte18/loomyard/internal/paths"` to the import block (not currently imported);
  keep `os`/`path/filepath` as used.
- **Commit:** `refactor(board): resolve cli_test.go _lyx paths via internal/paths`

### Card 11: board/config_test.go — route remaining _lyx/config mkdir literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/board/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** This file already builds the config file via `paths.ConfigFile(tmpDir, "board")`.
  Convert the remaining directory literals in each setup block: every
  `lyxDir := filepath.Join(tmpDir, "_lyx")` → `filepath.Join(tmpDir, paths.LyxDirName)`, and every
  `configDir := filepath.Join(lyxDir, "config")` → `paths.ConfigDir(tmpDir)`. Leave the
  `paths.ConfigFile` calls and assertions unchanged. `paths` is already imported.
- **Commit:** `refactor(board): resolve config_test.go _lyx paths via internal/paths`

### Card 12: board/boardtest/bench_test.go — route _lyx/config/board.yaml literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/board/boardtest/bench_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In the bench fixture setup, replace `lyxDir := filepath.Join(dir, "_lyx")`
  with `filepath.Join(dir, paths.LyxDirName)`, `configDir := filepath.Join(lyxDir, "config")` with
  `paths.ConfigDir(dir)`, and `configPath := filepath.Join(configDir, "board.yaml")` with
  `paths.ConfigFile(dir, "board")`. Keep the `os.Mkdir`/`os.MkdirAll` calls and assertions
  unchanged. Add `"github.com/Knatte18/loomyard/internal/paths"` to the import block (`package
  boardtest` does not import it yet); keep `os`/`path/filepath` as used.
- **Commit:** `refactor(boardtest): resolve bench_test.go _lyx paths via internal/paths`

## Batch Tests

`verify: go test -tags integration ./internal/board/...` runs the `internal/board` package and
its `boardtest` subpackage under the `integration` tag — exactly the code this batch edits. All
tests already pass; the refactor must keep them green. No new tests are added.
