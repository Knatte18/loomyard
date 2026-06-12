# Module: mux (design)

> **Status:** design, nothing implemented yet. Unlike the earlier sketch, every claim
> here is **grounded in hands-on testing** — see the evidence log
> [`mux-exploration.md`](mux-exploration.md) and the TUI-behavior reference
> [`../psmux-tui-behavior.md`](../psmux-tui-behavior.md). **v1 is deliberately tiny**
> (roadmap milestone 5); subprocess panes (6), the daemon (7), and Slack (8) are in
> [Deferred](#deferred).

The mux module manages [psmux](../vendor/psmux_scripting.md) — a Windows tmux-compatible
multiplexer (3.3.4) — so the Claude Code sessions running across a repo's worktrees can be
laid out, observed, and recovered. It is the Go reimplementation of millpy's `_psmux.py`
(which "doesn't work" and is not used as a reference).

Driven by `mhgo mux <subcommand>`; reads the worktree registry from
[`internal/state`](../shared-libs/state.md) and config from
[`internal/config`](../shared-libs/config.md). mux shells out to `psmux.exe` via Go `exec`
(no MSYS layer, so no slash-arg mangling — a hazard the probe harness hit from git-bash).

## Environment assumptions (verified)

- psmux **3.3.4** at `C:\Code\tools\bin\psmux.exe`; default pane shell = PowerShell 7.
- **Launch with explicit binary paths, never PATH aliases.** Bare `pwsh` resolved to a
  0-byte WindowsApps execution-alias stub that renders nothing under ConPTY; the explicit
  `C:\Code\tools\powershell7\pwsh.exe` works. Same discipline for `claude`.
- No Node/npm needed (claude is a native binary; mhgo is Go, psmux is Rust).

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
6. **mhgo never owns OS window management.** Popping ONE maximized terminal attached to a
   session is reliable (`mhgo mux attach`); precise multi-window docking and WT multi-tab
   launches are brittle → best-effort, not core. mux is host-agnostic; psmux auto-resizes to
   whatever client attaches.
7. **Crash recovery = mux's own journal + re-injection, NOT native `claude --resume`.** This
   is the biggest empirical correction (see [Resume](#resume-after-crash-the-corrected-model)).

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

Each column launches claude with a **mux-assigned session id**:
`claude --session-id <uuid> "<initial task>"` — the id is recorded in `local-state.json`
from t0 (groundwork for resume, decision 7), and the task is passed as the **positional
`[prompt]` arg**, never typed into a running TUI (see [Constraints](#what-actually-works)).

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

## Resume after crash — the corrected model

The earlier sketch (and roadmap milestone 7) assumed `mhgo mux resume` would relaunch each
pane with `claude --resume <session-id>`. **Hands-on testing shows that does not work for
mux's panes.** Determination:

- A **programmatically-driven** interactive claude in a psmux pane **does not persist its
  transcript** — only a ~100-byte `ai-title` stub is written, so `claude --resume` finds
  nothing. Verified across every variant (send-keys burst, char-by-char, positional `[prompt]`
  arg, default/explicit shell, attached/detached, +150 s idle, and an autonomous tool-using
  agent that created a file yet still wrote only the stub).
- Only a **human typing through an attached client** persists the full transcript (then
  native `--resume` works perfectly). `claude -p` headless also persists — but a headless,
  non-interactive feel is **explicitly out of scope** (the panes must feel like real
  interactive sessions). The discriminator is server-side injection vs. client-originated
  keystrokes; any programmatic driver is the former. (Symptom matches documented regression
  GitHub #60984 / flush-race #625; mechanism unproven, but the design no longer depends on it.)

**Therefore mux owns its own recovery data:**

```
mhgo mux resume:
  read local-state.json → rebuild windows + columns (layout string)
  per pane: relaunch a REAL interactive claude (full TUI, not -p) with the stored --session-id
            and re-inject opening context from mux's own journal
              ("Here is what you were doing before the crash: …")  +  the worktree's actual
              git state (diff / changed files) so the agent re-orients from ground truth
```

mux's journal is built by the `capture-pane` poller (the same poller used for idle detection
and pane-death detection — see daemon). Fidelity cost: the journal is rendered conversation
text, not exact tool-call/internal state. This is arguably *better* for an autonomous agent
than stale in-memory state: it re-orients from the repo's real state plus a recap. v1 lays the
groundwork (assign + store `--session-id`); the poller and `resume` ship with the daemon.

---

## Deferred

Post-v1, kept so the design isn't lost. Each maps to a roadmap milestone; do not build until
the milestone is reached.

### v2 — subprocess panes (milestone 6)

When Agent Dispatch is no longer enough, an orchestrator may spawn a subprocess (e.g. a
reviewer) whose pane appears **below** its parent in the same column; deeper spawns stack
further down.

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

**Mutual watchdog.** Both watch each other:

```
psmux crashes → daemon detects via cmd.Wait() → relaunches psmux
  → reads local-state.json → rebuilds layout → re-injects journal per pane (see Resume;
    NOT native claude --resume)
daemon crashes → next pane-died hook → run-shell -b → `mhgo mux ensure-daemon`
  → checks named pipe / PID → relaunches daemon
```

IPC is a Windows named pipe (`\\.\pipe\mhgo`), not HTTP: `mhgo mux event` is a short-lived
process that forwards the (pane-less) event and exits.

**Respawn on death.**

```
pane-died (or poller sees pane_dead) → scan list-panes for the dead %id
  → look up its worktree + session-id in local-state.json
  → respawn-pane -t %P -- relaunch interactive claude + re-inject journal context
  → Slack: "ℹ️ Recovered <worktree>"
crash-loop guard: N respawns within T minutes → stop, urgent Slack alert
```

> Note: roadmap milestone 7 still reads "respawns Claude with `--resume <session-id>`" — that
> phrasing predates this exploration and should be read as "relaunch + re-inject from journal";
> native `--resume` is not viable for programmatically-driven panes.

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
(the project segment encodes the cwd with `:`, `\`, and `.` all replaced by `-`). Sessions are
**not** portable across machines — and, per [Resume](#resume-after-crash-the-corrected-model),
programmatically-driven panes don't even populate them locally. `mhgo session push/pull` (e.g.
robocopy to a synced drive) only helps for human-typed sessions; for mux's autonomous panes the
**journal** is the portable record. `CLAUDE_CONFIG_DIR` redirects *all* config (incl.
credentials), so it is unsuitable for selective session sync.
