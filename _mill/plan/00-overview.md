# Plan: Extract internal/vscode; keep ide IDE-generic

```yaml
task: "Extract internal/vscode; keep ide IDE-generic"
slug: "extract-internal-vscode"
approved: true
started: "20260622-175518"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: extract-vscode
    file: 01-extract-vscode.md
    depends-on: []
    verify: go test ./...
```

## Shared Decisions

### Decision: direct-import-no-interface

- **Decision:** `internal/ide` imports `internal/vscode` directly and calls
  `vscode.WriteConfig` / `vscode.PickColor` / `vscode.Launch`. No `Backend`
  interface is introduced.
- **Rationale:** Exactly one IDE backend exists (VS Code). "Keep ide
  IDE-generic" is satisfied by factoring out the VS Code *details*, not by
  abstracting the backend (YAGNI).
- **Applies to:** all batches

### Decision: launcher-seam-stays-in-ide

- **Decision:** The injectable `var codeLauncher = launchCode` seam in
  `internal/ide/spawn.go` is retained in `ide` and re-pointed to
  `vscode.Launch` (`var codeLauncher = vscode.Launch`).
- **Rationale:** The seam is stubbed by three white-box test files
  (`spawn_test.go`, `cli_test.go`, `menu_test.go`); keeping it in `ide` means
  those files need zero change. Only the two tests that directly exercise moved
  symbols migrate.
- **Applies to:** all batches

### Decision: exported-api-naming

- **Decision:** Moved functions are exported as `vscode.WriteConfig` (was
  `writeVSCodeConfig`), `vscode.PickColor` (was `pickColor`), `vscode.Launch`
  (was `launchCode`). `palette` and `mainColor` stay **unexported**.
  `ErrIDEUnsupported` is renamed `vscode.ErrUnsupported`.
- **Rationale:** Idiomatic Go — package-qualified call sites avoid stutter
  (`vscode.WriteConfig`, not `vscode.WriteVSCodeConfig`). `palette`/`mainColor`
  are only used in-package and by white-box tests, so they stay unexported.
- **Applies to:** all batches

### Decision: behavior-preserving

- **Decision:** No behavior change. Same files written (`settings.json`,
  `tasks.json`), same palette, same `cmd /c code` launch command, same
  `.gitignore` registration, same error semantics. Only symbol names, package
  location, and the `ide` package doc comment change.
- **Rationale:** This is a physical extraction; the existing tests are the
  guardrail.
- **Applies to:** all batches

### Decision: errunsupported-build-neutral-placement

- **Decision:** `vscode.ErrUnsupported` is defined in a build-tag-neutral file
  (`internal/vscode/color.go`), never inside a `//go:build`-tagged file, because
  both `launch_windows.go` and `launch_other.go` build variants reference it.
- **Rationale:** A `var` referenced by both build variants must compile under
  every tag; placing it behind a build tag would break the other variant.
- **Applies to:** all batches

## All Files Touched

- `internal/ide/cli.go`
- `internal/ide/spawn.go`
- `internal/vscode/color.go`
- `internal/vscode/color_test.go`
- `internal/vscode/config.go`
- `internal/vscode/config_test.go`
- `internal/vscode/launch_other.go`
- `internal/vscode/launch_windows.go`
