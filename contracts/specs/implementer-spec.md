# Implementer contract: what loom's implementer row takes and delivers

> **Status: Contract — pinned.** This doc pins what every implementer that runs loom's implementer row takes in and delivers, so the rows after it never depend on which implementer ran.
> Durable reference doc — kept, not deleted on landing.
> How one implementer reaches these shapes is that implementer's own spec, such as [webster-spec.md](webster-spec.md).

## The row

Loom's implementer row is the recipe's `Webster` row, between `Plan-Bouncer` and the `Webster-Review` perch.
Its `Implementer` engine builds the row with the implementer `loom.yaml`'s `implementer` key names, read through the row's `implementer_setting: loom.implementer`.
The key is settable per task through `run_settings`, and a value outside the closed implementer table in `internal/shedrecipe` refuses when the recipe builds.
Every implementer builds the same row with the same edges, so no row is skipped or entered conditionally on which one runs.

| Value | Implementer |
| --- | --- |
| `webster` | One long-lived Merriam session forking one implementer per execution batch ([webster-spec.md](webster-spec.md)). |

## In

- The approved plan: the flat card list under `_lyx/plan/`, read through `internal/planparser`, the plan format's sole parser ([loom-plan-spec.md](loom-plan-spec.md)).
- The task branch, checked out in the task's worktree, with the plan committed on it.

How the cards are grouped into units of work is the implementer's own call, configured by the implementer's own config: webster batches through `batcher.yaml`.
The plan writer writes a flat plan and never groups its cards.

## Out

- **Commits on the task branch** that build the plan, leaving a clean tree.
  The implementer never pushes: the landing rows push the branch.
- **A durable run directory** of the implementer's own under `_lyx/`, holding its state file and run record, which Go commits once the row reports Done.
  Webster's is `_lyx/webster/`, holding `state.json`, `outcome.yaml` and `summary.md`.
- **An outcome**, returned as the row's shed outcome:

| Outcome | Meaning | Route |
| --- | --- | --- |
| Done | The plan is built and the plan-level verify passed. | `on_done` to `Webster-Bouncer`. |
| Stuck with a reroute | The implementer refused the approved plan with a new plan-conflict record for plan review. | `on_reroute` to `Plan-Burler-Review`, within shed's reroute cap. |
| Stuck | Any other fault, its reason naming the way forward. | The row has no `on_stuck`, so the run halts for a human. |

A Stuck reason follows [refusal-spec.md](refusal-spec.md).
A fault in the implementer's own config, such as a `batcher.yaml` that does not resolve, is a Stuck the operator fixes before re-stepping the row.

## See also

- [webster-spec.md](webster-spec.md) — the `webster` implementer's own cross-module contract.
- [shed-recipe-spec.md](shed-recipe-spec.md) — the recipe rows and the registry the `Implementer` engine sits in.
