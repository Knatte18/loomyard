# Batch: ide split

```yaml
task: "Rename Cobra modules to `<module>cli`, extract kernels as `<module>engine`"
batch: "ide split"
number: 4
cards: 3
verify: "go build ./... && go test ./... && go test -tags integration ./..."
depends-on: [3]
```

## Batch Scope

Split `internal/ide` into `internal/ideengine` (`Spawn`, `Menu`) and `internal/idecli`
(cobra command), delete `internal/ide`, retarget `cmd/lyx/main.go`. `ideengine` is the one
engine→engine edge: `menu.go` imports `boardengine` (created in batch 1). The
`codeLauncher` seam (`var codeLauncher = vscode.Launch` in `spawn.go`) is swapped by tests
in both halves, so it must be **exported** as a settable `ideengine.CodeLauncher`:
`spawn.go`/`menu.go` reference `CodeLauncher`, the in-package engine tests swap it
directly, and `idecli`'s `cli_test.go` swaps `ideengine.CodeLauncher` cross-package.

## Cards

### Card 12: Create `internal/ideengine` domain package

- **Context:**
  - `internal/ide/spawn.go`
  - `internal/ide/menu.go`
  - `internal/ide/spawn_test.go`
  - `internal/ide/menu_test.go`
  - `internal/ide/cli.go`
  - `internal/vscode/launch_windows.go`
  - `internal/vscode/config.go`
- **Edits:** none
- **Creates:**
  - `internal/ideengine/spawn.go`
  - `internal/ideengine/menu.go`
  - `internal/ideengine/spawn_test.go`
  - `internal/ideengine/menu_test.go`
- **Deletes:** none
- **Requirements:** Move `spawn.go` (`Spawn(l *paths.Layout, slug string) error`) and
  `menu.go` (`Menu(...)`) into `internal/ideengine` with package clause
  `package ide` → `package ideengine`. In `menu.go` add the `internal/boardengine` import
  and change `board.LoadConfig` and `board.New` to `boardengine.LoadConfig` /
  `boardengine.New` (method calls on the returned `Board` value, e.g. `b.HealthCheck`/
  `b.GetTask`, are unqualified and unchanged); remove the old `internal/board` import.
  **Export the `codeLauncher` seam:** rename `var codeLauncher = vscode.Launch` to the
  exported settable `var CodeLauncher = vscode.Launch` and update the call site in
  `spawn.go` (`codeLauncher(openDir)` → `CodeLauncher(openDir)`) and any reference in
  `menu.go`. Move the engine tests `spawn_test.go` and `menu_test.go`, changing them to
  package `ideengine` and swapping the in-package `CodeLauncher` directly; preserve the
  `//go:build integration` tag verbatim on `menu_test.go` (and on `spawn_test.go` only if
  the original carried it). `cli.go` is read-only Context (do not move — card 13). Do not
  delete `internal/ide` (card 14).
- **Commit:** `refactor(ide): extract ideengine domain package`

### Card 13: Create `internal/idecli` command package

- **Context:**
  - `internal/ide/cli.go`
  - `internal/ide/cli_test.go`
  - `internal/ide/spawn.go`
  - `internal/clihelp/exec.go`
- **Edits:** none
- **Creates:**
  - `internal/idecli/cli.go`
  - `internal/idecli/cli_test.go`
- **Deletes:** none
- **Requirements:** Move `cli.go` (`Command()` + the `RunCLI` seam) and `cli_test.go` into
  `internal/idecli` with package clause `package ide` → `package idecli`. Add the
  `internal/ideengine` import and qualify the engine entry points the command invokes
  (`Spawn`, `Menu`) as `ideengine.<Symbol>`. In `cli_test.go` swap the exported
  `ideengine.CodeLauncher` seam cross-package (replacing the old in-package `codeLauncher`
  swap). The `RunCLI` seam body stays exactly `clihelp.Execute(Command(), out, args)`.
  Preserve the `//go:build integration` tag verbatim on `cli_test.go`.
- **Commit:** `refactor(ide): extract idecli command package`

### Card 14: Retarget importer and delete `internal/ide`

- **Context:**
  - `internal/ide/cli.go`
  - `internal/ide/spawn.go`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:**
  - `internal/ide/cli.go`
  - `internal/ide/menu.go`
  - `internal/ide/spawn.go`
  - `internal/ide/cli_test.go`
  - `internal/ide/menu_test.go`
  - `internal/ide/spawn_test.go`
- **Requirements:** In `cmd/lyx/main.go` replace the `internal/ide` import with
  `internal/idecli` and change `ide.Command()` to `idecli.Command()` in `newRoot()`. Then
  delete the entire `internal/ide` directory.
- **Commit:** `refactor(ide): retarget importer and remove internal/ide`

## Batch Tests

`verify` is repo-wide (Tier 1 + Tier 2). The relocated `ideengine/menu_test.go` and
`idecli/cli_test.go` are `integration`-tagged, so the `-tags integration` run is required
to compile-and-run them. Moved coverage: `ideengine` `spawn_test`/`menu_test` (driving
`Spawn`/`Menu` with the swapped in-package `CodeLauncher`) and `idecli` `cli_test`
(driving the command through `RunCLI` with the cross-package `ideengine.CodeLauncher`
swap). The engine→engine edge (`ideengine` → `boardengine`) is compile-checked by
`go build ./...`. cmd/lyx guard tests self-derive and re-validate the renamed `idecli`
registration.
