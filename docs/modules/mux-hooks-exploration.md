# mux — CC hooks & `claude agents --json` exploration log

Empirical evidence for how the **mux** module ([`mux.md`](mux.md)) can drive psmux pane
**switching/focus** off Claude Code's *own* signals — its **hook system** and the
**`claude agents --json`** registry — rather than (or alongside) the `mhgo mux spawn`
replace-dispatch model. Companion to [`mux-exploration.md`](mux-exploration.md) (which proved
the psmux primitives); this file proves the **CC-side** primitives.

All claims below are hands-on, verified on this box unless marked **UNVERIFIED**. Probe
scaffolding lived in `.scratch/hook-probe/` (gitignored): a `logger.ps1` that dumps each hook's
raw stdin payload to a uniquely-named file, a `settings.json` wiring nine hook events to it, and
an isolated psmux server (`psmux -L mhgohookprobe`) running a **real interactive** `claude` — never
`claude -p` (headless is out of scope; panes must be interactive sessions).

Environment (verified 2026-06-15):
- claude **2.1.177** (native, `C:\Users\hanf\.local\bin\claude.exe`)
- psmux **3.3.4** (`C:\Code\tools\bin\psmux.exe`); pwsh **7.6.2**
- Probe model resolved to `claude-opus-4-8[1m]` (the box default).

---

## The brief, and the headline result

The question: can CC hooks + the agents JSON tell mux **which pane to focus when**, so pane
switching follows agent lifecycle automatically? The initial fear (from a docs-only reading) was a
**missing join key** — hooks know an agent's *type* but not its *session/pane*, while
`claude agents --json` knows the session but emits no events. **That fear is disproved.** The two
channels share usable join keys, and every hook payload is richer than documented:

1. **Every hook payload carries `session_id` = the firing session's own id** (+ `transcript_path`,
   `cwd`). A mux-spawned child therefore **self-identifies** on `SessionStart`/`Stop` → mux maps the
   event straight to the pane it launched.
2. **`SubagentStart`/`SubagentStop` carry `agent_id`** (not just `agent_type`), and that *same id*
   appears in `PostToolUse(Agent).tool_response.agentId`, `Stop.background_tasks[].id`, the agent's
   `outputFile` / `agent_transcript_path`, **and as the `id` field of `claude agents --json`**. One
   stable id threads the whole lifecycle and both channels.

So pane switching can be **event-driven** (hooks, ≈0 cost) with `claude agents --json` as an
occasional reconciler — not a sub-second poller.

---

## Channel 1 — `claude agents --json` (the "agents JSON")

A machine-wide **live registry of every running claude process** — the backing store for Claude
Code's `agents` view. **Verified.**

- **Schema.** Per entry: `pid`, `cwd`, `kind` (`interactive` | `background`), `startedAt` (epoch
  ms), `sessionId`. Background entries add `id` (short, e.g. `eeb0b443`), `name`, `state`
  (`blocked` | `failed` | …). Some entries add `status` (`idle` | `busy`).
- **Flags.** `--cwd <path>` filters to a subtree (matches all worktrees under a repo root);
  `--all` includes completed/failed sessions. `--json` **needs no TTY** → callable from Go `exec`.
  Bare `claude agents` requires a TTY and refuses under a pipe.
- **No control verbs.** `claude agents` only *dispatches* and *lists* — there is **no
  focus/kill/send** subcommand. mux drives psmux directly for all pane control.
- **`status` is live but best-effort.** Caught a real `idle→busy` flip on an unrelated session,
  so it tracks activity. **But it is inconsistently present for interactive sessions** (2 of 8 in a
  snapshot had it; absence is *not* explained by `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` — unset in a
  session that *did* report status — nor cleanly by version/age). Treat **absent `status` as
  "unknown", not "idle"**; it is likely heartbeat-tied.
- **Cost ≈ 800 ms/call** (5-call avg). Fine for an event-triggered reconcile; **too slow for
  sub-second idle polling** (capture-pane was ≈23 ms). → discovery/reconciliation channel, not a
  poller.

