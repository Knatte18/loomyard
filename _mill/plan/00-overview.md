# Plan: Design mhgo mux module

```yaml
task: Design mhgo mux module
slug: mhgo-mux-design
approved: false
started: 20260612-112852
parent: main
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
    name: Foundation - state layer and psmux helpers
    file: 01-foundation.md
    depends-on: []
    verify: go test ./internal/muxpoc/
  - number: 2
    name: CLI, subcommands, and main.go wire-up
    file: 02-cli-subcommands.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/muxpoc/ ./cmd/mhgo/
  - number: 3
    name: Daemon and live smoke test
    file: 03-daemon-smoke.md
    depends-on: [2]
    verify: go build ./...
```

## Shared Decisions

### Decision: package name is muxpoc, no collision with future internal/mux

- **Decision:** The new package is `internal/muxpoc` and the CLI subcommand is `mhgo muxpoc`. It is parallel to the eventual `internal/mux` (milestone 5) and will not be renamed or overwritten.
- **Rationale:** Building as `internal/mux` would occupy the real module's name and force an overwrite/confusion later. A parallel `muxpoc` can be kept as a reference and deleted cleanly.
- **Applies to:** all batches

### Decision: env sanitisation is mandatory, performed at psmux server spawn

- **Decision:** `sanitizeEnv(os.Environ())` removes `CLAUDECODE` and every `CLAUDE_CODE_*` variable from the `exec.Cmd.Env` of the psmux server process. `strippedEnvKeys` reports what was removed.
- **Rationale:** mhgo is launched from inside a Claude Code session in the primary use case, so the env carries `CLAUDE_CODE_CHILD_SESSION=1` and siblings. If those reach a pane's claude, it silently stops persisting its transcript, breaking `--resume`. Server-level sanitisation makes every pane and every subsequently spawned claude inherit a clean env.
- **Applies to:** all batches (state layer exposes helpers; up.go applies them)

### Decision: state file is .mhgo/muxpoc-state.json (gitignored layer)

- **Decision:** Machine-local state (psmux session name, pane IDs, claude session-ids) lives in `<cwd>/.mhgo/muxpoc-state.json`. The committed config layer is `_mhgo/`; `.mhgo/` is the gitignored runtime layer (already in .gitignore via `mhgo init`).
- **Rationale:** Session and pane IDs are machine-local; they must not be committed.
- **Applies to:** all batches

### Decision: atomic+locked state writes

- **Decision:** Every write to `muxpoc-state.json` is atomic (temp-file + rename, same pattern as `board.AtomicWrite`) and guarded by `internal/lock.AcquireWriteLock` on `.mhgo/muxpoc-state.lock`. Corrupt or unparseable state on read is treated as "no session" (log warning, return nil).
- **Rationale:** The foreground daemon recovers (writes state) concurrently with interactive subcommands. Without atomic+locked writes, a crash mid-write would corrupt the only resume record.
- **Applies to:** batches 1 and 2

### Decision: config comes from CLI flags, not internal/config

- **Decision:** `RunCLI` defines a `Config` struct populated from `flag.FlagSet` flags: `--psmux`, `--pwsh`, `--claude`, `--launch`, `--resume`, `--width`, `--height`, `--interval`. Built-in defaults point to the verified binary paths (`C:\Code\tools\bin\psmux.exe`, `C:\Code\tools\powershell7\pwsh.exe`). muxpoc does NOT read `internal/config` or `_mhgo/*.yaml`.
- **Rationale:** Keeps muxpoc self-contained. Makes tests and demos cheap: pass `--launch "Write-Host ready"` instead of spending claude tokens.
- **Applies to:** batches 2 and 3

### Decision: psmux server spawned windowless on Windows

- **Decision:** `spawnServer(cmd *exec.Cmd)` (build-tagged: `spawn_windows.go` / `spawn_other.go`) applies `HideWindow + CREATE_NO_WINDOW + CREATE_NEW_PROCESS_GROUP` on Windows, `Setsid` on other platforms. This keeps psmux and all its panes off-screen except for the one deliberate `mhgo muxpoc attach` pop.
- **Rationale:** Mirrors `internal/board`'s `spawnSync` pattern. muxpoc gets its own copy because board's helpers are unexported and board-specific.
- **Applies to:** batches 1 and 2

### Decision: isolated per-repo psmux socket

- **Decision:** `socketName(cwd string) string` derives a stable, sanitised string from the cwd directory name, prefixed `muxpoc-`. Example: `muxpoc-mhgo-mux-design`. All psmux commands pass `-L <socket>` to avoid touching the user's real psmux server.
- **Rationale:** Each repo's PoC is isolated. The socket name is stable (derived, not random) so repeated `up` calls reconnect to the same session.
- **Applies to:** all batches

### Decision: cwd-authoritative model

- **Decision:** All subcommands derive state file path, socket name, and session name from `os.Getwd()`. No registry, no config file lookup.
- **Rationale:** The PoC operates on the current working directory only — no worktree registry exists yet.
- **Applies to:** all batches

### Decision: verify commands are native Go (no PYTHONPATH= prefix)

- **Decision:** All `verify:` commands in per-batch frontmatter use `go test` / `go build` directly, no `PYTHONPATH=` prefix.
- **Rationale:** This is a Go project; the `verify-not-isolated` check applies only to Python/mill projects.
- **Applies to:** all batches

## All Files Touched

- `cmd/mhgo/main.go`
- `internal/muxpoc/attach.go`
- `internal/muxpoc/cli.go`
- `internal/muxpoc/cmd.go`
- `internal/muxpoc/daemon.go`
- `internal/muxpoc/down.go`
- `internal/muxpoc/muxpoc_smoke_test.go`
- `internal/muxpoc/review.go`
- `internal/muxpoc/spawn_other.go`
- `internal/muxpoc/spawn_windows.go`
- `internal/muxpoc/state.go`
- `internal/muxpoc/state_test.go`
- `internal/muxpoc/status.go`
- `internal/muxpoc/up.go`
