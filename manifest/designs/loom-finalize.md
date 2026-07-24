# loom: Finalize phase

> **Status: Design — not built.** Split out from [loom.md](loom.md) — Finalize is a substantial,
> fairly self-contained phase specification, distinct from the phase-machine core loom.md now
> focuses on. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle),
> when this lands the durable parts fold into the relevant package doc and this file is deleted.

## What it does

**Vital, not deferred** (unlike Raddle). Go-first: the happy path (no conflicts) is pure Go —
squash, push, done, zero LLM cost. An LLM is spawned only on merge conflict (during merge-in
from parent, or the merge to parent itself), escalating **the same way Builder escalates a
stuck batch to a fresh higher-capability model** (see
[webster-rewrite.md](webster-rewrite.md)) — not a `/model` switch inside a polluted session.

Mostly wiring on top of the already-built `warp` mechanics (absorbed into `fabric` once that
lands — see [fabric.md](fabric.md)); worktree/branch/junction/portal teardown is explicitly
**out of scope** — that's `warp cleanup`'s (future: `fabric`'s) already-existing, separate job,
which cannot run from inside the worktree being removed, the same reason `mill-cleanup` runs
from the hub, never a task worktree.

## PR creation, when configured

If `require_pr_to_base` is set, the PR title/body is dumped **verbatim** from the prose summary
artifact webster adds to its final action (see [webster-rewrite.md](webster-rewrite.md)) — no
dedicated LLM call needed in Finalize itself, since that summary is the only artifact with full
oversight of what was actually built, including deviations from the original plan.

## Config

`finalize.yaml` holds safe defaults (e.g. no direct merge to main without a PR); `loom.yaml` can
override the same keys for orchestrated runs — same shape as the existing "per-phase profiles
live in loom, not perch.yaml" precedent, generalized to any module loom drives as a black-box
gate.

## Related

- [loom.md](loom.md) — the phase machine Finalize is the last phase of.
- [webster-rewrite.md](webster-rewrite.md) — the summary artifact Finalize consumes verbatim for
  PR bodies, and the escalation pattern Finalize mirrors.
- [fabric.md](fabric.md) — the mechanics Finalize wires on top of.
