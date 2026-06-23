# Batch: lyx-config-command

```yaml
task: 'weft producers: _lyx/config, lyx config, codeguide'
batch: lyx-config-command
number: 4
cards: 5
verify: go test -tags integration ./internal/configcli/ ./cmd/lyx/
depends-on: [1, 3]
```

## Batch Scope

Deliver the `lyx config` command in a new `internal/configcli` package — the one layer permitted
to import both `internal/config` (for `Edit`) and `internal/weft` (for `sync`), plus the modules
for their `ConfigTemplate`. `lyx config <module>` edits that module's YAML and triggers
`weft sync`; bare `lyx config` opens an interactive numbered menu (cloning the `internal/ide`
menu pattern). The post-edit sync routes `weft.RunCLI` output to `io.Discard` so its JSON never
contaminates the interactive stream; the command prints its own human-readable confirmation.
Depends on `module-config-templates` (templates) and `config-edit-machinery` (`config.Edit`).
Batch-local decisions: the config base dir is `filepath.Join(l.WorktreeRoot, l.RelPath)` (the
host `_lyx` parent — correct from any subdir, not raw cwd); abort returns exit 1; the editor and
sync are injected into an internal `dispatch`/`menu` so unit tests avoid a real editor and real
git, mirroring how `internal/ide` tests call `Menu` directly.

## Cards

### Card 10: Create `internal/configcli` package + module registry

- **Context:**
  - `internal/board/template.go`
  - `internal/worktree/template.go`
  - `internal/weft/template.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/configcli/configcli.go`
- **Deletes:** none
- **Requirements:** Create package `configcli` in `internal/configcli/configcli.go`. Define an
  ordered registry `var registry = []struct{ Name string; Template func() string }{ {"board",
  board.ConfigTemplate}, {"worktree", worktree.ConfigTemplate}, {"weft", weft.ConfigTemplate} }`
  (imports `internal/board`, `internal/worktree`, `internal/weft`). Add a helper
  `func templateFor(name string) (func() string, bool)` that returns the registry entry's
  `Template` and `true` if `name` matches, else `(nil, false)`. Add a helper
  `func moduleNames() []string` returning the registry names in order (for usage/menu text).
  No `codeguide` entry (a future task adds one line here).
- **Commit:** `feat(configcli): add config module registry`

### Card 11: `dispatch` + `editOne` + public `RunCLI`

- **Context:**
  - `internal/config/edit.go`
  - `internal/weft/cli.go`
  - `internal/paths/paths.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/configcli/configcli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/configcli/configcli.go` add:
  - `type syncFunc func() int` — runs the post-edit sync and returns an exit code.
  - `func editOne(baseDir string, out io.Writer, module string, edit config.EditorFunc, sync syncFunc) int`:
    look up `templateFor(module)` (unknown → print `unknown config module: <module> (known: <moduleNames>)`
    to `out`, return 1); call `config.Edit(baseDir, module, template(), edit)`; if it returns
    `config.ErrAborted` print `aborted: _lyx/config/<module>.yaml left unchanged` to `out` and
    return 1; on any other error print the error to `out` and return 1; on success call `sync()`
    — if non-zero print `edited _lyx/config/<module>.yaml but weft sync failed` and return 1,
    else print `edited and synced _lyx/config/<module>.yaml` and return 0.
  - `func dispatch(l *paths.Layout, in io.Reader, out io.Writer, args []string, edit config.EditorFunc, sync syncFunc) int`:
    compute `baseDir := filepath.Join(l.WorktreeRoot, l.RelPath)`; if `len(args) >= 1` call
    `editOne(baseDir, out, args[0], edit, sync)`; else call `menu(l, baseDir, in, out, edit, sync)`
    (Card 12).
  - `func RunCLI(out io.Writer, args []string) int`: resolve `cwd, err := paths.Getwd()` then
    `l, err := paths.Resolve(cwd)` (on error print to `out`, return 1); build the real editor
    `config.DefaultEditor` and the real sync `func() int { return weft.RunCLI(io.Discard,
    []string{"sync"}) }`; return `dispatch(l, os.Stdin, out, args, config.DefaultEditor, realSync)`.
  Output is human-readable text (this is the interactive-command exception to JSON output); the
  discarded-writer sync is what keeps the stream clean.
