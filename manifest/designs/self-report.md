# self-report: two-tier friction capture (structural + per-phase)

> **Status: Design — not built.** Extends the shipped `selfreport` module (`lyx selfreport create`) with two automatic triggers, in addition to today's manual-only invocation. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold into `internal/loomengine`'s package doc (Tier 1) and the producer/agent-contract docs (Tier 2) when this lands, and this file is deleted.

## The problem: no session has full-run context

Millhouse's `mill-self-report` works because in Millhouse the LLM *is* the orchestrator — one session holds the whole run's conversation and can retrospectively notice friction across everything it just did. `loom` inverts this on purpose (see [loom.md](loom.md)): "No agent knows about rounds, gates, N-caps, finalize,
or the others.
Each phase becomes a pure function over files."
There is structurally no single LLM session with full-run context to reflect from — porting Millhouse's self-report model directly doesn't work here.
That isn't a missing feature, it's the design.

## Tier 1 — Go detects structurally, no LLM involved

loom's own status file (`.lyx/loom/status.json`, see [loom.md](loom.md#state--contracts)) records exactly the kind of anomaly Millhouse's self-report catches by an LLM noticing a pattern in its own transcript: crash-resumes, `stuck` escalations, repeated review rounds on the same finding.
Go can file these directly off its own history trail — deterministic, no LLM call, and strictly more complete than an LLM's approximate recall of its own session, since it reads an exact record instead of remembering one.

## Tier 2 — a narrow, per-agent friction note

Every spawned agent (producer, reviewer, webster implementer fork, ...) already writes one output file per the file-contract discipline.
Tier 2 adds one more, optional file to that same contract: a short friction note about anything unexpected **within its own scoped task** — the same self-report judgment call Millhouse's LLM makes today, just narrowed to what a single-purpose agent can actually see.
This does not violate the "doesn't know about the others" rule: an agent reporting on its own scope is the same as it already writing its own output file, not a window into the rest of the run.

**Explicit limitation, not a bug to fix:** a Tier 2 note can only ever describe friction inside its own narrow task.
It cannot notice a systemic problem that only shows up across several phases — that class of signal is Tier 1's job where it's structurally detectable, and out of scope here otherwise (see Open questions).

## Aggregation and the reflection step

Go collects every Tier 2 note emitted during a run (it reads every phase's output file regardless) and, at a natural end point — Finalize, or a `stuck` escalation — spawns **one** dedicated reflection agent over the aggregated dossier.
This mirrors the `Raddle` pattern (see [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots)): a fresh-context agent reading only the accumulated notes, not carrying the baggage of having "been there" for the whole run.
That agent makes the actual self-report judgment call (worth filing? one issue or several? title/body?) and invokes the shipped `lyx selfreport create` primitive to do the actual filing.

## Relationship to the shipped `selfreport` module

This design does not replace `lyx selfreport create` (shipped) — it changes **when/how** it gets invoked: today, manual only;
this adds two automatic triggers on top of the same primitive (Go directly, for Tier 1;
the aggregation/reflection agent, for Tier 2).

## Open questions

- Where Tier 2 notes physically live (a per-phase `_lyx/friction/<phase>.md`? appended to the status file?) is not yet pinned.
- Whether every phase gets this by default,
  or it's opt-in per producer/profile.
- Cross-phase semantic friction (a pattern only visible across several phases, but not a Tier-1-detectable structural event) is not addressed here — deferred, not designed.

## Related

- [loom.md](loom.md) — the phase machine and status-file contract Tier 1 reads from,
  and the `Raddle` pattern Tier 2's reflection step mirrors.
