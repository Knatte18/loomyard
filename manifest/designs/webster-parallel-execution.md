# webster: parallel card execution via worktrees + a DAG

> **Status: Speculative, explored twice, rejected for now both times.** Not pursued further until webster's card-list rewrite (`internal/websterengine`, landed) has real running mileage, and this looks worth the complexity by measured evidence, not estimate. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), if this is ever picked up the durable parts fold into `internal/websterengine`'s package doc and this file is deleted; if abandoned, this file is simply deleted.

## Why it's parked

[loom-plan-spec.md](../../contracts/specs/loom-plan-spec.md) is deliberately a flat, sequential card list with **no DAG wired into scheduling**.
Running cards as *parallel* forks would require reintroducing a DAG and worktree isolation, which reopens exactly the problem sequential execution avoids: git's index/staging area is a single shared file per working tree, so two forks concurrently committing — even to fully disjoint files — race on the same lock.
Current (2026) ecosystem guidance treats worktree isolation as effectively required for concurrent subagents on the same repo.
A declared-disjoint card pair that turns out (via deviation) to actually overlap is a **live corruption risk** in a concurrent-no-worktree model, not just a bookkeeping problem to fix after the fact as it is sequentially;
quarry would also see other forks' uncommitted, potentially syntactically-broken in-flight edits while serving a concurrent fork's query, since there's no filesystem isolation between them.

**A possible middle ground, if revisited:** let forks edit concurrently (the LLM-thinking-dominated part) but serialize the actual `git add`+commit+verify step through a mutex in webster's Go orchestration ("edit in parallel, land sequentially").
Even this requires *strictly enforced* file-disjointness (not just DAG-edge-absence) to be safe.
Not built.

## A possible unblocking shape (2026-08-20)

A structural alternative to the rejected concurrent-forks-in-one-tree shape above: each DAG-independent group (a batch, possibly one card) gets its own `fabric`-spawned worktree — genuinely its own git index/HEAD, which is what the rejected shape lacked — running the existing `Preflight → Webster → Finalize` row set unchanged, with `Webster`'s `Geometry.PlanDir` (already told, not derived) pointed at the source plan and a new batch-filter selecting the one group to run.
Merge-back reuses `fabric`'s existing merge machinery, not new infrastructure, and a group's completion gates on a build+test of the *merged* result rather than of the group's own worktree in isolation.
The ready set recomputes wave to wave rather than being precomputed upfront, so a group that turns out to depend on another's output is simply not ready yet instead of being mis-scheduled.

This shape is what makes the rejection at the top of this doc specific rather than general: that rejection was about concurrent forks sharing one checkout's git index, which worktree-per-group does not do.
The Status banner above still describes the earlier, rejected shape and has not been rewritten for this one — reconciling it belongs to the Someday `webster: worktree-per-card parallel execution` roadmap item, whenever it is picked up.

Grouping granularity (one card vs. several) is orthogonal to safety — the original rejection above conflated it with the working-tree-sharing hazard, but genuine worktree-per-lane isolation is what actually matters.
Not yet a plan: needs the DAG source (the Someday `quarry-backed plan symbol verification` roadmap item) and a writeup reconciling this with the still-open questions elsewhere in this doc (typical-plan wave-width evidence, the batchifier/planner change needed to emit groups).

## The case study (from an earlier, more detailed design draft, `websterv2.md`, now retired)

A card-level dependency analysis of the 42-card plan that built webster v1 overturned the naive "linear chain" assumption:

| Metric | Value |
|---|---|
| Cards | 42 |
| Batches (sequential) | 9 |
| True card-DAG depth (critical path) | 7 |
| Peak wave width | 10 |
| Cards off the critical path | 83% (35 of 42) |
| Wave widths (1→7) | 10, 9, 7, 7, 6, 2, 1 |

- **The batch DAG over-constrains.**
  Sequential batching's own dependency declarations were largely spurious at card granularity — ~26 cards (waves 1–3) could have run as three parallel waves instead of spread across four sequential batches.
- **File-conflicts barely bind** when the plan is create-then-extend — nearly every file-conflict pair is already dependency-ordered into different waves.
- **The tail is the real ceiling, not dependencies** — a hard funnel near the end of a plan (e.g. final registration → sandbox validation) crashes wave widths regardless of fork budget;
  speedup is front-loaded.
- **Honest speedup estimate: ~2–3× wall-clock**, discounted from a naively-computed 3–5× because a wave's wall-clock is its *slowest* card,
  and the heaviest implementation cards tend to sit on the critical path.
  Two caveats push it down further: semantic edges were *inferred* from card descriptions (real edit-time dependencies only add edges and shrink waves), and 42 cards is an atypically large plan — a routine 5–10-card task has little fan-out headroom and would show a speedup near 1×, dominated by warm-context, not parallelism.

## Decision gate, if ever revisited

Run the card-DAG width analysis across several *real* completed plans, weighted for **typical** task size, not this one outlier.
Wide (fat waves, short critical path, low file-conflict) → an executor might pay off.
Narrow (long critical path, most cards chained, or simply few cards) → parallelism won't materialize;
sequential is the complete correct design, not just the MVP.

## The separable, cheap win already taken

A planner that emits true card dependencies (`depends-on`) instead of an over-constrained batch line recovers most of the *width* insight with **no worktrees and no concurrent execution** — this is exactly what [loom-plan-spec.md](../../contracts/specs/loom-plan-spec.md) already does.
Only the *executor that actually runs the width* (this entry) remains parked.

## Relationship to quarry (Part B of the retired draft)

The retired `websterv2.md` draft also had a Part B — structured impact lookup via `go/packages`/`gopls` (find-all-references as a Go verb instead of LLM-driven grep).
That idea is superseded, not lost: it's the direct ancestor of [quarry](https://github.com/Knatte18/quarry), which generalizes it to a multi-language, daemon-based design.

## Related

- `internal/websterengine`'s package documentation — the sequential model this would extend.
- [loom-plan-spec.md](../../contracts/specs/loom-plan-spec.md) — already captures the cheap win (`depends-on`).
- [quarry](https://github.com/Knatte18/quarry) — Part B's successor.
