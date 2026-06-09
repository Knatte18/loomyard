# Batch: internal/git — extract RunGit

```yaml
task: "Extract shared infrastructure (config, git, lock)"
batch: "internal/git — extract RunGit"
number: 2
cards: 4
verify: go test ./internal/git/...
depends-on: []
```

## Batch Scope

This batch creates the `internal/git` package containing exactly one exported function — `RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)` — plus the platform-specific `hideProcWindow` helper that applies `CREATE_NO_WINDOW` on Windows. The package is built from three source files: `git.go` (shared logic), `git_windows.go` (Windows syscall), `git_other.go` (non-Windows no-op), and `git_test.go`. No board files are touched here. Batch 4 will import this package and remove the now-redundant `RunGit` and `hideProcWindow` from board.

## Cards

### Card 3: Create internal/git/git.go

- **Context:**
  - `internal/board/git.go`
- **Edits:** none
- **Creates:**
  - `internal/git/git.go`
- **Deletes:** none
- **Requirements:** Create `internal/git/git.go` with `package git`. Implement `RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)`: create `cmd := exec.Command("git", args...)`, set `cmd.Dir = cwd`, assign `bytes.Buffer` values to `cmd.Stdout` and `cmd.Stderr`, call `hideProcWindow(cmd)` (package-private; provided by platform files), call `cmd.Run()`. On success return `outBuf.String(), errBuf.String(), 0, nil`. On `*exec.ExitError`, return `outBuf.String(), errBuf.String(), exitErr.ExitCode(), nil`. On any other error return `"", "", -1, err`. Imports: `bytes`, `os/exec`. Do NOT import `syscall` in this file — `syscall` is only in `git_windows.go`.
- **Commit:** `feat(git): add internal/git package`

### Card 4: Create internal/git/git_windows.go

- **Context:**
  - `internal/board/spawn_windows.go`
- **Edits:** none
- **Creates:**
  - `internal/git/git_windows.go`
- **Deletes:** none
- **Requirements:** Create `internal/git/git_windows.go` with `package git`. No explicit `//go:build windows` tag — the `_windows.go` filename suffix is the build constraint (consistent with `board/spawn_windows.go`). Declare `const createNoWindow = 0x08000000` (same value as in `board/spawn_windows.go`; the two are independent — no sharing across packages). Implement `hideProcWindow(cmd *exec.Cmd)`: set `cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}`. Imports: `os/exec`, `syscall`.
- **Commit:** `feat(git): add Windows CREATE_NO_WINDOW helper`

### Card 5: Create internal/git/git_other.go

- **Context:** none
- **Edits:** none
- **Creates:**
  - `internal/git/git_other.go`
- **Deletes:** none
- **Requirements:** Create `internal/git/git_other.go`. First line (before `package` declaration): `//go:build !windows`. Package declaration: `package git`. Implement `hideProcWindow(cmd *exec.Cmd) {}` as an empty no-op function. Import: `os/exec`. This mirrors `board/spawn_other.go` which uses explicit `//go:build !windows`.
- **Commit:** `feat(git): add no-op hideProcWindow for non-Windows`

### Card 6: Create internal/git/git_test.go

- **Context:**
  - `internal/board/git.go`
- **Edits:** none
- **Creates:**
  - `internal/git/git_test.go`
- **Deletes:** none
- **Requirements:** Create `internal/git/git_test.go` with `package git_test`. Implement three tests:
  - `TestRunGit_Success`: call `git.RunGit([]string{"--version"}, ".")`, assert exit code is 0 and stdout is non-empty.
  - `TestRunGit_NonZeroExit`: call `git.RunGit([]string{"status"}, t.TempDir())` where `t.TempDir()` returns a fresh non-git directory, assert exit code is non-zero, stderr is non-empty, and the returned Go error is `nil` (only execution failures, not non-zero exits, return `err != nil`).
  - `TestRunGit_Cwd`: call `git.RunGit([]string{"init"}, t.TempDir())` to initialize a git repo in a temp dir, then call `git.RunGit([]string{"rev-parse", "--absolute-git-dir"}, tempDir)` and assert exit code 0 and stdout is non-empty, proving the command ran in the given directory. Use the same `tempDir` string for both calls.
- **Commit:** `test(git): add internal/git package tests`

## Batch Tests

`verify: go test ./internal/git/...` runs `git_test.go` against the new package. `TestRunGit_Success` verifies basic operation, `TestRunGit_NonZeroExit` verifies non-zero exit handling, and `TestRunGit_Cwd` verifies the cwd parameter is respected. Board is not touched in this batch.
