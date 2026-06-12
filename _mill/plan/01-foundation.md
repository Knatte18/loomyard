# Batch: Foundation - state layer and psmux helpers

```yaml
task: Design mhgo mux module
batch: Foundation - state layer and psmux helpers
number: 1
cards: 4
verify: go test ./internal/muxpoc/
depends-on: []
```

## Batch Scope

Delivers the pure-library foundation of `internal/muxpoc`: state persistence (load/save/delete with atomic write + advisory lock), env sanitisation, per-repo socket derivation, UUID generation, build-tagged windowless server spawn helpers, and low-level psmux command helpers. No `RunCLI` or subcommand implementations yet — those are Batch 2. This batch produces a compiling, fully-tested `internal/muxpoc` package that Batch 2 imports. The external interface Batch 2 consumes: `MuxpocState`, `LoadState`, `SaveState`, `DeleteState`, `sanitizeEnv`, `strippedEnvKeys`, `socketName`, `newSessionID` (from `state.go`); `spawnServer` (from spawn helpers); `PsmuxCmd`, `NewPsmuxCmd`, `LivePane` (from `cmd.go`). `cmd/mhgo/main.go` is NOT edited in this batch — no `RunCLI` exists yet.

## Cards

### Card 1: State types, load, save, delete, UUID

- **Context:**
  - `go.mod`
  - `internal/lock/lock.go`
  - `internal/board/git.go`
  - `internal/board/store.go`
- **Edits:** none
- **Creates:**
  - `internal/muxpoc/state.go`
- **Deletes:** none
- **Requirements:**
  Define package `muxpoc` in `internal/muxpoc/state.go`. Implement the following:
  - `type Pane struct` with JSON fields: `id string` (psmux pane `%id`, e.g. `%3`), `session_id string` (claude `--session-id` value), `kind string` (`"main"` or `"review"`).
  - `type MuxpocState struct` with JSON fields: `session string` (psmux session name), `socket string` (psmux `-L` socket name), `stripped_env []string` (keys removed from env at server spawn), `panes []Pane`.
  - Constants: `stateRelPath = ".mhgo/muxpoc-state.json"`, `lockRelPath = ".mhgo/muxpoc-state.lock"`.
  - `func LoadState(cwd string) (*MuxpocState, string, error)` — reads `<cwd>/stateRelPath` under a shared read lock on `<cwd>/lockRelPath`. Returns `(nil, "", nil)` if file is absent (`os.IsNotExist`). Returns `(nil, "<warn msg>", nil)` if file is corrupt/unparseable (no error returned — treat as no session). Returns `(*state, "", nil)` on success.
  - `func SaveState(cwd string, s *MuxpocState) error` — creates `.mhgo/` dir if absent, acquires exclusive write lock on `<cwd>/lockRelPath`, writes atomically (temp file in `.mhgo/` + `os.Rename`) — mirror the pattern in `board.AtomicWrite` and `store.Save`. Releases lock via `defer`.
  - `func DeleteState(cwd string) error` — removes `<cwd>/stateRelPath`. Returns nil if file is absent.
  - `func newSessionID() (string, error)` — generates a UUID v4 from `crypto/rand`: read 16 bytes, set version bits (4) and variant bits (RFC 4122), format as `xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx` using `encoding/hex`.
  - `func sanitizeEnv(environ []string) []string` — returns a new slice with every entry whose key is `CLAUDECODE` or starts with `CLAUDE_CODE_` removed. Key is the part before `=`.
  - `func strippedEnvKeys(environ []string) []string` — returns the keys (not full `KEY=VALUE` strings) that `sanitizeEnv` would remove, in the same order.
  - `func socketName(cwd string) string` — derives a stable socket name: take `filepath.Base(cwd)`, replace every character that is not `[a-zA-Z0-9_-]` with `-`, lowercase, prefix with `muxpoc-`. Example: `C:\Code\mhgo\wts\mhgo-mux-design` → `muxpoc-mhgo-mux-design`.
  Use `github.com/Knatte18/mhgo/internal/lock` for locking (same pattern as `store.Save`).
- **Commit:** `feat(muxpoc): state types, load/save/delete, sanitizeEnv, socketName, UUID`

### Card 2: State unit tests

