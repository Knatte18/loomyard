# Batch: Migrate callers

```yaml
task: "Extract internal/proc (cross-OS windowless + detached spawn)"
batch: "Migrate callers"
number: 2
cards: 6
verify: go test ./internal/git/... ./internal/board/... ./internal/weft/... ./internal/muxpoc/... ./internal/vscode/...
depends-on: [1]
```

## Batch Scope

Migrates all five existing packages to use `internal/proc`. Each package-level change is: delete the platform-split spawn/window files, update or create a replacement, and swap call sites to `proc.HideWindow` or `proc.Detach`. The `muxpoc` migration requires special handling: the deleted `spawn_windows.go`/`spawn_other.go` each contain two functions — `spawnServer` (deleted; replaced by `proc.Detach` inline) and `spawnAttach` (preserved; moved to new `spawnattach_windows.go`/`spawnattach_other.go`). All changes are behavior-preserving; no logic is altered.

## Cards

### Card 3: Migrate internal/git

- **Context:**
  - `internal/proc/proc_windows.go`
  - `internal/proc/proc_other.go`
  - `internal/git/git_windows.go`
  - `internal/git/git_other.go`
- **Edits:**
  - `internal/git/git.go`
- **Creates:** none
- **Deletes:**
  - `internal/git/git_windows.go`
  - `internal/git/git_other.go`
- **Requirements:**
  In `git.go`: remove the `hideProcWindow(cmd)` call; add import `"github.com/Knatte18/loomyard/internal/proc"`; call `proc.HideWindow(cmd)` in its place (same position in `RunGit`, directly before `err = cmd.Run()`). Remove the trailing comment `// no console window flash on Windows` or replace it with `// no console window on Windows`. Delete `git_windows.go` and `git_other.go` using `git rm`.
- **Commit:** `refactor(git): replace hideProcWindow with proc.HideWindow`

### Card 4: Migrate internal/board

- **Context:**
  - `internal/proc/proc_windows.go`
  - `internal/proc/proc_other.go`
  - `internal/board/spawn_windows.go`
  - `internal/board/spawn_other.go`
  - `internal/board/board.go`
- **Edits:** none
- **Creates:**
  - `internal/board/spawn.go`
- **Deletes:**
  - `internal/board/spawn_windows.go`
  - `internal/board/spawn_other.go`
- **Requirements:**
  New `internal/board/spawn.go` — package `board`; no build tag; file-level doc comment describing what `spawnSync` does (mirrors the existing comments in the deleted files); imports `"os"`, `"os/exec"`, `"path/filepath"`, `"github.com/Knatte18/loomyard/internal/proc"`; function `spawnSync(boardPath string) error` — calls `os.Executable()` to get `exe`, calls `filepath.Abs(boardPath)` to get `abs`, builds `cmd := exec.Command(exe, "board", "--board-path", abs, "sync")`, calls `proc.Detach(cmd)` to set SysProcAttr, leaves `cmd.Stdin`/`cmd.Stdout`/`cmd.Stderr` nil, calls `cmd.Start()` and returns its error with the comment `// intentionally not Wait()ed`. Delete `spawn_windows.go` and `spawn_other.go` using `git rm`.
- **Commit:** `refactor(board): replace platform spawn files with proc.Detach`

### Card 5: Migrate internal/weft

- **Context:**
  - `internal/proc/proc_windows.go`
  - `internal/proc/proc_other.go`
  - `internal/weft/spawn_windows.go`
  - `internal/weft/spawn_other.go`
  - `internal/weft/cli.go`
- **Edits:** none
- **Creates:**
  - `internal/weft/spawn.go`
- **Deletes:**
  - `internal/weft/spawn_windows.go`
  - `internal/weft/spawn_other.go`
- **Requirements:**
  New `internal/weft/spawn.go` — package `weft`; no build tag; file-level doc comment describing what `spawnPush` does; imports `"os"`, `"os/exec"`, `"path/filepath"`, `"github.com/Knatte18/loomyard/internal/proc"`; function `spawnPush(weftPath string) error` — returns `nil` immediately if `os.Getenv("WEFT_SKIP_GIT") == "1" || os.Getenv("WEFT_SKIP_PUSH") == "1"`; calls `os.Executable()` to get `exe`; calls `filepath.Abs(weftPath)` to get `abs`; builds `cmd := exec.Command(exe, "weft", "--weft-path", abs, "push")`; calls `proc.Detach(cmd)` to set SysProcAttr; leaves `cmd.Stdin`/`cmd.Stdout`/`cmd.Stderr` nil; calls `cmd.Start()` and returns its error with the comment `// intentionally not Wait()ed`. The env-skip early-return must appear before any other logic (matching the existing behavior in both spawn platform files). Delete `spawn_windows.go` and `spawn_other.go` using `git rm`.
