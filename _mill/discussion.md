# Discussion: Design mhgo mux module

```yaml
task: Design mhgo mux module
slug: mhgo-mux-design
status: discussing
parent: main
```

## Problem

mux is the riskiest module in mhgo: it must keep autonomous Claude Code sessions running
across a repo's worktrees alive and **recoverable** — including after the psmux server or the
whole machine dies. Before committing to the full module (roadmap milestone 5+), we want to be
sure the hard part actually works on this platform (Windows + psmux 3.3.4 + ConPTY), and have
a working reference to build the real module against.

This task did two things. First, **lock the design** through hands-on exploration — the
deliverable the brief asked for, now captured in committed docs (see Technical context).
Second, the scope was deliberately **expanded** (operator decision) to also build a small
**`muxpoc`** proof-of-concept: a single in-place column that proves the daemon/`--resume`
crash-survival mechanic end-to-end. If that works for one column, the architecture is proven
and `muxpoc` becomes the reference for the real `internal/mux`.

**Why now:** the persistence/resume behavior of claude inside psmux was genuinely uncertain
(it took a long investigation to find the root cause — see Decisions/env-hygiene). A PoC
de-risks milestone 5 before the real module is designed against unverified assumptions.

> **This discussion drives the implementation of `muxpoc` only.** The real `internal/mux`
> module is a separate, later task (milestone 5). mill-plan/mill-go should build the `muxpoc`
> PoC described here, grounded in the committed design docs.

## Scope

**In:**
- A new Go package `internal/muxpoc` plus a `muxpoc` case in `cmd/mhgo/main.go`, exposed as
  `mhgo muxpoc <subcommand>`. It is **reference-only and explicitly parallel to** the future
  `internal/mux` — same conventions, different name, so nothing has to be overwritten later.
- A **single in-place column** (operates on the current working directory; does **not** read a
  worktree registry — that module does not exist yet).
- Subcommands: `up`, `review`, `attach`, `status`, `down`, `daemon`.
- Mandatory **env sanitisation** when spawning the psmux server (strip `CLAUDECODE` and every
  `CLAUDE_CODE_*` from the child `exec.Cmd.Env`).
- A mux-assigned `--session-id` per pane, persisted in `<cwd>/.mhgo/muxpoc-state.json`
  (the **gitignored** `.mhgo/` layer — session/pane ids are machine-local; `_mhgo/` is the
  committed config layer and must not hold them).
- **Crash recovery**: rebuild the layout and relaunch `claude --resume <session-id>` per pane.
- A **reviewer pane stacked vertically below** the main pane (`review`).
- Unit tests for the cross-platform logic; a gated/manual live smoke test.

**Out:**
- `internal/mux` itself (the real module), the worktree registry/integration, multi-column
  layout-string rendering, `Ctrl+b` window overflow, the Slack relay.
- A detached/background daemon and the mutual-watchdog (the PoC daemon is a foreground poller).
- control-mode `-CC` streaming and the optional `capture-pane` journal (native `--resume`
  works, so the journal is **not** needed for the PoC).
- `claude -p` / any non-interactive mode — the pane must feel like a real interactive session.
- No separate `cmd/muxpoc` binary — it rides on the `mhgo` CLI.

## Decisions

### muxpoc is a separate, reference-only module — not `internal/mux`
- Decision: build the PoC as `internal/muxpoc` (+ `mhgo muxpoc`), living in parallel to the
  eventual `internal/mux`.
- Rationale: building the PoC *as* `internal/mux` would occupy the real module's name and force
  an overwrite/confusion when the real module is built; a parallel `muxpoc` can be kept as a
  reference and deleted cleanly later.
- Rejected: (a) build directly in `internal/mux`; (b) a standalone `cmd/muxpoc` binary
  (operator chose to wire it into the existing `mhgo` CLI for discoverability, since it is only
  a PoC used while `mux` does not exist).

### Single in-place column, no worktrees
- Decision: the PoC operates only on the current directory — one column, optionally one
  stacked reviewer.
- Rationale: the worktree module does not exist yet, and one column is enough to prove the hard
  part (daemon/resume). Multi-column is trivial layout work to add later.
- Rejected: waiting for the worktree module before any mux work.

