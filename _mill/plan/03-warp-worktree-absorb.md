# Batch: warp-worktree-absorb

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
batch: warp-worktree-absorb
number: 3
cards: 4
verify: go build ./... && go test -tags integration ./internal/warp/ ./internal/configreg/ ./internal/configcli/ ./internal/initcli/ ./internal/lyxtest/ ./cmd/lyx/
depends-on: [2]
```

## Batch Scope

Absorb the entire `internal/worktree` package into `internal/warp` and delete it. The lifecycle (`add`/`list`/`remove`), the weft-side junction wiring, launchers, portals, the empty-dir sweeper, and the config module all move; the config module is renamed `worktree` → `warp` (template `warp.yaml`, user file `_lyx/config/warp.yaml`). `warp.RunCLI` gains `add`/`list`/`remove` routing alongside `clone`. `cmd/lyx/main.go` drops the `worktree` case. This is a behaviour-preserving move — the worktree test suite moves verbatim and stays green. Junctions are still wired inside `Add` at this point (relocation happens in batch 4).

External interface batch 4 consumes: the moved `Worktree` facade (`New(cfg)`, `Add`/`Remove`/`List`), the unexported weft-wiring helpers (`createWeftWorktree`, `seedLyxJunction`, `seedGitExclude`, `weftRepoExists`, `weftBranchExists`, …), and `warp.Config`/`LoadConfig`/`ConfigTemplate`.

Batch-local decision: to avoid two `prune.go` files (the existing empty-dir sweeper vs. the future `prune` verb in batch 7), the sweeper file moves as `ancestors.go`. The `Worktree` struct and `New` keep their names (behaviour-preserving move; renaming is out of scope).

## Cards

### Card 7: Move worktree lifecycle core into warp

- **Context:**
  - `internal/warp/warp.go`
  - `internal/paths/paths.go`
  - `internal/paths/worktreelist.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/worktreelifecycle.go`
  - `internal/warp/add.go`
  - `internal/warp/remove.go`
  - `internal/warp/list.go`
- **Deletes:**
  - `internal/worktree/worktree.go`
  - `internal/worktree/add.go`
  - `internal/worktree/remove.go`
  - `internal/worktree/list.go`
- **Requirements:** Move `internal/worktree/worktree.go` → `internal/warp/worktreelifecycle.go` (keep `type Worktree struct { cfg Config }` and `func New(cfg Config) *Worktree`), `add.go` → `internal/warp/add.go` (keep `AddOptions`, `AddResult`, `func (w *Worktree) Add(...)`, `rollbackAdd`, `addOptionsFromEnv`), `remove.go` → `internal/warp/remove.go` (keep `RemoveResult`, `func (w *Worktree) Remove(...)`), `list.go` → `internal/warp/list.go` (keep `WorktreeEntry` alias + `func (w *Worktree) List(...)`). Change every package clause to `package warp`. Do not change logic — junction wiring stays inside `Add` for now. Resolve any symbol collision with `warp.go`/`clone.go` (there should be none).
- **Commit:** `feat(warp): move worktree lifecycle (add/remove/list) into warp`

### Card 8: Move weft-wiring, launchers, portals, sweeper into warp

- **Context:**
  - `internal/warp/add.go`
  - `internal/fslink/fslink.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/weftwiring.go`
  - `internal/warp/launchers.go`
  - `internal/warp/portals.go`
  - `internal/warp/ancestors.go`
- **Deletes:**
  - `internal/worktree/weft.go`
  - `internal/worktree/launchers.go`
  - `internal/worktree/portals.go`
  - `internal/worktree/prune.go`
- **Requirements:** Move `internal/worktree/weft.go` → `internal/warp/weftwiring.go` (keep `weftRepoExists`, `weftBranchExists`, `createWeftWorktree`, `seedLyxJunction`, `seedGitExclude`, `pushWeftBranch`, `removeHostJunction`, `removeWeftWorktree`), `launchers.go` → `internal/warp/launchers.go`, `portals.go` → `internal/warp/portals.go`, and `prune.go` → `internal/warp/ancestors.go` (keep `pruneEmptyAncestors`; renamed file only, to free `prune.go` for the verb in batch 7). Package clause → `package warp`; no logic change.
- **Commit:** `feat(warp): move weft-wiring, launchers, portals, ancestor-sweeper`

### Card 9: Move config, rename module worktree→warp, extend RunCLI and dispatch

- **Context:**
  - `internal/warp/worktreelifecycle.go`
  - `internal/warp/add.go`
  - `internal/warp/list.go`
  - `internal/warp/remove.go`
  - `internal/worktree/cli.go`
  - `internal/board/template.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/warp/warp.go`
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
  - `cmd/lyx/main.go`
- **Creates:**
  - `internal/warp/config.go`
  - `internal/warp/template.go`
  - `internal/warp/template.yaml`
- **Deletes:**
  - `internal/worktree/config.go`
  - `internal/worktree/template.go`
  - `internal/worktree/template.yaml`
  - `internal/worktree/cli.go`
- **Requirements:** Move `internal/worktree/config.go` → `internal/warp/config.go` (keep `type Config struct { BranchPrefix string }`, `func LoadConfig(baseDir, module string) (Config, error)`), `template.go` → `internal/warp/template.go` (keep `func ConfigTemplate() string` embedding the yaml), `template.yaml` → `internal/warp/template.yaml` (content unchanged: `branch_prefix: ${env:LYX_BRANCH_PREFIX:-}`). In `internal/warp/warp.go`, fold the dispatch logic from `internal/worktree/cli.go` into `RunCLI`: add `case "add"`, `case "list"`, `case "remove"` that resolve cwd via `paths.Getwd`/`paths.Resolve` (fail `ErrNotAGitRepo` first), call `LoadConfig(cwd, "warp")`, `New(cfg)`, and invoke `Add`/`List`/`Remove` with the same JSON output shapes and `addOptionsFromEnv` handling as the old worktree CLI. **Resolution hazard:** keep `paths.Resolve`/`LoadConfig` *inside* each of the add/list/remove cases (and the later checkout/status/reconcile/prune/cleanup cases) — do **not** hoist them to the top of `RunCLI`, because the existing `clone` case runs outside a git repo and must not resolve a layout. The old `worktree.RunCLI` resolved at the top; folding it in must move that resolution down into the per-verb cases. In `internal/configreg/configreg.go` replace the `{"worktree", worktree.ConfigTemplate}` entry with `{"warp", warp.ConfigTemplate}`, swap the import `internal/worktree` → `internal/warp`, and update the package doc comment listing modules. In `cmd/lyx/main.go` remove `case "worktree": return worktree.RunCLI(...)` and its import (the `warp` case from batch 2 now also serves add/list/remove). In `internal/configreg/configreg_test.go`, update the `TestNames` (and any `TestModules`) assertion `want := []string{"board", "worktree", "weft"}` → `[]string{"board", "warp", "weft"}` (and any other `"worktree"` literal) to match the renamed module.
- **Commit:** `feat(warp): move config module (worktree→warp); route add/list/remove`

### Card 10: Move worktree test suite into warp and delete the package

- **Context:**
  - `internal/warp/config.go`
  - `internal/warp/warp.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/configcli/configcli_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/initcli/initcli_test.go`
  - `internal/lyxtest/leaf_enforcement_test.go`
  - `cmd/lyx/main_test.go`
- **Creates:**
  - `internal/warp/add_test.go`
  - `internal/warp/remove_test.go`
  - `internal/warp/list_test.go`
  - `internal/warp/warp_test.go`
  - `internal/warp/config_test.go`
  - `internal/warp/template_test.go`
  - `internal/warp/launchers_test.go`
  - `internal/warp/portals_test.go`
  - `internal/warp/weftwiring_test.go`
  - `internal/warp/ancestors_test.go`
- **Deletes:**
  - `internal/worktree/add_test.go`
  - `internal/worktree/remove_test.go`
  - `internal/worktree/list_test.go`
  - `internal/worktree/cli_test.go`
  - `internal/worktree/config_test.go`
  - `internal/worktree/template_test.go`
  - `internal/worktree/launchers_test.go`
  - `internal/worktree/portals_test.go`
  - `internal/worktree/weft_test.go`
  - `internal/worktree/prune_test.go`
- **Requirements:** Move every `internal/worktree/*_test.go` into `internal/warp` under the new filenames (`cli_test.go` → `warp_test.go`, `weft_test.go` → `weftwiring_test.go`, `prune_test.go` → `ancestors_test.go`, others keep their base name). Change the package clause to `package warp`. Update every config-module string literal `"worktree"` → `"warp"` and any `LoadConfig(..., "worktree")` → `LoadConfig(..., "warp")` inside the tests; route config seeding through `warp.ConfigTemplate()` per the lyxtest leaf invariant (tests construct the `SeedConfig` map at the call site, never inside `lyxtest`). Adjust any test that asserts the CLI command name (`worktree` → the `warp` subcommand form). After this card the `internal/worktree` directory is empty and removed. Also fix the two `configcli` test consumers broken by this batch: in `internal/configcli/configcli_integration_test.go` replace the `internal/worktree` import + `worktree.New`/`worktree.Config`/`worktree.AddOptions`/`worktree.Add` symbols with the `internal/warp` equivalents (`warp.New`/`warp.Config`/`warp.AddOptions`), and change the config-module-name strings (`dispatch(..., []string{"worktree"})`, `paths.ConfigFile(".", "worktree")`) to `"warp"`; in `internal/configcli/configcli_test.go` change every config-module-name `"worktree"` literal (`editOne(..., "worktree", ...)`, `paths.ConfigFile(baseDir, "worktree")`) and the menu assertion `"worktree (configured)"` to `"warp"` / `"warp (configured)"`. Also fix three more consumers broken by the worktree deletion/rename: in `internal/initcli/initcli_test.go` swap the `internal/worktree` import for `internal/warp`, change the module list `[]string{"board", "worktree", "weft"}` → `"warp"`, and `worktree.LoadConfig(tmpDir, "worktree")` → `warp.LoadConfig(tmpDir, "warp")`; in `internal/lyxtest/leaf_enforcement_test.go` change the banned-import **string literal** `"github.com/Knatte18/loomyard/internal/worktree"` → `".../internal/warp"` and the `(board, worktree, weft)` comment text to `warp` (it is a string in the ban list, not a real import — the lyxtest leaf invariant now bans importing `internal/warp`); in `cmd/lyx/main_test.go` change the command invocation `run([]string{"worktree", "list"}, ...)` → `{"warp", "list"}`.
- **Commit:** `test(warp): move worktree test suite; delete internal/worktree`

## Batch Tests

`verify: go build ./... && go test -tags integration ./internal/warp/ ./internal/configreg/ ./internal/configcli/ ./internal/initcli/ ./internal/lyxtest/ ./cmd/lyx/`. The moved worktree suite (`add_test.go`, `remove_test.go`, `list_test.go`, `warp_test.go`, `config_test.go`, `template_test.go`, `launchers_test.go`, `portals_test.go`, `weftwiring_test.go`, `ancestors_test.go`) is the behaviour-preserving guardrail — it must pass unchanged in semantics. `go test ./internal/configreg/` confirms the module-list rename (`warp` registered, `worktree` gone) and the updated `configreg_test.go` assertion. `go test -tags integration ./internal/configcli/` confirms the two `configcli` test consumers (the integration test that imported the deleted `internal/worktree`, and the unit test that used the `"worktree"` module name) now compile and pass against `warp`. `go test ./cmd/lyx/` confirms the dispatch drop of the `worktree` case. `go build ./...` confirms no dangling `internal/worktree` references.