- **Commit:** `refactor(weft): replace platform spawn files with proc.Detach`

### Card 6: Migrate internal/muxpoc

- **Context:**
  - `internal/proc/proc_windows.go`
  - `internal/proc/proc_other.go`
  - `internal/muxpoc/spawn_windows.go`
  - `internal/muxpoc/spawn_other.go`
  - `internal/muxpoc/attach.go`
- **Edits:**
  - `internal/muxpoc/up.go`
- **Creates:**
  - `internal/muxpoc/spawnattach_windows.go`
  - `internal/muxpoc/spawnattach_other.go`
- **Deletes:**
  - `internal/muxpoc/spawn_windows.go`
  - `internal/muxpoc/spawn_other.go`
- **Requirements:**
  `internal/muxpoc/spawnattach_windows.go` — `//go:build windows`; package `muxpoc`; file-level doc comment: `// spawnattach_windows.go — Windows Terminal attach for muxpoc.`; imports `"fmt"` and `"os/exec"`; contains only the `spawnAttach` function copied verbatim from `spawn_windows.go` (the `wt.exe -w 0 -M` launcher with plain fallback). No syscall import; no constants.

  `internal/muxpoc/spawnattach_other.go` — `//go:build !windows`; package `muxpoc`; file-level doc comment: `// spawnattach_other.go — psmux attach for non-Windows.`; imports `"fmt"`, `"os"`, `"os/exec"`; contains only the `spawnAttach` function copied verbatim from `spawn_other.go` (inherited-stdio `cmd.Run()` variant).

  `internal/muxpoc/up.go` — add import `"github.com/Knatte18/loomyard/internal/proc"`; replace both `spawnServer(cmd)` calls with `proc.Detach(cmd)`. There are exactly two call sites in `up.go` (one in `coldStart` at line 92, one in `coldRecover` at line 183 per the current source). The surrounding code (`cmd.Env = clean` before the call, `cmd.Start()` after) is unchanged.

  Delete `spawn_windows.go` and `spawn_other.go` using `git rm`.
- **Commit:** `refactor(muxpoc): move spawnAttach to dedicated files, replace spawnServer with proc.Detach`

### Card 7: Migrate internal/vscode

- **Context:**
  - `internal/proc/proc_windows.go`
  - `internal/proc/proc_other.go`
- **Edits:**
  - `internal/vscode/launch_windows.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  In `launch_windows.go`: remove the `createNoWindow` constant declaration; remove the `"syscall"` import (it is no longer used); add import `"github.com/Knatte18/loomyard/internal/proc"`; replace the block `cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}` with `proc.HideWindow(cmd)`; update the comment directly above the call from `// Apply no-console-window flag pattern (from git_windows.go/spawn_windows.go)` to `// Apply no-console-window flag pattern (see internal/proc)`.
- **Commit:** `refactor(vscode): replace inline syscall with proc.HideWindow`

### Card 8: Update shared-libs docs

- **Context:**
  - `internal/proc/proc_windows.go`
  - `internal/proc/proc_other.go`
- **Edits:**
  - `docs/shared-libs/README.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  In `docs/shared-libs/README.md`, add `internal/proc` to the "Implementation-only libraries" bullet list (alphabetical order between `internal/lock` and `internal/state`): `` - `internal/proc` — cross-OS child-process window-hide (`HideWindow`) and detached-spawn (`Detach`) primitives ``. The list entry style must match the existing entries in that section (backtick-quoted package name, em-dash, one-line description).
- **Commit:** `docs(shared-libs): add internal/proc to implementation-only libraries`

## Batch Tests

`verify: go test ./internal/git/... ./internal/board/... ./internal/weft/... ./internal/muxpoc/... ./internal/vscode/...` runs all tests in every package touched by this batch. This covers: compilation correctness of all edited/created files, and the behavioral regression suite for each package. The `vscode` package has unit tests for color and config but no spawn test (the platform test for `Launch` would require a GUI environment); the verify command still catches compilation errors in `launch_windows.go`. The `muxpoc` smoke test exercises `cmd.go`/`up.go` logic paths that go through the newly inlined `proc.Detach`.
