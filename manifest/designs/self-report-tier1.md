# self-report Tier 1 — Go-detected structural anomalies

> **Status: Planned, design settled with the operator 2026-09-12.**
> Split out of the former `designs/self-report.md` (which covered both tiers as one doc) so Tier 1 can build independently of Tier 2 and of `loom-step.md` — no code dependency in any direction; all three can build in parallel. Extends the shipped `selfreport` module (`lyx selfreport create`) with an automatic trigger, in addition to today's manual-only invocation.

## What it is

`loom`'s own status file (`_lyx/loom/status.json`, see [loom.md](loom.md#state--contracts)) records exactly the kind of anomaly Millhouse's self-report catches by an LLM noticing a pattern in its own transcript: crash-resumes, `stuck` escalations, repeated review rounds on the same finding.
Go can file these directly off its own history trail — deterministic, no LLM call, and strictly more complete than an LLM's approximate recall of its own session, since it reads an exact record instead of remembering one.

This applies identically regardless of how a task is driven — plain `lyx loom run`, or the step-loop supervisor skill (see [loom-step.md](loom-step.md)). It costs nothing and needs no session watching, so there's no reason to gate it behind either driving mode.

## Relationship to the shipped `selfreport` module

This does not replace `lyx selfreport create` (shipped) — it adds an automatic trigger on top of the same primitive: today, manual only; Tier 1 has Go itself invoke it directly off the status file's own history, with no LLM judgment call involved.

## What needs to happen

1. Decide which status-file patterns are worth auto-filing (crash-resume, `stuck` escalation, N repeated review rounds on the same finding — a starting list, not exhaustive).
2. Wire the detection into loom's own status-file write path (or a post-hoc scan) so it fires deterministically, without needing any agent or session present.
3. Call `lyx selfreport create` with the detected pattern as the filing payload.

## Open questions

- Exact trigger list and thresholds (e.g. "repeated" = how many rounds) — not yet pinned.
- Whether this is on by default for every task, or opt-in per producer/profile.

## Related

- [loom.md](loom.md) — the phase machine and status-file contract this reads from.
- [self-report-tier2.md](self-report-tier2.md) — the sibling tier for friction an agent notices within its own scoped task, which Go cannot detect structurally.
