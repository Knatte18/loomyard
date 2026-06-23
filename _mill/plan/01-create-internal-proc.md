# Batch: Create internal/proc

```yaml
task: "Extract internal/proc (cross-OS windowless + detached spawn)"
batch: "Create internal/proc"
number: 1
cards: 2
verify: go test ./internal/proc/...
depends-on: []
```

## Batch Scope

Creates the new `internal/proc` package from scratch: two platform-split implementation files (`proc_windows.go`, `proc_other.go`) and matching test files (`proc_windows_test.go`, `proc_other_test.go`). This batch produces no changes to existing files — it is pure addition. Batch 2 depends on this batch and imports `github.com/Knatte18/loomyard/internal/proc`.

## Cards

### Card 1: Create proc implementation files

- **Context:**
  - `go.mod`
  - `internal/git/git_windows.go`
  - `internal/git/git_other.go`
  - `internal/board/spawn_windows.go`
  - `internal/board/spawn_other.go`
- **Edits:** none
- **Creates:**
  - `internal/proc/proc_windows.go`
  - `internal/proc/proc_other.go`
- **Deletes:** none
- **Requirements:**
  `internal/proc/proc_windows.go` — `//go:build windows`; package `proc`; package-level doc comment: `// Package proc provides cross-OS primitives for controlling child-process window visibility and detachment.` (on the Windows file only — the other file has only a build tag, no package doc); imports `"os/exec"` and `"syscall"`; defines `const createNoWindow uint32 = 0x08000000` and `const createNewProcessGroup uint32 = 0x00000200` (unexported); implements `HideWindow(cmd *exec.Cmd)` — assigns `cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}`; implements `Detach(cmd *exec.Cmd)` — assigns `cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | createNewProcessGroup}`. Neither function returns an error or touches any field of `cmd` other than `SysProcAttr`.

  `internal/proc/proc_other.go` — `//go:build !windows`; package `proc`; no package doc; imports `"os/exec"` and `"syscall"`; implements `HideWindow(cmd *exec.Cmd)` as a no-op (empty body — on non-Windows there are no console windows to suppress); implements `Detach(cmd *exec.Cmd)` — assigns `cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}` so the child runs in a new session and survives parent exit. Add an inline comment on the `Setsid: true` line explaining that Setsid is the non-Windows equivalent of the Windows `CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW` combination: it places the child in a new session (own process group, no controlling terminal) so the parent's Ctrl-C signal and exit do not reach it.
- **Commit:** `feat(proc): add internal/proc package — HideWindow and Detach primitives`

### Card 2: Create proc tests

- **Context:**
  - `internal/proc/proc_windows.go`
  - `internal/proc/proc_other.go`
- **Edits:** none
- **Creates:**
  - `internal/proc/proc_windows_test.go`
  - `internal/proc/proc_other_test.go`
- **Deletes:** none
- **Requirements:**
  Both test files use `package proc` (white-box access to unexported constants).

  `internal/proc/proc_windows_test.go` — `//go:build windows`; imports `"os/exec"` and `"testing"`.
  - `TestHideWindow`: create `cmd := exec.Command("cmd", "/c", "echo", "test")`; call `HideWindow(cmd)`; assert `cmd.SysProcAttr != nil`; assert `cmd.SysProcAttr.HideWindow == true`; assert `cmd.SysProcAttr.CreationFlags == createNoWindow`.
  - `TestDetach`: same dummy cmd; call `Detach(cmd)`; assert `cmd.SysProcAttr != nil`; assert `cmd.SysProcAttr.HideWindow == true`; assert `cmd.SysProcAttr.CreationFlags == createNoWindow|createNewProcessGroup`.
  - `TestHideWindowDoesNotSetDetachFlag`: call `HideWindow(cmd)`; assert `cmd.SysProcAttr.CreationFlags&createNewProcessGroup == 0` (HideWindow must not set the process-group flag).
  - `TestDetachSetsHideWindow`: call `Detach(cmd)`; assert `cmd.SysProcAttr.HideWindow == true` (detach implies windowless).
  No process is started in any test; the tests only inspect `SysProcAttr`.

  `internal/proc/proc_other_test.go` — `//go:build !windows`; imports `"os/exec"` and `"testing"`.
  - `TestHideWindowIsNoop`: create `cmd := exec.Command("true")`; call `HideWindow(cmd)`; assert `cmd.SysProcAttr == nil`.
  - `TestDetachSetsSetsid`: same dummy cmd; call `Detach(cmd)`; assert `cmd.SysProcAttr != nil`; assert `cmd.SysProcAttr.Setsid == true`.
  No process is started in any test.
- **Commit:** `test(proc): unit tests for HideWindow and Detach`

## Batch Tests

`verify: go test ./internal/proc/...` runs both `proc_windows_test.go` (on Windows) and `proc_other_test.go` (on non-Windows). Tests are pure field-inspection with no process spawning, so they are fast and deterministic. The build-tag split means each platform only compiles and runs its own test file, which is the correct behavior for platform-split code.
