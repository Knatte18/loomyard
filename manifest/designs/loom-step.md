# `lyx loom step` + an external supervisor skill

> **Status: Planned, design settled with the operator 2026-09-12.**
> Supersedes `designs/llm-driven-loom-alternative.md` — its "full prompt" approach (an LLM re-deriving loom's whole phase sequence in prose) is replaced by the much thinner design below. Independent of the self-report tasks (`self-report-tier1.md`, `self-report-tier2.md`) — related in spirit, no code dependency either direction; all three can build in parallel.

## The problem this responds to

`loom`'s whole design bet (`docs/overview.md` Principle 7) is that phase sequencing is deterministic Go, not LLM judgment — for good reason: an LLM re-deriving the phase table in a prompt drifts out of sync with the real code. But Go's periodic gates only catch what they were specifically built to check; a defect that falls between two gates passes through untouched, where a continuously-present operator (Millhouse's own model, always an LLM orchestrator) might have noticed it live. The `crucible-loom-glyph-hardening` campaign's rounds 5-7 showed this cuts both ways: continuous LLM presence catches things no rubric anticipated, but a *single* pass of it isn't reliably sufficient either — it took four rounds, rotated models, to close one seam.

## The fix: keep Go owning sequencing; add an optional live supervisor around one atomic primitive

Not proposed: an LLM re-implementing loom's phase table (the rejected `llm-driven-loom-alternative.md` approach). Instead:

1. **`lyx loom step`** — a new Go verb. Reads the task's own persisted state (`status.md`/`_lyx/loom/status.json`) exactly as `lyx loom run`'s internal loop already does, dispatches the single next producer, and returns — no looping. `loom` still owns 100% of "what's next"; nothing about phase order, gating, or producer dispatch moves into a prompt anywhere. Likely a thin CLI wrapper over the same internal step-dispatch `Shed`'s resume/pause machinery already implies exists, not new phase-machine logic.
2. **A `/ly-*` skill** (naming per `docs/overview.md`'s skill convention) for a manually-spawned agent: call `lyx loom step` repeatedly, read what each step actually produced (not just its exit code), watch for anything that looks wrong even if the step technically passed, clean up on a detected crash/stuck state, and stop when the task reaches a terminal or operator-decision state. Thin wrapper per Principle 7 — the skill carries no phase knowledge of its own. Where it notices real friction, it calls the shipped `lyx selfreport create` itself — it has exactly the full-run context a Tier 2-style aggregation pass (see `self-report-tier2.md`) exists to reconstruct after the fact for an unsupervised run, so it doesn't need that machinery when it's the one watching.
3. **Operator instruction, no new code**: the skill tells the operator to open `lyx reed attach <target>` in a side terminal before starting, for the same continuous-presence reasoning — the supervisor watches mechanically; a human watching live is a second, independent layer, cheap to add since `attach` already exists.

## What needs to happen

1. Design and build `lyx loom step`: confirm it can be a thin wrapper over existing internal dispatch rather than new phase logic; decide its return contract (what a caller needs to know: which phase ran, pass/fail, whether the task is now at a terminal/blocked state).
2. Write the `/ly-*` supervisor skill: the step-loop instructions, the anomaly-watching/cleanup responsibility, the `lyx selfreport create` call on noticed friction, and the `reed attach`-alongside operator instruction.

## Open questions

- `lyx loom step`'s exact return contract — not yet pinned.
- Whether the supervisor skill should itself be allowed to advance past a stuck/blocked gate, or must always hand that back to the operator — leans toward the latter (matches crucible's own "the push/merge decision is the operator's" rule) but not decided.

## Related

- [loom.md](loom.md) — the phase machine and producer table `step` dispatches against.
- `crucible/README.md` — the R5-R7 evidence informing why continuous presence alone isn't sufficient, and the clean-room/independent-verification discipline the supervisor skill borrows.
- [self-report-tier2.md](self-report-tier2.md) — the mechanism this skill substitutes for, specifically in the unsupervised-run case where no live session exists.
