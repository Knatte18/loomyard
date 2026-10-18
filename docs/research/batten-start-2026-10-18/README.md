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

Measured after cards 2 to 15, with the same command as above.
Every number comes from the integration-tier drive; none was taken live (see below).

| Step | Before | After |
|---|---|---|
| Worktree-Create + Seed-Child (`SeedChild_WritesASeedTheChildBootstrapAgreesWith`) | 0.13s | 0.15s |
| Seed-Child writing the run settings (`SeedChild_WritesTheRunSettingsIntoThePairsLocalLayer`) | 0.25s | 0.27s |
| Four-row run, `loom` | 0.37s | 0.38s |
| Four-row run, `darn` | 0.42s | 0.39s |
| Mid-list resume skipping the completed create row | 0.39s | 0.30s |
| Run-Shed, step-driven (`StepDrivenRunShed_ReturnsOnTheChildsStateChange`) | 2.13s | 2.11s |
| Run-Shed, paused child then teardown (`RunShedPausedChild_WaitsThenTearsDownOnceDone`) | 0.38s | 0.31s |
| Whole `TestBattenIntegration_Rows` | 7.75s | 8.69s |

The whole-test figure grew because the test gained steps, among them the start-cost step below; the steps present before moved by run-to-run noise.

### Command rows beyond their commit and process

The new step `StartCost_CommandRowsCostUnderTheBoundBeyondTheCommitAndTheProcess` steps a Command row whose switch is off (Seed-IDE) and Open-Window with a command that only starts a background process.
A history entry's timestamp has one-second resolution, so a row's cost is the wall time of its one step; the status commit is timed by wrapping the wired `CommitStatus` seam, and the process by running the same command once through `runRowCommand`.

| Row | Step | Status commit | Process | Beyond both |
|---|---|---|---|---|
| Seed-IDE, switch off | 10.0 ms | 9.3 ms | none | 0.7 ms |
| Open-Window, `sleep 0 &` | 13.4 ms | 8.6 ms | 3.9 ms | 0.9 ms |

Both rows sit far under the 100 ms bound the step asserts.
The reads in batten's per-transition path (the seam's marker, the config snapshot loads, the producer wrapping) cost about a millisecond together, so this card cuts nothing there.

### Not measured live

No live batten start was taken: a live start creates a pair through a mutating `lyx fabric` verb, which the implementing strand's sandbox rule refuses.
The orch adds the live after-numbers for Open-Window and Seed-IDE on the first batten start after landing, which is the decision record's open risk.
The drive's local bare origins also carry no network push, and the push is spawned detached after the card 10 change, so the start no longer waits on it.
The inside-a-row breakdown from a `lyx batten run -vv` trace is therefore still open, and any cost it finds outside batten's seam, config reads and producer wrapping lies in fabric (`fabric add` and its origin probes) or the status commit itself, not in this card's members.
