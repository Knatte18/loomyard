# Discussion: Extract internal/proc (cross-OS windowless + detached spawn)

```yaml
task: Extract internal/proc (cross-OS windowless + detached spawn)
slug: extract-internal-proc
status: discussing
parent: main
```

## Problem

Five packages (`board`, `weft`, `muxpoc`, `git`, `vscode`) each define their own copy of
the same Windows API constants (`createNoWindow = 0x08000000`,
`createNewProcessGroup = 0x00000200`) and platform-split files implementing two closely
related behaviors: making a child process windowless on Windows, and making it detached
so it survives the parent's exit. The duplication has grown silently across every new
module that needed a background spawn, and will continue growing as `mux` and `loom`
arrive. The goal is one centralized place — `internal/proc` — where all cross-OS
windowless and detached process launching lives, so future modules never copy these files
again.

## Scope

**In:**
- New `internal/proc` package with two public functions: `HideWindow(cmd *exec.Cmd)` and `Detach(cmd *exec.Cmd)`.
- Delete `internal/git/git_windows.go` and `internal/git/git_other.go`; replace the `hideProcWindow(cmd)` call in `git.RunGit` with `proc.HideWindow(cmd)`.
- Delete `internal/board/spawn_windows.go` and `internal/board/spawn_other.go`; collapse to a single `internal/board/spawn.go` that calls `proc.Detach(cmd)`.
- Delete `internal/weft/spawn_windows.go` and `internal/weft/spawn_other.go`; collapse to a single `internal/weft/spawn.go` that calls `proc.Detach(cmd)`.
- Delete `internal/muxpoc/spawn_windows.go` and `internal/muxpoc/spawn_other.go`; replace the two `spawnServer(cmd)` call sites in `up.go` with `proc.Detach(cmd)` directly.
- Replace the inline `syscall.SysProcAttr` block in `internal/vscode/launch_windows.go` with `proc.HideWindow(cmd)`.
- Unit tests for `internal/proc` verifying the `SysProcAttr` fields that each function sets.
- Update `docs/shared-libs/README.md` to list `internal/proc`.

**Out:**
- `muxpoc.spawnAttach` — launches a visible Windows Terminal (`wt.exe -w 0 -M`); no windowless pattern involved; stays in muxpoc.
- No changes to `internal/vscode/launch_other.go` (returns `ErrUnsupported`; no spawn).
- No changes to `internal/lyxtest` (uses raw `exec.Command` for test scaffolding; not production spawn).
- No changes to `cmd/lyx/main.go`.
- No new module subcommand (`lyx proc ...`); `internal/proc` is an infrastructure-only shared lib.
- No logic changes — behavior-preserving refactor only.

## Decisions

### Two functions, not one

- **Decision:** Expose `HideWindow(cmd)` (windowless-only) and `Detach(cmd)` (windowless + detached) as separate functions.
- **Rationale:** `git.RunGit` and `vscode.Launch` wait on the child (`cmd.Run()` / `cmd.Start()` for a visible app); they need windowless but not detached. Applying `Detach` to these would spuriously set `Setsid` on non-Windows for processes we block on — harmless but semantically wrong and misleading to a reader. The two use cases are genuinely distinct.
- **Rejected:** Single `Detach(cmd)` for everything — conflates unrelated behaviors.

### Detach implies HideWindow

- **Decision:** `Detach(cmd)` sets both the detach flag and the windowless flag on Windows (i.e., `CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP` + `HideWindow: true`). On non-Windows it sets `Setsid: true` only (no window concept).
- **Rationale:** Every existing detached spawn also suppresses the console window; there is no caller that wants detach without windowless. Combining them in one call keeps callers simple and matches every current usage exactly.
- **Rejected:** Requiring callers to call both `HideWindow` and `Detach`; unnecessary complexity.

### muxpoc.spawnServer deleted, not wrapped

- **Decision:** Delete `muxpoc.spawnServer` entirely. The two call sites in `up.go` call `proc.Detach(cmd)` directly.
- **Rationale:** `spawnServer` was a one-liner that set `SysProcAttr` — after extraction it becomes `proc.Detach(cmd)` and the wrapper adds no value. Callers are in the same package.
- **Rejected:** Keeping `spawnServer` as a wrapper around `proc.Detach` — dead indirection.

### board.spawnSync and weft.spawnPush collapse to single files

- **Decision:** Delete the `spawn_windows.go`/`spawn_other.go` pairs in `board` and `weft`. Replace with a single `spawn.go` (no build tag) in each package.
- **Rationale:** The only reason these were platform-split was to host the platform-specific `SysProcAttr` logic. Once that moves to `proc`, the remaining command-building code (`os.Executable`, `filepath.Abs`, `exec.Command`, `cmd.Start`) is platform-agnostic.
- **Rejected:** Keeping the split files but delegating to `proc.Detach` — unnecessary file count.

### Package named internal/proc

- **Decision:** `github.com/Knatte18/loomyard/internal/proc` with two platform-split implementation files: `proc_windows.go` and `proc_other.go`.
- **Rationale:** Follows existing codebase convention (`git_windows.go`/`git_other.go`, `fslink_windows.go`/`fslink.go`). The name `proc` is short, Go-idiomatic, and matches the task brief.
- **Rejected:** `internal/spawn`, `internal/procutil` — no precedent in this codebase.

## Technical context

### Codebase conventions

- Platform-split files use `//go:build windows` in the non-default file and `//go:build !windows` in the other (matching the existing `git_windows.go` / `git_other.go` and `fslink_windows.go` pattern).
- `internal/git` is an "implementation-only library" (listed in `docs/shared-libs/README.md` without its own module doc); `internal/proc` should be added to the same list.
- Module path: `github.com/Knatte18/loomyard`.

