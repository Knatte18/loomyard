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

### 2026-06-21 — after `optimize-test-suite`

The git-spawning tests in `internal/worktree`, `internal/weft`, and `internal/paths`
were migrated onto shared `lyxtest` fixtures, gated behind a build tag, and
parallelised. This splits the suite into **two tiers**:

```sh
# Tier 1 — default / offline loop. No build tag.
# Runs only the pure-unit + static-guard tests; spawns zero git subprocesses.
go test ./...

# Tier 2 — gated integration loop. Opt-in via the build tag.
# Runs the full git-spawning suite (real worktrees, commits, pushes, junctions).
go test -tags integration ./...
```

The headline guarantee is that **Tier 1 is offline**: `go test ./...` no longer
spawns `git` from the three migrated packages, so the default developer loop and CI
unit stage are fast and hermetic. The git round-trips moved into Tier 2, which you
run on demand (and in CI's integration stage).

#### Before / after wall-clock (uncached, `-count=1`)

| Package              | Tier 1 (untagged) before | Tier 1 (untagged) after | Tier 2 (integration) after |
|----------------------|--------------------------|-------------------------|----------------------------|
| `internal/worktree`  | 53.6 s                   | **1.06 s**              | 30.6 s                     |
| `internal/paths`     | 19.8 s                   | **0.17 s**              | 4.05 s                     |
| `internal/weft`      | not separately listed¹   | **0.22 s**              | 21.5 s                     |

¹ The 2026-06-15 block did not record `internal/weft` as its own row, so there is no
cited "before" untagged number; its pre-migration suite was untagged and git-spawning.

- **Full offline loop** (`go test ./... -count=1`): **~27.6 s**, down from **~82 s**.
  `internal/worktree` fell from the ~54 s floor to ~1.5 s in the default loop, so the
  floor is now `internal/board` (~24 s) and `internal/ide` (~12 s) — both unmigrated
  and out of scope for this task.
- Tier 1 for the three migrated packages totals **~1.5 s** — comfortably under the
  `< ~5 s` default-loop target. Tier 2 for the three packages runs the packages in
  parallel, so its wall-clock is bounded by the slowest (`worktree`, ~31 s), under the
  `< ~45 s` integration target. Per the discussion the `< ~5 s` figure is a target
  confirmed against the measured untagged baseline, not a hard precondition.

#### Equivalence guardrail

The post-migration test-name set is a **superset** of the pre-migration set for all
three packages (verified by diffing `-list` + `=== RUN` baselines): `worktree`
24 top-level / 58 subtests, `paths` 12 / 44, `weft` preserved with no net loss.
Intentional table-driven folds (same assertions, fewer top-level funcs):

- `weft`: `Commit_*` → `TestCommit`, `Push_*` → `TestPush`, `Status_*`/`Status_Junction*`
  → `TestStatus` + `TestStatus_Junction` (the `mklink`/`SKIP_MKLINK_TEST` junction case
  and the `scopedPathspec` assertion are kept as standalone funcs).
- `worktree`: `TestAdd` precondition subtests and `TestRemove` dirty-gate variants are
  table-driven over one shared `CopyPaired` base.

No assertion or named (sub)test was dropped.

#### `-race`

`go test -race -tags integration ...` (the second half of this batch's `verify`) was
**not run in this environment**: the race detector requires CGO, and no C compiler is
available on this machine. It must be run in a CGO-capable environment (CI) to close
the parallel-safety gate. By construction each parallel test takes an isolated
per-test `lyxtest` copy (`CopyHostHub`/`CopyPaired`/`CopyWeft` each build a fresh
temp-dir tree), so cross-test shared state is structurally avoided.

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
3. **Stay in the offline tier.** As of `optimize-test-suite`, `internal/worktree`,
   `internal/weft`, and `internal/paths` no longer spawn `git` in the default
   `go test ./...` loop — their git suites are behind `-tags integration`. The
   default-loop floor is now `internal/board` (~24 s) and `internal/ide` (~12 s),
   not `worktree`; applying the same shared-fixture + build-tag split to `board`
   (it spawns real commit/push cycles) is the next highest-leverage move. Only
   reach for `-tags integration` when you are changing worktree/weft/paths git
   behaviour — and budget ~31 s for that tier.
