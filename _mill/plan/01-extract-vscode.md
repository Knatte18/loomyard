# Batch: extract-vscode

```yaml
task: "Extract internal/vscode; keep ide IDE-generic"
batch: "extract-vscode"
number: 1
cards: 3
verify: go test ./...
depends-on: []
```

## Batch Scope

This batch performs the entire behavior-preserving extraction in one unit. It
creates the new `internal/vscode` package (config generation, color picker,
launcher), migrates the two white-box tests that exercise the moved symbols,
then rewires `internal/ide` to consume the new package and deletes the now-moved
`ide` files. It is a single batch because the moved code, its tests, and the
`ide` rewire all share the same small set of files and must compile together as
one consistent unit — splitting would leave an intermediate non-compiling state.

External interface produced for the rest of the codebase: the new package
`github.com/Knatte18/loomyard/internal/vscode` exposing `WriteConfig`,
`PickColor`, `Launch`, and `ErrUnsupported`. The only consumer is
`internal/ide/spawn.go` (rewired in Card 3). `cmd/lyx/main.go` is untouched — it
calls `ide.RunCLI`, whose signature does not change.

Batch-local decisions: none beyond the `## Shared Decisions` in the overview.

## Cards

### Card 1: Create internal/vscode package (production code)

- **Context:**
  - `internal/ide/vscode.go`
  - `internal/ide/color.go`
  - `internal/ide/launch_windows.go`
  - `internal/ide/launch_other.go`
  - `internal/paths/paths.go`
  - `internal/gitignore/gitignore.go`
- **Edits:** none
- **Creates:**
  - `internal/vscode/config.go`
  - `internal/vscode/color.go`
  - `internal/vscode/launch_windows.go`
  - `internal/vscode/launch_other.go`
- **Deletes:** none
- **Requirements:**
  - All four new files declare `package vscode`.
  - `internal/vscode/config.go`: port `writeVSCodeConfig` from
    `internal/ide/vscode.go` verbatim, exported as
    `func WriteConfig(worktreeDir, relpath, slug, color string) error`. Preserve
    the body exactly — same `settings.json` map (keys
    `workbench.colorCustomizations`, `files.watcherExclude` with `**/_lyx/**`,
    `window.title`, `workbench.startupEditor`, `workbench.secondarySideBar.defaultVisibility`),
    same `tasks.json` map ("Start Claude" shell task, `runOptions.runOn:
    folderOpen`), the same absent-only write guards, and the trailing
    `gitignore.Ensure(dir, ".vscode/")` call (import
    `github.com/Knatte18/loomyard/internal/gitignore`). Carry the
    package doc comment `// Package vscode ...` on this file describing the
    VS-Code-specific responsibilities (config generation, color picking, launch).
  - `internal/vscode/color.go`: port `pickColor` from `internal/ide/color.go`
    verbatim, exported as `func PickColor(l *paths.Layout) string` (import
    `github.com/Knatte18/loomyard/internal/paths`). Keep `palette` and
    `mainColor` as **unexported** package vars with identical hex values and
    order (green `#2d7d46` reserved at `palette[0]`). Define
    `var ErrUnsupported = errors.New("vscode launch unsupported on this platform")`
    in this build-tag-neutral file (was `ErrIDEUnsupported` in
    `internal/ide/color.go`).
  - `internal/vscode/launch_windows.go`: keep the `//go:build windows`
    constraint; port `launchCode` exported as `func Launch(worktreeDir string)
    error`, preserving the `cmd /c code <worktreeDir>` invocation, the
    `createNoWindow = 0x08000000` const, and the `syscall.SysProcAttr`
    no-console-window flags.
  - `internal/vscode/launch_other.go`: keep the `//go:build !windows`
    constraint; port `launchCode` exported as `func Launch(worktreeDir string)
    error` returning `ErrUnsupported`.
- **Commit:** `feat(vscode): add internal/vscode package extracted from ide`

### Card 2: Migrate white-box tests to internal/vscode

- **Context:**
  - `internal/ide/color_test.go`
  - `internal/ide/vscode_test.go`
  - `internal/vscode/color.go`
  - `internal/vscode/config.go`
- **Edits:** none
- **Creates:**
  - `internal/vscode/color_test.go`
  - `internal/vscode/config_test.go`