- **Context:**
  - `go.mod`
  - `internal/lock/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/muxpoc/state_test.go`
- **Deletes:** none
- **Requirements:**
  Define package `muxpoc` (same package, not `muxpoc_test`) in `internal/muxpoc/state_test.go`. Implement the following test functions, all runnable cross-platform with no live psmux:

  - `TestSanitizeEnv` — set a mock environ containing `CLAUDECODE=1`, `CLAUDE_CODE_SESSION_ID=abc`, `CLAUDE_CODE_CHILD_SESSION=1`, `CLAUDE_CODE_ENTRYPOINT=x`, `CLAUDE_CODE_SSE_PORT=9`, `HOME=/home/user`, `PATH=/usr/bin`, `MY_VAR=ok`. Call `sanitizeEnv`. Assert exactly `HOME`, `PATH`, `MY_VAR` remain. Assert `CLAUDECODE`, `CLAUDE_CODE_SESSION_ID`, `CLAUDE_CODE_CHILD_SESSION`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SSE_PORT` are absent.
  - `TestStrippedEnvKeys` — same mock environ. Call `strippedEnvKeys`. Assert the returned keys (in any order) are exactly `{CLAUDECODE, CLAUDE_CODE_SESSION_ID, CLAUDE_CODE_CHILD_SESSION, CLAUDE_CODE_ENTRYPOINT, CLAUDE_CODE_SSE_PORT}` with no duplicates.
  - `TestSocketName` — call `socketName("C:\\Code\\mhgo\\wts\\mhgo-mux-design")` on Windows-style path and `socketName("/home/user/repos/mhgo-mux-design")` on POSIX-style path. Assert both return a string starting with `muxpoc-`, containing only `[a-z0-9_-]` chars after the prefix, and being stable (two calls with same arg return same result).
  - `TestLoadStateMissing` — call `LoadState` with a temp dir that has no `.mhgo/muxpoc-state.json`. Assert `(nil, "", nil)` — no error, no state.
  - `TestLoadStateCorrupt` — write the string `"not valid json"` to `<tmpDir>/.mhgo/muxpoc-state.json` (create `.mhgo/` first). Call `LoadState`. Assert return is `(nil, <non-empty warn>, nil)` — corrupt file is logged as a warning, not an error, and does not panic.
  - `TestSaveLoadRoundtrip` — construct a `MuxpocState` with session `"test-session"`, socket `"muxpoc-test"`, stripped_env `["CLAUDECODE"]`, and one pane `{id:"%1", session_id:"sid-abc", kind:"main"}`. Save to temp dir. Load from same dir. Assert loaded state equals original (deep equality on all fields).
  - `TestNewSessionID` — call `newSessionID()` twice. Assert both calls succeed (no error). Assert both results match the format `xxxxxxxx-xxxx-4xxx-[89ab]xxx-xxxxxxxxxxxx` (version 4, variant RFC 4122). Assert the two IDs differ.
  - `TestDeleteStateMissing` — call `DeleteState` on a dir with no state file. Assert it returns nil (idempotent, no error).
- **Commit:** `test(muxpoc): unit tests for state layer, sanitizeEnv, socketName, UUID`

### Card 3: Build-tagged spawn helpers

- **Context:**
  - `internal/board/spawn_windows.go`
  - `internal/board/spawn_other.go`
- **Edits:** none
- **Creates:**
  - `internal/muxpoc/spawn_windows.go`
  - `internal/muxpoc/spawn_other.go`
- **Deletes:** none
- **Requirements:**
  Create `internal/muxpoc/spawn_windows.go` with build tag `//go:build windows` at the top (line 1, before `package`). Define package `muxpoc`. Declare the two Win32 creation-flag constants (same values as `internal/board/spawn_windows.go`: `createNewProcessGroup = 0x00000200`, `createNoWindow = 0x08000000`). Implement:
  - `func spawnServer(cmd *exec.Cmd)` — sets `cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | createNewProcessGroup}`. Called by `up.go` before `cmd.Start()` to launch the psmux server windowless and detached.
  - `func spawnAttach(psmuxPath, socket, session string) error` — launches Windows Terminal maximized and attached: `wt.exe -w 0 -M -- <psmuxPath> -L <socket> attach-session -t <session>`. Use `exec.Command("wt.exe", ...)`, do NOT set HideWindow (this is the deliberate visible pop). Return `cmd.Start()` error (not Wait — fire-and-forget). If `wt.exe` is not found, fall back to starting `cmd.Start()` on a plain `exec.Command(psmuxPath, "-L", socket, "attach-session", "-t", session)`.

  Create `internal/muxpoc/spawn_other.go` with build tag `//go:build !windows` (line 1). Define package `muxpoc`. Implement:
  - `func spawnServer(cmd *exec.Cmd)` — sets `cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}`.
  - `func spawnAttach(psmuxPath, socket, session string) error` — runs `exec.Command(psmuxPath, "-L", socket, "attach-session", "-t", session)` with `cmd.Stdin = os.Stdin`, `cmd.Stdout = os.Stdout`, `cmd.Stderr = os.Stderr` and returns `cmd.Run()` (blocks until user detaches — normal for non-Windows).