### Current duplication inventory

| Package | Files deleted | What replaces it |
|---|---|---|
| `internal/git` | `git_windows.go`, `git_other.go` | `proc.HideWindow(cmd)` in `git.go` |
| `internal/board` | `spawn_windows.go`, `spawn_other.go` | single `spawn.go` calling `proc.Detach` |
| `internal/weft` | `spawn_windows.go`, `spawn_other.go` | single `spawn.go` calling `proc.Detach` |
| `internal/muxpoc` | `spawn_windows.go`, `spawn_other.go` | `proc.Detach(cmd)` inline in `up.go` |
| `internal/vscode` | — (no files deleted) | replace inline syscall block in `launch_windows.go` |

### proc package API

```go
// proc_windows.go
package proc

import ("os/exec"; "syscall")

const (
    createNewProcessGroup = 0x00000200
    createNoWindow        = 0x08000000
)

// HideWindow prevents a console window from appearing for cmd on Windows.
func HideWindow(cmd *exec.Cmd) {
    cmd.SysProcAttr = &syscall.SysProcAttr{
        HideWindow:    true,
        CreationFlags: createNoWindow,
    }
}

// Detach launches cmd windowless and detached from the parent process on Windows.
// The child gets its own process group so parent Ctrl-C does not reach it.
func Detach(cmd *exec.Cmd) {
    cmd.SysProcAttr = &syscall.SysProcAttr{
        HideWindow:    true,
        CreationFlags: createNoWindow | createNewProcessGroup,
    }
}
```

```go
// proc_other.go
//go:build !windows

package proc

import ("os/exec"; "syscall")

// HideWindow is a no-op on non-Windows platforms (no console windows).
func HideWindow(cmd *exec.Cmd) {}

// Detach launches cmd in a new session so it survives the parent's exit.
func Detach(cmd *exec.Cmd) {
    cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
```

### Call-site changes (complete list)

**`internal/git/git.go`** — remove `hideProcWindow(cmd)` call, add `proc.HideWindow(cmd)`.  
**`internal/board/board.go`** — `spawnSync` call site unchanged; the function signature stays the same.  
**`internal/board/spawn.go`** (new single file) — builds cmd, calls `proc.Detach(cmd)`, calls `cmd.Start()`. Env-skip guards (`WEFT_SKIP_GIT`, etc.) do not apply here.  
**`internal/weft/spawn.go`** (new single file) — builds cmd, calls `proc.Detach(cmd)`, calls `cmd.Start()`. Preserves env-skip guards (`WEFT_SKIP_GIT`, `WEFT_SKIP_PUSH`).  
**`internal/muxpoc/up.go`** — replace `spawnServer(cmd)` with `proc.Detach(cmd)` at both call sites; add `proc` import. Note: `up.go` sets `cmd.Env = clean` before the spawn call; `proc.Detach` touches only `cmd.SysProcAttr` and does not clobber any previously set `cmd.Env`.  
**`internal/vscode/launch_windows.go`** — replace `syscall.SysProcAttr{HideWindow:true, CreationFlags:createNoWindow}` block with `proc.HideWindow(cmd)`; remove `createNoWindow` constant and `syscall` import; add `proc` import. The existing comment `// Apply no-console-window flag pattern (from git_windows.go/spawn_windows.go)` references files this task deletes — update the comment to reference `internal/proc` instead.

## Constraints

- All worktree/hub geometry resolves through `internal/paths` — `internal/proc` does not touch geometry, so this constraint is not relevant here.
- Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/paths` and `cmd/lyx/main.go`. `internal/proc` does not use either.
- This is a behavior-preserving refactor. No observable behavior changes. The existing test suites across all affected modules are the correctness guardrail.

## Testing

**`internal/proc`** — thin unit tests in `proc_windows_test.go` and `proc_other_test.go`:
- Windows tests: assert `cmd.SysProcAttr != nil` after `HideWindow`; assert `CreationFlags == createNoWindow` and `HideWindow == true`; assert `Detach` sets both `createNoWindow | createNewProcessGroup` and `HideWindow == true`.
- Non-Windows tests: assert `HideWindow` leaves `cmd.SysProcAttr == nil`; assert `Detach` sets `cmd.SysProcAttr.Setsid == true`.
- Tests operate on a freshly created `*exec.Cmd` (e.g. `exec.Command("true")`) — no process is started; the tests only inspect the SysProcAttr fields.

**Regression coverage** — no new integration tests needed. The existing suites in `board`, `weft`, `muxpoc`, `git`, and `vscode` continue to run unchanged and constitute the behavioral guardrail. The refactor is purely a call-site redirect.

## Q&A log

- **Q:** Two-function API (`HideWindow` + `Detach`) or single `Detach`? **A:** Two functions — `HideWindow` for synchronous subprocesses (git, vscode), `Detach` for fire-and-forget spawns that must survive parent exit.
- **Q:** Migrate `git.hideProcWindow` to use `proc.HideWindow`? **A:** Yes — delete `git_windows.go`/`git_other.go` and call `proc.HideWindow` in `RunGit`.
- **Q:** Migrate `vscode.launch_windows.go` inline syscall to `proc.HideWindow`? **A:** Yes.
- **Q:** Collapse `board`/`weft` spawn pairs to single files? **A:** Yes — the split existed only for the platform-specific SysProcAttr logic; once that moves to `proc`, a single `spawn.go` suffices.
- **Q:** Delete `muxpoc.spawnServer` entirely? **A:** Yes — it was a one-liner; callers call `proc.Detach(cmd)` directly. Goal is one centralized place for all detached spawning.
