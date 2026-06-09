# Design: mux — psmux session manager

This document captures the architectural decisions for the psmux-based session
manager component of mhgo. Nothing here is implemented yet; this is the design
to build toward.

## What problem this solves

Millhouse runs Claude Code sessions in psmux (a Windows tmux-compatible
terminal multiplexer). Managing those sessions by hand is fragile: panes crash,
Claude waits for input, worktrees multiply, and it is impossible to monitor
everything from a phone. This component automates the lifecycle.

## Naming convention

Go modules in mhgo are prefixed with `go` to distinguish them from their
Millhouse Python equivalents during parallel development:

| Millhouse (Python)      | mhgo (Go)                    |
|-------------------------|------------------------------|
| `_wiki.py`              | `internal/wiki` ✅ done      |
| `_worktree.py`          | `internal/goworktree`        |
| `_psmux.py`             | `internal/gomux` (this doc)  |
| `_config.py`            | `internal/goconfig`          |

---

## Layout: one window per repo, one column per worktree

Every repo gets one psmux window. Inside a window, each active worktree owns a
vertical column. When an orchestrator spawns a subprocess (e.g. a reviewer),
the subprocess pane appears directly below its parent in the same column. Deeper
spawns stack further down:

```
┌─────────────────┬─────────────────┬─────────────────┐
│ wt: feature-a   │ wt: feature-b   │ wt: feature-c   │
│ claude          │ claude          │ claude          │
│  └─ reviewer    │                 │  └─ implement   │
│      └─ sub-rev │                 │                 │
└─────────────────┴─────────────────┴─────────────────┘
                 (one psmux window per repo)
```

psmux does not natively model this tree — mhgo tracks parent/child pane
relationships itself in the local state file, and recomputes layout on each
mutation.

---

## Module split

The implementation spans three `internal/` packages:

### `internal/gomux`

Thin wrapper around the `psmux` CLI. Owns:
- Creating and destroying sessions, windows, panes
- Sending keystrokes (`send-keys`)
- Reading pane content (`capture-pane`)
- Registering and unregistering hooks
- Querying pane/window metadata via format variables

No business logic here — just psmux plumbing.

### `internal/gosession`

Claude session tracking. Owns:
- Mapping: worktree → pane ID → Claude session ID
- Reading and writing `.mhgo/local-state.json`
- Respawn logic: on `pane-died`, retrieve the session ID and call
  `psmux respawn-pane -t <pane> -- "claude --resume <session-id>"`

### `internal/gostate` *(may merge into gosession)*

