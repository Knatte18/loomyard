# `lyx loom step` + an external supervisor skill

> **Status: Shipped — `lyx loom step` and the `/ly:ly-supervise` skill both landed 2026-09-12.**
> Supersedes `designs/llm-driven-loom-alternative.md` — its "full prompt" approach (an LLM re-deriving loom's whole phase sequence in prose) is replaced by the much thinner design below. Independent of the self-report tasks (`self-report-tier1.md`, `self-report-tier2.md`) — related in spirit, no code dependency either direction; all three can build in parallel.

## The problem this responds to

`loom`'s whole design bet (`docs/overview.md` Principle 7) is that phase sequencing is deterministic Go, not LLM judgment — for good reason: an LLM re-deriving the phase table in a prompt drifts out of sync with the real code. But Go's periodic gates only catch what they were specifically built to check; a defect that falls between two gates passes through untouched, where a continuously-present operator (Millhouse's own model, always an LLM orchestrator) might have noticed it live. The `crucible-loom-glyph-hardening` campaign's rounds 5-7 showed this cuts both ways: continuous LLM presence catches things no rubric anticipated, but a *single* pass of it isn't reliably sufficient either — it took four rounds, rotated models, to close one seam.

## The fix: keep Go owning sequencing; add an optional live supervisor around one atomic primitive

Not proposed: an LLM re-implementing loom's phase table (the rejected `llm-driven-loom-alternative.md` approach). Instead:

1. **`lyx loom step`** — a new Go verb. Reads the task's own persisted state (`status.md`/`_lyx/loom/status.json`) exactly as `lyx loom run`'s internal loop already does, dispatches the single next producer, and returns — no looping. `loom` still owns 100% of "what's next"; nothing about phase order, gating, or producer dispatch moves into a prompt anywhere. Likely a thin CLI wrapper over the same internal step-dispatch `Shed`'s resume/pause machinery already implies exists, not new phase-machine logic.
2. **A `/ly-*` skill** (naming per `docs/overview.md`'s skill convention) for a manually-spawned agent: call `lyx loom step` repeatedly, read what each step actually produced (not just its exit code), watch for anything that looks wrong even if the step technically passed, clean up on a detected crash/stuck state, and stop when the task reaches a terminal or operator-decision state. Thin wrapper per Principle 7 — the skill carries no phase knowledge of its own. Where it notices real friction, it calls the shipped `lyx selfreport create` itself — it has exactly the full-run context a Tier 2-style aggregation pass (see `self-report-tier2.md`) exists to reconstruct after the fact for an unsupervised run, so it doesn't need that machinery when it's the one watching.
3. **Operator instruction, no new code**: the skill tells the operator to open `lyx reed attach <target>` in a side terminal before starting, for the same continuous-presence reasoning — the supervisor watches mechanically; a human watching live is a second, independent layer, cheap to add since `attach` already exists.

## What needs to happen

1. **Done** — `lyx loom step` shipped as a thin wrapper over existing internal dispatch, not new phase logic: it bootstraps idempotently exactly as `run` does, then drives `shedengine.Shed`'s own `Step` exactly once and returns.
   Its return contract is the ten-key envelope recorded below.
2. **Done** — the `/ly:ly-supervise` skill shipped at `plugins/ly/skills/ly-supervise/SKILL.md`: the step-loop instructions, the anomaly-watching/cleanup responsibility, the `lyx selfreport create` call on noticed friction, and the `reed attach`-alongside operator instruction.

## The settled contract

`lyx loom step`'s success envelope carries exactly ten keys, closed at that set — a key outside these ten has no test and no documented meaning.
The first six derive directly from `shedengine.StepResult`; the last four are computed by `internal/loomcli`:

- `producer` — the row name `shed.Step` just dispatched.
- `outcome` — the producer's outcome as a string.
- `output` — the artifact path the producer wrote, or empty.
- `next` — the row name `shed.Step` expects to dispatch next.
- `state` — the phase-machine state after this step (e.g. `running`, `stuck`, `blocked`, a terminal state).
- `reason` — the human-facing explanation attached to `state`, when one exists.
- `continue` — derived as `state == "running"`, so a caller never carries its own copy of the state vocabulary.
- `history_length` — the length of the persisted step history after this step.
- `next_interrupt_policy` — `internal/loomshed`'s `InterruptPolicyFor(next)`, telling a caller whether re-invoking after an interruption on the next row is safe (`"reinvoke"`) or must hand back to the operator (`"handback"`).
- `status_file` — the absolute path to the task's status file.

A hard producer error, or any other pre-producer failure, is never folded into an `ok` envelope carrying a failed state — it is an error envelope with a non-zero exit, carried through `output.ErrFields`.
That error envelope's `kind` field carries exactly one of a five-value vocabulary, declared as Go constants in `internal/loomcli/step.go` and asserted as an exact set by a test: `busy` (the run lock is already held), `unseeded` (the status file could not be seeded), `ownership` (the seeded status file belongs to a different task), `bootstrap` (any other pre-producer setup failure), and `producer` (the producer call itself returned a hard error).
The supervisor skill's one-retry rule applies to the `producer` kind alone — every other kind is handed back to the operator with no retry, because none of them can be fixed by running the same command again.

Whether the supervisor skill may itself advance past a stuck or blocked gate is settled in the direction this doc already leaned: never.
On any non-running state, and on any error envelope, the skill stops and hands back to the operator.
It never clears `state`, never edits the status file, never re-seeds, and never pushes.
This matches crucible's own "the push/merge decision is the operator's" rule.

## Related

- [loom.md](loom.md) — the phase machine and producer table `step` dispatches against.
- `crucible/README.md` — the R5-R7 evidence informing why continuous presence alone isn't sufficient, and the clean-room/independent-verification discipline the supervisor skill borrows.
- [self-report-tier2.md](self-report-tier2.md) — the mechanism this skill substitutes for, specifically in the unsupervised-run case where no live session exists.
