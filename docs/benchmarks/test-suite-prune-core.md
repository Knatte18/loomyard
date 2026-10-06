# Test suite prune, six packages

Measurement report for the `test-suite-prune-core` task: the before and after state of the test suites of `internal/fabricengine`, `internal/websterengine`, `internal/reedengine`, `internal/shuttleengine`, `internal/loomcli` and `internal/shedadapters`.
Per-test redundancy is in [test-redundancy.md](test-redundancy.md); how the harness runs is in [running-tests.md](running-tests.md).

## What was measured

Per package, on one Linux machine (12 logical CPUs):

- Top-level test count, wall time and serial time from `go run ./cmd/testtiming -tags integration,tmux`, run three times, reported as the median of each figure.
  Wall is the package's elapsed time in the `go test ./...` run, so it includes contention with the other packages running at the same time; serial is the sum of its tests' own elapsed times.
- Statement coverage of the package's own code from `go test -tags integration,tmux -cover -count=1 ./internal/<pkg>`, one run.
- The load average (`cat /proc/loadavg`) at the start of each of the three timing runs and of the coverage run.

Measured commit: `6e43ff13df64a8288ccb92c9a1618a30c578cec0`, after the `cmd/testtiming` changes of cards 1–4 and before any test of the six packages changed.
The per-package coverage profiles were kept uncommitted for naming lost blocks after the prune; they are reproducible from the measured commit with `go test -tags integration,tmux -count=1 -coverprofile=<file> ./internal/<pkg>`.

## Before

| Package | Tests | Wall (s) | Serial (s) | Coverage |
|---|---|---|---|---|
| `internal/fabricengine` | 698 | 43.59 | 115.63 | 81.9% |
| `internal/websterengine` | 456 | 10.66 | 12.22 | 86.2% |
| `internal/reedengine` | 330 | 24.73 | 24.49 | 82.4% |
| `internal/shuttleengine` | 218 | 0.47 | 0.34 | 90.6% |
| `internal/loomcli` | 278 | 89.00 | 89.36 | 63.4% |
| `internal/shedadapters` | 261 | 1.19 | 1.08 | 91.5% |

Wall and serial times vary with machine load: the three timing runs started at rising load, and run 1 was the quietest.
The per-run wall times, in seconds:

| Package | Run 1 | Run 2 | Run 3 |
|---|---|---|---|
| `internal/fabricengine` | 30.23 | 47.41 | 43.59 |
| `internal/websterengine` | 5.99 | 10.66 | 12.04 |
| `internal/reedengine` | 19.24 | 24.73 | 25.51 |
| `internal/shuttleengine` | 0.45 | 0.48 | 0.47 |
| `internal/loomcli` | 82.21 | 90.82 | 89.00 |
| `internal/shedadapters` | 0.59 | 1.36 | 1.19 |

### Load average at the start of each run

| Run | 1 min | 5 min | 15 min |
|---|---|---|---|
| Timing 1 | 2.58 | 2.88 | 2.56 |
| Timing 2 | 7.25 | 6.20 | 4.36 |
| Timing 3 | 11.60 | 9.63 | 6.46 |
| Coverage | 5.99 | 6.25 | 4.14 |

## After

The final card (`measure-after`) fills this section with the same table and loads, measured the same way, and names each statement block the prune lost.
