# Batch: CLI, subcommands, and main.go wire-up

```yaml
task: Design mhgo mux module
batch: CLI, subcommands, and main.go wire-up
number: 2
cards: 7
verify: go build ./... && go test ./internal/muxpoc/ ./cmd/mhgo/
depends-on: [1]
```

## Batch Scope

Delivers a fully-working `mhgo muxpoc` CLI (excluding daemon): `RunCLI` with a complete `Config` struct and flag parsing, and the five interactive subcommands `up`, `review`, `attach`, `status`, `down`. Also wires `muxpoc` into `cmd/mhgo/main.go`. After this batch, `mhgo muxpoc up/review/attach/status/down` all work end-to-end. The `daemon` subcommand is stubbed via Batch 3. External interface Batch 3 consumes: `Config` (from `cli.go`), `cmdUp`/recovery helpers (from `up.go`), `PsmuxCmd` (from `cmd.go`).

Batch-local decisions:
- `cli.go` defines the complete `Config` struct — it replaces the stub defined in `cmd.go` (Card 4 of Batch 1). Card 5 (cli.go) must update `cmd.go` to remove the stub `Config` and import `Config` from `cli.go`. Since both files are in the same package (`muxpoc`), no cross-package import is needed — but the `Config` struct must live in exactly one file. Move it to `cli.go` and delete the stub from `cmd.go`.
- `up.go` exports `coldRecover(out io.Writer, cfg Config, cwd string, state *MuxpocState, mux PsmuxCmd) int` and `coldStart(out io.Writer, cfg Config, cwd string, mux PsmuxCmd) int` as package-level functions (lowercase OK since daemon.go in the same package calls them).
- `attach.go` calls `spawnAttach` from `spawn_windows.go`/`spawn_other.go` (Card 3).
- `status.go` calls `mux.listPanes` (Card 4) and `LoadState` (Card 1).

## Cards

### Card 5: CLI entry point and Config struct

- **Context:**
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/cmd.go`
  - `internal/board/cli.go`
  - `go.mod`
- **Edits:**
  - `internal/muxpoc/cmd.go`
- **Creates:**
  - `internal/muxpoc/cli.go`
- **Deletes:** none
- **Requirements:**
  **Edit `cmd.go`:** Remove the stub `Config` struct added in Card 4. Keep all other symbols unchanged.

  **Create `internal/muxpoc/cli.go`:**
  - Define `type Config struct` with all fields: `PsmuxPath string`, `PwshPath string`, `ClaudePath string`, `LaunchTpl string`, `ResumeTpl string`, `Width int`, `Height int`, `Interval time.Duration`. This is the canonical `Config` type for the whole package.
  - Implement `func RunCLI(out io.Writer, args []string) int`:
    - Create a `flag.NewFlagSet("muxpoc", flag.ContinueOnError)` with `fs.SetOutput(os.Stderr)`.
    - Define flags with these defaults:
      - `--psmux`: `C:\Code\tools\bin\psmux.exe`
      - `--pwsh`: `C:\Code\tools\powershell7\pwsh.exe`
      - `--claude`: `""` (empty — implementers find claude on PATH via `exec.LookPath("claude")` at use time; if `--claude` is set, use that path directly)
      - `--launch`: `%CLAUDE% --session-id %SID% %TASK%` (template for fresh claude launch; `%CLAUDE%` replaced with resolved claude path, `%SID%` with session-id, `%TASK%` with task prompt — for tests pass `--launch "Write-Host ready"`)
      - `--resume`: `%CLAUDE% --resume %SID%` (template for resume launch)
      - `--width`: `220`
      - `--height`: `50`
      - `--interval`: `2s` (use `fs.Duration`)
    - Parse args. On parse error return 1.
    - Populate a `Config` from the parsed flags.
    - Extract `rest := fs.Args()`. If `len(rest) < 1`, print usage to stderr and return 1.
    - `subcommand := rest[0]`.
    - Switch on `subcommand`:
      - `"up"`: `return cmdUp(out, cfg)`
      - `"review"`: `return cmdReview(out, cfg)`
      - `"attach"`: `return cmdAttach(out, cfg)`
      - `"status"`: `return cmdStatus(out, cfg)`
      - `"down"`: `return cmdDown(out, cfg)`
      - `"daemon"`: `return cmdDaemon(out, cfg)`
      - default: `fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", subcommand); return 1`
  Note: `cmdDaemon` is not implemented in this batch — it will be created in Batch 3. Add a placeholder in a new file `daemon.go` with `func cmdDaemon(out io.Writer, cfg Config) int { return output.Err(out, "not yet implemented") }` as part of this card so the package compiles. Batch 3 will overwrite that file.
