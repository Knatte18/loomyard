# Plan: Extract internal/proc (cross-OS windowless + detached spawn)

```yaml
task: "Extract internal/proc (cross-OS windowless + detached spawn)"
slug: extract-internal-proc
approved: false
started: 20260623-120601
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: Create internal/proc
    file: 01-create-internal-proc.md
    depends-on: []
    verify: go test ./internal/proc/...
  - number: 2
    name: Migrate callers
    file: 02-migrate-callers.md
    depends-on: [1]
    verify: go test ./internal/git/... ./internal/board/... ./internal/weft/... ./internal/muxpoc/... ./internal/vscode/...
```

## Shared Decisions

### Decision: Behavior-preserving refactor

- **Decision:** No logic changes. `proc.HideWindow` and `proc.Detach` replicate the exact `SysProcAttr` assignments that each package sets today. The compile-time behavior (constants, flags) is identical.
- **Rationale:** The task objective is consolidation, not feature change. The existing test suites across all affected packages are the behavioral guardrail.
- **Applies to:** all batches

### Decision: Deletes via `git rm`

- **Decision:** All file deletions use `git rm <path>` so the removal is staged for the next commit. Do not use `os.Remove` or shell `rm` for tracked files.
- **Rationale:** `git rm` stages the deletion correctly; bare file removal leaves a tracked path as "deleted in working tree" and can cause merge confusion.
- **Applies to:** batch 2

### Decision: proc package name and file naming

- **Decision:** Package `proc` at `internal/proc`; platform-split files are `proc_windows.go` (`//go:build windows`) and `proc_other.go` (`//go:build !windows`). Tests follow `proc_windows_test.go` / `proc_other_test.go`. New muxpoc files for the relocated `spawnAttach` function are `spawnattach_windows.go` (`//go:build windows`) and `spawnattach_other.go` (`//go:build !windows`).
- **Rationale:** Matches the existing codebase convention (`git_windows.go`/`git_other.go`, `fslink_windows.go`/`fslink.go`).
- **Applies to:** batch 1, batch 2

### Decision: proc.Detach touches only SysProcAttr

- **Decision:** `proc.Detach` assigns only `cmd.SysProcAttr`. It never touches `cmd.Env`, `cmd.Stdin`, `cmd.Stdout`, or `cmd.Stderr`.
- **Rationale:** `muxpoc/up.go` sets `cmd.Env = clean` before calling `spawnServer`. Replacing `spawnServer` with `proc.Detach` must not disturb that assignment. The single-field assignment is a documented invariant for all callers.
- **Applies to:** all batches

## All Files Touched

- `docs/shared-libs/README.md`
- `internal/board/spawn.go`
- `internal/git/git.go`
- `internal/muxpoc/spawnattach_other.go`
- `internal/muxpoc/spawnattach_windows.go`
- `internal/muxpoc/up.go`
- `internal/proc/proc_other.go`
- `internal/proc/proc_other_test.go`
- `internal/proc/proc_windows.go`
- `internal/proc/proc_windows_test.go`
- `internal/vscode/launch_windows.go`
- `internal/weft/spawn.go`
