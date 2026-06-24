# Batch: lyx-update-init

```yaml
task: "Extract yamlengine and migrate config via lyx update"
batch: lyx-update-init
number: 6
cards: 6
verify: go test ./internal/configreg/ ./internal/configsync/ ./internal/update/ ./internal/initcli/ ./internal/configcli/... ./cmd/lyx/
depends-on: [1, 3, 4]
```

## Batch Scope

Add the `lyx update` command and rework `lyx init`, both driven by a shared module
registry and a shared reconcile engine. New packages: `internal/configreg` (the
registry, moved out of `configcli` to break the board↔registry import cycle),
`internal/configsync` (reconcile-over-registry via `yamlengine.Reconcile` + atomic
writes), `internal/update` (the `lyx update` CLI), and `internal/initcli` (the `lyx
init` CLI, moved out of the `board` package). `configcli` is refactored to consume
`configreg`; `cmd/lyx/main.go` routes `init`→`initcli` and adds `update`→`update`.
Both `init` and `update` reconcile every registry module from its template — `init`
is "reconcile against an absent file"; this is also the migration path for existing
commented-out config files. `init` now materializes `weft.yaml` too.

Batch-local decisions: `update` resolves the host baseDir as
`filepath.Join(l.WorktreeRoot, l.RelPath)` (identical to `configcli` dispatch; the
host `_lyx` junction makes this the same physical file weft reads). `lyx update` is
dry-run by default and writes only with `--apply`. CLI output is JSON via
`internal/output`.

## Cards

### Card 16: configreg registry package

- **Context:**
  - `internal/configcli/configcli.go`
  - `internal/board/template.go`
  - `internal/worktree/template.go`
  - `internal/weft/template.go`
- **Edits:** none
- **Creates:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Deletes:** none
- **Requirements:**
  - Create package `configreg`. Define `type Module struct { Name string; Template func() string }`. Add `func Modules() []Module` returning, in order, `{"board", board.ConfigTemplate}`, `{"worktree", worktree.ConfigTemplate}`, `{"weft", weft.ConfigTemplate}`. Add `func Template(name string) (func() string, bool)` (lookup by name) and `func Names() []string` (ordered names) — mirroring the helpers currently in `configcli`. This package imports `board`/`worktree`/`weft`; it must NOT import `configcli` or `initcli`/`update`.
  - Godoc every exported symbol.
  - configreg_test.go: assert `Names()` equals `["board","worktree","weft"]` in order; `Template("weft")` returns ok and a func whose output equals `weft.ConfigTemplate()`; `Template("nope")` returns `false`.
- **Commit:** `feat(configreg): module registry shared by config, update, init`

### Card 17: configsync reconcile-over-registry

- **Context:**
  - `internal/configreg/configreg.go`
  - `internal/yamlengine/reconcile.go`
  - `internal/paths/paths.go`
  - `internal/fsx/fsx.go`
- **Edits:** none
- **Creates:**
  - `internal/configsync/configsync.go`
  - `internal/configsync/configsync_test.go`
- **Deletes:** none
- **Requirements:**
  - Create package `configsync`. Define `type Result struct { Module string; Added, Removed []string; Applied bool }`. Add `func ReconcileAll(baseDir string, apply bool) ([]Result, error)`: for each `configreg.Modules()` entry, compute `cfgPath := paths.ConfigFile(baseDir, m.Name)`, read existing bytes (absent file → empty `[]byte`, not an error), call `merged, added, removed, err := yamlengine.Reconcile([]byte(m.Template()), existing)`; when `apply` is true AND the file is absent OR `len(added)+len(removed) > 0`, write `merged` via `fsx.AtomicWriteBytes(cfgPath, merged)` and set `Applied=true`; append a `Result`. Return the slice. (When `apply` is false, never write; `Applied` is false.)
  - Godoc every exported symbol.
  - configsync_test.go (use `t.TempDir` as baseDir, create `_lyx/config/`): seed `board.yaml` missing a key and containing a stale key; run `ReconcileAll(dir, false)` → asserts `Added`/`Removed` for board, files unchanged on disk; run `ReconcileAll(dir, true)` → file rewritten with merged content (missing key added, stale removed, surviving value preserved), `Applied=true`; an absent `weft.yaml` is created from its template on apply; a fully-reconciled file reports empty deltas and `Applied=false` on a second apply (idempotent).
- **Commit:** `feat(configsync): reconcile all module configs against their templates`

### Card 18: lyx update command

- **Context:**
  - `internal/configsync/configsync.go`
  - `internal/paths/paths.go`
  - `internal/output/output.go`
  - `internal/configcli/configcli.go`
- **Edits:** none
- **Creates:**
  - `internal/update/update.go`
  - `internal/update/update_test.go`
- **Deletes:** none
- **Requirements:**
  - Create package `update`. Add `func RunCLI(out io.Writer, args []string) int`: parse a `flag.FlagSet` with a `--apply` bool (default false → dry-run). Resolve the layout via `paths.Getwd()` + `paths.Resolve(cwd)` and compute `baseDir := filepath.Join(l.WorktreeRoot, l.RelPath)` (same as `configcli.dispatch`). Call `configsync.ReconcileAll(baseDir, apply)`. On success emit JSON via `internal/output` with shape `{"ok":true,"applied":<bool>,"modules":[{"module":..,"added":[..],"removed":[..],"applied":<bool>}]}`. On error, emit the error JSON and return 1. Dry-run (no `--apply`) writes nothing and reports what WOULD change.
  - Godoc every exported symbol. Match the JSON-on-stdout, exit-1-on-error convention used by the other CLIs.
  - update_test.go: dry-run over a temp baseDir reports per-module added/removed and writes nothing; `--apply` writes the reconciled files and reports `applied:true`; assert the JSON shape parses and `ok:true`.