- **Commit:** `feat(muxpoc): RunCLI, Config struct, and flag parsing`

### Card 6: up subcommand (cold-start and cold-recover)

- **Context:**
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/cmd.go`
  - `internal/muxpoc/cli.go`
  - `internal/muxpoc/spawn_windows.go`
  - `internal/muxpoc/spawn_other.go`
  - `internal/output/output.go`
  - `docs/modules/mux.md`
- **Edits:** none
- **Creates:**
  - `internal/muxpoc/up.go`
- **Deletes:** none
- **Requirements:**
  Create `internal/muxpoc/up.go`. Implement `func cmdUp(out io.Writer, cfg Config) int`:
  1. `cwd, err := os.Getwd()`. On error return `output.Err(out, ...)`.
  2. `state, warn, err := LoadState(cwd)`. On error return `output.Err`. If `warn != ""`, print to stderr.
  3. `mux := NewPsmuxCmd(cfg)`.
  4. If `state != nil`: call `mux.hasSession(state.Session)`. If up → return `output.Ok(out, map[string]any{"session_id": state.Panes[0].SessionID, "socket": state.Socket, "stripped_env": state.StrippedEnv, "message": "already up"})`. If not up → `return coldRecover(out, cfg, cwd, state, mux)`.
  5. If `state == nil`: `return coldStart(out, cfg, cwd, mux)`.

  Implement `func coldStart(out io.Writer, cfg Config, cwd string, mux PsmuxCmd) int`:
  1. Derive `sock := socketName(cwd)`.
  2. Derive `sessionName := sock` (session name = socket name, stable per repo).
  3. `sid, err := newSessionID()`. On error return `output.Err`.
  4. Build sanitised env: `clean := sanitizeEnv(os.Environ())`. `stripped := strippedEnvKeys(os.Environ())`.
  5. Resolve claude path: if `cfg.ClaudePath != ""` use it; else `exec.LookPath("claude")`. On not-found return `output.Err`.
  6. Build psmux new-session command: `exec.Command(cfg.PsmuxPath, "-L", sock, "new-session", "-d", "-s", sessionName, "-x", fmt.Sprintf("%d", cfg.Width), "-y", fmt.Sprintf("%d", cfg.Height), cfg.PwshPath)`.
  7. Set `cmd.Env = clean`. Apply `spawnServer(cmd)`. Call `cmd.Start()`. On error return `output.Err`.
  8. Sleep 500ms (give psmux time to start). Check `mux.hasSession(sessionName)` — if still not up after 3 retries (200ms apart), return `output.Err`.
  9. Build launch command string: `expandTpl(cfg.LaunchTpl, sid, "")` then replace `%CLAUDE%` with resolved claude path.
  10. Run `send-keys` to launch claude: `mux.run("send-keys", "-t", sessionName, launchCmd, "Enter")`. On error return `output.Err`.
  11. Save state: `SaveState(cwd, &MuxpocState{Session: sessionName, Socket: sock, StrippedEnv: stripped, Panes: []Pane{{ID: "", SessionID: sid, Kind: "main"}}})`. (Pane ID is empty on cold start — it is not needed for resume; the whole session is rebuilt on recover.)
  12. Return `output.Ok(out, map[string]any{"session_id": sid, "socket": sock, "stripped_env": stripped, "message": "started"})`.

  Implement `func coldRecover(out io.Writer, cfg Config, cwd string, state *MuxpocState, mux PsmuxCmd) int`:
  1. Build sanitised env from `os.Environ()` (strip CLAUDE_CODE_* vars).
  2. Resolve claude path same as coldStart.
  3. Build new-session command targeting same `state.Socket` and `state.Session`. Set `cmd.Env = clean`. Apply `spawnServer(cmd)`. `cmd.Start()`. Sleep 500ms + retry check same as coldStart.
  4. For each pane in `state.Panes`: build resume command `expandTpl(cfg.ResumeTpl, pane.SessionID, "")` with `%CLAUDE%` replaced. Run `mux.run("send-keys", "-t", state.Session, resumeCmd, "Enter")`. If `pane.Kind == "review"`, first do `mux.run("split-window", "-t", state.Session, "-v", "-p", "30", cfg.PwshPath)`.
  5. Return `output.Ok(out, map[string]any{"session": state.Session, "socket": state.Socket, "stripped_env": state.StrippedEnv, "recovered_panes": len(state.Panes), "message": "cold-recovered"})`.
- **Commit:** `feat(muxpoc): up subcommand — cold-start and cold-recover`

### Card 7: review subcommand

- **Context:**
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/cmd.go`
  - `internal/muxpoc/cli.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/muxpoc/review.go`
