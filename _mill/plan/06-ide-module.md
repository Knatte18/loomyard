# Batch: ide-module

```yaml
task: 'Extend worktree module: portals and launchers'
batch: 'ide-module'
number: 6
cards: 7
verify: go test ./internal/ide/... ./cmd/mhgo/...
depends-on: [1, 2, 4]
```

## Batch Scope

This batch adds the new `mhgo ide` module (`internal/ide`): `ide spawn <slug>`
generates a worktree's `.vscode/` config (only when absent), assigns a title-bar
color, registers `.vscode/` in the managed `.gitignore`, and launches VS Code;
`ide menu` is the one interactive picker over active worktrees (slug + title via
the board facade, hard-erroring through `board.HealthCheck` when the board is
absent). It wires `case "ide"` into `cmd/mhgo/main.go`. VS Code launch and the
menu are Windows-only (POSIX no-ops/errors with a clear message); config
generation and color picking are cross-platform. Mill values (palette, settings
keys, `cmd /c code`) are baked from `## Shared Decisions` — no external Python is
read. The package doc comment lives atop `cli.go`. Cards are verified together at
batch end.

## Cards

### Card 19: Color palette and picker

- **Context:**
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/ide/color.go`
  - `internal/ide/color_test.go`
- **Deletes:** none
- **Requirements:** In a new `package ide`, define `palette = []string{"#2d7d46",
  "#7d2d6b", "#2d4f7d", "#7d5c2d", "#6b2d2d", "#2d6b6b", "#4a2d7d", "#7d462d"}`
  and `mainColor = "#2d7d46"`. Add `pickColor(l *paths.Layout) string`: collect
  the set of `titleBar.activeBackground` hex values (lowercased) from each sibling
  worktree's `<l.Container>/<dir>/<l.RelPath>/.vscode/settings.json`
  (`workbench.colorCustomizations.titleBar.activeBackground`), skipping the main
  worktree and any dir without a readable settings file; return the first
  palette color whose lowercase is not `mainColor` AND not in the used set; if
  every non-green color is used, return the first non-green
  (`palette[1]`). A missing container/dirs yields the first non-green.
  `color_test.go`: green is never returned for a child; first-unused-non-green is
  chosen given a set of sibling colors; wrap-around returns the first non-green
  when all are used; unreadable/missing settings are ignored.
- **Commit:** `feat(ide): add worktree color palette and picker`

### Card 20: VS Code config generation

- **Context:**
  - `internal/gitignore/gitignore.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/ide/vscode.go`
  - `internal/ide/vscode_test.go`
- **Deletes:** none
- **Requirements:** Add `writeVSCodeConfig(worktreeDir, relpath, slug, color
  string) error` that writes, ONLY IF ABSENT (never clobbering operator edits),
  into `dir := filepath.Join(worktreeDir, relpath)` the files
  `<dir>/.vscode/settings.json` and `<dir>/.vscode/tasks.json`.
  `settings.json` (marshaled JSON) carries: `workbench.colorCustomizations` =
  `{titleBar.activeBackground: color, titleBar.activeForeground: "#ffffff",
  titleBar.inactiveBackground: color, titleBar.inactiveForeground: "#ffffffaa"}`;
  `window.title` = `slug`; `workbench.startupEditor: "none"`;
  `workbench.secondarySideBar.defaultVisibility: "hidden"` (verify the exact
  panel-hiding key against the installed VS Code during implementation and adjust
  if needed). `tasks.json` defines one `Start Claude` shell task with
  `runOptions.runOn: "folderOpen"` running `claude` in a dedicated integrated
  terminal. After writing, register `.vscode/` via `gitignore.Ensure(dir,
  ".vscode/")` (the committed root that holds `_mhgo/` and `.gitignore`).
  `vscode_test.go`: both files created when absent; neither
  clobbered when already present; `.vscode/` registered in `.gitignore`; the
  expected settings keys and the folderOpen task are present.
- **Commit:** `feat(ide): generate non-clobbering VS Code settings and tasks`

### Card 21: VS Code launch (build-tag split)

- **Context:**
  - `internal/muxpoc/spawn_windows.go`
  - `internal/git/git_windows.go`
- **Edits:** none
- **Creates:**
  - `internal/ide/launch_windows.go`
  - `internal/ide/launch_other.go`
- **Deletes:** none
- **Requirements:** Add `launchCode(worktreeDir string) error` with a build-tag
  split. Windows (`//go:build windows`): run `exec.Command("cmd", "/c", "code",
  worktreeDir)` (so `code.cmd` resolves via the full PATH), applying the
  no-console-window flag pattern from `git_windows.go`/`spawn_windows.go`; return
  any start error. POSIX (`//go:build !windows`): return an error "ide launch
  unsupported on this platform". (The injectable seam used by tests is declared in
  `spawn.go`, card 22, not here.)
- **Commit:** `feat(ide): add platform-split VS Code launcher`

### Card 22: Spawn

- **Context:**
  - `internal/paths/paths.go`
  - `internal/ide/color.go`
  - `internal/ide/vscode.go`
  - `internal/ide/launch_windows.go`
