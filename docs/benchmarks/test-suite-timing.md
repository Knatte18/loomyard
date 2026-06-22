# Test-suite timing

Recorded per-package and per-test wall-clock for the whole repo, so a slow suite
is visible on its own rather than hidden in one combined number. This file holds
the **numbers**; for how to run the suite, the two tiers, and the timing harness
that produces these tables, see [running-tests.md](running-tests.md). For the
board command hot path specifically, see [board-performance.md](board-performance.md).

To reproduce the current numbers: `go run ./cmd/testtiming` (fast) or
`go run ./cmd/testtiming -full` (integration).

## Reading the tables

There are two tiers, and they are **different test sets, not the same tests run
twice** (full explanation in [running-tests.md](running-tests.md#the-two-tiers)):

- **Tier 1** — the default offline loop (`go test ./...`): fast, no git.
- **Tier 2** — the opt-in integration loop (`go test -tags integration ./...`):
  Tier 1 plus the real-git tests; slow by design.

**Compare _down_ a column** (is this package fast in the loop I run?), **never
_across_** — Tier 2 is the superset, so its larger numbers are expected, not a
regression.

Numbers are wall-clock on Windows and **noisy** (Windows file I/O + Defender +
process-creation tax — see [board-performance.md](board-performance.md#process-startup-context));
treat them as order-of-magnitude.

## Current best times

As of **2026-06-22** (after `optimize-remaining-test-suites`).

- Machine: Intel Core Ultra 7 155U, `windows/amd64`, 14 logical CPUs
- Go 1.26.4, default GC, `GOMAXPROCS` = NumCPU (14)

### Headline

| Loop | Command | Wall-clock |
|------|---------|-----------|
| **Tier 1** — offline, default | `go test ./... -count=1` | **~3.5 s** |
| **Tier 2** — integration, opt-in | `go test -tags integration ./... -count=1` | **~42 s** |

Tier 1 is offline repo-wide: zero git subprocesses. Tier 2's wall-clock is bounded
by its single slowest package (`internal/board/boardtest`, ~42 s), since `go test`
runs packages in parallel.

### Per package (uncached, `-count=1`)

Each column is a separate run. The **Tier 2 cost** column says where that package's
integration time actually goes.

| Package | Tier 1 (offline) | Tier 2 (integration) | Where the Tier 2 cost is |
|---------|------------------|----------------------|--------------------------|
| `internal/board/boardtest` | 2.0 s          | **41.8 s** | real git commit/push, incl. one real GitHub push |
| `internal/worktree`        | 0.7 s          | **30.6 s** | real `git worktree` add/remove, junctions        |
| `internal/weft`            | 0.8 s          | **19.7 s** | real git sync/status round-trips                 |
| `internal/ide`             | 0.6 s          | 13.9 s     | spawns the binary, drives the TUI                |
| `internal/lyxtest`         | no test files¹ | 5.8 s      | builds the shared git fixture templates          |
| `internal/paths`           | 0.6 s          | 4.9 s      | mirrored-path filesystem geometry                |
| `internal/git`             | no test files¹ | 1.4 s      | gated git-wrapper tests                          |
| `internal/board`           | 0.9 s          | 1.2 s      | heavy tests relocated to `boardtest`             |
| `cmd/lyx`                  | 1.0 s          | 1.3 s      | —                                                |
| `internal/muxpoc`          | 1.6 s          | 1.5 s      | —                                                |
| `config`, `fsx`, `gitignore`, `lock`, `output`, `state` | < 1.2 s each | < 1.2 s each | pure unit, no git |

¹ No untagged test files — every test in the package needs `-tags integration`, so
the package is absent from the default `-list` and contributes nothing to Tier 1.

**Why `boardtest` is ~42 s in Tier 2:** that column is the relocated real-git
suite. It is supposed to be heavy and only runs when you opt in. In Tier 1 the same
package is 2.0 s because its git tests are gated out.

## History (trend log)

Append-only: each block is the state **at that revision** and is frozen, so the
trend stays visible. Newest first. The "Current best times" section above always
reflects the latest block.

### 2026-06-22 — after `optimize-remaining-test-suites`

The git-spawning tests in `internal/board` (`git_test.go`, `sync_test.go`) and
`internal/ide` (`cli_test.go`, `menu_test.go`) were gated behind the `integration`
build tag and relocated into `internal/board/boardtest`. The `render_test.go` and
`store_test.go` top-level functions were folded into table-driven tests. Seven
git/sync tests moved from `internal/board` (Tier 1) into `internal/board/boardtest`
(Tier 2). This completes the two-tier split across the whole repo.

#### Before / after wall-clock (uncached, `-count=1`)

| Package                    | Tier 1 before | Tier 1 after | Tier 2 after |
|----------------------------|---------------|--------------|--------------|
| `internal/board`           | ~24 s         | **0.7 s**    | ~1.2 s       |
| `internal/board/boardtest` | ~3.9 s        | **2.0 s**    | ~41.8 s      |
| `internal/ide`             | ~12 s         | **0.6 s**    | ~13.9 s      |
| `internal/git`             | —             | no tests¹    | ~1.4 s       |

¹ `internal/git` has no untagged test files — all its tests require `-tags integration`.

- **Full offline loop** (`go test ./... -count=1`): **~3.5 s**, down from **~27.6 s**
  (itself down from ~82 s after the prior task). The floor is now the build/link
  overhead across packages; no single package dominates.
- **Tier 1 is now offline repo-wide**: `go test ./...` spawns zero git subprocesses.
  The board git/sync tests and the `internal/git` tests are absent from the default
  `-list` and only appear under `-tags integration`.
- **Tier 2 full wall-clock**: ~42 s, bounded by `internal/board/boardtest`.

#### Equivalence guardrail

The post-change test-name set is a **superset** of the pre-change set, verified by
diffing `-list` + `=== RUN` baselines. The seven git/sync tests relocated from
`internal/board` (Tier 1) into `internal/board/boardtest` (Tier 2) are present in
the union of both packages under `-tags integration`:

- `TestCommitPush` (3 subtests), `TestIntegrationCommitPush`, `TestIntegrationPull`,
  `TestPull`, `TestSyncCommitsAndPushes`, `TestSyncCoalescesBurstIntoOneCommit`,
  `TestSyncSkipPushCommitsLocallyOnly`, `TestSyncCleanTreeIsNoOp`,
  `TestSyncIgnoresLockfiles` — all present in `board/boardtest` under integration.

Table-driven folds in `internal/board` (`render_test.go`, `store_test.go`): assertions
are preserved; no named (sub)test was dropped. The superset check is computed against
the **union across `internal/board` (untagged) + `internal/board/boardtest`
(integration)**.

#### Parallel safety

The moved board tests (`git_test.go`, `sync_test.go`) use `t.Setenv` (`BOARD_SKIP_GIT`,
`BOARD_SKIP_PUSH`) and remain serial — Go forbids `t.Parallel()` after `t.Setenv`.
The `internal/ide` test `cli_test.go` uses `os.Chdir` (a process-global seam) and
remains serial; `menu_test.go` uses `t.Setenv("BOARD_SKIP_GIT", "1")` in every
test function and likewise remains serial. The `lyxtest` per-test fixture copies
(`CopyHostHub`, `CopyWeft`, `CopyPaired`; `CopyBoardRepo` was evaluated and not
needed — all sync tests use `CopyWeft` directly) are isolated per-test filesystem
trees with no shared mutable state, so any test that does not use `t.Setenv` /
`os.Chdir` may safely call `t.Parallel()`. The `-race` detector is not a
precondition (no CGO in this environment); it may be run opportunistically in a
CGO-capable CI.

### 2026-06-21 — after `optimize-test-suite`

The git-spawning tests in `internal/worktree`, `internal/weft`, and `internal/paths`
were migrated onto shared `lyxtest` fixtures, gated behind a build tag, and
parallelised. This introduced the two-tier split (later completed for board/ide on
2026-06-22).

#### Before / after wall-clock (uncached, `-count=1`)

| Package              | Tier 1 before          | Tier 1 after | Tier 2 after |
|----------------------|------------------------|--------------|--------------|
| `internal/worktree`  | 53.6 s                 | **1.06 s**   | 30.6 s       |
| `internal/paths`     | 19.8 s                 | **0.17 s**   | 4.05 s       |
| `internal/weft`      | not separately listed¹ | **0.22 s**   | 21.5 s       |

¹ The 2026-06-15 block did not record `internal/weft` as its own row, so there is no
cited "before" untagged number; its pre-migration suite was untagged and git-spawning.

- **Full offline loop** (`go test ./... -count=1`): **~27.6 s**, down from **~82 s**.
  `internal/worktree` fell from the ~54 s floor to ~1.5 s in the default loop, so at
  this revision the floor was `internal/board` (~24 s) and `internal/ide` (~12 s) —
  both unmigrated and out of scope for that task (addressed on 2026-06-22).
- Tier 1 for the three migrated packages totalled **~1.5 s**. Tier 2 for the three
  packages ran in parallel, bounded by the slowest (`worktree`, ~31 s).

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

### 2026-06-15 — after `paths-subpath-mirroring`

Full suite, uncached (`go test ./... -count=1`): **~82 s wall-clock**. This predates
the two-tier split — every git-spawning test still ran in the default loop.

`go test` runs packages in parallel (up to `GOMAXPROCS`), so wall-clock (~82 s) is
well under the sum of per-package times (~148 s) — roughly a 1.8× overlap. The
single longest package therefore set the floor: `internal/worktree` at ~54 s
could not be hidden by parallelism.

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

| Test                                  | Suite     | Time    |
|---------------------------------------|-----------|---------|
| `TestAdd`                             | worktree  | 13.76 s |
| `TestCommitPush`                      | board     | 12.75 s |
| `TestRemove`                          | worktree  | 10.01 s |
| `TestMirroredMethods`                 | paths     |  7.47 s |
| `TestRunCLI`                          | worktree  |  7.20 s |
| `TestSyncIgnoresLockfiles`            | board     |  6.66 s |
| `TestWriteLaunchers`                  | worktree  |  5.61 s |
| `TestSyncCleanTreeIsNoOp`             | board     |  5.27 s |
| `TestSyncCommitsAndPushes`            | board     |  4.50 s |
| `TestPull`                            | board     |  4.46 s |
| `TestList`                            | paths     |  4.41 s |
| `TestAddRollback`                     | worktree  |  4.33 s |
| `TestSyncCoalescesBurstIntoOneCommit` | board     |  3.67 s |
| `TestMenuNumericSelection`            | ide       |  3.30 s |
| `TestConcurrentReadsDuringUpserts`    | boardtest |  3.18 s |

At this revision `worktree` and `board` dominated (~64 % of the sum): both spawn real
`git` and touch the filesystem heavily, and the ~30 ms process-creation tax per `git`
invocation compounds across the many calls each test makes. The two-tier migrations
(2026-06-21 and 2026-06-22) moved this cost into Tier 2.
