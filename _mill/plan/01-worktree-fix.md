# Batch: worktree-fix

```yaml
task: "Fix failing TestRunCLI in internal/worktree"
batch: "worktree-fix"
number: 1
cards: 2
verify: go test -tags integration ./internal/worktree/
depends-on: []
```

## Batch Scope

This batch delivers the actual bug fix: `TestRunCLI` goes from FAIL to PASS. The fixture
`setupCLIRepo` in `cli_test.go` writes the worktree config to the stale `_lyx/worktree.yaml`
path while `RunCLI`→`LoadConfig` reads `_lyx/config/worktree.yaml`; the fix routes the write
through `paths.ConfigDir`/`paths.ConfigFile` so it lands where the loader looks. The sibling
`config_test.go` already writes via `paths.ConfigFile` but still hardcodes its `_lyx`/`config`
mkdir — tightened here so the whole package is clean. No production code changes. No external
interface is produced for later batches; all four batches are independent.

## Cards

### Card 1: Fix setupCLIRepo to write config via internal/paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/worktree/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `setupCLIRepo`, replace the stale config write. Currently it builds
  `lyxDir := filepath.Join(f.Hub, "_lyx")`, `os.MkdirAll(lyxDir, 0755)`, then
  `os.WriteFile(filepath.Join(lyxDir, "worktree.yaml"), []byte("branch_prefix: wt-\n"), 0644)`.
  Change it to: `os.MkdirAll(paths.ConfigDir(f.Hub), 0755)` for the directory, and
  `os.WriteFile(paths.ConfigFile(f.Hub, "worktree"), []byte("branch_prefix: wt-\n"), 0644)`
  for the file — so the config lands at `_lyx/config/worktree.yaml` where `LoadConfig` reads
  it. Remove the now-unused `lyxDir` local. Add `"github.com/Knatte18/loomyard/internal/paths"`
  to the import block (this file does not import it yet). Keep the existing `os`, `path/filepath`,
  `lyxtest`, and `worktree` imports as they remain used. Do not change `decodeResult`, the
  `TestRunCLI` subtests, or any assertion — `UnknownSubcommand` keeps passing because `bogus`
  now reaches the `default` case in `internal/worktree/cli.go` (exit 1 / `ok:false`).
- **Commit:** `fix(worktree): write test config via paths.ConfigFile so TestRunCLI passes`

### Card 2: Route config_test.go mkdir through paths helpers

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/worktree/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In each of the three setup blocks (`TestLoadConfig_HappyPath`,
  `TestLoadConfig_EmptyBranchPrefix`, `TestLoadConfig_EnvResolution`), replace
  `lyxDir := filepath.Join(tmpDir, "_lyx")` with `lyxDir := filepath.Join(tmpDir, paths.LyxDirName)`
  and `configDir := filepath.Join(lyxDir, "config")` with `configDir := paths.ConfigDir(tmpDir)`.
  Leave the existing `os.Mkdir(lyxDir, ...)` then `os.Mkdir(configDir, ...)` two-step calls and
  the `paths.ConfigFile(tmpDir, "worktree")` file writes unchanged. `paths` is already imported.
  `TestLoadConfig_NotInitialized` creates no `_lyx` dir — leave it untouched.
- **Commit:** `refactor(worktree): resolve config_test.go _lyx paths via internal/paths`

## Batch Tests

`verify: go test -tags integration ./internal/worktree/` runs the full `internal/worktree`
package under the `integration` build tag (the package's tests, incl. `cli_test.go` and
`config_test.go`, are `//go:build integration`). This is the red→green gate: `TestRunCLI`
(`List`, `UnknownSubcommand`, `RemoveWithForceFlag`) must pass, and the pre-existing
`TestLoadConfig_*` plus weft/add/seeder tests must stay green. Scope is the single affected
package.