**Join keys it exposes:** `sessionId` (↔ interactive sessions / mux's pane→session map) and `id`
(↔ the `agent_id` from hook payloads, for background/async subagents).

---

## Channel 2 — Claude Code hooks (verified live)

Nine events were wired to a logger and fired from a real interactive session. **Seven fired and
were captured**: `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `SubagentStart`,
`SubagentStop`, `Stop`. (`SessionEnd` did not fire — session stayed alive; `Notification` did not
fire — see Open.)

### Hook command mechanics (verified)

- **Commands run under git-bash (`/usr/bin/bash`) on Windows — NOT cmd/pwsh.** First probe failed
  with `/usr/bin/bash: line 1: C:Codetoolspowershell7pwsh.exe: command not found` — bash ate the
  Windows backslashes. **Fix: POSIX/forward-slash paths or a PATH-resolved binary.** This is benign
  for the real design — a hook command of `mhgo mux …` resolves fine under bash — but any path with
  backslashes in a hook command is silently destroyed. (Mirrors the send-keys slash-arg hazard in
  [`mux-exploration.md`](mux-exploration.md), different surface.)
- **Synchronous**, JSON delivered on **stdin**, one process per fire. No hooks-trust prompt blocked
  an autonomous `--dangerously-skip-permissions` launch.
- **Common fields on every event:** `session_id`, `transcript_path`, `cwd`, `hook_event_name`.
  Most also carry `permission_mode` (`default` | `bypassPermissions`) and `effort` (`{level}`).

### Per-event payloads (verified)

| Event | Key fields beyond the common set |
|---|---|
| `SessionStart` | `source` (`startup`), **`model`** (`claude-opus-4-8[1m]`) |
| `UserPromptSubmit` | `prompt` (full text). **Also fires for injected `<task-notification>` results** when an async subagent returns. |
| `PreToolUse` (matcher `Task\|Agent`) | `tool_name` = **`Agent`**, `tool_input` = `{description, prompt, subagent_type}`, `tool_use_id`. Fires *before* dispatch. |
| `PostToolUse` (matcher `Task\|Agent`) | `tool_response` = `{isAsync:true, status:"async_launched", **agentId**, resolvedModel, **outputFile**, canReadOutputFile}`, `duration_ms`. |
| `SubagentStart` (matcher `*`) | **`agent_id`**, `agent_type` (e.g. `general-purpose`). |
| `SubagentStop` (matcher `*`) | **`agent_id`**, `agent_type`, **`agent_transcript_path`**, `last_assistant_message`, `background_tasks[]`, `stop_hook_active`. |
| `Stop` | `last_assistant_message`, **`background_tasks[]`** (`{id, type:"subagent", status, description, agent_type}`), `stop_hook_active`. |

### The Agent tool is async/background (verified, important)

Dispatching a subagent does **not** block the parent turn. Observed order:
`PreToolUse(Agent)` → `PostToolUse(Agent)` returns immediately (`async_launched`, with `agentId` +
`outputFile`) → `SubagentStart` → parent's `Stop` fires **while the subagent is still running**
(`background_tasks[].status:"running"`) → subagent finishes → its result is injected into the parent
as a **`<task-notification>` `UserPromptSubmit`** → `SubagentStop` (real one) → parent `Stop`. The
subagent runs **in-process** under the parent (its `outputFile` lives under the parent session's
temp dir); it is **not** a separate OS process and **not** a psmux pane.

### Quirk: `SubagentStop` fires for the main agent too (verified)

Three `SubagentStop` events fired for one dispatched subagent. **One** was the real subagent
(`agent_type:"general-purpose"`, `agent_id` matching the `SubagentStart`/`PostToolUse` id, last
message = the subagent's output). **Two** had **empty `agent_type`** and `last_assistant_message`
equal to the *main* session's replies — i.e. the root agent's own turn segments also emit
`SubagentStop`. **Filter rule:** a real dispatched subagent ⇒ `agent_type != ""` **and** `agent_id`
matches a prior `SubagentStart`. Reproduced in a second probe.

### Corrections to the docs-only reference

The earlier desk reference (an agent reading code.claude.com/docs) was **over-broad and wrong on the
load-bearing point**. Confirmed wrong: "`SubagentStop` carries only the agent type, no id" — it
carries `agent_id` *and* `agent_transcript_path`. Unconfirmed here: most of its ~25-event list
(`TaskCreated`, `TeammateIdle`, `PostToolBatch`, `CwdChanged`, `WorktreeCreate`, …). Only the seven
events above are proven to exist/fire on 2.1.177. **Do not enshrine the rest without a live probe.**

---

## How pane switching actually composes (the design takeaways)

Two integration surfaces, **both with a working join key now**:

### A. Mux-spawned standalone children — the primary path

In the [`mux spawn`](mux.md#target-model-mux-spawn-replaces-agent-dispatch-proven-feasible--muxpoc)
model each child is a *real* claude with a **mux-assigned `--session-id`** in a pane mux owns. Ship
each child a `--settings` with hooks that call back:

```
child SessionStart      → mhgo mux on-start  --session-id <own>   # confirm/repair pane↔session map
child UserPromptSubmit   → mhgo mux on-active --session-id <own>   # pane became active
child Stop               → mhgo mux on-idle   --session-id <own>   # IMMEDIATE idle/needs-input edge; last_assistant_message says what it's waiting on
child Notification       → (optional) delayed needs-input nudge — NOT on the critical path; see Open
```

Join key = the child's **own `session_id`** (present in every payload). **Proven.** This gives
**event-driven, per-pane** active/idle edges — replacing both the capture-pane idle poller *and* the
spotty agents-JSON `status` for the focus decision. mux's switch logic becomes: on a child going
idle, `select-pane`/`select-window` to the next active pane.

**`Stop` is the operative idle/needs-input edge — not `Notification`.** `Stop` fires the instant a
turn ends and carries `last_assistant_message` (so mux sees *whether* the pane is asking a question)
and `background_tasks[]` (so mux sees if work is still in flight). It is immediate. `Notification`
(`idle_prompt`) is a *delayed* (~60 s) redundant nudge, and `Notification` (`permission_prompt`)
cannot occur for autonomous mux children (they run with bypass). So mux keys focus on `Stop`, and
`Notification` is at most a belt-and-suspenders Slack ping — see Open.

### B. In-process async subagents — if the orchestrator keeps the Agent tool

The orchestrator's own hooks see every async dispatch: `PreToolUse(Agent)` (subagent_type + prompt,
*before* launch) and `SubagentStart`/`SubagentStop`/`Stop.background_tasks[]` keyed by **`agent_id`**
— and that `agent_id` is the **`id`** in `claude agents --json`. So mux can track in-flight
subagents without the orchestrator calling `mhgo` explicitly. Two ways to use it:

- **Observe only:** reflect in-flight subagents in the hub view; no pane per in-process subagent
  (they have none).
- **Intercept & redirect (evaluate, not proven):** a `PreToolUse(Agent)` hook *can* block/deny a
  tool and inject context. In principle it could **deny the in-process dispatch and instead
  `mhgo mux spawn`** a real pane — making the replace-dispatch model automatic and transparent
  (orchestrator keeps calling the Agent tool). **The hard part is the result contract:** the parent
  expects the tool's `outputFile`; a redirected pane would have to write that same file for the
  parent to "receive" the result. Worth a dedicated spike before committing.

### C. `claude agents --json` = reconciliation layer

Run it **on a hook event** (not on a timer): join `sessionId` (interactive panes) and `id`↔`agent_id`
(async subagents) to detect orphaned panes, dead sessions, or untracked agents, and to read `cwd`.
800 ms is fine at event cadence.

### Trigger model

```
CC hook fires (≈0 cost)  →  mhgo mux <verb> --session-id|--agent-id <x>
                          →  (optional) one `claude agents --json` reconcile
                          →  psmux select-pane / select-window / render-layout
```

Event-driven beats polling here on every axis: latency (hook is immediate vs 500 ms poll),
cost (no idle CPU), and fidelity (payload says *what* the pane is waiting on via
`last_assistant_message` / `background_tasks`).

---

## Bonus findings (useful elsewhere in mux)

- **`transcript_path` is handed to every hook** → mux can tail the real JSONL for its resume
  journal without `capture-pane` scraping (relates to [`mux.md`](mux.md) resume design).
- **`agent_transcript_path`** points at each subagent's own JSONL
  (`…\<session>\subagents\agent-<agent_id>.jsonl`).
- **Async-subagent result delivery** is a synthetic `<task-notification>` `UserPromptSubmit` — a
  clean, parseable seam if mux ever needs to mirror sub-results.

---

## Open / UNVERIFIED

- [ ] **`Notification` payload** (`notification_type` `permission_prompt` / `idle_prompt`) — never
  captured across three probes. `permission_prompt` could not be forced: this box auto-approves Bash
  commands in `default` mode (even `curl`), and autonomous mux children run with bypass anyway, so it
  is **not a real signal for mux**. `idle_prompt` did not fire in the observed window (it appears to
  need ~60 s of idle). **Reframed as non-critical:** `Stop` already gives the immediate idle edge
  (above); `Notification` is only a possible delayed Slack nudge. If the daemon/Slack milestone wants
  it, re-probe with an idle wait > 60 s or a tool that is genuinely permission-gated on this box.
- [ ] **`SessionEnd`** (session stayed alive — never fired). Confirm it fires on real exit and
  carries `session_id` for pane teardown.
- [ ] **Hooks reloaded mid-session?** Whether editing the orchestrator's settings after launch takes
  effect without restart (matters if mux installs orchestrator-side hooks late). Not tested.
- [ ] **Do async subagents ever surface in `claude agents --json` live** (with their `agent_id` as
  `id`) while running, or only background-*dispatched* ones? The in-process ones here used an
  `outputFile` under the parent; their live registry visibility was not directly captured.
- [ ] **Intercept-and-redirect spike** (B): can a `PreToolUse(Agent)` deny + `mhgo mux spawn` while
  still satisfying the parent's `outputFile` result contract?

---

## Bottom line

CC hooks are a **viable, low-cost, event-driven foundation for pane switching** — a real upgrade
over the capture-pane poller for the *focus* decision — because (1) each session's hooks carry its
**own `session_id`**, and (2) subagent hooks carry a stable **`agent_id`** that also keys
`claude agents --json`. The join-key problem that looked like a showstopper does not exist on
2.1.177. mux should: install callback hooks in each spawned child (path A), keep `mux spawn` owning
pane creation, use `claude agents --json` as an event-time reconciler, and treat the
`PreToolUse(Agent)` intercept (path B) as a separate, higher-risk spike gated on solving the result
contract. Remember the platform gotcha: **hook commands run under git-bash — POSIX paths only.**
