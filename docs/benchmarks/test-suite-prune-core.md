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

Measured commit: `b536cd6579e2ac32b985cb2a9fb3f440d6b41389`, after cards 1–16, on the same machine with the same commands as the before section.
The `.scratch/after/` directory kept the raw timing and coverage output uncommitted.

| Package | Tests | Wall (s) | Serial (s) | Coverage |
|---|---|---|---|---|
| `internal/fabricengine` | 610 | 29.27 | 108.25 | 81.9% |
| `internal/websterengine` | 175 | 2.48 | 6.25 | 86.3% |
| `internal/reedengine` | 212 | 18.96 | 18.84 | 82.4% |
| `internal/shuttleengine` | 129 | 0.43 | 0.33 | 90.6% |
| `internal/loomcli` | 167 | 77.30 | 78.82 | 63.4% |
| `internal/shedadapters` | 151 | 0.86 | 0.78 | 91.6% |

### Delta against before

| Package | Tests | Wall (s) | Serial (s) | Coverage |
|---|---|---|---|---|
| `internal/fabricengine` | -88 | -14.32 | -7.38 | 0.0 |
| `internal/websterengine` | -281 | -8.18 | -5.97 | +0.1 |
| `internal/reedengine` | -118 | -5.77 | -5.65 | 0.0 |
| `internal/shuttleengine` | -89 | -0.04 | -0.01 | 0.0 |
| `internal/loomcli` | -111 | -11.70 | -10.54 | 0.0 |
| `internal/shedadapters` | -110 | -0.33 | -0.30 | +0.1 |

No package's wall time rose, so no scenario merge needs naming against a slowdown.

The per-run wall times, in seconds:

| Package | Run 1 | Run 2 | Run 3 |
|---|---|---|---|
| `internal/fabricengine` | 29.24 | 30.13 | 29.27 |
| `internal/websterengine` | 2.35 | 2.48 | 2.70 |
| `internal/reedengine` | 18.50 | 19.68 | 18.96 |
| `internal/shuttleengine` | 0.43 | 0.49 | 0.42 |
| `internal/loomcli` | 77.19 | 77.67 | 77.30 |
| `internal/shedadapters` | 0.95 | 0.86 | 0.70 |

Timing run 1 reported `internal/reedengine` as FAIL under full-suite contention; its test name was not captured, and the package passed in timing runs 2 and 3, in `-count=3` alone and alongside the heaviest tmux packages.
The failure did not reproduce, so it is recorded here and not diagnosed.

### Load average at the start of each run

| Run | 1 min | 5 min | 15 min |
|---|---|---|---|
| Timing 1 | 1.78 | 3.90 | 3.83 |
| Timing 2 | 3.45 | 3.58 | 2.99 |
| Timing 3 | 2.18 | 3.85 | 3.36 |
| Coverage | 1.77 | 3.88 | 3.62 |

The after runs started at lower and steadier load than the before runs, which started at 2.58, 7.25 and 11.60 on the 1-minute average, so part of each wall-time delta is quieter load rather than the prune; serial time carries less of that.

### Lost coverage

No statement block covered before is uncovered after.
The coverage percentages are equal or higher for every package, and for the five packages other than `internal/websterengine`, matching blocks by position finds no block covered in the before profile and uncovered in the after profile.
For `internal/websterengine`, production code changed since the measured commit, so block positions shifted; matching blocks by source text instead of position finds none covered before and uncovered after, and 52 blocks of the before profile that no longer exist in the production code.
