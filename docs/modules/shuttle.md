# Module: shuttle (design)

> **Status: Design — not built.** Per the [documentation lifecycle](../overview.md#documentation-lifecycle),
> this file is deleted when the module lands and its durable parts fold into the package header
> and `overview.md`.

The name is from weaving: the **shuttle** carries the [weft](../overview.md#weft-overlay-model)
thread back and forth across the loom — one pass. Here, one shuttle run = **one agent run**: it
carries the prompt *in* to the provider and the result *out* over the file contract. (It is
deliberately **not** called `agent` — that word is already loaded in Claude Code: the `Agent`
tool, `claude agents --json`, subagents, agent teams. Naming the module `agent` would collide
exactly where the module talks *to* Claude's agent machinery.)

`internal/shuttle` runs **one** LLM agent as an interactive session and returns its result. It is
the unit [`review`](review.md) and [`loom`](loom.md) call once per spawn — "run this producer /
handler / progress-judge, give me back its output files." shuttle owns *which provider* (via an
engine), *the prompt envelope*, and *what "done" means*. It does **not** own panes, layout, or
psmux mechanics — it asks [`mux`](mux.md) for a [strand](mux.md#the-strand-model) and drives the
LLM in it.

## Interactive, never headless — the economic constraint

Every agent runs as an **interactive psmux session**, never headless `claude -p`. This is an
**economic constraint, not a technical one**: Anthropic is removing subscription coverage for
headless `claude -p` (announced for 2026-06-15, postponed but expected), so headless would force
API billing. Interactive sessions keep the subscription, and psmux is what makes a
programmatically-driven session interactive. (Recorded in the project
[`CLAUDE.md`](../../CLAUDE.md).) This single fact is *why* the whole
[`proc → mux → shuttle`](README.md) stack exists instead of a plain headless `exec`.

## Engines — provider-agnostic

shuttle runs a provider through an **engine**: a per-LLM adapter that knows how to launch and
drive its provider as a psmux session — construct the launch command, inject the prompt,
recognize the completion edge, locate the output. A **Claude engine** now; Gemini etc. later.

The **verdict/output contract is provider-invariant** — structured findings + a report file,
regardless of which model produced them. That invariance is exactly what makes engines swappable:
a review handler can be replaced with a different model **without touching the review machinery or
loom**. **Non-Claude support is not a current priority** — the engine seam exists so the
dependency is isolated, not because a second engine is imminent.

## The file contract

The only channel in and out of a shuttle run is files — the same discipline that makes loom's
phases pure functions (see [loom.md](loom.md#why--the-inversion)):

- **In.** The task/prompt is handed to the provider as the **launch `[prompt]` arg**, never typed
  into a running TUI (multi-line paste into a live pane is unreliable — see
  [mux.md](mux.md#what-actually-works-empirical-guardrails)). For a review that means brief +
  profile (target, fasit, rubric, fix-scope).
- **Out.** The agent writes its structured result to a **file** the caller reads. The file
  hand-off is what makes the run "return" to its caller — screen-scraping is a fallback for
  liveness, never the result channel.

## How one run goes

```
shuttle.Run(spec{role, worktree, prompt, engine})
  1. engine builds the Claude command + --settings  → "claude --session-id X "<prompt>" --settings <hooks+guardrails>"
                                                       + the resume command "claude --resume X"
                                                       + the Stop hook writes turn-end to a file
  2. mux.AddStrand{ name, cmd, resumeCmd, worktree, display }  → mux spawns it (opaque), records the strand
                                                       (display is generic: anchor/height/focus — NOT "role")
  3. wait on shuttle's completion file (+ output file) → engine interprets last_assistant_message (done vs asking)
  4. read the agent's output file(s)                → return to review/loom; mux.RemoveStrand when done
```

shuttle translates its domain (a handler, a producer) into a **generic** `display` spec for
`AddStrand` — `mux` never sees the role, only `{anchor, height, focus}`
([mux.md](mux.md#why-a-closed-generic-vocabulary-not-a-type-field)).

Completion is **shuttle's own concern**, not mux's. shuttle composes the Claude `--settings` so the
`Stop` hook routes to a **file** shuttle waits on (fitting the file contract), and interprets
`last_assistant_message` (done vs. asking). It also composes the `PreToolUse` guardrails (deny the
in-process `Agent` + `AskUserQuestion` tools so nested work can never go invisible or block on a
hidden dialog). All of this — hooks, marker grammar, the resume command — is **opaque to
[`mux`](mux.md)**, which just spawns the command string shuttle hands it
([mux.md](mux.md#completion-and-hooks-live-in-shuttle-not-mux)).

## In-agent interrupt (optional)

[Pause](loom.md#graceful-pause) normally lands at a Go loop's **step boundary** — the leaf agent
finishes its small unit. To pause *faster* than that, shuttle can **interrupt-and-hold** a live
agent: send `ESC` to its pane (via [`mux`](mux.md) send-keys), leaving the interactive session
**alive and idle** rather than killed. Resume continues the warm session in place — no relaunch, no
`claude --resume`. It is **optional**: with [Builder](loom.md#builder--a-go-loop-advance-the-sibling-of-review-converge)
decomposed into batches/cards the boundary wait is short, so this is a latency nicety, not a
requirement.

"Halted vs done" rides the **file contract**, not a special hook: a held agent has not written its
result file (and its pane is still alive). `mux`'s only liveness signal is `pane-died`, and
completion semantics are shuttle's — so **no "halted" hook is needed**, and none fires in mux.

## Escalation — fresh spawn, not in-session /model

When an evaluator finds a worker stuck ([Builder](loom.md#builder--a-go-loop-advance-the-sibling-of-review-converge)),
escalation is a **fresh spawn of a higher-capability model** (Haiku → Sonnet) that reads the
durable reports — **not** a `/model` switch inside the stuck session. In-session `/model` is
possible (the session is interactive) but counter-productive for escalation: the new model would
inherit the stuck session's polluted context, which is exactly what you want to escape. A clean
spawn reading the reports starts fresh.

## Dependencies

- [`internal/mux`](mux.md) — `AddStrand`/`RemoveStrand` for the pane; drives it (launch, send-keys,
  hooks) via the engine
- engine adapters — the per-provider launch/drive logic