- **Deletes:** none
- **Requirements:**
  - `internal/vscode/color_test.go`: port `internal/ide/color_test.go` verbatim
    into `package vscode`, replacing every `pickColor(` call site with
    `PickColor(`. References to `palette` and `mainColor` stay unchanged
    (white-box, same package). Keep all four tests:
    `TestPickColorNeverReturnsGreen`, `TestPickColorFirstUnusedNonGreen`,
    `TestPickColorWrapAroundAllUsed`, `TestPickColorIgnoresUnreadable`.
  - `internal/vscode/config_test.go`: port `internal/ide/vscode_test.go` into
    `package vscode`, replacing every `writeVSCodeConfig(` call site with
    `WriteConfig(`. Keep all three tests:
    `TestWriteVSCodeConfigCreatesFilesWhenAbsent`,
    `TestWriteVSCodeConfigDoesNotClobber`,
    `TestWriteVSCodeConfigRegistersInGitignore`.
  - Both files remain white-box (`package vscode`, not `vscode_test`) so they
    keep access to `palette` / `mainColor`. Carry no `//go:build` tags (the
    originals have none).
- **Commit:** `test(vscode): migrate color and config tests from ide`

### Card 3: Rewire ide and delete moved files

- **Context:**
  - `internal/vscode/config.go`
  - `internal/vscode/color.go`
  - `internal/vscode/launch_other.go`
- **Edits:**
  - `internal/ide/spawn.go`
  - `internal/ide/cli.go`
- **Creates:** none
- **Deletes:**
  - `internal/ide/vscode.go`
  - `internal/ide/color.go`
  - `internal/ide/launch_windows.go`
  - `internal/ide/launch_other.go`
  - `internal/ide/color_test.go`
  - `internal/ide/vscode_test.go`
- **Requirements:**
  - `internal/ide/spawn.go`: add import
    `github.com/Knatte18/loomyard/internal/vscode`. Change `color := pickColor(l)`
    to `color := vscode.PickColor(l)`; change the
    `writeVSCodeConfig(worktreeDir, l.RelPath, slug, color)` call to
    `vscode.WriteConfig(worktreeDir, l.RelPath, slug, color)`; change
    `var codeLauncher = launchCode` to `var codeLauncher = vscode.Launch`. Do
    not change the `Spawn` flow shape, the `openDir` computation, or the
    `codeLauncher(openDir)` call. The `codeLauncher` seam stays in this file.
  - `internal/ide/cli.go`: rewrite the `// Package ide ...` doc comment so it
    describes the generic spawn/menu/dispatch responsibility with VS Code
    specifics delegated to `internal/vscode` (config generation, color palette,
    launch command now live in `internal/vscode`). Do not change `RunCLI` or any
    dispatch logic — only the doc comment.
  - Delete the six listed `internal/ide` files (the moved production files and
    the two migrated test files). After deletion, `internal/ide` retains only
    `cli.go`, `spawn.go`, `menu.go`, and the unchanged tests `cli_test.go`,
    `menu_test.go`, `spawn_test.go`.
  - The result must compile module-wide and leave no dangling references to
    `pickColor`, `writeVSCodeConfig`, `launchCode`, or `ErrIDEUnsupported` in
    `internal/ide`.
- **Commit:** `refactor(ide): rewire spawn to internal/vscode, drop moved files`

## Batch Tests

`verify: go test ./...` (Go native runner, no `PYTHONPATH=` prefix — Go project).

The module is small (~16 internal packages, Go build cache makes unchanged
packages instant after the first run), so the unbounded `go test ./...` is the
right scope here rather than a per-package list: this is a cross-package move
that must (a) compile the entire module — catching any break in the lone
consumer `cmd/lyx` whose `ide.RunCLI` call must still resolve — and (b) run
`internal/paths/enforcement_test.go`, the tree-wide scan that fails the build if
the new `internal/vscode` package were to use raw `os.Getwd` or `git rev-parse`
(it does not — `PickColor` takes a resolved `*paths.Layout`). A per-package list
would miss the whole-module compile guarantee.

Key scenarios covered by the migrated/retained tests:
- `internal/vscode/config_test.go` — config files created when absent, never
  clobbered, `.vscode/` registered in `.gitignore`.
- `internal/vscode/color_test.go` — picker never returns green, picks first
  unused non-green, wraps when all used, ignores unreadable siblings.
- `internal/ide/spawn_test.go` (unchanged) — end-to-end `Spawn` flow with the
  stubbed `codeLauncher`, asserting written `settings.json`/`tasks.json` and
  color, i.e. the rewired `vscode.*` call path under the mandatory gate.

The integration-tagged `cli_test.go` and `menu_test.go` are NOT run by
`go test ./...` (no `-tags integration`); per the discussion's deliberate
operator decision the integration variant is an optional, non-blocking gate.
