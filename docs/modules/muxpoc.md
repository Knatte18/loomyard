# Module: muxpoc

> **Status:** shipped proof-of-concept. `internal/muxpoc` is a fully functional psmux
> session orchestrator that coexists with the planned `internal/mux` module (roadmap
> milestone 5). It proves the risky parts of milestones 6 (subprocess/reviewer panes)
> and 7 (daemon crash-recovery) ahead of the clean design. **Do not mark `mux` milestones
> Done and do not rewrite `mux.md` to "implemented"** — muxpoc and mux are separate;
> see [`mux.md`](mux.md) for the forward design.

Muxpoc is a **proof-of-concept psmux session orchestrator** for driving `claude` TUIs
across panes. It launches a Windows Terminal window (or interactive psmux on non-Windows)
with a vertically-stacked column layout where the bottom pane is active and dominates the
display. One psmux server per repo (socket + session name both derived from `cwd`). Driven
by `mhgo muxpoc <subcommand>`; flag-driven configuration (no YAML, no `internal/config`).

## What problem this solves

A Claude Code session running across a git worktree may spawn a reviewer or implementer
agent into a pane below it — these agents run as real interactive `claude` sessions in
the same multiplexer, visible and resumable if the server crashes. Muxpoc proves this is
feasible before the clean `mux` module lands.

## Architecture

### Dispatch and entry point

`cmd/mhgo/main.go` labels muxpoc "proof-of-concept psmux mux" and routes `mhgo muxpoc`
to `internal/muxpoc.RunCLI`. All muxpoc I/O goes through JSON (via `internal/output`);
no YAML config, no `internal/config` — all configuration is flag-driven.

### Socket and session naming

Two functions derive the socket and session names from the current working directory,
stably and consistently:

- **`socketName(cwd)`** (`state.go`): takes `filepath.Base(cwd)`, replaces every
  non-alphanumeric, non-dash, non-underscore character with a dash, lowercases, and
  prefixes with `muxpoc-`. Example: `C:\Code\mhgo\wts\docs-stale-sweep` →
  `muxpoc-docs-stale-sweep`.
- Socket and session name are **always the same** — stable per repo, so one server per
  cwd.

### Subcommands

| Command | Does |
|---|---|
| `mhgo muxpoc up` | Cold-start a new psmux session with a primary `claude` pane, or cold-recover an existing one whose server crashed (if state exists and session is dead). |
| `mhgo muxpoc review` | Add a reviewer pane to the active session via `split-window` and launch a new `claude` instance with a fresh session ID. |
| `mhgo muxpoc attach` | Pop the session into a maximized Windows Terminal window (Windows) or interactive psmux (non-Windows). |
| `mhgo muxpoc status` | Report comprehensive status: whether state exists, whether the server is running, live pane information from the server, and saved pane metadata. |
| `mhgo muxpoc down` | Stop the psmux server and delete the state file — intentional shutdown, distinct from a crash. |
| `mhgo muxpoc daemon` | Foreground polling loop that monitors the psmux session and recovers it if it crashes (crash-loop guarded: max 3 recoveries within 60s). |

### State model

Muxpoc persists runtime state to `.mhgo/muxpoc-state.json` (the **gitignored runtime-state
dir**, not the removed config layer — see [`../shared-libs/state.md`](../shared-libs/state.md)):

```go
type MuxpocState struct {
    Session     string   // psmux session name
    Socket      string   // psmux -L socket name
    StrippedEnv []string // keys removed from env at server spawn
    Panes       []Pane   // panes in the session
}

type Pane struct {
    ID        string // psmux pane ID, e.g. "%3"
    SessionID string // claude --session-id value
    Kind      string // "main" or "review"
}
```

- **Reads** acquire a shared lock on `.mhgo/muxpoc-state.lock`.
- **Writes** acquire an exclusive lock and perform atomic writes (temp + rename) via
  `board.AtomicWrite`.
- **Corrupt state** (unparseable JSON) logs a warning and is treated as no session
  (graceful degradation).
