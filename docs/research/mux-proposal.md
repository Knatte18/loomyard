# Proposal: how to build `mux` (single-worktree agent stack)

> **Status:** proposal, separate from the older mux design (superseded — mux is now built as
> `internal/muxengine`; see the package documentation and
> [overview.md#modules](../overview.md#modules)). It folds in the
> hands-on findings from [`mux-hooks-exploration.md`](mux-hooks-exploration.md) and the proven
> primitives in [`muxpoc.md`](../overview.md#modules), and it adopts a **module split** (below). Nothing here
> is built yet. When accepted, the mux design should be re-scoped to match (its "column per worktree"
> content moves to `mplex`).

## The module split (recommended)

Two modules, built in priority order:

| Module | Scope | Priority |
|---|---|---|
| **`mux`** | **One worktree at a time.** You open a terminal in the worktree you're working in (as today); mux runs the psmux session *for that worktree* — an **orchestrator claude plus the sub-agents it spawns**, stacked as visible panes you can watch and type in. | **Now** |
| **`mplex`** | **Many worktrees.** One psmux instance with **a column per work-folder**, tiling several worktrees at once. | **Later / low** |

This split is endorsed because the single-worktree agent stack is exactly where the hard parts are
already **proven** (muxpoc: spawn, dominant-bottom layout, crash recovery) and where the exploration's
event-driven model lands cleanly. `mplex` is additive layout work that can reuse `claude agents --json
--cwd` and the supervisor for cross-worktree discovery — so it loses nothing by waiting.

The rest of this doc is **`mux` only**. `mplex` is sketched briefly at the end.

---

## What `mux` is

Within one worktree: an **orchestrator** Claude Code session that spawns sub-agents (reviewer,
implementer, …) as **real interactive `claude` processes in stacked psmux panes**, replacing the
invisible in-process Agent tool. Nesting is orchestrator → child → grandchild (**≤3 deep**); only the
deepest pane is active (ancestors block waiting on it), so the **bottom/active pane dominates** the
column height. Driven by `lyx mux <subcommand>`; one psmux session per worktree.

This is muxpoc's proven model, productionised and made **event-driven** with the hook findings.

## Load-bearing decisions (each grounded in a verified finding)

1. **Direct-launch, never attach.** `lyx mux spawn` launches a **fresh `claude` process directly in
   the pane** — *not* `claude --bg` + `claude attach`. The live latency test showed `claude attach` →
   `cc-daemon` adds perceptible per-keystroke lag; direct launch in a pane is materially snappier. The
   supervisor (`claude --bg`) is **explicitly not** the interactive path here — it is reserved for a
   possible future *headless* mode (see [Out of scope](#out-of-scope-for-mux)).
2. **mux owns the pane↔session map.** It **assigns each pane a `--session-id` at launch** and records
   it in local-state from t0. This id is the join key for everything (hooks, resume, reconciliation).
3. **Task injected at launch** as the positional `[prompt]` arg. Multi-line prompts cannot be typed
   into a running TUI (paste-buffer drops content; bracketed paste submits per `\n`).
4. **Env hygiene is mandatory.** Strip `CLAUDE_CODE_*` / `CLAUDECODE` when spawning the psmux server
   → every pane's claude is a clean top-level session → transcripts persist → `--resume` works.
5. **Event-driven via per-child hooks (replaces the capture-pane idle poller).** Each spawned claude
   is launched with a `--settings` whose hooks call back `lyx mux …`, keyed by **its own
   `session_id`** (present in every payload — verified). Focus follows `Stop`:
   ```jsonc
   // injected into every spawned child's --settings (commands run under git-bash → POSIX paths / PATH binary)
   {
     "hooks": {
       "SessionStart":     [{ "hooks": [{ "type": "command", "command": "lyx mux on-start  --session-id $SID" }] }],
       "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "lyx mux on-active --session-id $SID" }] }],
       "Stop":             [{ "hooks": [{ "type": "command", "command": "lyx mux on-idle   --session-id $SID" }] }],
       "PreToolUse":       [{ "matcher": "Agent", "hooks": [{ "type": "command", "command": "lyx mux deny-agent" }] }]
     }
   }
   ```
   - `Stop` is the **immediate** idle/needs-input edge (carries `last_assistant_message` +
     `background_tasks`) → on it, mux `select-pane`s focus back to the parent / next active pane.
   - `SessionStart` confirms/repairs the pane↔session map (carries the child's own `session_id`).
   - **`lyx` is invoked PATH-resolved** so the git-bash hook executor finds it (a literal Windows
     path with backslashes is destroyed by bash — verified failure).
6. **Deny-guardrail keeps work visible.** The `PreToolUse` matcher on `Agent` **denies** the in-process
   Agent tool and injects a reason steering the model to run `lyx mux spawn` instead — so nested
   delegation can't slip back to an invisible in-process subagent. (Deny path is a pending spike.)
7. **Layout: bottom-active-dominant vertical stack**, hand-rendered `window_layout` string (preset
   layouts can't express "bottom dominant"); checksum reproducible in Go. Proven in muxpoc.
8. **Crash recovery: native `claude --resume <session-id>` per pane** (works because of decision 4),
   rebuilding the layout from local-state. Proven in muxpoc end-to-end.
9. **`claude agents --json` is the reconciler, not a poller.** Join `sessionId` to local-state **on a
   hook event** to find orphaned panes / dead sessions / untracked processes; ~800 ms, so never on a
   tight timer. Its `status` field is best-effort — don't depend on it for idle (hooks own that).

## The spawn lifecycle and the result contract (the one hard design choice)

`lyx mux spawn` must behave **exactly like an Agent-tool dispatch** so the orchestrator can swap to it
transparently:

- **Task in:** the orchestrator (via its **Bash tool**, not the Agent tool) runs `lyx mux spawn` with
  the brief/prompt; mux launches the child in a new bottom pane (decisions 1–5).
- **Result out — the load-bearing part:** the child writes its **structured result to a file**
  (JSON / `<brief>.out.md`); the orchestrator reads that file. *This file hand-off is what makes the
  child "return" the way the Agent tool's return value does.* `capture-pane` is a **liveness fallback
  only**, never the result channel.
- **Lifecycle:** the parent blocks on the child; while it runs the child is the active, dominant
  bottom pane; on the child's `Stop`, focus returns to the parent.

This result contract is **the** thing to nail in a spike before building — it is what separates a
robust `mux spawn` from a fragile one.

## Subcommands (v1)

| Command | Does |
|---|---|
| `lyx mux up` | Boot the orchestrator claude in this worktree's psmux session: env-stripped server, assigned `--session-id`, hooks installed, layout seeded. Idempotent / cold-recovers a crashed session. |
| `lyx mux spawn` | (Called by the orchestrator) launch a child agent in a stacked bottom pane; inject task; wire the result file. |
| `lyx mux attach` | Pop one **maximized** terminal attached to the session (real TTY → claude renders; the orchestrator itself never needs to attach — it observes via the result files / hooks). |
| `lyx mux on-start \| on-active \| on-idle` | Hook callbacks (per-child, keyed by `--session-id`): update state + drive focus. |
| `lyx mux deny-agent` | The `PreToolUse(Agent)` guardrail: emit the deny + steer-to-spawn decision. |
| `lyx mux resume` | Rebuild layout from local-state and `claude --resume <id>` each pane (env-stripped). |
| `lyx mux status` | Join local-state with `claude agents --json` → panes, sessions, orphans. |
| `lyx mux down` | Stop the session; mark intentional shutdown (so `up` won't recover). |

## v1 scope (smallest useful) and spikes-first

**v1:** `up` + `spawn` (one level deep) + `attach` + the `on-start`/`on-idle` hooks + `down`. That is a
visible orchestrator that can spawn one watchable, resumable sub-agent and auto-focus it — the core
value. Stacking to ≤3 deep, `resume`, `status`, and the deny-guardrail come right after.

**Do these spikes before committing the design:**
1. **Result contract** — child-writes-file ↔ parent-reads, with a real reviewer brief.
2. **Deny-guardrail** — confirm `PreToolUse(Agent)` deny + steer actually redirects the model to
   `lyx mux spawn`.
3. **Orchestrator bootstrap + pre-granted permissions** — so it can run `lyx mux spawn` autonomously
   without hanging on a permission prompt (`--dangerously-skip-permissions` or a scoped allowlist).

## Out of scope for `mux`

- **Multi-worktree tiling** → `mplex` (below).
- **Headless / fire-and-forget agents + monitoring** → could later use Claude Code's **supervisor**
  (`claude --bg`, `state`/`logs`/needs-input from `claude agents --json`) — see
  [`mux-hooks-exploration.md` §D](mux-hooks-exploration.md). Deliberately not the interactive path
  (latency), but a natural home for "dispatch and walk away" work and the Slack signal
  (`state==blocked` = needs human).

---

## `mplex` (future, low priority — sketch only)

One psmux instance, **a column per worktree**, tiling several worktrees at once (the old mux design's
"v1"). It composes over `mux`: each column is a worktree's orchestrator. Cross-worktree discovery can
reuse **`claude agents --json --cwd <repo>`** (sessions per subtree) and the supervisor for headless
columns. Overflow/orchestrator-switch via psmux **windows** inside one attached client (proven in
`mux-exploration.md`). Build only after `mux` is solid.

---

## Relationship to existing docs

- [`muxpoc.md`](../overview.md#modules) — already proves spawn, dominant-bottom layout, and crash recovery; `mux`
  productionises it.
- [`mux-hooks-exploration.md`](mux-hooks-exploration.md) — the evidence for decisions 1, 4–6, 8–9 and
  the §A-vs-§D split.
- `internal/muxengine` (see the package documentation and
  [overview.md#modules](../overview.md#modules)) — the as-built module this proposal informed
  (its column-per-worktree part remains a candidate for a future `mplex`).
