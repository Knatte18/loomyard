# `lyx loom step` + an external supervisor skill, unifying self-report's two tiers

> **Status: Planned, design settled with the operator 2026-09-12.**
> Supersedes both `designs/llm-driven-loom-alternative.md` (its "full prompt" approach — an LLM re-deriving loom's whole phase sequence in prose — is replaced by the much thinner design below) and `designs/self-report.md` (folded in below: Tier 1 is unchanged, Tier 2 is now scoped specifically to unsupervised runs).

## The problem this responds to

`loom`'s whole design bet (`docs/overview.md` Principle 7) is that phase sequencing is deterministic Go, not LLM judgment — for good reason: an LLM re-deriving the phase table in a prompt drifts out of sync with the real code. But Go's periodic gates only catch what they were specifically built to check; a defect that falls between two gates passes through untouched, where a continuously-present operator (Millhouse's own model, always an LLM orchestrator) might have noticed it live. The `crucible-loom-glyph-hardening` campaign's rounds 5-7 showed this cuts both ways: continuous LLM presence catches things no rubric anticipated, but a *single* pass of it isn't reliably sufficient either — it took four rounds, rotated models, to close one seam.

Separately, `self-report.md` already named a real gap: `loom` deliberately has no single session with full-run context to reflect from ("each phase becomes a pure function over files... that isn't a missing feature, it's the design"), so Millhouse's own self-report model (one orchestrator LLM noticing friction across everything it just did) doesn't port directly.

## The fix: keep Go owning sequencing; add an optional live supervisor around one atomic primitive

Not proposed: an LLM re-implementing loom's phase table (the rejected `llm-driven-loom-alternative.md` approach). Instead:

1. **`lyx loom step`** — a new Go verb. Reads the task's own persisted state (`status.md`/`_lyx/loom/status.json`) exactly as `lyx loom run`'s internal loop already does, dispatches the single next producer, and returns — no looping. `loom` still owns 100% of "what's next"; nothing about phase order, gating, or producer dispatch moves into a prompt anywhere. Likely a thin CLI wrapper over the same internal step-dispatch `Shed`'s resume/pause machinery already implies exists, not new phase-machine logic.
2. **A `/ly-*` skill** (naming per `docs/overview.md`'s skill convention) for a manually-spawned agent: call `lyx loom step` repeatedly, read what each step actually produced (not just its exit code), watch for anything that looks wrong even if the step technically passed, clean up on a detected crash/stuck state, and stop when the task reaches a terminal or operator-decision state. Thin wrapper per Principle 7 — the skill carries no phase knowledge of its own.
3. **Operator instruction, no new code**: the skill tells the operator to open `lyx reed attach <target>` in a side terminal before starting, for the same continuous-presence reasoning — the supervisor watches mechanically; a human watching live is a second, independent layer, cheap to add since `attach` already exists.

## Self-report's two tiers, reconciled against this

- **Tier 1 (Go-detected structural anomalies — crash-resumes, `stuck` escalations, repeated review rounds)** is unchanged by this doc and applies identically whether a task runs via plain `lyx loom run` or via the step-loop skill. It costs nothing and needs no session watching.
- **Tier 2 (per-agent friction notes, aggregated and reflected on by one dedicated agent at a natural end point)** was designed specifically to work around loom having no single full-context session. **The step-loop supervisor IS such a session, when it's running.** So Tier 2's aggregation-and-reflection machinery is scoped to the case that actually needs it: a task run unsupervised, via plain `lyx loom run` with nobody watching live. When the step-loop skill is driving instead, the supervisor notices friction directly (it has the context Tier 2 was reconstructing after the fact) and calls the shipped `lyx selfreport create` itself — no separate aggregation pass needed for that run.

## What needs to happen

1. Design and build `lyx loom step`: confirm it can be a thin wrapper over existing internal dispatch rather than new phase logic; decide its return contract (what a caller needs to know: which phase ran, pass/fail, whether the task is now at a terminal/blocked state).
2. Write the `/ly-*` supervisor skill: the step-loop instructions, the anomaly-watching/cleanup responsibility, the `lyx selfreport create` call on noticed friction, and the `reed attach`-alongside operator instruction.
3. Build self-report's Tier 1 (Go-detected structural anomalies from the status file) exactly as `self-report.md` designed it — unaffected by the skill's existence.
4. Build Tier 2's aggregation + one-reflection-agent pass, explicitly scoped in its own doc/comment to the unsupervised `lyx loom run` path only, so it's never confused for something the supervisor skill also needs.

## Open questions

- `lyx loom step`'s exact return contract — not yet pinned.
- Where Tier 2 notes physically live (`self-report.md`'s own open question, still open) — a per-phase file, or appended to the status file.
- Whether the supervisor skill should itself be allowed to advance past a stuck/blocked gate, or must always hand that back to the operator — leans toward the latter (matches crucible's own "the push/merge decision is the operator's" rule) but not decided.

## Related

- [loom.md](loom.md) — the phase machine and producer table `step` dispatches against.
- `crucible/README.md` — the R5-R7 evidence informing why continuous presence alone isn't sufficient, and the clean-room/independent-verification discipline the supervisor skill borrows.
