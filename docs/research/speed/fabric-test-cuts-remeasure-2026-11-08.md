# Fabricengine test cuts re-measure, 2026-11-08

The `internal/fabricengine` tier 1 time with and without the two test cuts `f2ac2bf7` (folds, tier 0 moves) and `cbdfc33d`
(tests from the coverage study's removable set), measured on one machine in one worktree.

## Answer in brief

- The cuts save about 4.3 s of the package's own time (37.0 s against 41.3 s, median of two runs per state).
  The gate's wall for the package alone falls by about 4.2 s (42.7 s against 46.9 s).
- One whole tier 1 pass falls from 61.0 s to 56.5 s, 4.5 s, and the package inside it from 44.7 s to 40.8 s, 3.9 s.
  One pass per state is a single sample.
- The coverage study estimated the dropped tests at up to 78 s standalone.
  The measured saving is about 5 percent of that.
  The tests run in parallel inside the package, so their standalone costs do not add up to wall time.
- The package is still the pass's last package and its wall, at 72 percent of the pass.

## Method

The "after" state is the worktree as branched from `main` at `cbdfc33d`.
The "before" state is the same tree after `git revert --no-commit cbdfc33d f2ac2bf7`, restored with `git reset --hard`.
Each state ran the package twice, alternating after, before, after, before, then one whole pass each.
No code or test changed; the worktree ends clean.

The gate carries a package forward from a green record when the tree and the args after `--` match an earlier run,
and then prints `discovered 0`, `wall 0.0s`.
Every run therefore used args no earlier run of its state had used:

| Run | Command |
|---|---|
| Package, runs 1 and 2 | `lym gate test --tier 1 ./internal/fabricengine -- -count=1` |
| Package, runs 3 and 4 | `lym gate test --tier 1 ./internal/fabricengine -- -count=1 -shuffle=off` |
| Whole, before | `lym gate test --tier 1 --all -- -count=1` |
| Whole, after | `lym gate test --tier 1 --all -- -count=1 -timeout=20m` |

`-shuffle=off` is go test's default order and `-timeout=20m` is the value tier 1's `GOFLAGS` already sets, so neither changes a test.
`--all` is needed for the whole pass: without it the scope is the diff from the merge base, which is empty in the after state.

The first whole pass in the after state ran with `-count=1` and discovered 4753 tests in 44.0 s.
It carried `internal/fabricengine` from the package runs and so measured nothing for it.
It is left out of the results, and the pass was repeated with the args above.

## Runs

The package time is the `ok` line's time for `internal/fabricengine` in the run's go test output.
The gate wall includes `go build ./...` and `go vet` for the package.
Load is the one-minute average from `/proc/loadavg` at the start of the run.

| # | State | Scope | Start load | Gate wall | Package time | Tests discovered |
|---|---|---|---|---|---|---|
| 1 | after | package | 0.76 | 42.1 s | 36.5 s | 1549 |
| 2 | before | package | 3.65 | 47.8 s | 42.2 s | 1897 |
| 3 | after | package | 1.94 | 43.2 s | 37.5 s | 1549 |
| 4 | before | package | 1.86 | 45.9 s | 40.4 s | 1897 |
| 5 | before | whole | 1.85 | 61.0 s | 44.7 s | 6650 |
| 6 | after | whole | 1.99 | 56.5 s | 40.8 s | 6302 |

Run 2 started at a higher load than the others, from the revert command and the tail of the previous gate process, and ran slowest.
Runs 3 to 6 waited for a one-minute load under 2.0 before starting.

## Saving

| Figure | Before | After | Saving |
|---|---|---|---|
| Package gate wall, median of runs 1 to 4 per state | 46.9 s | 42.7 s | 4.2 s |
| Package time, median of runs 1 to 4 per state | 41.3 s | 37.0 s | 4.3 s |
| Package time in the whole pass | 44.7 s | 40.8 s | 3.9 s |
| Whole pass wall | 61.0 s | 56.5 s | 4.5 s |

Within a state the two package runs differ by 1.1 s (after) and 1.8 s (before), so the saving is about two to four times the spread.
The 348 fewer discovered tests are the cuts' effect on the count.

## Suspend check

`journalctl` for 19:47 to 20:00 local time, which covers runs 1 to 6 bar the last minute, holds no suspend, resume or sleep entry once the `memsample` timer lines are set aside.
No run was affected by a suspend window.

## Reading

- The cuts took out about 18 percent of the package's tests and about 10 percent of its time.
- The 78 s estimate counted each dropped test's standalone time.
  The package's tests share a wall of about 40 s, so removing tests that ran beside others returns far less than their sum.
- The pass stays above 56 s, and the package is still the longest, so the cuts alone do not move the pass off the package.