Persistent local state. The state file `.mhgo/local-state.json` is
gitignored and machine-local (Claude session IDs are not portable across
machines — they reference JSONL files under `%USERPROFILE%\.claude\projects\`).

```json
{
  "repos": {
    "mhgo": {
      "psmux_window": "mhgo",
      "worktrees": {
        "feature-a": {
          "pane_id": "%3",
          "claude_session": "abc-123",
          "children": [
            { "pane_id": "%4", "claude_session": "def-456", "children": [] }
          ]
        }
      }
    }
  }
}
```

---

## Daemon

### Why a daemon is needed

psmux hooks (event callbacks) only fire while the psmux server is running. If
psmux crashes, hooks stop — nothing inside psmux can detect that psmux itself
has died. A process running **outside** psmux is required to detect and recover
from a psmux crash.

### What the daemon does

`mhgo start` launches the daemon as a standalone Windows process (not inside a
psmux pane). It runs until explicitly stopped with `mhgo stop`.

Internal goroutines:

```
mhgo daemon
  ├── goroutine: psmux watcher  — cmd.Wait() blocks until psmux exits; recovers on crash
  ├── goroutine: hook listener  — named-pipe server; receives mhgo event calls from hooks
  ├── goroutine: respawner      — handles pane-died; looks up session ID; calls respawn-pane
  ├── goroutine: Slack inbound  — Socket Mode listener; routes Slack messages → send-keys
  └── goroutine: Slack outbound — capture-pane on hook events → filter → post to Slack
```

### Mutual watchdog

Both the daemon and psmux watch each other, so both must fail simultaneously
for the system to go fully dark:

```
psmux crashes
  → daemon detects via cmd.Wait() (no polling — OS notifies Go when process exits)
  → daemon relaunches psmux
  → reads .mhgo/local-state.json
  → rebuilds window/pane layout
  → claude --resume <session-id> in each pane

daemon crashes
  → next psmux hook fires
  → hook calls: mhgo ensure-daemon
  → ensure-daemon checks named pipe / PID file
  → not alive → launches daemon in background
```

`ensure-daemon` is baked into `mhgo event` so that every hook call also acts as
a daemon health check. No extra hooks are needed for this.

### IPC: named pipe

psmux hooks call `mhgo event --type <event> --pane <id>` — a short-lived
process that sends the event to the daemon and exits. The transport is a Windows
named pipe (`\\.\pipe\mhgo`), not HTTP.

```
psmux hook → mhgo.exe event --type pane-died --pane %3
                │
                └─ net.Dial("pipe", `\\.\pipe\mhgo`) → send JSON → exit

daemon ← net.Listen("pipe", `\\.\pipe\mhgo`) → receives event → dispatches
```

---

## psmux hooks

Hooks registered at startup. Every hook also implicitly checks daemon health
via `ensure-daemon`:

```powershell
# Process lifecycle
psmux set-hook -g pane-died     "run-shell -b 'mhgo event --type pane-died --pane #{pane_id}'"
psmux set-hook -g alert-silence "run-shell -b 'mhgo event --type silence --pane #{pane_id}'"
psmux set-hook -g alert-bell    "run-shell -b 'mhgo event --type bell --pane #{pane_id}'"

# Layout tracking
psmux set-hook -g after-new-window   "run-shell -b 'mhgo event --type new-window --window #{window_id}'"
psmux set-hook -g after-split-window "run-shell -b 'mhgo event --type new-pane --pane #{pane_id}'"
psmux set-hook -g after-kill-pane    "run-shell -b 'mhgo event --type kill-pane --pane #{pane_id}'"

# Daemon health
psmux set-hook -g client-attached "run-shell -b 'mhgo ensure-daemon'"
```

`alert-silence` requires `monitor-silence N` to be set on the window:
```powershell
psmux set-window-option monitor-silence 15
```

### Detecting Claude's state

psmux cannot distinguish "Claude is thinking" from "Claude is waiting for
input" — both are silence. The detection strategy after an `alert-silence`
event:

1. `capture-pane -p -t <pane>` — read the last N lines of pane output
2. Pattern-match the last line against known Claude wait-states (e.g. prompts,
   `[y/n]` questions, the `◇`/`▶` input symbols Claude shows in the terminal)
3. If match → notify Slack; if ambiguous → log and continue
4. On timeout/ambiguity: Haiku fallback (rare, not continuous) — send the
   captured text to Haiku and ask "does this require user input? yes/no"

Note: `pipe-pane` (stream all pane output to a file) does **not** work on
Windows psmux. `capture-pane` on-demand is the only read mechanism.

---

## Respawn on crash

```
pane-died hook
  → mhgo event --type pane-died --pane %P
  → daemon looks up session ID in local-state.json
  → psmux respawn-pane -t %P -- "claude --resume <session-id>"
  → update state: new pane ID (respawn reuses the slot)
  → Slack: "ℹ️ Respawned Claude in [worktree-name]"

Crash-loop guard: if N respawns within T minutes → stop respawning, send urgent Slack alert
```

---

## Slack relay (bidirectional)

One Slack channel per worktree. The daemon bridges both directions:

```
Outbound:
  alert-silence / pane-died event
    → capture-pane → filter (only important events)
    → POST to #mhgo-<worktree> via Slack API

Inbound:
  User types in #mhgo-<worktree>
    → Slack bot (Socket Mode) receives event
    → daemon routes to correct pane
    → psmux send-keys -t <pane> "<message>" Enter
```

Channel-to-pane mapping lives in `.mhgo/local-state.json`.

### Events worth sending to Slack

| Event | Urgency |
|-------|---------|
| Claude needs human input | 🔴 urgent |
| Respawn failed | 🔴 urgent |
| Crash loop detected | 🔴 urgent |
| Claude respawned (auto-recovery) | ℹ️ info |
| Task completed | ℹ️ info |

Everything else is noise. The outbound filter is intentionally conservative.

---

## mill-start vs autonomous mode

The `mill-start` phase is interactive by design — the user is present and
Claude may use `AskUserQuestion` (interactive multi-choice dialogs). The daemon
does not interfere during mill-start.

After mill-start, execution is autonomous. The daemon takes over monitoring:
the user can walk away and observe via Slack on their phone.

---

## Session files and portability

Claude Code stores conversation transcripts locally:
```
%USERPROFILE%\.claude\projects\<project>\<session-id>.jsonl
```

Sessions are **not** portable across machines. `claude --resume <id>` only
works on the machine where the JSONL file exists. Cross-machine resume would
require manually copying the JSONL files (e.g. via robocopy to Google Drive),
which mhgo may expose as an optional `mhgo session push/pull` in future.

`CLAUDE_CONFIG_DIR` is a supported env var that redirects where Claude stores
its config and sessions, but it moves all config (including credentials), which
makes it unsuitable for selective session sync.

---

## Open questions

- Exact pane layout algorithm for the column-per-worktree model (psmux does not
  have a first-class "column" concept; columns are achieved via horizontal splits
  with vertical sub-splits).
- Whether `internal/gostate` merges into `internal/gosession` or stays separate.
- Slack bot scopes and Socket Mode setup (needs a Slack app with
  `chat:write`, `channels:read`, and `connections:write`).
- Whether the Haiku fallback for ambiguous silence events is worth implementing
  in v1 or deferred.