- State is **machine-local** — session IDs and pane IDs reference JSONL files under
  `%USERPROFILE%\.claude\projects\` and a running psmux server, both specific to this
  machine.

### Recovery model

State survives a server crash. On `up` when state exists but the session is dead, `up`
calls `coldRecover`:

1. **Re-launch the psmux server** — same socket and session name, targeting the same
   `.mhgo/muxpoc-state.json`.
2. **Restart each pane** — for each pane in saved state, launch `claude --resume <session-id>`.
   psmux reassigns fresh pane IDs across a restart, so recovery captures the new IDs
   from the server.
3. **Re-tile** — apply the column layout so the bottom pane dominates, matching the
   original layout.
4. **Save the refreshed state** — update pane IDs and write back to disk.

`down` deletes the state file to mark intentional shutdown, so `up` will not recover;
`down` is idempotent (kills the server and removes state regardless of whether they exist).

### Layout: vertical column, active-pane dominant

The session window is a single **vertical column** of panes. The bottom (active) pane
receives ~55% of the window height; ancestor panes above share the remainder equally.
This reflects the nesting model: orchestrator → child → grandchild (≤3 deep), where
only the deepest pane is active and the ancestors are blocked waiting on it.

- **`buildColumnLayout(w, h, ids)`** (`cmd.go`): pure function (no I/O) that takes
  window dimensions and pane IDs (ordered top to bottom) and emits a checksum-prefixed
  tmux window-layout string. Verified by unit tests.
- **`layoutChecksum(s)`** (`cmd.go`): computes the tmux window-layout checksum
  (rotate-right-1 accumulate, 16-bit) so the layout string is valid for `select-layout`.
- **`applyColumnLayout(session)`** (`cmd.go`): queries the live pane order from the
  server, builds the layout, applies it via `select-layout`, and focuses the active
  (bottom) pane.

Preset layouts like `even-vertical` cannot express "bottom dominant", so muxpoc
renders the layout string directly.

### Environment sanitization

`sanitizeEnv(environ)` (`state.go`) removes `CLAUDECODE` and every key starting with
`CLAUDE_CODE_` from the environment, returning a clean slice. This is **mandatory**:
muxpoc is typically launched from inside a Claude Code session, which carries
`CLAUDE_CODE_CHILD_SESSION=1` and related vars. If these bleed into the psmux server
process, child `claude` instances treat themselves as nested children and silently stop
persisting their transcripts — breaking resume. By stripping at the server launch, all
panes (and any subagents they spawn) inherit a clean env.

- **`strippedEnvKeys(environ)`** returns the keys (not values) that were stripped, for
  recording in state.

### Configuration: flags only

All configuration is via command-line flags (no `internal/config`, no YAML):

- `-psmux`: path to `psmux.exe` (default: `C:\Code\tools\bin\psmux.exe`)
- `-pwsh`: path to PowerShell (default: `C:\Code\tools\powershell7\pwsh.exe`)
- `-claude`: path to `claude` binary (default: find on PATH)
- `-launch`: template for new claude launch (default: `%CLAUDE% --session-id %SID% %TASK%`)
- `-resume`: template for claude resume (default: `%CLAUDE% --resume %SID%`)
- `-width`, `-height`: psmux window dimensions (default: 220 × 50)
- `-interval`: poll interval for daemon and status checks (default: 2s)

Templates are expanded via `expandTpl(tpl, sid, task)` which replaces `%SID%` with
session ID and `%TASK%` with task (if any). Additionally, `%CLAUDE%` is replaced with
the resolved claude path at the call sites in `up.go` and `review.go`.

### Low-level psmux operations

`PsmuxCmd` wraps interaction with the psmux server:

- **`run(args ...string)`** — runs a psmux command with the socket argument prepended,
  discarding output.
- **`output(args ...string)`** — runs a psmux command and captures stdout.
- **`hasSession(name)`** — checks whether a session exists (returns `(true, nil)` on
  exit 0, `(false, nil)` on exit 1, error on other failures).
- **`listPanes(session)`** — parses `list-panes` output into `[]LivePane` (pure parsing
  via `parsePaneList`, unit-testable).
- **`activePaneID(session)`** — returns the active pane ID.
- **`windowSize(session)`** — returns window dimensions (pure parsing via
  `parseWindowSize`).
- **`paneIDsTopToBottom(session)`** — returns pane IDs ordered top to bottom (pure
  parsing via `parsePaneOrder`).

All output-parsing helpers are pure functions in `cmd.go`, unit-tested without psmux.

### Daemon: crash recovery with loop guard

`daemon` runs a long-lived foreground process that monitors the psmux session and
recovers it if it crashes:

1. **Poll at configurable interval** (default: 2s).
2. **On crash:** attempt recovery via `coldRecover`, up to **3 times within 60 seconds**.
3. **Crash-loop guard:** maintain a ring of recovery timestamps; prune old ones outside
   the window. If the ring hits the limit, give up rather than loop forever.
4. **Clean shutdown on SIGINT/SIGTERM** — returns exit code 0.

The guard is daemon-process-local; a new daemon invocation resets the ring. This
prevents a permanently-broken session from eating resources.

### Process spawning: platform-specific

Muxpoc uses `//go:build` tags for platform-specific behavior:

- **`spawn_windows.go`** (Windows): `spawnServer` launches psmux with `CREATE_NO_WINDOW`
  and `CREATE_NEW_PROCESS_GROUP` flags (detached, invisible). `spawnAttach` tries to
  launch Windows Terminal (`wt.exe -w 0 -M -- ...`) and falls back to plain psmux if
  not found.
- **`spawn_other.go`** (non-Windows): `spawnServer` uses `Setsid` (new session, detached).
  `spawnAttach` runs psmux interactively with inherited stdio (blocks until user detaches).

### Tests

- **`cli_test.go`** — unit tests for CLI parsing and dispatch (pure, no exec).
- **`cmd_test.go`** — unit tests for low-level psmux operations: `parsePaneList`,
  `parseWindowSize`, `parsePaneOrder`, `buildColumnLayout`, `layoutChecksum` (all pure,
  no psmux).
- **`state_test.go`** — unit tests for state persistence: `sanitizeEnv`, `socketName`,
  `newSessionID` (pure).
- **`muxpoc_smoke_test.go`** — smoke test exercising the full `up` → `status` → `down`
  flow with a real psmux server.

### Dependencies

- **External binaries:** `psmux.exe` (Windows tmux-compatible multiplexer),
  `claude` (Claude Code TUI), `pwsh` (PowerShell), Windows Terminal (for
  `attach` on Windows).
- **Internal modules:** `output` (JSON result formatting), `lock` (state file locking),
  `board` (for `AtomicWrite`, a cross-module reach worth noting — see
  [`../shared-libs/state.md`](../shared-libs/state.md) for plans to relocate
  `AtomicWrite` / `PathGuard`).

### Relationship to planned `mux`

Muxpoc is a **shipped proof-of-concept** that proves subprocess panes and daemon
crash-recovery are feasible. The planned `internal/mux` (roadmap milestone 5,
[`mux.md`](mux.md)) is the clean forward design. **They coexist**. Muxpoc is
dispatched in `cmd/mhgo/main.go` and fully functional; mux is still unbuilt. Do
not conflate the two or mark mux milestones Done based on muxpoc shipping —
muxpoc proved the risky parts, but mux will have a different design (e.g.,
config-driven, not flag-driven; integrated with `internal/state`; multi-window
overflow handling; the orchestrator as its own window; etc.).
