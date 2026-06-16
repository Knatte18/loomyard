# Module: mux (design)

> **Status:** design, nothing implemented yet. Unlike the earlier sketch, every claim
> here is **grounded in hands-on testing** — see the evidence log
> [`mux-exploration.md`](mux-exploration.md), the CC-hooks / `claude agents --json`
> evidence log [`mux-hooks-exploration.md`](mux-hooks-exploration.md) (event-driven
> pane switching), and the TUI-behavior reference
> [`../psmux-tui-behavior.md`](../psmux-tui-behavior.md). **v1 is deliberately tiny**
> (roadmap milestone 5); subprocess panes (6), the daemon (7), and Slack (8) are in
> [Deferred](#deferred).
>
> **Note:** A working proof-of-concept of the daemon and pane-recovery model already
> exists in `internal/muxpoc` — see [modules/muxpoc.md](muxpoc.md) for the shipped
> implementation that validates the hard parts of v2 and v7.

The mux module manages [psmux](../vendor/psmux_scripting.md) — a Windows tmux-compatible
multiplexer (3.3.4) — so the Claude Code sessions running across a repo's worktrees can be
laid out, observed, and recovered. It is the Go reimplementation of millpy's `_psmux.py`
(which "doesn't work" and is not used as a reference).

Driven by `lyx mux <subcommand>`; reads the worktree registry from
[`internal/state`](../shared-libs/state.md) and config from
[`internal/config`](../shared-libs/config.md). mux shells out to `psmux.exe` via Go `exec`
(no MSYS layer, so no slash-arg mangling — a hazard the probe harness hit from git-bash).

## Environment assumptions (verified)

- psmux **3.3.4** at `C:\Code\tools\bin\psmux.exe`; default pane shell = PowerShell 7.
- **Launch with explicit binary paths, never PATH aliases.** Bare `pwsh` resolved to a
  0-byte WindowsApps execution-alias stub that renders nothing under ConPTY; the explicit
  `C:\Code\tools\powershell7\pwsh.exe` works. Same discipline for `claude`.
- No Node/npm needed (claude is a native binary; Loomyard is Go, psmux is Rust).
- **Loomyard MUST sanitize the psmux child env — this is mandatory, not defensive.** The *primary*
  use case is **claude itself running `lyx` to spawn reviewers/implementers**, so Loomyard is normally
  launched from inside a Claude Code session and its env carries `CLAUDE_CODE_CHILD_SESSION=1`
  (+ `CLAUDECODE`, `CLAUDE_CODE_SESSION_ID`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SSE_PORT`). If
  these bleed into psmux, the pane's claude treats itself as a nested child and **silently stops
  persisting its transcript** — breaking resume. Since Loomyard (Go) is the chokepoint, it builds the
  **psmux server's `exec.Cmd.Env` without those vars** → server + all panes + all claude inherit a
  clean env. Crucially, agent panes spawned later inherit the *server's* env, so they stay clean
  even when the spawning `lyx` call was itself launched by a poisoned claude — as long as the
  server was started clean. See [Resume](#resume-after-crash-native---resume-with-env-hygiene).

## Design model (the load-bearing decisions)

1. **Layout = columns.** One full-height vertical column per worktree. Rows were rejected
   (4 rows ≈ 16 lines tall = unusable); 4 columns ≈ 69 cols wide is acceptable, 3 ≈ 91
   comfortable on a 1440p/27″ screen.
2. **A column is a self-owned subtree.** v1: one pane per column. v2: the same column gains
   extra panes stacked **downward** (dispatched agents) — no architectural change, just more
   panes. psmux models this natively (`{…}` = columns, `[…]` = a vertical sub-stack).
3. **mux renders the layout string itself.** `select-layout even-horizontal` is fine for a
   flat row (v1) but **flattens** vertical sub-stacks, so once a column owns a stack mux must
   emit the `window_layout` string directly. The tmux layout checksum (rotate-right-1
   accumulate, 16-bit) is verified and reproducible in Go; `select-layout "<csum>,<body>"`
   applies it atomically and honors sizing.
4. **The orchestrator/hub is its own psmux *window*, not a column** — keeps the worktree
   overview at fewer, wider columns.
5. **Overflow & orchestrator-switch use psmux *windows* inside ONE attached client** — not
   Windows-Terminal tabs, not multiple psmux clients. `Ctrl+b N` / `select-window` flips the
   single viewport. This is the only "tab" mechanism mux can drive without client-mirroring,
   smallest-client-wins shrinkage, or WT-quoting fragility.
6. **Loomyard never owns OS window management.** Popping ONE maximized terminal attached to a
   session is reliable (`lyx mux attach`); precise multi-window docking and WT multi-tab
   launches are brittle → best-effort, not core. mux is host-agnostic; psmux auto-resizes to
   whatever client attaches.
7. **Crash recovery via native `claude --resume`, given env hygiene.** mux assigns each pane a
   `--session-id` at launch and `mhgo mux resume` relaunches `claude --resume <id>` per pane. The
   one requirement: **strip the inherited Claude-Code parent-session env before launching claude**
   (see [Resume](#resume-after-crash-native---resume-with-env-hygiene)).

## Target model: `mux spawn` replaces Agent dispatch (proven feasible — muxpoc)

The end state mux is built toward: **CC stops dispatching sub-agents through the in-process
Agent tool and instead calls `lyx mux spawn`, which launches a real interactive `claude`
session in a pane below the orchestrator.** The spawned session must otherwise behave *exactly
as if it had been spawned via the Agent tool* — same contract:

- **Task in.** The orchestrator hands it a brief/prompt as the positional `[prompt]` arg at
  launch — the same envelope an Agent-tool dispatch carries (brief_path, subagent_type, model,
  round).
- **Result out.** The worker writes its structured result to a **file** (JSON / `<brief>.out.md`)
  that the orchestrator reads. **This file hand-off is what makes the worker "return" to the
  parent the way the Agent tool's return value does.** Screen-scraping via `capture-pane` is a
  fallback for liveness/idle detection, **not** the result channel — coordination must not depend
  on TUI rendering.
- **Lifecycle.** The parent blocks on the child as before; while it runs the child is the active,
  bottom, dominant pane (below).

Why this over Agent dispatch: the work becomes **visible** (watched in a pane), **interactive**
(the human can intervene), and **crash-survivable** (each worker is a real `--session-id` session,
recoverable via `--resume`). Agent dispatch stays fine and remains the default until this lands;
`mux spawn` is a **drop-in replacement that preserves the dispatch semantics**, swapping an
invisible ephemeral in-process subagent for a live, persistent one.

### Column layout: the active bottom pane dominates (proven)

The proven muxpoc model for stacked panes is **not** even distribution. Nesting goes orchestrator
→ child → grandchild (≤3 deep, not parallel), and **only the deepest/bottom pane is active** —
every ancestor is blocked waiting on its child. So the bottom (active) pane gets the **majority of
the height (~55–60%)** and ancestors collapse to equal, compact strips. Rendered via a hand-built
`window_layout` string (decision 3) — preset layouts like `even-vertical` cannot express "bottom
dominant". Verified live: 2 panes → 56% bottom; 3 panes → 60% bottom with 9+9-row ancestors;
preserved across crash+recover; focus set on the active bottom pane.

### What is real today, and the glue that remains

Every hard primitive is proven end-to-end in muxpoc (see [`mux-exploration.md`](mux-exploration.md)):
clean-env psmux boot, interactive claude launch, **claude spawning its own child pane by running
`mhgo` from its own Bash tool**, dominant-bottom layout, and `claude --resume` restoring each
pane's *distinct* context after a `kill-server`. A realistic first cut — **one `mhgo` command per
worktree** (open VS Code per worktree as today; run one terminal command that boots the
orchestrator and everything it needs) — needs only **glue**, not new primitives:

1. **Orchestrator bootstrap + permissions.** Launch the orchestrator claude with a real
   bootstrap prompt/skill (not an empty session) and **pre-granted permissions**
   (`--dangerously-skip-permissions` or a scoped allowlist) so it can run `mhgo mux spawn`
   autonomously without hanging on a permission prompt.
2. **File/JSON result channel** (above) — the one architectural decision that determines robust
   vs. fragile.
3. **Orchestration logic** — already exists in millhouse (mill-go etc.); the work is running it
   as a *live pane-driving session* rather than an in-process Agent-tool loop.

> **Robustness gap to close:** cold-recover currently relaunches `claude --resume <id>`
> unconditionally. If a pane crashed *before it had any conversation* (no transcript yet),
> `--resume` errors with "No conversation found with session ID". Recover must detect this and
> fall back to a fresh `--session-id` launch. (An empty, never-used session has nothing to
> resume — observed directly; it is expected, not a mux bug.)

## v1 — one window per repo, one column per worktree (milestone 5)

Every repo gets one psmux window. Each active worktree (from the registry) owns one
full-height column. No parent/child panes, no daemon, no hooks. `even-horizontal` gives
equal columns; no layout math needed yet.

```
┌─────────────────┬─────────────────┬─────────────────┐
│ wt: feature-a   │ wt: feature-b   │ wt: feature-c   │
│ claude          │ claude          │ claude          │
└─────────────────┴─────────────────┴─────────────────┘
                 (one psmux window per repo)
```

Each column launches claude with a **mux-assigned session id**, after **stripping the inherited
Claude-Code parent-session env** (`CLAUDE_CODE_CHILD_SESSION`, `CLAUDECODE`,
`CLAUDE_CODE_SESSION_ID`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SSE_PORT` — see
[Resume](#resume-after-crash-native---resume-with-env-hygiene)): `claude --session-id <uuid>
"<initial task>"`. The id is recorded in `local-state.json` from t0 (so resume works, decision 7),
and the task is passed as the **positional `[prompt]` arg**, never typed into a running TUI (see
[Constraints](#what-actually-works)). The env strip is mandatory or the pane's claude won't persist
its transcript.

### v1 subcommands

| Command | Does |
|---|---|
| `mhgo mux sync` | Reconcile the psmux window against the worktree registry: a column per worktree in registry order; add columns for new worktrees, flag columns whose worktree is gone. Re-renders the layout. |
| `mhgo mux attach` | Pop / attach one maximized terminal to the repo's psmux window. The popped terminal has a real TTY so claude renders there; the orchestrator itself never needs to attach — it observes via `capture-pane`. |

### Naming

Go packages carry **no prefix** — `internal/mux`, matching `internal/board` (an early
`gomux` draft was dropped). v1 can be a single `internal/mux` package; a `session`/`state`
split is only worth it once the daemon arrives.

## What actually works (empirical guardrails)

These are the tested facts any implementation must respect. Full evidence in
[`mux-exploration.md`](mux-exploration.md).

- **Scripting contract: `send-keys` + `capture-pane` is reliable** by pane name or `%id`.
  Use plain `capture-pane -p` (primary == alternate buffer on this build). Captured text is
  *rendered* — long lines come back wrapped at pane width.
- **`pane_current_command` is always `shell`** on Windows psmux — useless for "what's running
  in this pane". Use `capture-pane` content or `pane_pid`.
- **Claude's TUI renders and is fully drivable** in a pane. Marker grammar for a parser:
  `❯ ` = input line (echo or empty), `● ` = an assistant response, `✻ Verb for Ns` =
  completion, `✽`/`·` = spinner. `❯` is present in *every* state → never an idle signal.
  **Idle vs busy keys on the status bar: `shortcuts` (idle) / `interrupt` (busy)** as ASCII
  tokens (robust across the non-ASCII-space quirk seen on older claude builds).
- **Give claude its task at launch as the `[prompt]` arg.** Multi-line prompts cannot be
  typed into a running TUI: `paste-buffer` silently drops content and bracketed paste submits
  on every `\n`. Reuse turns are single-line and must send `Esc` first to clear leaked
  auto-suggest.
- **Teammate-mode does not auto-spawn panes here** (a delegated agent ran in-process). mux
  owns pane creation (`split-window` + launch); it does not rely on claude populating panes.
- **`pipe-pane` does not work on Windows psmux** (exit 0, no data) → no streaming-to-file.
  The only read mechanism is `capture-pane` on demand; a monitor must **poll-and-diff**
  (~500 ms; capture latency ≈ 23 ms, so ~4–5 % wall-clock).
- **Hooks are a convenience, not a foundation.** `pane-died` fires via `run-shell -b` (needs
  `set-option -g remain-on-exit on`; fires with no client attached) — but **format vars do
  not expand in hook commands**, so the hook is a bare trigger that can't say *which* pane
  died (the handler must scan `list-panes -F "#{pane_id} #{pane_dead}"`). `monitor-silence` /
  `alert-silence` are **silently accepted but non-functional** → no built-in "agent went idle"
  signal. (`set-window-option` doesn't exist; use `set-option -w`.)
- **Smallest client wins.** Two clients on one session shrink the window to the smaller; a
  pop-up helper must pop **maximized** or be the sole client. `detach-client` by name is
  unsupported.

## Resume after crash — native `--resume` with env hygiene

`mhgo mux resume` rebuilds the layout from `local-state.json` and relaunches `claude --resume
<session-id>` per pane. **This works for programmatically-driven panes** — verified end-to-end
twice (independent thread + in-session): a full transcript persisted (~14 KB, real
`user`/`assistant` records) and after a `kill-server` crash the resumed pane recalled its codeword.

```
mhgo mux resume:
  read local-state.json → rebuild windows + columns (layout string)
  per pane: <strip Claude-Code parent-session env>  (see below)
            claude --resume <stored session-id>   # full TUI, native resume
```

**The one requirement — mhgo must sanitize the psmux child env (mandatory):**
`CLAUDE_CODE_CHILD_SESSION=1` (prime culprit), plus `CLAUDECODE`, `CLAUDE_CODE_SESSION_ID`,
`CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SSE_PORT`, must not reach the pane. If they do, claude
treats the pane as a **nested child session and suppresses transcript writing** — leaving only a
~100-byte `ai-title` stub, so `--resume` finds nothing. This single inherited-env effect caused
every "doesn't persist" result during exploration; it is **not** send-keys, the visible window, or
the model. **This is the common path, not an edge case:** the primary way mux is used is *claude
itself running `mhgo` to spawn reviewers/implementers*, so mhgo is normally launched from inside a
Claude Code session and inherits these vars. Because mhgo (Go) spawns psmux, it is the natural
chokepoint: build the **psmux server's `exec.Cmd.Env` without** these vars (verified-fallback: clear
them in the pane right before the `claude` launch). Agent panes spawned later inherit the server's
(clean) env, so they stay clean even when the spawning `mhgo` call came from a poisoned claude —
provided the server was started clean. (Untested nuance: psmux env-passthrough on attach — verify
at implementation; the per-launch clear is the proven fallback.)

`claude -p` headless also persists, but a non-interactive feel is **out of scope** — the panes
must feel like real interactive sessions, and with env hygiene they are, *and* resumable.

> The `capture-pane` journal (see daemon) is now an **optional** higher-availability log /
> belt-and-suspenders — useful for streaming and as a recap supplement, but no longer required
> for resume. v1 lays the groundwork: assign + store `--session-id` and strip the env at launch.

---

## Deferred

Post-v1, kept so the design isn't lost. Each maps to a roadmap milestone; do not build until
the milestone is reached.

### v2 — subprocess panes (milestone 6)

This is the [target model](#target-model-mux-spawn-replaces-agent-dispatch-proven-feasible--muxpoc)
realized: `mhgo mux spawn` replaces Agent dispatch, spawning a subprocess (e.g. a reviewer) whose
pane appears **below** its parent in the same column; deeper spawns stack further down (≤3 deep).
The bottom/active pane dominates the column height; ancestors collapse to compact strips.

```
┌─────────────────┬─────────────────┐
│ wt: feature-a   │ wt: feature-c   │
│ claude          │ claude          │
│  └─ reviewer    │  └─ implement   │
│      └─ sub-rev │                 │
└─────────────────┴─────────────────┘
```

This is where the **layout-string renderer** (decision 3) becomes mandatory: `even-horizontal`
would flatten the stack, so mux tracks parent/child pane relationships in `local-state.json`
and emits the full `window_layout` string on each mutation. `Ctrl+b z` (per-pane zoom) is the
read/type grip when a column gets crowded.

### mux daemon (milestone 7)

A process running **outside** psmux is required to detect that psmux itself died (nothing
inside psmux can). `mhgo mux start` launches it as a standalone Windows process (not in a
pane), running until `mhgo mux stop`.

Internal goroutines (corrected for what actually works):

```
mux daemon
  ├── psmux watcher  — cmd.Wait() blocks until psmux exits; recovers on crash
  ├── capture poller — per pane: capture-pane diff ~500ms → append to journal;
  │                    derive state from markers (shortcuts=idle, interrupt=busy);
  │                    detect death via pane_dead. THIS is the monitoring foundation.
  ├── pane-died hook — optional low-latency nudge: run-shell -b → mhgo mux event;
  │                    bare trigger (no pane id), so the handler scans list-panes.
  ├── Slack inbound  — Socket Mode listener; routes Slack messages → send-keys
  └── Slack outbound — journal/state events → filter → post to Slack
```

**Why the poller, not hooks:** `pipe-pane` is dead, `monitor-silence`/`alert-silence` are
non-functional, and `pane-died` can't identify its pane — so event-driven monitoring via
hooks is insufficient. The capture-pane poller is the real mechanism; it simultaneously feeds
the resume journal, idle detection, and death detection. `pane-died` is at most a nudge to
wake the poller.

**Push alternative — control-mode `-CC`:** a single `psmux -CC attach` control client *does*
work on Windows and pushes live `%output %<pane> <data>` for all panes (verified — this is the
real-time channel `pipe-pane` fails to provide), plus `%begin/%end`-framed command responses.
Trade-off: `%output` is the **raw VT100 byte stream** (needs ANSI-stripping), whereas
`capture-pane` returns already-rendered text. So: use `capture-pane` polling for the journal +
idle detection (simpler), and reserve `-CC` for true streaming (the Slack relay) or to replace
N pollers with one control client. **Recovery uses `respawn-pane`** — it reuses the same pane
id and revives a dead pane in place (layout untouched); it respawns the default shell, into
which mux then launches `claude --resume <session-id>` (with the parent-session env stripped).

**Mutual watchdog.** Both watch each other:

```
psmux crashes → daemon detects via cmd.Wait() → relaunches psmux
  → reads local-state.json → rebuilds layout → claude --resume <id> per pane
    (parent-session env stripped — see Resume)
daemon crashes → next pane-died hook → run-shell -b → `mhgo mux ensure-daemon`
  → checks named pipe / PID → relaunches daemon
```

IPC is a Windows named pipe (`\\.\pipe\mhgo`), not HTTP: `mhgo mux event` is a short-lived
process that forwards the (pane-less) event and exits.

**Respawn on death.**

```
pane-died (or poller sees pane_dead) → scan list-panes for the dead %id
  → look up its worktree + session-id in local-state.json
  → respawn-pane -t %P  (revive shell, same id) → launch claude --resume <id> (env stripped)
  → Slack: "ℹ️ Recovered <worktree>"
crash-loop guard: N respawns within T minutes → stop, urgent Slack alert
```

### Slack relay (milestone 8)

One channel per worktree; the daemon bridges both directions.

```
Outbound: poller state-change (needs-input / done / recovered) → filter → POST #mhgo-<worktree>
Inbound:  user types in #mhgo-<worktree> → Socket Mode → daemon → send-keys to pane
```

Outbound filter stays conservative — needs-human-input 🔴, recovery-failed 🔴, crash-loop 🔴,
recovered ℹ️, task-completed ℹ️. Channel↔pane mapping lives in `local-state.json`. "Needs
input" is derived by the poller from the `shortcuts` idle marker plus prompt-pattern matching
(there is no `alert-silence` to lean on).

### mill-start vs autonomous mode

`mill-start` is interactive by design (the user is present; claude may ask questions) — the
daemon does not interfere. After mill-start, execution is autonomous and the daemon monitors,
so the user can walk away and watch via Slack.

### Session files and portability (milestone 11)

Claude stores transcripts at `%USERPROFILE%\.claude\projects\<project>\<session-id>.jsonl`
(the project segment encodes the cwd with `:`, `\`, and `.` all replaced by `-`). With env
hygiene (see [Resume](#resume-after-crash-native---resume-with-env-hygiene)) mux's panes populate
these normally. Sessions are **not** portable across machines, though — `claude --resume` only
works where the JSONL exists. `mhgo session push/pull` (e.g. robocopy to a synced drive) would
copy them for cross-machine resume. `CLAUDE_CONFIG_DIR` redirects *all* config (incl.
credentials), so it is unsuitable for selective session sync.