- **Commit:** `feat(muxpoc): build-tagged psmux server spawn helpers (Windows/other)`

### Card 4: Psmux command helpers

- **Context:**
  - `go.mod`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/muxpoc/cmd.go`
- **Deletes:** none
- **Requirements:**
  Create `internal/muxpoc/cmd.go`. Define package `muxpoc`. Implement:
  - `type Config struct` (minimal stub for batch 1 — will be extended by `cli.go` in Batch 2): fields `PsmuxPath string`, `PwshPath string`, `ClaudePath string`, `LaunchTpl string`, `ResumeTpl string`, `Width int`, `Height int`.
  - `type PsmuxCmd struct` with field `cfg Config`.
  - `func NewPsmuxCmd(cfg Config) PsmuxCmd` — constructor.
  - `func (p PsmuxCmd) run(args ...string) error` — builds `exec.Command(p.cfg.PsmuxPath, append([]string{"-L", socketArg(p.cfg)}, args...)...)`, sets `cmd.Stdout = io.Discard`, `cmd.Stderr = io.Discard`, returns `cmd.Run()`. Uses `socketArg` helper.
  - `func (p PsmuxCmd) output(args ...string) (string, error)` — same as `run` but captures stdout via `cmd.Output()`.
  - `func (p PsmuxCmd) hasSession(name string) (bool, error)` — runs `p.run("has-session", "-t", name)`. Returns `(true, nil)` on exit 0, `(false, nil)` on exit 1 (session absent — normal, not an error). Returns `(false, err)` on any other error. Use `(*exec.ExitError)` type assertion to distinguish exit 1 from real errors.
  - `type LivePane struct` with JSON fields: `ID string` (`json:"id"`), `Dead bool` (`json:"dead"`), `Width int` (`json:"width"`), `Height int` (`json:"height"`).
  - `func (p PsmuxCmd) listPanes(session string) ([]LivePane, error)` — runs `list-panes -t <session> -F "#{pane_id} #{pane_dead} #{pane_width} #{pane_height}"` via `p.output(...)`. Parse each line: split on spaces, `pane_dead` is `"1"` → Dead=true. Return `[]LivePane`. Return `nil, nil` if the output is empty (no panes).
  - `func socketArg(cfg Config) string` — helper that calls `cwd, _ := os.Getwd()` then `return socketName(cwd)`. Used by `run` and `output` to inject the per-repo `-L <socket>` argument. (The socket is always derived from the process cwd — the cwd-authoritative model.)
  - `func expandTpl(tpl, sid, task string) string` — replaces `%SID%` with `sid` and `%TASK%` with `task` in `tpl`. Used by up.go and daemon.go to build claude launch/resume commands.
- **Commit:** `feat(muxpoc): PsmuxCmd helpers (run, output, hasSession, listPanes, expandTpl)`

## Batch Tests

The `verify` command is `go test ./internal/muxpoc/`. This runs `state_test.go` (Card 2) against `state.go` (Card 1). Cards 3 and 4 (`spawn_*.go`, `cmd.go`) are compilation-verified by the build step implied by `go test`. There are no unit tests for the spawn helpers (they call OS-level syscall APIs not mockable without integration infrastructure); `cmd.go` helpers are tested indirectly in Batch 2 tests via `cmdStatus`. The state tests cover all the load/save/sanitize/socket/UUID logic exhaustively.