- **Edits:** none
- **Creates:**
  - `internal/ide/spawn.go`
  - `internal/ide/spawn_test.go`
- **Deletes:** none
- **Requirements:** Add a package-level injectable seam `var codeLauncher =
  launchCode` (overridable in tests). Add `Spawn(l *paths.Layout, slug string)
  error`: compute `worktreeDir := l.WorktreePath(slug)`, `color := pickColor(l)`,
  call `writeVSCodeConfig(worktreeDir, l.RelPath, slug, color)`, then open the
  worktree at its relpath (the dir holding `_mhgo/` and `.vscode/`) via
  `codeLauncher(filepath.Join(worktreeDir, l.RelPath))`. `spawn_test.go`: with
  `codeLauncher` stubbed to record its argument (no real VS Code), `Spawn`
  generates the `.vscode/` config under the target worktree and invokes the
  launcher with the opened dir; a second `Spawn` does not clobber existing
  `.vscode/` files.
- **Commit:** `feat(ide): add ide spawn`

### Card 23: Menu

- **Context:**
  - `internal/paths/paths.go`
  - `internal/board/board.go`
  - `internal/board/config.go`
  - `internal/output/output.go`
  - `internal/ide/spawn.go`
- **Edits:** none
- **Creates:**
  - `internal/ide/menu.go`
  - `internal/ide/menu_test.go`
- **Deletes:** none
- **Requirements:** Add `Menu(l *paths.Layout, in io.Reader, out io.Writer)
  error` (or an int-returning CLI-style fn — match what `cli.go` in card 24
  needs). Discover active worktrees via `paths.List(l.Cwd)`, excluding the
  `Main==true` entry and keeping only those whose `<path>/<l.RelPath>/_mhgo`
  exists; slug = `filepath.Base(path)`. Build the board facade: `cfg, _ :=
  board.LoadConfig(l.Cwd, "board")`; `b := board.New(cfg)`; if `b.HealthCheck()
  != nil` return a HARD error (board must be present). Resolve each slug's title
  ONLY through the board facade (`b.GetTask(slug)` → `Task.Title`, or
  `b.ListTasksBrief()` keyed by slug); never stat the board dir directly. Print a
  numbered picker `N) <slug> — <title>` to `out`, read a line from `in`: a number
  opens that worktree via `Spawn(l, chosenSlug)`; `q` quits; zero active
  worktrees prints a message and returns success; invalid input re-prompts or
  errors. `menu_test.go` (stub `codeLauncher`): discovery excludes main and
  requires `_mhgo/`; titles come from the board facade; a HARD error when
  `HealthCheck` fails (board dir absent); a numeric selection maps to the correct
  worktree and calls `Spawn`; the zero-worktree path prints its message.
- **Commit:** `feat(ide): add interactive ide menu`

### Card 24: CLI router

- **Context:**
  - `internal/paths/paths.go`
  - `internal/output/output.go`
  - `internal/ide/spawn.go`
  - `internal/ide/menu.go`
- **Edits:** none
- **Creates:**
  - `internal/ide/cli.go`
  - `internal/ide/cli_test.go`
- **Deletes:** none
- **Requirements:** Add `RunCLI(out io.Writer, args []string) int` with the
  package doc comment atop the file. Resolve `cwd, err := paths.Getwd()` then `l,
  err := paths.Resolve(cwd)` (JSON error + exit 1 on failure). Subcommands:
  `spawn <slug>` → `Spawn(l, slug)` then `output.Ok`; `menu` → `Menu(l, os.Stdin,
  out)` (the documented interactive exception). Missing slug / unknown subcommand
  → `output.Err` with usage. `cli_test.go`: `spawn` dispatch with a stubbed
  `codeLauncher`; unknown-subcommand and missing-slug error envelopes; usage on
  no args.
- **Commit:** `feat(ide): add ide CLI router`

### Card 25: Wire `case "ide"` into main

- **Context:**
  - `internal/ide/cli.go`
- **Edits:**
  - `cmd/mhgo/main.go`
  - `cmd/mhgo/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `cmd/mhgo/main.go`, import
  `github.com/Knatte18/mhgo/internal/ide` and add `case "ide": return
  ide.RunCLI(out, moduleArgs)` to the dispatch switch; add `ide` to the module
  list in the package doc comment. In `cmd/mhgo/main_test.go`, add a dispatch
  case asserting `mhgo ide` routes to the ide module (e.g. an unknown ide
  subcommand or usage path returns the expected exit code), mirroring the existing
  per-module dispatch tests.
- **Commit:** `feat(mhgo): route ide module in main dispatcher`

## Batch Tests

`verify: go test ./internal/ide/... ./cmd/mhgo/...` runs the new ide suite
(`color_test.go`, `vscode_test.go`, `spawn_test.go`, `menu_test.go`,
`cli_test.go`) with the `codeLauncher` seam stubbed so no real VS Code opens, plus
the updated `cmd/mhgo` dispatch test. Windows-only launch paths are exercised
through the stub; the POSIX `launch_other.go` error path compiles under non-
Windows builds. Scope is the new `ide` package plus the `cmd/mhgo` dispatcher
this batch edits.