- **Deletes:** none
- **Requirements:**
  Create `internal/muxpoc/review.go`. Implement `func cmdReview(out io.Writer, cfg Config) int`:
  1. `cwd, _ := os.Getwd()`.
  2. `state, _, err := LoadState(cwd)`. On error return `output.Err`. If `state == nil` return `output.Err(out, "no active session: run 'mhgo muxpoc up' first")`.
  3. `mux := NewPsmuxCmd(cfg)`. Check `mux.hasSession(state.Session)` — if not up return `output.Err(out, "session not running: run 'mhgo muxpoc up' first")`.
  4. `sid, err := newSessionID()`. On error return `output.Err`.
  5. Resolve claude path same as up.go.
  6. Split-window: `mux.run("split-window", "-t", state.Session, "-v", "-p", "30", cfg.PwshPath)`. On error return `output.Err`.
  7. Build launch command via `expandTpl(cfg.LaunchTpl, sid, "")` with `%CLAUDE%` replaced. Send keys: `mux.run("send-keys", "-t", state.Session, launchCmd, "Enter")`.
  8. Append a new `Pane{ID: "", SessionID: sid, Kind: "review"}` to `state.Panes`. `SaveState(cwd, state)`.
  9. Return `output.Ok(out, map[string]any{"session_id": sid, "socket": state.Socket, "message": "review pane added"})`.
- **Commit:** `feat(muxpoc): review subcommand — split-window reviewer pane`

### Card 8: attach subcommand

- **Context:**
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/cmd.go`
  - `internal/muxpoc/cli.go`
  - `internal/muxpoc/spawn_windows.go`
  - `internal/muxpoc/spawn_other.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/muxpoc/attach.go`
- **Deletes:** none
- **Requirements:**
  Create `internal/muxpoc/attach.go`. Implement `func cmdAttach(out io.Writer, cfg Config) int`:
  1. `cwd, _ := os.Getwd()`.
  2. `state, _, err := LoadState(cwd)`. On error return `output.Err`. If `state == nil` return `output.Err(out, "no active session")`.
  3. `mux := NewPsmuxCmd(cfg)`. Check `mux.hasSession(state.Session)` — if not up return `output.Err(out, "session not running")`.
  4. Call `spawnAttach(cfg.PsmuxPath, state.Socket, state.Session)`. On error return `output.Err`.
  5. Return `output.Ok(out, map[string]any{"session": state.Session, "socket": state.Socket, "message": "attach launched"})`.
- **Commit:** `feat(muxpoc): attach subcommand — pop maximized terminal`

### Card 9: status subcommand

- **Context:**
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/cmd.go`
  - `internal/muxpoc/cli.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/muxpoc/status.go`
