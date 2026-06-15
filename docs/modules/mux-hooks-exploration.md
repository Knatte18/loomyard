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
   `cwd`). **This is the load-bearing fact**, because mux's model is *one separate `claude` process
   per pane* (see [the model](#how-pane-switching-actually-composes-the-design-takeaways)): each
   spawned process **self-identifies** on its own `SessionStart`/`Stop` → mux maps the event straight
   to the pane it launched and assigned the `--session-id` to.
2. *(For the mechanism mux replaces, not uses.)* The in-process Agent-tool subagents also carry a
   stable **`agent_id`** in `SubagentStart`/`SubagentStop` — the *same id* used by
   `PostToolUse(Agent).tool_response.agentId`, `Stop.background_tasks[].id`, the agent's
   `outputFile` / `agent_transcript_path`, **and the `id` field of `claude agents --json`**. mux does
   **not** use the Agent tool, so this id matters only as a guardrail/monitoring hook (§B) — but it
   does disprove the "no join key exists" fear outright.

So pane switching can be **event-driven** (hooks, ≈0 cost) off each process's own `session_id`, with
`claude agents --json` as an occasional reconciler — not a sub-second poller.

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

The model first, because it dictates which hooks matter: **mux never uses Claude Code's in-process
Agent tool.** Every agent — the orchestrator and every descendant — is a **separate OS `claude`
process** that `mhgo mux spawn` launches in its own psmux pane, with its **task injected as the launch
`[prompt]` arg** and a **mux-assigned `--session-id`**. A parent spawns a child by running
`mhgo mux spawn` through its **Bash tool**, not the Agent tool. So the operative signals are each
process's **own session-scoped hooks**, keyed by the session id mux already assigned — *not* the
orchestrator's subagent hooks.

### A. Each pane is a separate injected process — its own hooks drive switching (THE model)

Ship each spawned `claude` a `--settings` whose hooks call back, keyed by its **own `session_id`**:

```
SessionStart      → mhgo mux on-start  --session-id <own>   # confirm/repair pane↔session map
UserPromptSubmit   → mhgo mux on-active --session-id <own>   # pane became active
Stop               → mhgo mux on-idle   --session-id <own>   # IMMEDIATE idle/needs-input edge; last_assistant_message says what it's waiting on
Notification       → (optional) delayed needs-input nudge — NOT on the critical path; see Open
```

Join key = the process's **own `session_id`** (present in every payload). **Proven — the probe
session *was* exactly this case:** a fresh `claude` process spawned into a psmux pane, task injected
at launch, with a mux-assigned `--session-id`; its `SessionStart`/`UserPromptSubmit`/`Stop` all
carried that id. This gives **event-driven, per-pane** active/idle edges — replacing both the
capture-pane idle poller *and* the spotty agents-JSON `status` for the focus decision. Switch logic:
on a child `Stop` (idle/done), `select-pane`/`select-window` to the next active pane (e.g. back to
the parent). It scales to the v2 stack (orchestrator → child → grandchild, ≤3 deep): every level is
its own `mhgo mux spawn` process, so **no Agent tool appears anywhere in the tree.**

**`Stop` is the operative idle/needs-input edge — not `Notification`.** `Stop` fires the instant a
turn ends and carries `last_assistant_message` (so mux sees *whether* the pane is asking a question)
and `background_tasks[]` (so mux sees if work is still in flight). It is immediate. `Notification`
(`idle_prompt`) is a *delayed* (~60 s) redundant nudge, and `Notification` (`permission_prompt`)
cannot occur for autonomous mux children (they run with bypass). So mux keys focus on `Stop`, and
`Notification` is at most a belt-and-suspenders Slack ping — see Open.

### B. Guardrail: block the in-process Agent tool so nothing goes invisible

The one threat to the "everything lives in a pane" invariant: a spawned `claude` could still invoke
its **in-process Agent tool**, running nested work invisibly (no pane, no session id of its own).
Hold the invariant with a `PreToolUse` matcher on `Agent` in each child's settings that **denies**
the tool with a reason steering the model to run `mhgo mux spawn` instead. (`PreToolUse` can deny +
inject context per the reference; the deny-and-steer path itself is **not yet probed** — worth a
quick spike.)

Everything in the verified in-process subagent lifecycle above — `SubagentStart`/`SubagentStop`,
`agent_id`, the async `outputFile` — therefore documents the mechanism mux **replaces**, not one it
integrates. It is retained for two reasons: it is precisely what this guardrail intercepts, and the
`agent_id`↔`claude agents --json` `id` link is how a monitor would even *notice* an un-redirected
in-process dispatch slipping past the guardrail.

### C. `claude agents --json` = reconciliation layer, keyed by `sessionId`

Run it **on a hook event** (not on a timer): each spawned process appears as its own entry
(`pid` + `sessionId`), so join on **`sessionId`** to mux's pane→session map — detect orphaned panes,
dead sessions, untracked processes, and read `cwd`. (`id`↔`agent_id` matters only for the in-process
case the guardrail blocks.) 800 ms is fine at event cadence.

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
- [ ] **Guardrail spike** (§B): does a `PreToolUse` matcher on `Agent` reliably **deny** the
  in-process Agent tool and inject a reason that steers the model to `mhgo mux spawn` instead? The
  deny-and-steer path is not yet probed.

---

## Bottom line

mux's model is **one separate `claude` process per pane, spawned by `mhgo mux spawn` (via Bash) with
the task injected at launch — never the in-process Agent tool.** Given that, CC hooks are a
**viable, low-cost, event-driven foundation for pane switching**, a real upgrade over the
capture-pane poller for the *focus* decision, because **each spawned process's hooks carry its own
`session_id`** — the exact key mux assigned it. The probe proved this directly (the probe session
*was* such a spawned, injected process). mux should: (1) ship each spawned `claude` callback hooks
in `--settings` keyed by its own session id (§A), keying focus on `Stop`; (2) keep `mhgo mux spawn`
owning pane creation at every level of the ≤3-deep stack; (3) add a `PreToolUse(Agent)` **deny**
guardrail so nested work can't slip back in-process (§B, spike pending); (4) use `claude agents
--json` as an event-time reconciler keyed by `sessionId`. The `agent_id`/`SubagentStop` machinery is
the in-process mechanism mux **replaces** — relevant only to the guardrail. Platform gotcha:
**hook commands run under git-bash — POSIX paths only.**
