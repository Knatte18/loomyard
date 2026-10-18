# Batten start: before numbers

A measurement report of a batten start on the code as it stood before any later card of the fast-start plan.
It records one run on one machine and is not claiming to stay true.

## What was measured, and where it came from

Measured in the integration-tier drive only:

- `Worktree-Create` plus `Seed-Child` over a hubforge hub with local bare origins, one step of `TestBattenIntegration_Rows` (`SeedChild_WritesASeedTheChildBootstrapAgreesWith`).
- The four-row run `FourRowRun_SeedsChildCommitsAndTearsDown`, once for `loom` and once for `darn`.
- The `Run-Shed` wait steps `StepDrivenRunShed_ReturnsOnTheChildsStateChange` and `RunShedPausedChild_WaitsThenTearsDownOnceDone`.

Command:

```
lyx gate test --tags integration ./internal/battencli -- -run '^TestBattenIntegration_Rows$' -v
```

Each number is the subtest wall time `go test -v` prints for that step.
The status history's transition timestamps are written by the same steps but are not separable per row inside a step, so the table carries step totals.

Not measured live: the inside-a-row breakdown (fabric add and its origin probes, the status write, the no-op marker, the merge probe, the status commit, the status push, the config reads) from a `lyx batten run -vv` trace.
A live start creates a pair through a mutating `lyx fabric` verb, which the implementing strand's sandbox rule refuses, and the hubforge origins are local bare repositories, so a network push is not represented either.
The inside-a-row breakdown is left to card 16's drive, which takes the trace when the machine allows it.

## Before numbers

| Step | Wall time |
|---|---|
| Worktree-Create + Seed-Child (`SeedChild_WritesASeedTheChildBootstrapAgreesWith`) | 0.13s |
| Seed-Child writing the run settings (`SeedChild_WritesTheRunSettingsIntoThePairsLocalLayer`) | 0.25s |
| Four-row run, `loom` | 0.37s |
| Four-row run, `darn` | 0.42s |
| Mid-list resume skipping the completed create row | 0.39s |
| Run-Shed, step-driven (`StepDrivenRunShed_ReturnsOnTheChildsStateChange`) | 2.13s |
| Run-Shed, paused child then teardown (`RunShedPausedChild_WaitsThenTearsDownOnceDone`) | 0.38s |
| Whole `TestBattenIntegration_Rows` | 7.75s |

The step-driven Run-Shed wait is the one step above two seconds; the rest of the start's rows each cost well under half a second against local origins.

## After

To be filled by card 16 (`batten-start-after-numbers`), which adds its numbers beside these.
