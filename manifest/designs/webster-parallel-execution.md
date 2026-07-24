# webster: parallel card execution via worktrees + a DAG

> **Status: Speculative, explored twice, rejected for now both times.** Not pursued further until
> [webster-rewrite.md](webster-rewrite.md) is real and running, and this looks worth the
> complexity by measured evidence, not estimate. Per the [documentation
> lifecycle](../../docs/overview.md#documentation-lifecycle), if this is ever picked up the
> durable parts fold into `internal/websterengine`'s package doc and this file is deleted; if
> abandoned, this file is simply deleted.

## Why it's parked

[plan-format-v3.md](plan-format-v3.md) is deliberately a flat, sequential card list with **no
DAG wired into scheduling**. Running cards as *parallel* forks would require reintroducing a DAG
and worktree isolation, which reopens exactly the problem sequential execution avoids: git's
index/staging area is a single shared file per working tree, so two forks concurrently
committing — even to fully disjoint files — race on the same lock. Current (2026) ecosystem
guidance treats worktree isolation as effectively required for concurrent subagents on the same
repo for exactly this reason. A declared-disjoint card pair that turns out (via deviation) to
actually overlap is a **live corruption risk** in a concurrent-no-worktree model, not just a
bookkeeping problem to fix after the fact as it is sequentially; codeintel would also see other
forks' uncommitted, potentially syntactically-broken in-flight edits while serving a concurrent
fork's query, since there's no filesystem isolation between them.

**A possible middle ground, if this is ever revisited:** let forks edit concurrently (the
LLM-thinking-dominated part) but serialize the actual `git add`+commit+verify step through a
mutex in webster's Go orchestration ("edit in parallel, land sequentially"). Even this requires
*strictly enforced* file-disjointness (not just DAG-edge-absence) to be safe. Not built.

## The case study (from an earlier, more detailed design draft, `websterv2.md`, now retired)

A card-level dependency analysis of the 42-card plan that built webster v1 overturned the naive
"linear chain" assumption:

| Metric | Value |
|---|---|
| Cards | 42 |
| Batches (sequential) | 9 |
| True card-DAG depth (critical path) | 7 |
| Peak wave width | 10 |
| Cards off the critical path | 83% (35 of 42) |
| Wave widths (1→7) | 10, 9, 7, 7, 6, 2, 1 |

- **The batch DAG over-constrains.** Sequential batching's own dependency declarations were
  largely spurious at card granularity — ~26 cards (waves 1–3) could have run as three parallel
  waves instead of spread across four sequential batches.
- **File-conflicts barely bind** when the plan is create-then-extend — nearly every file-conflict
  pair is already dependency-ordered into different waves.
- **The tail is the real ceiling, not dependencies** — a hard funnel near the end of a plan (e.g.
  final registration → sandbox validation) crashes wave widths regardless of fork budget;
  speedup is front-loaded.
- **Honest speedup estimate: ~2–3× wall-clock**, discounted from a naively-computed 3–5× because
  a wave's wall-clock is its *slowest* card, and the heaviest implementation cards tend to sit on
  the critical path. Two caveats push the real number down further: semantic edges were
  *inferred* from card descriptions (real edit-time dependencies only add edges and shrink
  waves), and 42 cards is an atypically large plan — a routine 5–10-card task has little fan-out
  headroom and would show a speedup near 1×, dominated by warm-context, not parallelism.

## Decision gate, if ever revisited

Run the card-DAG width analysis across several *real* completed plans, weighted for **typical**
task size, not this one outlier. Wide (fat waves, short critical path, low file-conflict) → an
executor might pay off. Narrow (long critical path, most cards chained, or simply few cards) →
parallelism won't materialize; sequential is the complete correct design, not just the MVP.

## The separable, cheap win already taken

A planner that emits true card dependencies (`depends-on`) instead of an over-constrained batch
line recovers most of the *width* insight with **no worktrees and no concurrent execution** —
this is exactly what [plan-format-v3.md](plan-format-v3.md) already does. Only the *executor
that actually runs the width* (this entry) remains parked.

## Relationship to codeintel (Part B of the retired draft)

The retired `websterv2.md` draft also had a Part B — structured impact lookup via
`go/packages`/`gopls` (find-all-references as a Go verb instead of LLM-driven grep). That idea is
superseded, not lost: it's the direct ancestor of the [codeintel](codeintel-redesign.md)
proposal, which generalizes it to a multi-language, daemon-based design.

## Related

- [webster-rewrite.md](webster-rewrite.md) — the sequential model this would extend.
- [plan-format-v3.md](plan-format-v3.md) — already captures the cheap win (`depends-on`).
- [codeintel-redesign.md](codeintel-redesign.md) — Part B's successor.
