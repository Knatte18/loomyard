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

As of **2026-07-12** (fresh baseline — **regression recorded, not yet fixed**).

- Machine: Intel Core Ultra 7 155U, `windows/amd64`, 14 logical CPUs
- Go 1.26.4, default GC, `GOMAXPROCS` = NumCPU (14)
- Method: median of 3 warm runs per tier via `go run ./cmd/testtiming[ -full]`
  (`-count=1` set by the harness)

### Headline

| Loop | Command | Wall-clock | vs. 2026-06-23 |
|------|---------|-----------|----------------|
| **Tier 1** — offline, default | `go test ./... -count=1` | **~44 s** (spread 43–49 s) | was ~3.5 s — **~13× regression** |
| **Tier 2** — integration, opt-in | `go test -tags integration ./... -count=1` | **~181 s** (spread 173–238 s) | was ~65 s — **~2.8× regression**, plus **2 packages FAIL** |

The ~a-dozen modules landed since 2026-06-23 (mux/shuttle/burler/perch/warp/
stencil/selfreport/modelspec/…) each brought tests; the regression is real
execution time, not compilation.

### Two RED packages (Tier 2)

- `internal/initengine` — `TestInit_FirstRun`: `len(result.Modules) = 7; want 3`.
  Stale hardcoded assertion: `Init` reconciles one config per registered module
  (`configsync.ReconcileAll` iterates `configreg.Modules()`), and the registry has
  grown from 3 to 7 modules. Test maintenance, not a product bug.
- `internal/ideengine` — `TestMenuExcludesMain`, `TestMenuRequiresLyxDir`,
  `TestMenuNumericSelection`: `board health check failed: Stat : path not found`
  (empty path). **Real product bug in `lyx ide menu`**: the board-dir-geometry
  migration made `boardengine.Config.Path` caller-set (`yaml:"-"`), `boardcli`
  was updated (`cfg.Path = hubgeometry.BoardDir(layout.Hub)`), but
  `ideengine/menu.go` still calls `boardengine.New(cfg)` with `Path == ""`, so
  `HealthCheck()` stats an empty path. `menu.go` is the only missed
  `LoadConfig` caller.

### Tier 1: where the time goes

The offline tier's premise — zero git subprocesses repo-wide — no longer holds.
The cost is process spawns and fixture I/O from **untagged** tests:

| Package | Tier 1 elapsed | Cause |
|---------|---------------|-------|
| `internal/boardcli`   | ~38–40 s | 31 `seedCwd` calls, each a real `git init`; every in-process `RunCLI` spawns `git rev-parse` via `hubgeometry.Resolve` |
| `internal/perchcli`   | ~23–28 s | untagged `lyxtest.CopyPaired[Local]` git-fixture copies + `git ls-files` / `git log` assertions |
| `cmd/lyx`             | ~22–24 s | `TestCrossCompileLinux` (whole-module `GOOS=linux go build`, ~3–8 s) + `main_test.go` 3× `git init` |
| `internal/muxcli`     | ~16–18 s | untagged `CopyPaired` git-fixture copies |
| `internal/configcli`  | ~6–10 s  | 3× `git init` in untagged tests |

Everything else is < 13 s per package and mostly contention-inflated (see the
noise note below). The sibling CLI packages `idecli`, `initcli`, `weftcli`, and
`warpcli` already keep their git-touching tests behind `//go:build integration` —
the packages above simply did not follow that established pattern, and nothing
machine-enforces the tier premise.