- **Commit:** `feat(update): add lyx update command (dry-run default, --apply writes atomically)`

### Card 19: lyx init moves to initcli, reconciles all modules

- **Context:**
  - `internal/board/init.go`
  - `internal/configsync/configsync.go`
  - `internal/paths/paths.go`
  - `internal/gitignore/gitignore.go`
  - `internal/output/output.go`
  - `internal/board/config.go`
  - `internal/worktree/config.go`
  - `internal/weft/config.go`
- **Edits:** none
- **Creates:**
  - `internal/initcli/initcli.go`
  - `internal/initcli/initcli_test.go`
- **Deletes:**
  - `internal/board/init.go`
  - `internal/board/init_test.go`
- **Requirements:**
  - Create package `initcli` with `func RunInit(out io.Writer, args []string) int`, porting the directory + `.gitignore` scaffolding from `internal/board/init.go`: resolve cwd via `paths.Getwd()`; create `<cwd>/_lyx` (use `paths.LyxDirName`) and `paths.ConfigDir(cwd)`; maintain the `.gitignore` managed block via `gitignore.Ensure(cwd, ".lyx/")`. Then materialize ALL module config files by calling `configsync.ReconcileAll(cwd, true)` (this creates `board.yaml`, `worktree.yaml`, AND `weft.yaml` from their live templates). Emit a JSON summary via `internal/output` reporting the `_lyx`/gitignore status and the per-module results (created vs already-present). Preserve idempotency: a second run must not clobber existing config files (Reconcile preserves user values; ReconcileAll only writes when there is a delta) and must not duplicate the gitignore block.
  - DELETE `internal/board/init.go` and `internal/board/init_test.go` (the `board` package no longer owns init). Do not leave a `board.RunInit` symbol.
  - Godoc every exported symbol.
  - initcli_test.go (use `t.TempDir` + `t.Chdir`): first run creates `_lyx/config/{board,worktree,weft}.yaml` as live YAML and the `.gitignore` managed block, with a correct JSON envelope; a freshly-init'd dir then passes strict load — call `board.LoadConfig(cwd,"board")`, `worktree.LoadConfig(cwd,"worktree")`, and `weft.LoadConfig(filepath.Join(cwd))` (weft reads `_lyx/config/weft.yaml` under the same dir in this test) without error; second run is idempotent (config files and gitignore unchanged).
- **Commit:** `feat(initcli): move lyx init out of board, scaffold all module configs`

### Card 20: configcli uses configreg + centralized paths

- **Context:**
  - `internal/configreg/configreg.go`
  - `internal/paths/paths.go`
  - `internal/config/edit.go`
- **Edits:**
  - `internal/configcli/configcli.go`
  - `internal/configcli/menu.go`
  - `internal/configcli/configcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `internal/configcli/configcli.go`: remove the local `registry` var and rewrite `templateFor`/`moduleNames` to delegate to `configreg.Template`/`configreg.Names` (or replace their call sites with the `configreg` functions directly). Drop the now-unused `board`/`worktree`/`weft` imports if they become unused. `editOne`/`dispatch`/`RunCLI` keep their behavior and signatures.
  - In `internal/configcli/menu.go`: replace the `filepath.Join(baseDir, "_lyx", "config", name+".yaml")` literal with `paths.ConfigFile(baseDir, name)`, and use `configreg.Names()` wherever the module list was sourced from the local registry.
  - Update `internal/configcli/configcli_test.go` if it referenced the removed local `registry`/`templateFor`/`moduleNames` internals; assert behavior via the public `RunCLI`/`dispatch` surface where possible.
- **Commit:** `refactor(configcli): consume configreg registry and centralized paths`

### Card 21: route init→initcli and add update in main

- **Context:**
  - `internal/initcli/initcli.go`
  - `internal/update/update.go`
  - `internal/board/cli.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `cmd/lyx/main.go`: change the `case "init":` branch to `return initcli.RunInit(out, moduleArgs)` (was `board.RunInit`); add a `case "update":` branch returning `update.RunCLI(out, moduleArgs)`. Add the `initcli` and `update` imports; keep the `board` import (still used for `board.RunCLI`). Update the package doc comment's module list: `init` now scaffolds all module configs (board, worktree, weft) and `.gitignore`, and add an `update` line ("reconcile module configs against templates — see internal/update.RunCLI").
  - Update `cmd/lyx/main_test.go`: adjust the `init` dispatch test for `initcli` and add a test that `update` routes to `update.RunCLI` (e.g. asserts the command is recognized / produces JSON, mirroring the existing per-module dispatch tests).
- **Commit:** `feat(lyx): route init to initcli and add update command`

## Batch Tests

`verify: go test ./internal/configreg/ ./internal/configsync/ ./internal/update/ ./internal/initcli/ ./internal/configcli/... ./cmd/lyx/`
covers the new registry/reconcile/CLI packages, the moved init (including the
fresh-init-passes-strict-load assertion), the refactored configcli, and the main
dispatch routes. Scope is bounded to the packages this batch creates or edits. The
`board` package is intentionally NOT in scope here — it is only losing `init.go`,
verified by batch 5's board run and the final build.
