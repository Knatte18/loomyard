# Module: agent (design)

> **Status: Design — not built.** Per the [documentation lifecycle](../overview.md#documentation-lifecycle),
> this file is deleted when the module lands and its durable parts fold into the package
> header and `overview.md`.

`internal/agent` runs **one** LLM agent as an interactive session and returns its result over
the **file contract**. It is the unit [`loom`](loom.md) and [`review`](loom.md#lyx-review--the-generic-reviewer-module)
call once per spawn — "run this producer / handler / progress-judge, give me back its output
files." agent owns *which provider* (via an engine), *the prompt envelope*, and *what "done"
means*. It does **not** own pane placement (that is [`shed`](shed.md)) or psmux mechanics
(that is [`mux`](mux.md)).

## Interactive, never headless — the economic constraint

Every agent runs as an **interactive psmux session**, never headless `claude -p`. This is an
**economic constraint, not a technical one**: Anthropic is removing subscription coverage for
headless `claude -p` (announced for 2026-06-15, postponed but expected), so headless would
force API billing. Interactive sessions keep the subscription, and psmux is what makes a
programmatically-driven session interactive. (Recorded in the project
[`CLAUDE.md`](../../CLAUDE.md).) This single fact is *why* the whole
[`proc → mux → shed → agent`](../overview.md#execution-stack) stack exists instead of a plain
headless `exec`.

## Engines — provider-agnostic

agent runs a provider through an **engine**: a per-LLM adapter that knows how to launch and
drive its provider as a psmux session — construct the launch command, inject the prompt,
recognize the completion edge, locate the output. A **Claude engine** now; Gemini etc. later.

The **verdict/output contract is provider-invariant** — structured findings + a report file,
regardless of which model produced them. That invariance is exactly what makes engines
swappable: a review handler can be replaced with a different model **without touching the
review machinery or loom**. **Non-Claude support is not a current priority** — the engine
seam exists so the dependency is isolated, not because a second engine is imminent.

## The file contract

The only channel in and out of an agent is files — the same discipline that makes loom's
phases pure functions (see [loom.md](loom.md#why--the-inversion)):

- **In.** The task/prompt is handed to the provider as the **launch `[prompt]` arg**, never
  typed into a running TUI (multi-line paste into a live pane is unreliable — see
  [mux.md](mux.md#what-actually-works-empirical-guardrails)). For a review that means brief +
  profile (target, fasit, rubric, fix-scope).
- **Out.** The agent writes its structured result to a **file** the caller reads. The file
  hand-off is what makes the agent "return" to its caller — screen-scraping is a fallback for
  liveness, never the result channel.

## How one spawn runs

```
agent.Run(spec{role, worktree, prompt, engine})
  1. shed.SpawnAgent(role, worktree)        → a placed, visible pane (shed picks the column)
  2. engine.Launch(pane, prompt)            → claude --session-id <id> "<prompt>", env sanitized by mux
  3. wait on the Stop hook                  → last_assistant_message says done vs. asking;
                                              claude agents --json state==blocked = needs-input
  4. read the agent's output file(s)        → return to loom/review
```

Completion is **event-driven** off Claude Code's own `Stop` hook (keyed by the session id mux
assigned), not a poll — see [mux.md](mux.md#claude-code-hooks--the-event-driven-signal). A
`PreToolUse` guardrail denies the in-process `Agent` and `AskUserQuestion` tools so nested
work can never go invisible or block on a dialog the operator can't see
([mux.md](mux.md#pretooluse-guardrails)).

## Dependencies

- [`internal/shed`](shed.md) — obtains the placed pane to run in
- [`internal/mux`](mux.md) — drives the pane (launch, send-keys, hooks) via the engine
- engine adapters — the per-provider launch/drive logic