**Attribution noise:** per-package and per-test elapsed numbers are inflated by
CPU contention (`go test` runs ~50 packages in parallel; the sum of package times
is ~300–450 s against a ~44 s wall-clock). Trust the wall-clock; treat per-test
numbers as attribution, not absolute cost. In-memory tests (e.g.
`internal/clihelp`'s `TestExecute_*`) showing 4–5 s each are pure
scheduling-delay artifacts.

### Tier 2: where the time goes

| Package | Tier 2 elapsed | Where the cost is |
|---------|---------------|-------------------|
| `internal/warpengine`          | **~127 s** | real `git worktree` add/remove, junctions, host↔weft topology (successor of `internal/worktree`, whose floor was ~61 s with far fewer tests) |
| `internal/boardcli`            | ~56 s  | the same untagged git-spawn cost as Tier 1, plus tag-gated tests |
| `internal/initengine`          | ~52 s  | paired-fixture copies per test (FAIL — see above) |
| `internal/perchcli`            | ~50 s  | fixture copies + weft-sync git assertions |
| `internal/boardengine/boardtest` | ~47 s | real local git commit/push, parallelized |
| `internal/perchengine`         | ~41 s  | run-loop state machinery over fixtures |
| `cmd/lyx`                      | ~36 s  | cross-compile + registration/help-tree over the full binary |

The floor is `internal/warpengine`: its slowest single tests run 25–36 s each
(`TestCleanup_LiveBranchNeverDeleted` 35.9 s, `TestRemove` 30.8 s,
`TestWeftForkPointSubtaskIsolation` 25.8 s) — fixture-copy I/O plus real git
under parallel filesystem contention, the same deterministic-I/O floor the
2026-06-23 block described for `worktree`, now ~2× because warpengine has ~2×
the test surface.

### Slowest 10 top-level tests (Tier 1, median run)

| Test | Package | Elapsed |
|------|---------|---------|
| `TestCLIStrictPayloadShapes` | `internal/boardcli` | 11.8 s |
| `TestCLILookupContract` | `internal/boardcli` | 11.4 s |
| `TestRunCLI_ResolvesLayoutAndConfig` | `internal/muxcli` | 5.6 s |
| `TestCLIContract` | `internal/boardcli` | 5.5 s |
| `TestExecute_ConcurrentInvocationsDoNotCrossExitCodes` | `internal/clihelp` | 5.4 s¹ |
| `TestConcurrentReadsDuringUpserts` | `internal/boardengine/boardtest` | 5.4 s |
| `TestRunCLI_Pause_InvalidRunID` | `internal/perchcli` | 5.2 s |
| `TestExecute_UnknownSubcommandReturnsOneAndWritesUnknownCommand` | `internal/clihelp` | 4.9 s¹ |
| `TestWrapRun_ShortCircuitsAfterAbort` | `internal/clihelp` | 4.8 s¹ |
| `TestCrossCompileLinux` | `cmd/lyx` | 3.2–8.1 s across runs |

¹ Pure in-memory cobra-tree tests — their elapsed is contention artifact, not
real cost (see the attribution-noise note above).

## History (trend log)

### 2026-06-23 — state after `optimize-integration-tier` (was "Current best times")

- Machine: Intel Core Ultra 7 155U, `windows/amd64`, 14 logical CPUs
- Go 1.26.4, default GC, `GOMAXPROCS` = NumCPU (14)

#### Headline

| Loop | Command | Wall-clock |
|------|---------|-----------|
| **Tier 1** — offline, default | `go test ./... -count=1` | **~3.5 s** |
| **Tier 2** — integration, opt-in | `go test -tags integration ./... -count=1` | **~65 s** |

Tier 1 is offline repo-wide: zero git subprocesses. Tier 2's wall-clock is bounded
by its single slowest package (`internal/worktree`, ~61 s), since `go test`
runs packages in parallel.

#### Per package (uncached, `-count=1`)

Each column is a separate run. The **Tier 2 cost** column says where that package's
integration time actually goes.

| Package | Tier 1 (offline) | Tier 2 (integration) | Where the Tier 2 cost is |
|---------|------------------|----------------------|--------------------------|
| `internal/worktree`        | 0.7 s          | **60.8 s** | real `git worktree` add/remove, junctions        |
| `internal/weft`            | 0.8 s          | **41.5 s** | real git sync/status round-trips                 |
| `internal/board/boardtest` | 2.0 s          | **31.2 s** | real git commit/push (local only, parallelized)  |
| `internal/ide`             | 0.6 s          | 25.8 s     | spawns the binary, drives the TUI                |
| `internal/lyxtest`         | no test files¹ | 11.1 s     | builds the shared git fixture templates          |
| `internal/hubgeometry`     | 0.6 s          | 8.2 s      | mirrored-path filesystem geometry                |
| `internal/muxpoc`          | 1.6 s          | 3.0 s      | —                                                |
| `internal/git`             | no test files¹ | 2.0 s      | gated git-wrapper tests                          |
| `cmd/lyx`                  | 1.0 s          | 2.3 s      | —                                                |
| `internal/vscode`          | no test files¹ | 3.2 s      | vscode configuration generation                  |
| `internal/board`           | 0.9 s          | 1.3 s      | heavy tests relocated to `boardtest`             |
| `configengine`, `fsx`, `gitignore`, `fslink`, `lock`, `output`, `state` | < 1.2 s each | < 1.2 s each | pure unit, no git |

¹ No untagged test files — every test in the package needs `-tags integration`, so
the package is absent from the default `-list` and contributes nothing to Tier 1.

**Why `worktree` is the Tier 2 floor:** `worktree` now dominates because boardtest
is parallelized (local git tests no longer run serially) and runs at ~31 s, while
worktree's fixture I/O (copying four repos per test under parallel contention) is
filesystem-bound at ~61 s. The network-test removal (Decision A) eliminated the
real-GitHub-push noise source; the floor is now deterministic local I/O.

Append-only: each block is the state **at that revision** and is frozen, so the
trend stays visible. Newest first. The "Current best times" section above always
reflects the latest block.

### 2026-06-23 — after `optimize-integration-tier`

Removed the two real-GitHub network tests (`TestIntegrationCommitPush`,
`TestIntegrationPull`; network noise source), parallelized boardtest's local git
tests (explicit skip-flags replace `BOARD_SKIP_*` env seams), and trimmed the
unused weft-bare repo from worktree tests' fixture copies (filesystem I/O reduction).
The Tier 2 wall-clock increased (net: ~65 s vs. prior ~42 s) because worktree fixture
I/O now dominates after boardtest parallelization; the floor shift is deterministic.
All prior Tier 1 (~3.5 s) overhead is preserved.

#### Wall-clock (median of 4 warm runs, uncached, `-count=1`)

| Tier | Before | After | Change |
|------|--------|-------|--------|
| **Tier 1** (offline) | ~3.5 s | ~3.5 s | **unchanged** |
| **Tier 2** (integration) | ~42 s | **~65 s** | +23 s (floor shift: board was floor; worktree now floor due to fixture I/O under parallel contention) |

#### Per-package Tier 2 times (median of 4 runs)

| Package | Before | After | Change | Note |
|---------|--------|-------|--------|------|
| `internal/worktree` | ~30.6 s | **60.8 s** | +30.2 s | Lean fixture saves ~25% per test, but parallel contention on filesystem still bounds wall-clock; floor now dominates |
| `internal/weft` | ~19.7 s | **41.5 s** | +21.8 s | Reflects fixture trim and parallel contention; `TestWeftSpawnPushesWeftBranch` now exercises weft-bare with full fixture |
| `internal/board/boardtest` | **~41.8 s** | **31.2 s** | **−10.6 s** | **Parallelized** local git tests; no more `BOARD_SKIP_*` env seam forcing serial; now runs 26 s of git logic in parallel (was serial) |
| `internal/ide` | 13.9 s | **25.8 s** | +11.9 s | Fixture overhead shared across longer worktree runs |
| `internal/lyxtest` | 5.8 s | **11.1 s** | +5.3 s | Template-build cost unchanged; fixture copies now overlap with longer tests |
| `internal/hubgeometry` | 4.9 s | **8.2 s** | +3.3 s | Fixture overhead in parallel contention |
| `internal/muxpoc` | 1.5 s | 3.0 s | +1.5 s | Minor shift |
| Other packages | < 2 s each | < 2 s each | **unchanged** | No git integration tests |

**Floor shift explanation:** The prior ~42 s floor was boardtest running serially (git tests
forced serial by `t.Setenv` + `BOARD_SKIP_*` env leakage). With boardtest parallelized
(~31 s median, 26 s of local git logic now in parallel), the new floor is worktree's
fixture-I/O burden (~61 s wall-clock, limited by parallel filesystem contention, not git
spawn count). The real-GitHub network test removal (network round-trip ~1 s per run, noisy)
eliminated the noise source documented in the prior block; the new ~65 s is deterministic.

#### Test-name equivalence guardrail

The post-change test-name set is a **justified subset** of the pre-change set:

**Removed (documented coverage mapping):**
- `TestIntegrationCommitPush` → covered by `git_test.go:TestCommitPush` (local git)
- `TestIntegrationPull` → covered by `git_test.go:TestPull` (local git)

Rationale: The two network tests cloned a real GitHub repo and added network latency
(~1 s, noisy) without unique coverage — local bare-repo tests already exercise the
same git operations. Removal eliminates the noise source and simplifies the suite to
100% local, deterministic git.

**Added:**
- `TestWeftSpawnPushesWeftBranch` (new test closing a pre-existing gap: verifies weft
  branch lands on weft-bare under the full `CopyPaired` fixture with weft push enabled)

**All other names preserved:** Tests modified by the parallelization and fixture
changes (boardtest, worktree) kept their original names; only `t.Setenv` calls and
fixture builders were swapped. Verified via `go test -tags integration ./... -list
'.*'` — no test name vanished except the two documented network tests.

#### Slowest 15 top-level tests (median run)

| Test | Package | Median |
|------|---------|--------|
| `TestRemoveSubpathJunction` | `internal/worktree` | 17.6 s |
| `TestWeftSpawnPushesWeftBranch` | `internal/worktree` | 16.8 s |
| `TestRemoveHostJunctionRemoved` | `internal/worktree` | 15.5 s |
| `TestAddRollback` | `internal/worktree` | 14.4 s |
| `TestWeftSpawnSeedsExclude` | `internal/worktree` | 14.2 s |
| `TestConcurrentReadsDuringUpserts` | `internal/board/boardtest` | 13.6 s |
| `TestWeftSpawnPairedWorktrees` | `internal/worktree` | 13.0 s |
| `TestWeftSpawnCreatesJunction` | `internal/worktree` | 12.4 s |
| `TestWeftRollbackOnPostHostCreateFailure` | `internal/worktree` | 12.0 s |
| `TestSyncCleanTreeIsNoOp` | `internal/board/boardtest` | 11.9 s |
| `TestSyncCommitsAndPushes` | `internal/board/boardtest` | 11.7 s |
| `TestSyncCoalescesBurstIntoOneCommit` | `internal/board/boardtest` | 11.6 s |
| `TestSyncIgnoresLockfiles` | `internal/board/boardtest` | 11.4 s |
| `TestPull_FastForward` | `internal/weft` | 9.4 s |
| `TestCopyPaired` | `internal/lyxtest` | 8.8 s |

### 2026-06-22 — after `prune-board-tests`

Pruned test-suite **function count** by folding clusters of single-shape tests into
table-driven tests and dropping redundant assertions per layer-ownership rules
(unit tests own business logic; facade tests own persistence wiring; CLI tests own
JSON envelope shape + exit codes). No assertion with unique coverage was dropped;
all drops are documented in the equivalence guardrail below. Wall-clock time is
**unchanged** — the target here is function count and signal-to-noise, not
performance. (Performance optimization was already completed in the two prior tasks.)

#### Top-level test-function count (before / after)

| Package         | Before | After | Reduction |
|-----------------|--------|-------|-----------|
| `internal/board` | 61     | 37    | 24 (39.3%) |
| `internal/worktree` | 22  | 19    | 3 (13.6%) |
| `internal/weft`  | 20     | 15    | 5 (25.0%) |
| `internal/ide`   | 20     | 11    | 9 (45.0%) |
| `internal/muxpoc` | 19    | 14    | 5 (26.3%) |
| **Total**        | **142** | **96** | **46 (32.4%)** |

#### Statement coverage — unchanged / ≥ floor

Per-package coverage remains at or above the documented floor for each package:

| Package | Coverage | Floor |
|---------|----------|-------|
| `internal/board` | 62.5% | 62.5% |
| `internal/worktree` | 68.6% | 68.6% |
| `internal/weft` | 64.6% | 64.6% |
| `internal/ide` | 75.4% | 75.4% |
| `internal/muxpoc` | 33.0% | 33.0% |

#### Equivalence guardrail

The post-prune test-name set is a **justified subset** of the pre-prune set. The
prior tasks (`optimize-test-suite` and `optimize-remaining-test-suites`) enforced a
strict **superset** guardrail: no test was ever dropped, only folded or relocated.
This task relaxes that constraint to a **subset ⊂ pre** with a coverage-floor check:
every removed name from the baseline must map to a surviving `t.Run` subtest or to a
documented drop. Uniquely-covered assertions are preserved.

**Folded names** (original top-level func name now a `t.Run` subtest):

**board (19 folded into other top-level funcs + 5 rewritten as table-driven + 2 dropped):**

Folded into subtests of other top-level functions (net reduction of 19):
- TestAbsolutePathPassthrough → TestLoadConfig/TestAbsolutePathPassthrough
- TestCLIGetNonexistentTask → TestCLIErrorAndEdgeCases/TestCLIGetNonexistentTask
- TestCLIGetTask → TestCLIContract/TestCLIGetTask
- TestCLIListTasks → TestCLIContract/TestCLIListTasks
- TestCLINotInitialized → TestCLIErrorAndEdgeCases/TestCLINotInitialized
- TestCLIRemoveNonexistentTask → TestCLIErrorAndEdgeCases/TestCLIRemoveNonexistentTask
- TestCLIRerender → TestCLIContract/TestCLIRerender
- TestCLISetPhase → TestCLIContract/TestCLISetPhase
- TestCLIUpsertTask → TestCLIContract/TestCLIUpsertTask
- TestDefaultOutputs → TestOutputs/TestDefaultOutputs
- TestDefaultsReturned → TestLoadConfig/TestDefaultsReturned
- TestErrorNotInitialized → TestLoadConfig/TestErrorNotInitialized
- TestInitCreatesStructure → TestInitFirstRun/TestInitCreatesStructure
- TestInitGitignoreBlock → TestInitFirstRun/TestInitGitignoreBlock
- TestInitJSONShape → TestInitFirstRun/TestInitJSONShape
- TestLoadConfig_FallbackPathResolution → TestLoadConfig/TestLoadConfig_FallbackPathResolution
- TestMalformedYAMLError → TestLoadConfig/TestMalformedYAMLError
- TestOutputsFromConfig → TestOutputs/TestOutputsFromConfig
- TestRelativePathResolution → TestLoadConfig/TestRelativePathResolution
- TestRenderBrief → TestRenderProposalAndShapesHomepage/TestRenderBrief
- TestRenderDependencies → TestRenderProposalAndShapesHomepage/TestRenderDependencies
- TestRenderExtendedTitle → TestRenderSidebarExtendedTitle/TestRenderExtendedTitle
- TestRenderIsolatedTask → TestRenderProposalAndShapesHomepage/TestRenderIsolatedTask
- TestRenderLayerBuckets → TestRenderProposalAndShapesHomepage/TestRenderLayerBuckets
- TestRenderMissingDependency → TestRenderProposalAndShapesHomepage/TestRenderMissingDependency
- TestRenderOrphanDetection → TestRenderSingleTask/TestRenderOrphanDetection
- TestRenderSidebarBlanks → TestRenderSidebarExtendedTitle/TestRenderSidebarBlanks
- TestRenderSpecialBucketTask → TestRenderProposalAndShapesHomepage/TestRenderSpecialBucketTask
- TestRenderTaskIDFormatting → TestRenderProposalAndShapesHomepage/TestRenderTaskIDFormatting

Rewritten as table-driven within the original function name (still top-level, no net reduction):
- TestRenderSingleTask — table-driven within same function
- TestRenderStatusVariants — table-driven within same function
- TestRenderToDisk — table-driven within same function
- TestRerender — facade persistence wiring (Home.md and _Sidebar.md written)
- TestUpsertTask — facade unique assertions (tasks.json and Home.md written)

**board (2 dropped with documented justification):**
- TestRemoveTask — owned by `store_test.go:TestRemoveTaskMissing` (business logic owner)
- TestRenderTaskStatus — strict subset of TestRenderStatusVariants (all status variants covered)

(New subtest added: TestRenderDeferredTask is a new row in TestRenderProposalAndShapesHomepage covering the deferred-task bucket path; included to preserve 62.5% coverage floor per Card 1 note.)

**worktree (4 folded):**
- TestWeftPrechecksHardRequireWeftRepo → TestWeftPrechecks/TestWeftPrechecksHardRequireWeftRepo
- TestWeftPrechecksRejectExistingWeftWorktree → TestWeftPrechecks/TestWeftPrechecksRejectExistingWeftWorktree
- TestWeftPrechecksRejectExistingWeftBranch → TestWeftPrechecks/TestWeftPrechecksRejectExistingWeftBranch
- TestWeftHostPristineEnforced → TestWeftPrechecks/TestWeftHostPristineEnforced

**weft (7 folded):**
- TestLoadConfig_DefaultWhenNoYAML → TestLoadConfig/TestLoadConfig_DefaultWhenNoYAML
- TestLoadConfig_OverrideFromYAML → TestLoadConfig/TestLoadConfig_OverrideFromYAML
- TestLoadConfig_MissingLyx → TestLoadConfig/TestLoadConfig_MissingLyx
- TestPullIntegration_FastForward → dropped — strict subset of `sync_test.go:TestPull_FastForward`
- TestPushIntegration_CommitLandsOnBare → TestPushIntegration/TestPushIntegration_CommitLandsOnBare
- TestPushIntegration_RebaseRetryOnNFF → TestPushIntegration/TestPushIntegration_RebaseRetryOnNFF (note: this test did not actually set up non-FF scenario; folded for clarity)
- TestSyncIntegration_EventuallyPushed → TestPushIntegration/TestSyncIntegration_EventuallyPushed

**ide (9 dropped):**
- TestMenuZeroWorktreeMessage — dropped; covered by TestMenuRequiresLyxDir (identical assertion: "no active worktrees")
- TestPickColorFirstUnusedNonGreen → TestPickColor/TestPickColorFirstUnusedNonGreen
- TestPickColorIgnoresUnreadable → TestPickColor/TestPickColorIgnoresUnreadable
- TestPickColorNeverReturnsGreen → TestPickColor/TestPickColorNeverReturnsGreen
- TestPickColorWrapAroundAllUsed → TestPickColor/TestPickColorWrapAroundAllUsed
- TestRunCLIMissingSlug → TestRunCLIErrors/TestRunCLIMissingSlug
- TestRunCLINoArgs → TestRunCLIErrors/TestRunCLINoArgs
- TestRunCLIUnknownSubcommand → TestRunCLIErrors/TestRunCLIUnknownSubcommand
- TestSpawnCallsCodeLauncher → TestSpawn/TestSpawnCallsCodeLauncher
- TestSpawnColorSelection → dropped; covered by TestSpawnGeneratesConfig + vscode_test.go:TestWriteVSCodeConfigCreatesFilesWhenAbsent (color key existence asserted; color choice is color_test's responsibility)
- TestSpawnDoesNotClobber → TestSpawn/TestSpawnDoesNotClobber
- TestSpawnGeneratesConfig → TestSpawn/TestSpawnGeneratesConfig

**muxpoc (8 folded):**
- TestLayoutChecksumIsFourHexDigits → TestLayoutChecksum/TestLayoutChecksumIsFourHexDigits
- TestLayoutChecksumMatchesPsmux → TestLayoutChecksum/TestLayoutChecksumMatchesPsmux
- TestRunCLINoSubcommandFails → TestRunCLIErrors/TestRunCLINoSubcommandFails
- TestRunCLIUnknownFlagFails → TestRunCLIErrors/TestRunCLIUnknownFlagFails
- TestRunCLIUnknownSubcommandFails → TestRunCLIErrors/TestRunCLIUnknownSubcommandFails
- TestSanitizeEnv → TestEnvFiltering/TestSanitizeEnv
- TestSocketNameStability → TestSocketName/TestSocketNameStability
- TestStrippedEnvKeys → TestEnvFiltering/TestStrippedEnvKeys

(The SocketName inline stability check, which had no top-level func name, is folded into TestSocketName/Stability and recorded here for name-map clarity.)

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

The git-spawning tests in `internal/worktree`, `internal/weft`, and `internal/hubgeometry`
were migrated onto shared `lyxtest` fixtures, gated behind a build tag, and
parallelised. This introduced the two-tier split (later completed for board/ide on
2026-06-22).

#### Before / after wall-clock (uncached, `-count=1`)

| Package              | Tier 1 before          | Tier 1 after | Tier 2 after |
|----------------------|------------------------|--------------|--------------|
| `internal/worktree`  | 53.6 s                 | **1.06 s**   | 30.6 s       |
| `internal/hubgeometry` | 19.8 s               | **0.17 s**   | 4.05 s       |
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
| `configengine`                    |  1.1 s |
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