- **Deletes:** none
- **Requirements:**
  Create `internal/muxpoc/status.go`. Implement `func cmdStatus(out io.Writer, cfg Config) int`:
  1. `cwd, _ := os.Getwd()`.
  2. `haveState := false`. `var state *MuxpocState`. `state, _, err := LoadState(cwd)`. On err return `output.Err`. If `state != nil` set `haveState = true`.
  3. `mux := NewPsmuxCmd(cfg)`. `serverUp := false`. If `state != nil`: call `mux.hasSession(state.Session)` → `serverUp`. Ignore errors (server may be down).
  4. `session := ""`, `socket := ""`, `strippedEnv := []string(nil)`, `statePanes := []Pane(nil)`. If `state != nil` populate from state.
  5. `livePanes := []LivePane(nil)`. If `serverUp` call `mux.listPanes(state.Session)` → `livePanes`. Ignore errors.
  6. Return `output.Ok(out, map[string]any{"have_state": haveState, "server_up": serverUp, "session": session, "socket": socket, "stripped_env": strippedEnv, "state_panes": statePanes, "live_panes": livePanes})`.
  Note: all seven fields (`have_state`, `server_up`, `session`, `socket`, `stripped_env`, `state_panes`, `live_panes`) must appear in the output even when their values are empty/nil/false — the status JSON shape is contractual.
- **Commit:** `feat(muxpoc): status subcommand — JSON session/pane status`

### Card 10: down subcommand

- **Context:**
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/cmd.go`
  - `internal/muxpoc/cli.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/muxpoc/down.go`
- **Deletes:** none
- **Requirements:**
  Create `internal/muxpoc/down.go`. Implement `func cmdDown(out io.Writer, cfg Config) int`:
  1. `cwd, _ := os.Getwd()`.
  2. `state, _, err := LoadState(cwd)`. On err return `output.Err`. If `state == nil` return `output.Ok(out, map[string]any{"message": "no active session"})`.
  3. `mux := NewPsmuxCmd(cfg)`. Run `mux.run("kill-server")` — ignore error (server may already be dead; `down` is idempotent).
  4. `DeleteState(cwd)`. Ignore error (file may already be gone).
  5. Return `output.Ok(out, map[string]any{"session": state.Session, "message": "session stopped and state deleted"})`.
  This is the **intentional teardown** path. A crash does NOT run `down`, so state survives the crash → `up` cold-recovers. Deleting state on `down` is what distinguishes "I'm done" from "it crashed".
- **Commit:** `feat(muxpoc): down subcommand — kill-server and delete state`

### Card 11: Wire muxpoc into cmd/mhgo/main.go

- **Context:**
  - `internal/muxpoc/cli.go`
  - `cmd/mhgo/main_test.go`
- **Edits:**
  - `cmd/mhgo/main.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  Edit `cmd/mhgo/main.go`:
  - Add import `"github.com/Knatte18/mhgo/internal/muxpoc"` alongside the existing `board` import.
  - Add `case "muxpoc": return muxpoc.RunCLI(out, moduleArgs)` in the `switch module` block, before the `default` case.
  - Update the package doc comment's `// Modules:` list to include `muxpoc  proof-of-concept psmux mux — see internal/muxpoc.RunCLI for subcommands`.
  No other changes. Do not touch the `run` function signature or any other case.
- **Commit:** `feat(cmd/mhgo): add muxpoc module dispatch`

## Batch Tests

The `verify` command is `go build ./... && go test ./internal/muxpoc/ ./cmd/mhgo/`. `go build ./...` catches compilation errors in the new files. `go test ./internal/muxpoc/` re-runs the state tests from Batch 1 and any new tests. `go test ./cmd/mhgo/` runs `main_test.go` — the `TestRunUnknownModule` case ensures the new `muxpoc` case is wired correctly (a dispatch to a real module must not trigger the "unknown module" path). There are no dedicated test files for the subcommand implementations in this batch — they are covered by the state tests (Card 2) and integration-level smoke tests in Batch 3. The Config struct and RunCLI flag parsing are verified by compilation and by `mhgo muxpoc --help` not panicking (covered by the existing `TestRunNoArgs` test path, which exercises `run` with a minimal arg list).
