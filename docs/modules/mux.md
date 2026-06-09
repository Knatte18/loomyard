# Module: mux (sketch)

> **Status:** sketch — nothing here is implemented. This is the design to build
> toward. **v1 is deliberately tiny** (roadmap milestone 6); everything richer
> (subprocess panes, the daemon, Slack) is in [Deferred](#deferred) and gated
> behind later milestones.

The mux module manages [psmux](../vendor/psmux_scripting.md) — a Windows
tmux-compatible terminal multiplexer — so the Claude Code sessions running across a
repo's worktrees can be laid out, observed, and (later) recovered. It is the Go port
of millpy's `_psmux.py`.

Driven by `mhgo mux <subcommand>`; reads the worktree registry from
[`internal/state`](../shared-libs.md#internalstate) and config from
[`internal/config`](../shared-libs.md#internalconfig).

## Why start tiny

Today, mill orchestrates parallel work with **Agent Dispatch** (in-session
subagents), and it works better than expected. So mux does **not** need to model a
process tree or recover crashes to be useful on day one — it just needs to give each
worktree a visible terminal column. The subprocess tree, the crash-recovery daemon,
and the Slack relay are real goals, but they are *later* phases, not v1.

## v1: one window per repo, one column per worktree

Every repo gets one psmux window. Inside it, each active worktree (from the worktree
registry) owns one vertical column. That is the entire v1 model — no parent/child
panes, no daemon, no event hooks.

```
┌─────────────────┬─────────────────┬─────────────────┐
│ wt: feature-a   │ wt: feature-b   │ wt: feature-c   │
│ claude          │ claude          │ claude          │
└─────────────────┴─────────────────┴─────────────────┘
                 (one psmux window per repo)
```

### v1 subcommands (proposed)

| Command | Does |
|---|---|
| `mhgo mux sync` | Reconcile the psmux window against the worktree registry: a column for each worktree, in registry order. Add columns for new worktrees, note columns whose worktree is gone. |
| `mhgo mux attach` | Attach the terminal to the repo's psmux window. |

### Naming

Go packages in mhgo carry **no prefix** — the package is `internal/mux`, matching
`internal/board` (an earlier draft used a `go*` prefix like `gomux`; that convention
was dropped). v1 may be a single `internal/mux` package; a `session`/`state` split
is only worth it once the daemon arrives.

### v1 layout note

psmux has no first-class "column" concept; a column is achieved with horizontal
splits (and, later, vertical sub-splits for child panes). v1 only needs the
top-level horizontal split per worktree — the column math stays trivial until
subprocess panes (v2) make it a tree.

---

## Deferred

Everything below is **post-v1**, kept here so the design is not lost. Each maps to a
roadmap milestone. Do not build these until the milestone is reached.

### v2 — subprocess panes (milestone 7)

When Agent Dispatch is no longer enough, an orchestrator may spawn a subprocess
(e.g. a reviewer) whose pane appears **below** its parent in the same column; deeper
spawns stack further down.

```
┌─────────────────┬─────────────────┐
│ wt: feature-a   │ wt: feature-c   │
│ claude          │ claude          │
│  └─ reviewer    │  └─ implement   │
│      └─ sub-rev │                 │
└─────────────────┴─────────────────┘
```

psmux does not model this tree natively — mux would track parent/child pane
relationships in `local-state.json` and recompute layout on each mutation.

### The daemon (milestone 8)

psmux hooks fire only while the psmux server is running; nothing *inside* psmux can
detect that psmux itself died. A process running **outside** psmux is required to
detect and recover from a crash. `mhgo mux start` would launch this daemon as a
standalone Windows process (not inside a pane), running until `mhgo mux stop`.

Internal goroutines:

```
mux daemon
  ├── psmux watcher  — cmd.Wait() blocks until psmux exits; recovers on crash
  ├── hook listener  — named-pipe server; receives mhgo event calls from hooks
  ├── respawner      — handles pane-died; looks up session ID; respawn-pane
  ├── Slack inbound  — Socket Mode listener; routes Slack messages → send-keys
  └── Slack outbound — capture-pane on hook events → filter → post to Slack
```

**Mutual watchdog.** Both watch each other, so both must fail to go fully dark:

```
psmux crashes → daemon detects via cmd.Wait() → relaunches psmux
  → reads local-state.json → rebuilds layout → claude --resume <id> per pane

daemon crashes → next psmux hook fires → hook calls `mhgo mux ensure-daemon`
  → checks named pipe / PID → not alive → relaunches daemon
```

`ensure-daemon` is baked into every `mhgo mux event` call, so each hook doubles as a
daemon health check.

**IPC: named pipe.** Hooks call `mhgo mux event --type <event> --pane <id>` — a
short-lived process that sends the event over a Windows named pipe (`\\.\pipe\mhgo`,
not HTTP) and exits; the daemon listens on the same pipe.

**psmux hooks** (registered at startup):

```powershell
psmux set-hook -g pane-died        "run-shell -b 'mhgo mux event --type pane-died --pane #{pane_id}'"
psmux set-hook -g alert-silence    "run-shell -b 'mhgo mux event --type silence --pane #{pane_id}'"
psmux set-hook -g after-split-window "run-shell -b 'mhgo mux event --type new-pane --pane #{pane_id}'"
psmux set-hook -g client-attached  "run-shell -b 'mhgo mux ensure-daemon'"
psmux set-window-option monitor-silence 15
```

**Detecting Claude's state.** psmux cannot distinguish "Claude is thinking" from
"Claude is waiting for input" — both are silence. After `alert-silence`:
`capture-pane -p -t <pane>` to read the last lines, pattern-match against known
Claude wait-states (prompts, `[y/n]`, the `◇`/`▶` input symbols); ambiguous cases
fall back to a (rare) Haiku yes/no check. Note `pipe-pane` does **not** work on
Windows psmux — `capture-pane` on demand is the only read mechanism.

**Respawn on crash.**

```
pane-died → look up session ID in local-state.json
  → psmux respawn-pane -t %P -- "claude --resume <id>" → update state
  → Slack: "ℹ️ Respawned Claude in <worktree>"
crash-loop guard: N respawns within T minutes → stop, urgent Slack alert
```

### Slack relay (milestone 9)

One channel per worktree; the daemon bridges both directions.

```
Outbound: alert-silence / pane-died → capture-pane → filter → POST #mhgo-<worktree>
Inbound:  user types in #mhgo-<worktree> → Socket Mode → daemon → send-keys to pane
```

Outbound filter is intentionally conservative — only: needs-human-input 🔴,
respawn-failed 🔴, crash-loop 🔴, respawned ℹ️, task-completed ℹ️. Everything else is
noise. Channel↔pane mapping lives in `local-state.json`.

### mill-start vs autonomous mode

`mill-start` is interactive by design (the user is present; Claude may use
`AskUserQuestion`) — the daemon does not interfere. After mill-start, execution is
autonomous and the daemon takes over monitoring, so the user can walk away and watch
via Slack.

### Session files and portability (milestone 12)

Claude stores transcripts at
`%USERPROFILE%\.claude\projects\<project>\<session-id>.jsonl`. Sessions are **not**
portable across machines — `claude --resume <id>` only works where the JSONL exists.
Cross-machine resume needs copying those files (e.g. robocopy to a synced drive),
which `mhgo session push/pull` may expose later. `CLAUDE_CONFIG_DIR` redirects *all*
config (including credentials), so it is unsuitable for selective session sync.
