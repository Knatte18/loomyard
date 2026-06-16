# Test-suite timing

Per-suite and per-test wall-clock for the whole repo, so a slow suite is visible
on its own rather than hidden in one combined number. This complements
[board-performance.md](board-performance.md), which benchmarks the board command
hot path specifically; this file times the **`go test` suites** themselves.

## How to run

```sh
# Per-package (suite) timing — the headline numbers below.
go test ./... -count=1

# Per-test timing, structured (parse Elapsed from the JSON stream).
go test ./... -count=1 -json

# One package, verbose, with per-test seconds:
go test ./internal/worktree -count=1 -v
```

`-count=1` disables the test cache so every run is honest; without it, unchanged
packages report `(cached)` in ~0 s and the numbers lie.

## Context

Numbers are wall-clock on Windows and are **noisy** (Windows file I/O + Defender +
process-creation tax — see [board-performance.md](board-performance.md#process-startup-context)).
Treat them as order-of-magnitude. Record a new dated block per revision rather than
editing the old one, so the trend stays visible.

- Machine: Intel Core Ultra 7 155U, `windows/amd64`, 14 logical CPUs
- Go 1.26.4, default GC, `GOMAXPROCS` = NumCPU (14)
- Endpoint security active (≈30 ms process-creation tax per spawned process)

## Results

### 2026-06-15 — after `paths-subpath-mirroring`

Full suite, uncached (`go test ./... -count=1`): **~82 s wall-clock**.

`go test` runs packages in parallel (up to `GOMAXPROCS`), so wall-clock (~82 s) is
well under the sum of per-package times (~148 s) — roughly a 1.8× overlap. The
single longest package therefore sets the floor: `internal/worktree` at ~54 s
cannot be hidden by parallelism.

#### Per package (test suite)

| Suite (`internal/…` unless noted) | Time   |
|-----------------------------------|--------|
| `worktree`                        | 53.6 s |
| `board`                           | 41.4 s |
| `paths`                           | 19.8 s |
| `ide`                             | 19.2 s |
| `board/boardtest`                 |  3.9 s |
| `muxpoc`                          |  2.6 s |
| `git`                             |  1.8 s |
| `output`                          |  1.5 s |
| `cmd/lyx`                         |  1.3 s |
| `config`                          |  1.1 s |
| `lock`                            |  0.9 s |
| `gitignore`                       |  0.5 s |
| **Sum**                           | **147.5 s** |
| **Wall-clock (parallel)**         | **~82 s**   |

#### Slowest individual tests (top-level)

| Test                                          | Suite       | Time    |
|-----------------------------------------------|-------------|---------|
| `TestAdd`                                      | worktree    | 13.76 s |
| `TestCommitPush`                               | board       | 12.75 s |
| `TestRemove`                                   | worktree    | 10.01 s |
| `TestMirroredMethods`                          | paths       |  7.47 s |
| `TestRunCLI`                                    | worktree    |  7.20 s |
| `TestSyncIgnoresLockfiles`                     | board       |  6.66 s |
| `TestWriteLaunchers`                           | worktree    |  5.61 s |
| `TestSyncCleanTreeIsNoOp`                      | board       |  5.27 s |
| `TestSyncCommitsAndPushes`                     | board       |  4.50 s |
| `TestPull`                                      | board       |  4.46 s |
| `TestList`                                      | paths       |  4.41 s |
| `TestAddRollback`                              | worktree    |  4.33 s |
| `TestSyncCoalescesBurstIntoOneCommit`          | board       |  3.67 s |
| `TestMenuNumericSelection`                     | ide         |  3.30 s |
| `TestConcurrentReadsDuringUpserts`             | boardtest   |  3.18 s |
| `TestList`                                      | worktree    |  3.14 s |
| `TestMenuRequiresLyxDir`                       | ide         |  2.94 s |
| `TestSyncSkipPushCommitsLocallyOnly`           | board       |  2.73 s |
| `TestCreatePortalMultipleSubpaths`             | worktree    |  2.53 s |
| `TestCreatePortalRootRelPath`                  | worktree    |  2.39 s |

### Where the time goes

- **`worktree` and `board` dominate (~64 % of the sum).** Both spawn real `git`
  subprocesses and touch the filesystem heavily — `TestAdd`/`TestRemove` create and
  tear down worktrees, `board` sync tests run real commit/push cycles. The ~30 ms
  process-creation tax per `git` invocation compounds across the many git calls each
  test makes.
- **`paths` is now ~20 s.** `TestMirroredMethods` (7.5 s) and `TestList` (4.4 s)
  were added/extended by the `paths-subpath-mirroring` work; they exercise the new
  subpath-mirrored geometry across multiple subpath levels, each doing filesystem
  setup.
- **`ide` (~19 s)** is mostly menu/CLI tests that spawn the binary and drive a TUI.
- The remaining eight packages together cost < 14 s — they are not worth optimizing.

### Reducing wall-clock

If the suite feels slow locally, the highest-leverage levers, in order:

1. **Rely on the test cache** — drop `-count=1` for iterative runs; only changed
   packages re-run, so a no-op `go test ./...` returns in ~1 s.
2. **Scope to the package you're editing** — `go test ./internal/paths` is ~20 s
   vs ~82 s for everything.
3. **The floor is `internal/worktree` (~54 s).** Shaving the full-suite wall-clock
   below ~54 s requires speeding up that suite specifically (fewer real-git
   round-trips, or shared fixtures), not adding parallelism — the other packages
   already overlap with it.