### Env hygiene is mandatory and is the load-bearing mechanism
- Decision: `muxpoc` builds the psmux **server's** `exec.Cmd.Env` with `CLAUDECODE` and all
  `CLAUDE_CODE_*` variables removed (a `sanitizeEnv` helper). The server — and therefore every
  pane and every claude launched under it — inherits a clean, top-level environment.
- Rationale: this was the single root cause of a long persistence mystery. When mhgo is launched
  from inside a Claude Code session (the **primary** use case — claude itself running `mhgo` to
  spawn reviewers/implementers), the env carries `CLAUDE_CODE_CHILD_SESSION=1` (+ `CLAUDECODE`,
  `CLAUDE_CODE_SESSION_ID`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SSE_PORT`). If these reach a
  pane, claude treats itself as a nested child and **silently stops persisting its transcript**,
  which breaks `--resume`. Verified twice (in-session + an independent thread): with the env
  stripped, a full transcript persists (~14 KB, real `user`/`assistant` records) and
  `claude --resume` recalls context after a `kill-server` crash.
- Rejected: clearing the env per-launch inside the pane (works, kept as a documented fallback,
  but server-spawn sanitisation is cleaner and covers panes spawned later by a poisoned caller);
  doing nothing and assuming a clean env (fails in the primary case).

### Recovery uses native `claude --resume`, not a re-injection journal
- Decision: `mhgo muxpoc up` (cold start) and the daemon (hot recovery) rebuild the layout and
  run `claude --resume <stored session-id>` per pane.
- Rationale: with env hygiene, native resume works — it is simpler and higher-fidelity (full
  tool history) than re-injecting a scraped transcript.
- Rejected: a `capture-pane` re-injection journal as the *primary* mechanism (it was the
  fallback while the persistence cause was unknown; now optional/unneeded for the PoC).

### Reviewer pane stacks vertically downward
- Decision: `review` does `split-window -v` below the column and launches a claude in the new
  pane; state records it as a stacked pane and recovery re-creates the stack.
- Rationale: matches the "a column is a self-owned subtree" design (a dispatched agent appears
  below its parent). For one column, a direct vertical split suffices — no layout-string
  renderer needed yet (that is a real-`mux` concern).
- Rejected: separate windows/columns for reviewers (rejected in the design; rows/grids were
  rejected for the operator's screen).

### The daemon is a foreground poller (for the PoC)
- Decision: `mhgo muxpoc daemon` runs in the foreground, polls the psmux server every
  `--interval` (default **2 s**), and on death rebuilds + resumes, logging each recovery; it
  blocks until interrupted. Liveness is checked with `has-session <name>` (not `list-sessions`,
  which can auto-start an empty server on a dead socket and give a false "up").
- **Crash-loop guard:** cap recoveries at **N within a rolling window T** (e.g. 3 recoveries / 60 s);
  on exceedance, stop recovering and log a clear "crash-loop, giving up" line rather than respawn
  a permanently-failing pane forever. The counter is **daemon-process-local** (resets if the
  daemon itself is restarted — acceptable for the PoC); after give-up the daemon stops touching
  the session, so a subsequent `mhgo muxpoc status` simply shows `server_up:false`.
- Rationale: enough to prove crash-survival and let a human watch it. `cmd.Wait()` on the server
  is not available because psmux spawns the server detached, so polling is used.
- Rejected (deferred to real `mux`): detached background daemon, mutual watchdog, named-pipe IPC.

### Subcommand contracts
- `up` — fresh start if no state; **cold-recover** (rebuild + `--resume`) if state exists but the
  session is gone; no-op if already up. Emits the assigned `--session-id` and `stripped_env`.
- `review` — `split-window -v` below the column; launch a fresh claude (`--session-id`); append a
  stacked pane to state. Requires a running session.
- `attach` — pop **one maximized** Windows Terminal attached to the session (the single visible
  pop; best-effort per design decision 6). Requires a running session.
- `status` — JSON only; no side effects. Fields:
  `{have_state, server_up, session, socket, stripped_env, state_panes, live_panes}` where
  `live_panes` is `[{id, dead, width, height}]` from `list-panes`.
- `down` — **intentional teardown:** `kill-server` **and delete** `muxpoc-state.json`. (A crash
  does NOT run `down`, so state survives a crash → `up` cold-recovers. Deleting state on `down`
  is what distinguishes "I'm done" from "it crashed".)
- `daemon` — as above.

### State durability and concurrency
- Decision: every write to `muxpoc-state.json` is **atomic** (temp file + rename) and guarded by
  `internal/lock` (the daemon and an interactive `up`/`review` can both write).
- Missing state → "no session" (fresh start on `up`). **Corrupt/unparseable** state → treat as no
  session and log it, rather than crash; the operator can `down` to clear and start over.
- Rationale: the foreground daemon recovers (writes state) concurrently with interactive commands;
  without atomic+locked writes a crash mid-write could corrupt the only resume record.

### Supporting decisions (verified, baked in)
- Launch with **explicit binary paths** (`C:\Code\tools\powershell7\pwsh.exe`, the explicit
  claude path) — bare `pwsh` is a broken WindowsApps alias under ConPTY.
- Drive psmux via Go `exec` (no MSYS slash-arg mangling).
- `set-option -g remain-on-exit on` so a dead pane stays observable (`pane_dead`) and its id can
  be reused by `respawn-pane` / re-derived on rebuild.
- Use plain `capture-pane -p`; idle vs busy detection keys on status-bar tokens `shortcuts` /
  `interrupt`. Give claude its task as the positional `[prompt]` arg, never typed into a live
  TUI (`paste-buffer` drops content; bracketed paste submits on each newline).
- **Config source = CLI flags, not `_mhgo/`.** The binary paths and launch/resume templates come
  from flags (`--psmux`, `--pwsh`, `--claude`, `--launch`, `--resume`, `--width`, `--height`,
  `--interval`) with built-in defaults. muxpoc does **not** read `internal/config` / `_mhgo/*.yaml`
  — it stays self-contained (and state lives in `.mhgo/`, not the committed `_mhgo/` layer). This
  is the configurable launch source tests use for a cheap placeholder.
- An isolated per-repo socket (`psmux -L muxpoc-<dir>`) so the PoC never touches the operator's
  real psmux server.

## Technical context

mill-plan/mill-go should read these committed artifacts — they are the grounding and contain
the full empirical evidence:

- **`docs/modules/mux.md`** — the revised mux design (the brief's deliverable): design model,
  the "what actually works" guardrails, the env-hygiene/resume model, and the deferred
  daemon/Slack/session-sync layers. `muxpoc` is the one-column slice of this.
- **`docs/modules/mux-exploration.md`** — the hands-on evidence log: the scripting contract,
  marker grammar, windowing, the full persistence/`--resume` investigation and its env-hygiene
  resolution, hooks, `respawn-pane`, and control-mode `-CC` findings.
- **`docs/psmux-tui-behavior.md`** — prior empirical reference (claude TUI states, idle markers,
  multi-line submission, `pipe-pane` is dead, capture latency).
- **`docs/vendor/psmux_scripting.md`** — psmux command/hook reference.

Codebase conventions to follow (mirror `internal/board`):
- `cmd/mhgo/main.go` is a thin dispatcher (`switch module`); add `case "muxpoc"`.
- Each module exposes `RunCLI(out io.Writer, args []string) int`.
- JSON output via `internal/output.Ok(w, fields)` / `output.Err(w, msg)`.
- Windowless/detached spawning via build-tagged `spawn_windows.go` / `spawn_other.go`
  (`HideWindow` + `CREATE_NO_WINDOW` [+ `CREATE_NEW_PROCESS_GROUP`] on Windows, no-ops
  elsewhere) — needed so psmux/claude/git children don't flash console windows, and so the
  one **visible** `attach` pop is the deliberate exception. muxpoc gets its **own** copy of these
  files (mirroring board's *pattern*) — board's `spawnSync`/helpers are unexported and
  board-specific (hardcode `mhgo board sync`), so they cannot be imported.
- Shared libs available: `internal/config`, `internal/git`, `internal/lock`, `internal/output`.
- No external deps for a UUID — generate a v4 from `crypto/rand`.

Gotchas surfaced during exploration: `pipe-pane` does not work on Windows psmux (poll, don't
stream); psmux hooks fire but format vars don't expand inside hook commands and
`monitor-silence`/`alert-silence` are non-functional; `set-window-option` is an unknown command
(use `set-option -w`); `has-session <name>` is the reliable liveness check (`list-sessions` can
auto-start an empty server on a dead socket).

## Constraints

- Windows-first: PowerShell 7 panes, psmux 3.3.4, ConPTY. The package compiles cross-platform
  (build-tagged spawn helpers) but is only useful on Windows.
- The pane must feel like a **real interactive** claude session — `claude -p`/headless is out.
- mhgo must **not** own OS window management beyond popping one maximized terminal (best-effort).
- Each repo's PoC runs on its own isolated `psmux -L` socket.
- mhgo is normally launched from inside a Claude Code session → env sanitisation is not optional.

## Testing

- **TDD candidate — `sanitizeEnv`** (the load-bearing function): with `CLAUDECODE` /
  `CLAUDE_CODE_*` set in the process env, it must drop exactly those and keep unrelated vars; a
  companion `strippedEnvKeys` reports what was removed (surfaced in `status` for observability).
- Unit tests (no live psmux): state save/load roundtrip and missing-file case; launch/resume
  command templating (`%CLAUDE%`/`%SID%` substitution); per-repo socket derivation
  (stable + sanitised). All run cross-platform.
- Live smoke (gated behind a build tag or env, or run manually — must not run in normal CI since
  it needs psmux and would spend claude tokens): `up` reports the stripped env and a clean
  in-pane env (`$env:CLAUDECODE` empty in the pane — proves server-env sanitisation propagates);
  `review` stacks a reviewer below; `kill-server` then `up`/daemon cold-recovers both panes and
  resumes; with a real claude, the resumed session recalls a codeword set before the crash.
- Keep the launch command configurable so tests/demos use a cheap placeholder
  (`Write-Host ready`) instead of a token-spending claude.
- State durability: a unit test that a **corrupt/unparseable** `muxpoc-state.json` is treated as
  "no session" (not a crash); and that concurrent writes go through atomic-write + `internal/lock`.
- `status` shape: assert the JSON envelope carries
  `{have_state, server_up, session, socket, stripped_env, state_panes, live_panes}`.

## Q&A log

- **Q:** Should the PoC be built as the real `internal/mux`? **A:** No — a separate
  reference-only `internal/muxpoc`, parallel to the future module, so nothing is overwritten.
- **Q:** Separate `cmd/muxpoc` binary or wired into mhgo? **A:** Wired into mhgo as
  `mhgo muxpoc` (it's only a PoC used while mux doesn't exist).
- **Q:** What is the PoC actually proving? **A:** The hard part — a daemon that keeps a Claude
  session alive across a psmux crash and recovers it with `--resume`, for one in-place column.
- **Q:** Does `claude --resume` even work for programmatically-driven psmux panes? **A:** Yes,
  *if* the inherited `CLAUDE_CODE_*` env is stripped. That env (present because mhgo is launched
  from inside a Claude Code session) was the sole cause of the earlier non-persistence; not
  send-keys, not the visible window, not the model. Verified twice.
- **Q:** Where is the env stripped — claude launch, or psmux spawn? **A:** At psmux **server**
  spawn (Go `Cmd.Env`), so all panes inherit clean; per-launch clear is the documented fallback.
- **Q:** Journal + re-injection or native resume? **A:** Native `claude --resume`; the journal
  is optional and not needed for the PoC.
- **Q:** Worktrees in the PoC? **A:** No — in-place, single column only.
- **Q:** Where does state live, given it holds machine-local ids? **A:** `<cwd>/.mhgo/` (the
  gitignored layer), not the committed `_mhgo/` config layer. *(review r1 gap)*
- **Q:** What do `down` and `attach` do? **A:** `down` = `kill-server` + delete state (intentional
  teardown; a crash leaves state so `up` recovers); `attach` = pop one maximized WT. *(review r1 gap)*
- **Q:** How are concurrent state writes handled? **A:** Atomic write (temp+rename) + `internal/lock`;
  corrupt/missing state → treat as no session + log. *(review r1 gap)*
