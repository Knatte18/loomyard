# Batch: config-family

```yaml
task: "Fix failing TestRunCLI in internal/worktree"
batch: "config-family"
number: 2
cards: 5
verify: go test -tags integration ./internal/config/ ./internal/configcli/ ./internal/configsync/
depends-on: []
```

## Batch Scope

The consistency sweep across the three config-handling packages: `internal/config`,
`internal/configcli`, and `internal/configsync`. Every hardcoded `_lyx`/config path literal
in their test files is replaced with the matching `internal/paths` helper per the Shared
Decision recipe. All edits are behaviour-preserving (the tests already pass); the goal is to
remove latent migration breakage. Independent of all other batches (no shared files).

## Cards

### Card 3: config/config_test.go — route all _lyx/config literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/config/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Apply the Shared-Decision recipe throughout: every
  `lyxDir := filepath.Join(tmpDir, "_lyx")` → `filepath.Join(tmpDir, paths.LyxDirName)`
  (including the standalone one in the not-initialized test near the end of the file);
  every `configDir := filepath.Join(lyxDir, "config")` → `paths.ConfigDir(tmpDir)`; every
  `yamlFile := filepath.Join(configDir, "board.yaml")` → `paths.ConfigFile(tmpDir, "board")`;
  and the one `filepath.Join(configDir, "test.yaml")` → `paths.ConfigFile(tmpDir, "test")`.
  Keep the two-step `os.Mkdir` sequences and all assertions unchanged. Add
  `"github.com/Knatte18/loomyard/internal/paths"` to the import block (not currently imported);
  keep `os`/`path/filepath` as used.
- **Commit:** `refactor(config): resolve config_test.go _lyx paths via internal/paths`

### Card 4: config/edit_test.go — route remaining _lyx/config mkdir literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/config/edit_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** This file already builds config file paths via `paths.ConfigFile(tmpDir, "testmod")`.
  Convert only the remaining directory literals: every `lyxDir := filepath.Join(tmpDir, "_lyx")`
  → `filepath.Join(tmpDir, paths.LyxDirName)`, and every `configDir := filepath.Join(lyxDir, "config")`
  → `paths.ConfigDir(tmpDir)`. Leave the `paths.ConfigFile` calls and all assertions unchanged.
  `paths` is already imported.
- **Commit:** `refactor(config): resolve edit_test.go _lyx paths via internal/paths`

### Card 5: configcli/configcli_test.go — route config dir/file literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/configcli/configcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace every `configDir := filepath.Join(baseDir, "_lyx", "config")` with
  `paths.ConfigDir(baseDir)`. Replace every `os.WriteFile(filepath.Join(configDir, "board.yaml"), ...)`
  argument `filepath.Join(configDir, "board.yaml")` with `paths.ConfigFile(baseDir, "board")`, and
  the single `filepath.Join(configDir, "worktree.yaml")` with `paths.ConfigFile(baseDir, "worktree")`.
  Keep the `os.MkdirAll(configDir, ...)` calls and all assertions unchanged. `paths` is already
  imported.
- **Commit:** `refactor(configcli): resolve configcli_test.go _lyx paths via internal/paths`

### Card 6: configcli/configcli_integration_test.go — use paths for the relative config assert

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/configcli/configcli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace the relative-path construction
  `configRelPath := filepath.Join("_lyx", "config", "worktree.yaml")` with
  `configRelPath := paths.ConfigFile(".", "worktree")` (yields the same relative
  `_lyx/config/worktree.yaml`). Do NOT change the `strings.Contains(string(allFilesOut), "_lyx")`
  assertion — that is a string-content check on git output, not a path construction. `paths` is
  already imported.
- **Commit:** `refactor(configcli): build config pathspec via paths.ConfigFile in integration test`

### Card 7: configsync/configsync_test.go — route config dir/file literals through paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/configsync/configsync_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace every `configDir := filepath.Join(tmpDir, "_lyx", "config")` (three
  occurrences) with `paths.ConfigDir(tmpDir)`. Replace every `filepath.Join(configDir, "board.yaml")`
  with `paths.ConfigFile(tmpDir, "board")`, and `filepath.Join(configDir, "weft.yaml")` with
  `paths.ConfigFile(tmpDir, "weft")`. Keep the `os.MkdirAll(configDir, ...)` calls and assertions
  unchanged. Add `"github.com/Knatte18/loomyard/internal/paths"` to the import block (this file —
  `package configsync` — does not import it yet); keep `os`/`path/filepath` as used.
- **Commit:** `refactor(configsync): resolve configsync_test.go _lyx paths via internal/paths`

## Batch Tests

`verify: go test -tags integration ./internal/config/ ./internal/configcli/ ./internal/configsync/`
runs exactly the three packages this batch edits, under the `integration` tag. All tests in
these packages already pass; the refactor must keep them green. No new tests are added.
