# Batch: misc

```yaml
task: "Fix failing TestRunCLI in internal/worktree"
batch: "misc"
number: 4
cards: 5
verify: go test -tags integration ./cmd/lyx/ ./internal/initcli/ ./internal/update/ ./internal/ide/ ./internal/weft/
depends-on: []
```

## Batch Scope

The remaining single-file sweeps across five otherwise-unrelated packages (`cmd/lyx`,
`internal/initcli`, `internal/update`, `internal/ide`, `internal/weft`). Each has exactly one
test file that hardcodes `_lyx`/config path segments; all are routed through `internal/paths`
per the Shared Decision recipe. Grouped into one batch because each change is a tiny,
independent, behaviour-preserving substitution. Independent of all other batches.

## Cards

### Card 11: cmd/lyx/main_test.go — route _lyx/config/board.yaml literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Across all three setup sites, replace `lyxDir := filepath.Join(cwd, "_lyx")`
  with `filepath.Join(cwd, paths.LyxDirName)`, `configDir := filepath.Join(lyxDir, "config")` with
  `paths.ConfigDir(cwd)`, and `configPath := filepath.Join(configDir, "board.yaml")` with
  `paths.ConfigFile(cwd, "board")`. The third site has only `lyxDir`/`configDir` (no file write) —
  convert those two and keep the block otherwise intact. Do NOT touch the
  `run([]string{"config"}, ...)` call (that is a CLI subcommand name, not a path). Keep the
  `os.Mkdir` sequences and assertions unchanged. Add `"github.com/Knatte18/loomyard/internal/paths"`
  to the import block (`package main` does not import it yet). **Keep `path/filepath`:** the three
  `lyxDir := filepath.Join(cwd, "_lyx")` sites (lines 47/83/187) convert to
  `filepath.Join(cwd, paths.LyxDirName)`, which retains a `filepath.` reference, so the import is
  still used (this file does NOT orphan `path/filepath`, unlike the combined-form files in the
  Shared-Decision orphan rule). Keep `os` as used.
- **Commit:** `refactor(lyx): resolve main_test.go _lyx paths via internal/paths`

### Card 12: initcli/initcli_test.go — route _lyx/config literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/initcli/initcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace `configDir := filepath.Join(tmpDir, "_lyx", "config")` with
  `paths.ConfigDir(tmpDir)`; the dynamic `cfgPath := filepath.Join(configDir, module+".yaml")`
  with `paths.ConfigFile(tmpDir, module)`; and
  `boardPath := filepath.Join(tmpDir, "_lyx", "config", "board.yaml")` with
  `paths.ConfigFile(tmpDir, "board")`. Keep assertions unchanged. Add
  `"github.com/Knatte18/loomyard/internal/paths"` to the import block (not currently imported);
  keep `os`/`path/filepath` as used.
- **Commit:** `refactor(initcli): resolve initcli_test.go _lyx paths via internal/paths`

### Card 13: update/update_test.go — route _lyx/config literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/update/update_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace both `configDir := filepath.Join(tmpDir, "_lyx", "config")`
  occurrences with `paths.ConfigDir(tmpDir)`; `boardPath := filepath.Join(configDir, "board.yaml")`
  with `paths.ConfigFile(tmpDir, "board")`; and `weftPath := filepath.Join(configDir, "weft.yaml")`
  with `paths.ConfigFile(tmpDir, "weft")`. Keep assertions unchanged. Add
  `"github.com/Knatte18/loomyard/internal/paths"` to the import block (`package update` does not
  import it yet). **All 4 `filepath.` sites are config-path conversions** — after the
  substitution there are zero remaining `filepath.` references, so **remove the now-unused
  `path/filepath` import** (keep `os`, still used). The `go build`/`verify` gate fails on an
  unused import if it is left in.
- **Commit:** `refactor(update): resolve update_test.go _lyx paths via internal/paths`

### Card 14: ide/menu_test.go — route _lyx/config literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/ide/menu_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace each bare `filepath.Join(<base>, "_lyx")` inside an `os.MkdirAll(...)`
  call with `filepath.Join(<base>, paths.LyxDirName)`, preserving the original base argument
  (`mainWorktreePath` in one site, `childPath` in two). Replace every
  `configDir := filepath.Join(mainWorktreePath, "_lyx", "config")` with
  `paths.ConfigDir(mainWorktreePath)`, and every
  `boardConfigPath := filepath.Join(configDir, "board.yaml")` with
  `paths.ConfigFile(mainWorktreePath, "board")`. Do NOT touch the `git config user.email` /
  `git config user.name` invocations (those are git subcommands). Keep assertions unchanged.
  `paths` is already imported.
- **Commit:** `refactor(ide): resolve menu_test.go _lyx paths via internal/paths`

### Card 15: weft/config_test.go — route remaining _lyx/config mkdir literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/weft/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `TestLoadConfig_HappyPath`, replace `lyxDir := filepath.Join(tmpDir, "_lyx")`
  with `filepath.Join(tmpDir, paths.LyxDirName)` and `configDir := filepath.Join(lyxDir, "config")`
  with `paths.ConfigDir(tmpDir)`. The file write already uses `paths.ConfigFile(tmpDir, "weft")` —
  leave it. Do NOT touch the `TestConfigDirs` table cases (the `"_lyx"` / `"_codeguide"` strings
  there are parser input data, not paths). `paths` is already imported.
- **Commit:** `refactor(weft): resolve config_test.go _lyx mkdir paths via internal/paths`

## Batch Tests

`verify: go test -tags integration ./cmd/lyx/ ./internal/initcli/ ./internal/update/ ./internal/ide/ ./internal/weft/`
runs exactly the five packages this batch edits, under the `integration` tag. All tests in these
packages already pass; the refactor must keep them green. No new tests are added.
