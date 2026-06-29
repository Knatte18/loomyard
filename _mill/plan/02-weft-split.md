# Batch: weft split

```yaml
task: "Rename Cobra modules to `<module>cli`, extract kernels as `<module>engine`"
batch: "weft split"
number: 2
cards: 3
verify: "go build ./... && go test ./... && go test -tags integration ./..."
depends-on: [1]
```

## Batch Scope

Split `internal/weft` into `internal/weftengine` (domain kernel) and `internal/weftcli`
(cobra command), delete `internal/weft`, and retarget every importer. The asymmetry vs.
board: weft's `spawn.go` (`spawnPush`) is a **CLI-only** mechanism (its sole caller is the
sync `RunE`), so it goes to `weftcli` and stays unexported; weft has no engine `Sync()`.
`scopedPathspec` (in `weft.go`, called by the cli `PersistentPreRunE`) moves to
`weftengine` and must be **exported** as `ScopedPathspec`. Importers retargeted this
batch: `cmd/lyx/main.go`, `internal/configreg/configreg.go`,
`internal/configreg/configreg_test.go`, `internal/configcli/configcli.go`,
`internal/configcli/configcli_integration_test.go` (its `weft.RunCLI` only — its `warp.*`
usage is retargeted in batch 3), and `internal/initcli/initcli_test.go`.

## Cards

### Card 5: Create `internal/weftengine` domain package

- **Context:**
  - `internal/weft/weft.go`
  - `internal/weft/config.go`
  - `internal/weft/sync.go`
  - `internal/weft/status.go`
  - `internal/weft/template.go`
  - `internal/weft/template.yaml`
  - `internal/weft/config_test.go`
  - `internal/weft/sync_test.go`
  - `internal/weft/status_test.go`
  - `internal/weft/template_test.go`
  - `internal/weft/weft_integration_test.go`
  - `internal/weft/cli.go`
- **Edits:** none
- **Creates:**
  - `internal/weftengine/weft.go`
  - `internal/weftengine/config.go`
  - `internal/weftengine/sync.go`
  - `internal/weftengine/status.go`
  - `internal/weftengine/template.go`
  - `internal/weftengine/template.yaml`
  - `internal/weftengine/config_test.go`
  - `internal/weftengine/sync_test.go`
  - `internal/weftengine/status_test.go`
  - `internal/weftengine/template_test.go`
  - `internal/weftengine/weft_integration_test.go`
- **Deletes:** none
- **Requirements:** Move the domain files (`weft.go` package doc + domain constants
  `commitMessage`/`lockDirName`/`writeLockFile`/`pushLockFile` and `scopedPathspec`;
  `config.go`; `sync.go` with `Commit`/`Push`/`Pull`/`SyncOptions`; `status.go` with
  `Status`; `template.go` with `ConfigTemplate`; the `template.yaml` asset) and their
  domain `*_test.go` files into `internal/weftengine` with package clause
  `package weft` → `package weftengine`. **Rename `scopedPathspec` → exported
  `ScopedPathspec`** (it is called cross-package by the weftcli `PersistentPreRunE` in
  card 6). Keep `commitMessage`/`lockDirName`/`writeLockFile`/`pushLockFile` unexported
  (used only by `sync.go`). `cli.go` is read-only Context here only to confirm which
  symbols the cli half consumes — do not move it (card 6). Preserve the `//go:build
  integration` tag verbatim on `sync_test.go`, `status_test.go`, and
  `weft_integration_test.go`. Do not delete `internal/weft` yet (card 7).
- **Commit:** `refactor(weft): extract weftengine domain package`

### Card 6: Create `internal/weftcli` command package

- **Context:**
  - `internal/weft/cli.go`
  - `internal/weft/spawn.go`
  - `internal/weft/cli_test.go`
  - `internal/weft/weft.go`
  - `internal/weft/sync.go`
  - `internal/clihelp/exec.go`