- **Commit:** `feat(configcli): implement lyx config <module> dispatch with discarded sync output`

### Card 12: Interactive bare-`lyx config` menu

- **Context:**
  - `internal/ide/menu.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/configcli/configcli.go`
- **Creates:**
  - `internal/configcli/menu.go`
- **Deletes:** none
- **Requirements:** In `internal/configcli/menu.go` add
  `func menu(l *paths.Layout, baseDir string, in io.Reader, out io.Writer, edit config.EditorFunc, sync syncFunc) int`
  following the `internal/ide/menu.go` stdlib pattern (no external TUI lib): print a numbered
  list of `moduleNames()`, each marked `(configured)` if
  `filepath.Join(baseDir, "_lyx", "config", name+".yaml")` exists via `os.Stat` else `(default)`;
  read one line from `in` with `bufio.NewReader(...).ReadString('\n')`; `q` quits (return 0);
  parse with `strconv.Atoi`, validate the 1-indexed range (invalid → print error to `out`, return
  1); on a valid choice call `editOne(baseDir, out, <chosen name>, edit, sync)` and return its
  code.
- **Commit:** `feat(configcli): add interactive bare lyx config menu`

### Card 13: Wire `cmd/lyx/main.go` + docs

- **Context:**
  - `internal/configcli/configcli.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `docs/roadmap.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `cmd/lyx/main.go` add `case "config": return configcli.RunCLI(out,
  moduleArgs)` to the dispatch switch, add the `internal/configcli` import, and add a `config`
  line to the module-list doc comment. In `docs/roadmap.md` update milestone 7 / "Task 008" to
  record that the `lyx config` menu interface landed while `_codeguide` junction activation and
  the codeguide config schema remain deferred to a later task. In `docs/overview.md` update the
  Status bullets (lines ~130-134) to the same effect: `lyx config` shipped; `_codeguide`
  activation still pending. Do not claim any codeguide work was done.
- **Commit:** `feat(lyx): wire lyx config command and update docs`

### Card 14: Tests — menu, dispatch, e2e sync

- **Context:**
  - `internal/configcli/configcli.go`
  - `internal/configcli/menu.go`
  - `internal/config/edit.go`
  - `internal/ide/menu_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `cmd/lyx/main.go`
  - `cmd/lyx/main_test.go`
- **Edits:**
  - `cmd/lyx/main_test.go`
- **Creates:**
  - `internal/configcli/configcli_test.go`
- **Deletes:** none
- **Requirements:** In `internal/configcli/configcli_test.go`: (a) unit-test `dispatch`/`editOne`
  with a fake `config.EditorFunc` (writes known valid YAML) and a fake `syncFunc` (records it was
  called, returns 0) over a temp `baseDir` with `_lyx/` present — assert success message, file
  written, and sync invoked; (b) unknown-module → exit 1 with the known-modules message and sync
  NOT called; (c) abort path → fake editor returns error, assert exit 1, abort message, sync NOT
  called; (d) menu — feed `in` a selection and a `q`, assert correct module routed and range
  validation; (e) an `//go:build integration` e2e test using `CopyPaired`: run `dispatch` with a
  fake editor and the REAL sync (`weft.RunCLI`), then assert `_lyx/config/<module>.yaml` is
  committed in the weft worktree and that the host worktree's tracked tree does not contain it.
  In `cmd/lyx/main_test.go` add a case asserting `run([]string{"config"}, out)` routes to the
  config command (e.g. errors cleanly when `_lyx` is absent rather than hitting the unknown-module
  default).
- **Commit:** `test(configcli): cover dispatch, menu, abort, and e2e sync`

## Batch Tests

`verify: go test -tags integration ./internal/configcli/ ./cmd/lyx/` runs the untagged
menu/dispatch unit tests (fake editor + fake sync) plus the integration-tagged e2e test (real
`weft.RunCLI` over a `CopyPaired` fixture) and the `cmd/lyx` routing test. The e2e test is the
proof that the edit writes through to the weft repo and the host stays pristine; the unit tests
pin the abort/unknown-module branches and that sync is skipped on abort.
