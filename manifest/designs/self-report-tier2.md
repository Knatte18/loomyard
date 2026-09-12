# self-report Tier 2 — per-agent friction notes, aggregated for unsupervised runs

> **Status: Shipped, design settled with the operator 2026-09-12.**
> Split out of the former `designs/self-report.md` (which covered both tiers as one doc) so Tier 2 can build independently of Tier 1 and of `loom-step.md` — no code dependency in any direction; all three can build in parallel.

## The problem: no session has full-run context

Millhouse's `mill-self-report` works because in Millhouse the LLM *is* the orchestrator — one session holds the whole run's conversation and can retrospectively notice friction across everything it just did. `loom` inverts this on purpose (see [loom.md](loom.md)): "No agent knows about rounds, gates, N-caps, finalize, or the others. Each phase becomes a pure function over files." There is structurally no single LLM session with full-run context to reflect from — porting Millhouse's self-report model directly doesn't work here. That isn't a missing feature, it's the design.

## The mechanism

Every spawned agent (producer, reviewer, webster implementer fork, ...) already writes one output file per the file-contract discipline. Tier 2 adds one more, optional file to that same contract: a short friction note about anything unexpected **within its own scoped task** — the same self-report judgment call Millhouse's LLM makes today, just narrowed to what a single-purpose agent can actually see. This does not violate the "doesn't know about the others" rule: an agent reporting on its own scope is the same as it already writing its own output file, not a window into the rest of the run.

**Explicit limitation, not a bug to fix:** a Tier 2 note can only ever describe friction inside its own narrow task. It cannot notice a systemic problem that only shows up across several phases — that class of signal is Tier 1's job where it's structurally detectable (see [self-report-tier1.md](self-report-tier1.md)), and out of scope here otherwise.

## Aggregation and the reflection step

Go collects every Tier 2 note emitted during a run (it reads every phase's output file regardless) and, at a natural end point — Finalize, or a `stuck` escalation — spawns **one** dedicated reflection agent over the aggregated dossier. This mirrors the `Raddle` pattern (see [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots)): a fresh-context agent reading only the accumulated notes, not carrying the baggage of having "been there" for the whole run. That agent makes the actual self-report judgment call (worth filing? one issue or several? title/body?) and invokes the shipped `lyx selfreport create` primitive to do the actual filing.

## Scope: this is specifically for the unsupervised path

This aggregation-and-reflection machinery exists to work around loom having no single full-context session — but a task driven by the step-loop supervisor skill (see [loom-step.md](loom-step.md)) *is* such a session while it's running. When that skill is driving, the supervisor notices friction directly and calls `lyx selfreport create` itself — no separate aggregation pass needed for that run. So Tier 2 as designed here is scoped specifically to a task run via plain `lyx loom run`, with nobody watching live. This is a documentation/scoping note, not a code dependency — Tier 2 can be built and shipped whether or not `loom-step.md`'s skill exists yet.

## Relationship to the shipped `selfreport` module

This does not replace `lyx selfreport create` (shipped) — it adds an automatic trigger on top of the same primitive: today, manual only; this adds the aggregation/reflection agent as a second automatic trigger (alongside Tier 1's direct Go trigger).

## How the design settled

- Where notes physically live: `.lyx/loom/friction/`, via `loomengine.LoomFrictionDir` built on `LoomScratchDir`, because the Durable-vs-Ephemeral State Invariant puts never-tracked files under `.lyx` and `_lyx` would drag in the Fabric Git Invariant's commit-seam machinery for a file deleted minutes later.
- Default-on versus opt-in per producer or profile: default-on, with one global `loom.yaml` key (`friction`) that is both the model spec and the kill switch, because the feature's value is breadth of coverage and per-row opt-in would mean a Tier 2 key on five different recipe engines whose row names are durable on-disk identities.
- Cross-phase semantic friction: still explicitly deferred, and now recorded as a stated limitation of the shipped design rather than an open question — a Tier 2 note can only ever describe friction inside its own narrow task.

## Related

- [loom.md](loom.md) — the phase machine and status-file contract Tier 1 reads from, and the `Raddle` pattern this tier's reflection step mirrors.
- [self-report-tier1.md](self-report-tier1.md) — the sibling tier for structural anomalies Go can detect directly.
- [loom-step.md](loom-step.md) — the supervisor-skill alternative that substitutes for this tier's purpose when it's the one driving a task.