- **Edits:** none
- **Creates:**
  - `internal/weftcli/cli.go`
  - `internal/weftcli/spawn.go`
  - `internal/weftcli/cli_test.go`
- **Deletes:** none
- **Requirements:** Move `cli.go` (with the rich `PersistentPreRunE`, the hidden
  `--weft-path` bypass, `Command()`, and the `RunCLI` seam), `spawn.go` (`spawnPush` —
  CLI-only caller, stays unexported), and `cli_test.go` into `internal/weftcli` with
  package clause `package weft` → `package weftcli`. Add the `internal/weftengine` import
  and qualify every engine symbol the cli half uses as `weftengine.<Symbol>` — including
  `weftengine.ScopedPathspec` (replacing the old in-package `scopedPathspec` call at the
  `PersistentPreRunE`), plus `Commit`/`Push`/`Pull`/`SyncOptions`/`Status` and any other
  engine symbols referenced. The `RunCLI` seam body stays exactly
  `clihelp.Execute(Command(), out, args)`. Preserve the `//go:build integration` tag
  verbatim on `cli_test.go`.
- **Commit:** `refactor(weft): extract weftcli command package`

### Card 7: Retarget importers and delete `internal/weft`

- **Context:**
  - `internal/weft/weft.go`
  - `internal/weft/cli.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
  - `internal/configcli/configcli.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/initcli/initcli_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/weft/weft.go`
  - `internal/weft/config.go`
  - `internal/weft/sync.go`
  - `internal/weft/status.go`
  - `internal/weft/template.go`
  - `internal/weft/template.yaml`
  - `internal/weft/spawn.go`
  - `internal/weft/cli.go`
  - `internal/weft/config_test.go`
  - `internal/weft/sync_test.go`
  - `internal/weft/status_test.go`
  - `internal/weft/template_test.go`
  - `internal/weft/weft_integration_test.go`
  - `internal/weft/cli_test.go`
- **Requirements:** In `cmd/lyx/main.go` replace the `internal/weft` import with
  `internal/weftcli` and change `weft.Command()` to `weftcli.Command()`. In
  `internal/configreg/configreg.go` replace the `internal/weft` import with
  `internal/weftengine` and change `{"weft", weft.ConfigTemplate}` to
  `{"weft", weftengine.ConfigTemplate}` (module name string stays `"weft"`). In
  `internal/configreg/configreg_test.go` replace the `internal/weft` import with
  `internal/weftengine` and change `weft.ConfigTemplate()` to
  `weftengine.ConfigTemplate()`. In `internal/configcli/configcli.go` replace the
  `internal/weft` import with `internal/weftcli` and change the `realSync` body's
  `weft.RunCLI(w, []string{"sync"})` to `weftcli.RunCLI(...)`. In
  `internal/configcli/configcli_integration_test.go` change only the `weft.RunCLI`
  call(s) to `weftcli.RunCLI` and update its `internal/weft` import to `internal/weftcli`
  — leave its `warp.*` usage and `internal/warp` import untouched (batch 3 handles warp).
  In `internal/initcli/initcli_test.go` replace the `internal/weft` import with
  `internal/weftengine` and change `weft.LoadConfig` to `weftengine.LoadConfig`. Then
  delete the entire `internal/weft` directory.
- **Commit:** `refactor(weft): retarget importers and remove internal/weft`

## Batch Tests

`verify` is repo-wide for the same two reasons as batch 1: `cmd/lyx/main.go` imports
every module (rename compile errors surface only repo-wide), and the relocated weft
suites `cli_test.go`, `sync_test.go`, `status_test.go`, and `weft_integration_test.go`
are `integration`-tagged (invisible to plain `go test ./...`). Moved coverage: the
weftengine domain suites (`config_test`, `sync_test`, `status_test`, `template_test`,
`weft_integration_test`) and the weftcli `cli_test`. The `internal/configcli` integration
test and `internal/initcli` test re-exercise the retargeted seams; the cmd/lyx guard
tests self-derive and re-validate the renamed `weftcli` registration.
