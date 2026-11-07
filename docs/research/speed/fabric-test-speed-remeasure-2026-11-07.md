# Fabric test speed re-measure, 2026-11-07

One whole tier 1 pass and one `internal/fabricengine` pass after the fabric test speed tasks landed, read against W1 of `tier1-test-revision-2026-11-06/report.md`.

## Answer in brief

- The whole pass took 65.4 s against W1's 75.0 s; the expected 56 s was not reached.
- `internal/fabricengine` took 47.7 s inside the whole pass and 41.5 s alone, against W1's 57.0 s and the expected 36 s.
  It is still above the 35 s at which the other packages set the wall.
- The hub's tier commands do not carry `{test_targets}`: tier 1's command is `go build ./... && go vet {targets} && go test -json {targets} {args}`.
  The 48 s expectation therefore does not apply to this run.
- Neve was not quiet during either pass, so both figures read high against W1's quiet-machine numbers by an unmeasured amount.

## Method

Both commands ran once, in the worktree of this task, on a clean tree at `0c52eae1`.

| Pass | Command | Expected |
|---|---|---|
| Whole | `lym gate test --tier 1 --all -- -count=1` | about 56 s from these tasks alone, about 48 s with `{test_targets}` |
| Package | `lym gate test --tier 1 ./internal/fabricengine -- -count=1` | about 36 s |

The package command as written ran nothing:
the gate's green record carried the package forward from the whole pass over the unchanged tree and printed `discovered 0`, `wall 0.0s`.
The record carries a repeat only on an exact match of the args after `--`,
so the measured package pass ran `lym gate test --tier 1 ./internal/fabricengine -- -count=1 -shuffle=off`.
`-shuffle=off` is go test's default order and changes no test.

## Load

`lym beam status` refuses in a task worktree, so the load comes from `/proc/loadavg` and `ps`, and the runs in flight from `lym loom peers`.

| Before | Load average (1, 5, 15 min) | Other work |
|---|---|---|
| Whole pass | 3.34, 5.21, 7.40 | no peer run in `lym loom peers`; two interactive `claude` sessions started seconds earlier, one other gate process |
| Package pass | 6.36, 5.99, 7.49 | the same sessions; the whole pass's own tail was still in the 1-minute average |

W1 started at a one-minute load of 1.8, so these runs started under a higher load than W1 did and were not idle.

## Results

| Figure | W1 | Expected | This run |
|---|---|---|---|
| Whole pass wall, including `go build ./...` and `go vet` | 75.0 s | about 56 s | 65.4 s |
| `internal/fabricengine` inside the whole pass | 57.0 s | about 36 s | 47.7 s |
| `internal/fabricengine` pass wall, alone | not measured | | 47.1 s |
| `internal/fabricengine` package time, alone | not measured | about 36 s | 41.5 s |

The other packages of the whole pass were `internal/fabriccli` at 18.0 s, `internal/gatecli` at 13.9 s and `internal/webstercli` at 11.2 s.
`internal/fabricengine` is still the pass's last package and its wall, now 73 percent of the pass.

## Reading

- The tasks cut the package by 9.3 s inside the whole pass and the whole pass by 9.6 s,
  against expected cuts of 21 s and 19 s.
  The cut is real but about half of the expected one on a loaded machine.
- Alone, the package took 6.2 s less than inside the whole pass, the share the other packages' contention cost it.
  Neither figure is below 35 s.
- Whether a quiet machine closes the gap to 36 s is not answered:
  a re-run with no other session active is the one measurement that would separate load from a shortfall.
- Switching the hub's tier commands to `{test_targets}` stays open and is the expected 8 s of the 48 s figure.
