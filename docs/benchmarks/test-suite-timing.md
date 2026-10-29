# Test-suite timing

Wall-clock for the whole suite, measured across machines, operating systems and endpoint-security setups.
This file is the **cross-environment overview**;
for how to run the suite and what the two tiers are, see [running-tests.md](running-tests.md).
For the board command hot path specifically, see [board-performance.md](board-performance.md).

Reproduce with `go run ./cmd/testtiming` (Tier 1) or `go run ./cmd/testtiming -full` (Tier 2).

> **Compressed 2026-08-10.** This file previously carried a per-task trend log with test-name equivalence maps, folded-subtest tables and coverage floors — none of which is timing data.
> That detail is in git history;
> what remains is the environment comparison plus the levers that actually moved the numbers.

## Reading the tables

Two tiers, and they are **different test sets, not the same tests run twice**:

- **Tier 1** — the default offline loop (`go test ./... -count=1`): fast, no git.
- **Tier 2** — the opt-in integration loop (`go test -tags integration ./... -count=1`): Tier 1 plus the real-git tests.

**Compare _down_ a column, never _across_.**
Tier 2 is a superset of Tier 1, so its larger numbers are expected.
Rows below are also **not all on the same revision** — treat the table as a shape comparison (where does the cost land), not an apples-to-apples benchmark.
Numbers are wall-clock and noisy;
treat them as order-of-magnitude.

Method throughout: median of 3 warm runs per tier, `go build ./...` first.

## All environments

| Environment | AV / security | Tier 1 | Tier 2 | Date |
|---|---|---|---|---|
| Intel Core Ultra 7 155U, Windows 11 (native) | **Cortex XDR** (corporate) | 9.95 s | 131.7 s | 2026-07-13 |
| Intel Core Ultra 7 155U, WSL2 (same laptop) | Cortex on the host, absent inside the VM | 9.01 s | 34.9 s | 2026-08-10 |
| Ryzen 7 9800X3D, Windows 11 (native) | Defender ON | 3.29 s | 18.67 s | 2026-07-13 |
| Ryzen 7 9800X3D, Windows 11 (native) | Defender EXCLUDED | 1.53 s | 16.09 s | 2026-07-13 |
| Ryzen 7 9800X3D, WSL2 (same PC) | no corporate AV; Defender in-VM state unverified | 3.07 s | 6.02 s | 2026-08-08 |
| Ryzen AI 7 445, Linux (bare metal) | none | 1.03 s | 4.97 s | 2026-07-13 |
| Ryzen AI 7 445, Linux (bare metal, after test-seam fixes) | none | 3.86 s | 6.48 s | 2026-08-01 |

The work laptop OSL-1046, an Intel Core Ultra 7 366H on bare-metal Ubuntu 24.04 with Cortex XDR, is measured through `lym gate test` instead, before and after the gate's `-trimpath`, in [gate-test-xdr.md](gate-test-xdr.md).

## What the spread shows

- **Tier 2 traces the AV and OS-boundary tax:** Cortex XDR (131.7 s) → Defender (18.67 s) → clean Windows (16.09 s) → WSL2 (6.02 s) → bare-metal Linux (4.97–6.48 s).
  Tier 2 is dominated by real `git`-subprocess spawns;
  inside WSL2 those are Linux `fork`/`exec` over ext4 that never touch the Windows process-creation or NTFS stack.
- **Tier 1 tracks CPU, not the OS boundary.** It is compile plus in-process execution with no git spawning, so moving to WSL2 barely moves it (9.95 → 9.01 s on the same laptop). Defender does tax it — the 9800X3D A/B pair shows ~54 % on Tier 1 against only ~14 % on Tier 2, so the scanner's cost lands on file reads/writes and allocation-heavy in-process work, not on process creation.
- **Clean Windows is still ~3× slower than Linux on Tier 2** (16.09 s vs 4.97 s) with no AV on either side.
  That is the irreducible cost of Windows process-spawn + NTFS + junctions vs POSIX `fork` + ext4 + symlinks;
  it is not AV and does not go away.
- **WSL2 recovers most of it on the same hardware** — the 9800X3D goes 16–19 s native to 6.02 s under WSL2, landing near bare-metal Linux despite running in a Hyper-V VM.
- **The 155U's original 131.7 s was Cortex plus a weak CPU, not Defender.** Even with Defender on, the 9800X3D ran Tier 1 in a third of the 155U's time.

### Where the time goes

On Windows the Tier 2 floor is **I/O-bound**: `internal/fabricengine`'s real git-worktree work, throttled by AV and NTFS.
On Linux and on the 9800X3D under WSL2 that work is nearly free, so the floor **inverts to time-bound** — tests that sit in real wall-clock grace/deadline windows (`buildercli`'s poll deadlines) and therefore do not shrink with faster I/O.

The Intel 155U under WSL2 is the exception: it stays I/O-bound, with `internal/fabricengine` alone at ~30.8 s of a ~34.9 s wall-clock.
Cortex is verified absent inside the VM, and the host-side agent is an unlikely explanation — real-time scanning hooks file open/create/close, and WSL2 opens `ext4.vhdx` once for the life of the distro, so guest-internal git churn produces no Windows-visible file operations.
The straightforward reading is that a 15 W ultrabook's virtualized I/O is simply slow enough that git still dominates.

## Levers that moved the numbers

Each of these was measured, and each is the reason a row above differs from the one before it.

| Lever | Effect | Where |
|---|---|---|
| **Hermetic git test environment** (`gitkit.HermeticGitEnv()` via `TestMain`) | Tier 2 ~208 s → ~128 s | The operator's global gitconfig carried `core.fsmonitor=true`, causing hundreds of `fsmonitor--daemon` + auto-`maintenance` spawns per run (308 in one package alone, 60 % of its git process-seconds). Full trail in [fixture-copy.md](fixture-copy.md). |
| **cobra's Windows mousetrap check disabled** (`cobra.MousetrapHelpText = ""` in `internal/clihelp`) | Tier 1 ~29 s → ~9.95 s on Windows | Every `Execute()` called `mousetrap.StartedByExplorer()` — a `CreateToolhelp32Snapshot` walk of the whole OS process table. A CPU profile showed 99 % of `internal/clihelp`'s samples inside that syscall, and every `*cli` package paid it per test. |
| **Real-time-wait tests given seams** (`ghAuthTokenTimeout` const → var; `--wait 1ns` on `await-batch`) | Tier 1 6.23 → 3.86 s, Tier 2 33.4 → 6.48 s on Linux | Two tests blocked on production timeouts (5 s and ~30 s) to prove those timeouts are honoured. Overriding the timeout proves the same thing in milliseconds. |
| **Two-tier split, machine-enforced** (`//go:build integration` + `cmd/lym/tierpurity_test.go`) | The single ~82 s loop became ~3.5 s / ~42 s | Tier 1 spawns no `git init` / `git worktree add` / fixture-tree copies repo-wide. Not "zero processes" — untagged tests reaching `hubgeometry.Resolve` on error paths still spawn one cheap, expected-to-fail `git rev-parse`, which the guard deliberately permits. |

**Attribution noise.** Per-package elapsed is inflated by CPU contention — `go test` runs ~60 packages in parallel, and the sum of package times typically runs 3–6× the wall-clock.
Trust the wall-clock;
treat per-package numbers as attribution, not absolute cost.

## Environment notes

Caveats that qualify specific rows.

- **Intel 155U, WSL2 (2026-08-10)** — repo on WSL2-native ext4 (`/dev/sdd`), not `/mnt/c`;
  Ubuntu 24.04.1, WSL kernel 6.18.33.2, Go 1.26.5, revision `faa0fe2b`.
  Cortex verified absent inside the VM but live on the host and never excluded.
  The revision differs from the native-155U row it is compared against, so the ~3.8× Tier 2 gap is environment *and* a month of code.
  Tier 2's spread was wide (30.0–49.4 s) — expected on a thermally-constrained ultrabook under 63 parallel test binaries.
  The first run of each tier was discarded: `go build ./...` warms the package cache but does not link test binaries, so the first `go test` pays for all 63 (Tier 1 measured 29.75 s cold against 7.94–9.85 s warm).
- **Ryzen 9800X3D, WSL2 (2026-08-08)** — same physical box and Windows build as the Defender A/B rows, so it is effectively "same hardware, Linux kernel instead of Windows".
  Defender's state *inside* the VM was never checked or excluded.
  Go was installed fresh to `~/go-linux` to avoid measuring `/mnt/c` boundary crossings.
- **Ryzen 9800X3D, Defender A/B (2026-07-13)** — the same box measured twice, once with real-time protection active and once with the repo + `%TEMP%` excluded.
  No Cortex on this machine, so the A→B delta is a single-variable Defender tax.
- **Ryzen AI 7 445, Linux (2026-07-13 and 2026-08-01)** — the two rows differ because of code changes between them (new packages, re-tiered tests), not environment.
  Getting the suite green on Linux first needed a portability pass;
  see [linux-portability-survey.md](../research/linux-portability-survey.md).

## Trend log

Wall-clock at each revision that moved it, oldest last.
Machine is the Intel 155U on native Windows unless noted — the only environment measured continuously.

| Date | Tier 1 | Tier 2 | What changed |
|---|---|---|---|
| 2026-08-13 (Linux) | 3.60 s | 17.18 s | Unblocked `t.Parallel()` on hub-fixture tests that used to `t.Chdir`/`os.Chdir` — reedcli, loomengine, and a since-retired module's pause suite gained it; eight files total moved onto `RunCLIIn`'s explicit-cwd seam. Payoff is architectural, not wall-clock: on this machine `go test` already runs packages concurrently, so intra-package parallelism recovered close to nothing (measured against the same suite pre-migration: 3.75 s / 18.22 s, both within run-to-run noise) |
| 2026-08-01 (Linux) | 3.86 s | 6.48 s | `ghAuthTokenTimeout` var-seam and `--wait 1ns` removed two real-time waits |
| 2026-08-01 (Linux) | 6.23 s | 33.40 s | New `githubclient`/`webstercli` real-time-wait tests became the floor by default |
| 2026-07-13 | 9.95 s | 131.7 s | Mousetrap disabled; the lingering-child test re-tiered to Tier 2; boardtest writer-iterations cut 50 → 10 |
| 2026-07-13 | ~29 s | ~128 s | Hermetic git test environment landed |
| 2026-07-12 | ~36 s | ~208 s | Two red packages fixed (stale module-count assertion; `ideengine` menu missing `cfg.Path`) |
| 2026-07-12 | ~44 s | ~181 s | Regression recorded: ~a dozen new modules brought untagged git-spawning tests |
| 2026-06-23 | ~3.5 s | ~65 s | Real-GitHub network tests removed, boardtest parallelized — floor shifted to `worktree` fixture I/O |
| 2026-06-22 | ~3.5 s | ~42 s | board/ide git tests gated and relocated — two-tier split complete repo-wide |
| 2026-06-21 | ~27.6 s | — | `worktree`/`weft`/`hubgeometry` migrated onto shared `gitkit` fixtures and gated — the split's first half |
| 2026-06-15 | ~82 s (single loop) | — | Pre-split baseline: every git-spawning test ran in the default loop |

The 2026-08-13 row's near-zero payoff is a property of this machine, not of `t.Parallel()` itself: this same table's [All environments](#all-environments) section already records Tier 2 at 4.97 s on bare-metal Linux against 131.7 s on the Cortex-XDR Windows laptop, and it is the slower, I/O-bound environments — where `go test`'s cross-package concurrency is already saturated by AV/NTFS overhead — that stand to gain the most from a package's own tests running in parallel rather than serially within it.

The 2026-07-13 mousetrap block corrected two earlier causal claims: `cmd/lym`'s guard tests cost ~0.25 s combined in isolation (not the AST-walk cost earlier blocks attributed to them), and 44 of a since-retired module's 45 tests summed to under 1 s (its earlier 12–19 s was contention attribution plus the one lingering-child test).
Both were parallel-contention artifacts, which is the standing hazard when reading per-package numbers.

## test-suite-measure: before and after the speed fixes

A before-state and an after-speed state on the same pre-retag test set (no tagged test file added, removed or retagged between them), so the two are comparable.
This section is a measurement report: it records one run and is not meant to stay true.

### Before state (2026-10-05)

```yaml
machine: AMD Ryzen AI 7 445 w/ Radeon 840M, 12 threads
os: Linux 7.0.0-31-generic (Ubuntu), bare metal
go: go1.26.0 linux/amd64
revision: fca2f0607
load_average_before: 3.08 1.75 1.57
load_average_after: 9.01 8.14 4.47
tier_1_top_level_tests: 4264
integration_top_level_tests: 5593
tier_1_wall: 9.02 s
integration_wall: 57.05 s
```

Method: `go build ./...` first, then four runs each of `go run ./cmd/testtiming` and `go run ./cmd/testtiming -full -top 20`, the first of each discarded as cold, and the median wall time of the remaining three per tier.
Tier 1 runs were 9.06, 9.01 and 9.02 s.
Integration runs were 69.37, 57.03 and 57.05 s, so the median ignores one outlier.
The load average before is the machine idle apart from the editor and other sessions' idle tmux panes, taken right after `go build ./...`;
the load average after is the tail of this run's own test load.
No other loom run or test suite ran during the measurement, and card 9 holds the same condition.
Test counts sum the `TESTS` column of the final run of each tier and cover top-level tests only.

The integration run's twenty slowest top-level tests (run 4):

| Test | Package | Elapsed |
|---|---|---|
| `TestWatchdogIntegration_ExitsAfterIdleCyclesAndReleasesLock` | `internal/reedcli` | 17.41 s |
| `TestWatchdogReap_DescendantClosureConfirmedExited` | `internal/reedcli` | 15.86 s |
| `TestRun_Cancellation` | `internal/verifyrun` | 10.31 s |
| `TestEnforcement_FabricVocabulary` | `internal/lymcwd` | 9.09 s |
| `TestWatchdogIntegration_ResizeAppliesOnlyToThatWorktree` | `internal/reedcli` | 5.53 s |
| `TestVerify_CancelledRunLeavesNoRecord` | `internal/verifytree` | 5.02 s |
| `TestCrossCompileLinux` | `cmd/lym` | 2.81 s |
| `TestIntegrationDriverBootstrap_ReturnsWithoutWaitingOnTheDriver` | `internal/loomcli` | 2.78 s |
| `TestBattenIntegration_RunShedPausedChild_WaitsThenTearsDownOnceDone` | `internal/battencli` | 2.72 s |
| `TestBuildInto_UnwritableDirReturnsError` | `internal/testkit/lymbin` | 2.39 s |
| `TestExitSweep_QuietZeroExitNeitherArmsNorSweeps` | `cmd/lym` | 2.35 s |
| `TestExitSweep_InvalidConfigWarnsAndKeepsExitCode` | `cmd/lym` | 2.32 s |
| `TestExitSweep_ConfiguredCountKeepsNewestSeededTraces` | `cmd/lym` | 2.28 s |
| `TestRootHookWritesTraceFileOnNonZeroExit` | `cmd/lym` | 2.27 s |
| `TestBuild_ProducesExecutableBinary` | `internal/testkit/lymbin` | 2.20 s |
| `TestVerbCases_CleanState` | `internal/fabricengine` | 2.10 s |
| `TestWatchdogSelfHeal_SurvivesInducedTmuxFailure` | `internal/reedengine` | 2.07 s |
| `TestConcurrentReadsDuringUpserts` | `internal/boardengine/boardtest` | 1.76 s |
| `TestNaming_TaskWorktreeRolesParentResumeAndRemove` | `internal/reedcli` | 1.40 s |
| `TestBattenIntegration_StepDrivenRunShed_ReturnsAfterOnePollInterval` | `internal/battencli` | 1.39 s |

### After speed (2026-10-05)

```yaml
machine: AMD Ryzen AI 7 445 w/ Radeon 840M, 12 threads
os: Linux 7.0.0-31-generic (Ubuntu), bare metal
go: go1.26.0 linux/amd64
revision: 41ee008f7
load_average_before: 5.08 4.82 4.06
load_average_after: 22.31 12.01 6.88
tier_1_top_level_tests: 4264
integration_top_level_tests: 5593
tier_1_wall: 5.97 s
integration_wall: 30.41 s
```

Same method as the before-state, on the tree after cards 3–8 and before any retag.
Tier 1 runs were 5.88, 5.97 and 6.01 s.
Integration runs were 30.41, 30.00 and 35.98 s.
The test counts equal the before-state's, so the two states cover the same set.
The load average before is higher than the before-state's, since other sessions were running on the machine; it works against the after-state numbers, which still came out lower.
Tier 1 also fell, to 5.97 s from 9.02 s; the cause was not isolated, and Tier 1 wall time moves with machine load.

The integration run's twenty slowest top-level tests (run 4):

| Test | Package | Elapsed |
|---|---|---|
| `TestWatchdogIntegration_ExitsAfterIdleCyclesAndReleasesLock` | `internal/reedcli` | 3.02 s |
| `TestBattenIntegration_RunShedPausedChild_WaitsThenTearsDownOnceDone` | `internal/battencli` | 2.97 s |
| `TestMergeVerbs_ForeignMergeState_EverySideAndShapeRefuses` | `internal/fabricengine` | 2.93 s |
| `TestExitSweep_QuietZeroExitNeitherArmsNorSweeps` | `cmd/lym` | 2.91 s |
| `TestMidMerge_ForeignState_EverySideAndShape` | `internal/fabricengine` | 2.88 s |
| `TestIntegrationDriverBootstrap_ReturnsWithoutWaitingOnTheDriver` | `internal/loomcli` | 2.83 s |
| `TestExitSweep_InvalidConfigWarnsAndKeepsExitCode` | `cmd/lym` | 2.79 s |
| `TestCrossCompileLinux` | `cmd/lym` | 2.79 s |
| `TestBuild_ProducesExecutableBinary` | `internal/testkit/lymbin` | 2.78 s |
| `TestWatchdogReap_DescendantClosureConfirmedExited` | `internal/reedcli` | 2.68 s |
| `TestWatchdogIntegration_ResizeAppliesOnlyToThatWorktree` | `internal/reedcli` | 2.43 s |
| `TestMerge_NoUpstreamSidePassesVacuously` | `internal/fabricengine` | 2.42 s |
| `TestVerbCases_CleanState` | `internal/fabricengine` | 2.42 s |
| `TestConcurrentReadsDuringUpserts` | `internal/boardengine/boardtest` | 2.37 s |
| `TestMerge_MessagePrecedence` | `internal/fabricengine` | 2.37 s |
| `TestExitSweep_ConfiguredCountKeepsNewestSeededTraces` | `cmd/lym` | 2.34 s |
| `TestMerge_DirtyTargetHalts` | `internal/fabricengine` | 2.23 s |
| `TestRootHookWritesTraceFileOnNonZeroExit` | `cmd/lym` | 2.19 s |
| `TestWatchdogSelfHeal_SurvivesInducedTmuxFailure` | `internal/reedengine` | 2.11 s |
| `TestWatchdogReap_LoopStaysLiveDuringReap` | `internal/reedcli` | 1.90 s |

### Post-retag (2026-10-05)

```yaml
machine: AMD Ryzen AI 7 445 w/ Radeon 840M, 12 threads
os: Linux 7.0.0-31-generic (Ubuntu), bare metal
go: go1.26.0 linux/amd64
revision: 04d9b57c0
load_average_before: 3.56 2.70 3.03
tier_1_top_level_tests: 4274
integration_top_level_tests: 5533
tmux_top_level_tests: 4319
tier_1_wall: 8.38 s
integration_wall: 34.00 s
tmux_wall: 231.56 s
```

Same method as the before-state: `go build ./...` first, then four runs each of `go run ./cmd/testtiming`, `-tags integration` and `-tags tmux`, the first of each discarded as cold, and the median wall time of the other three.
Tier 1 runs were 8.20, 8.38 and 8.27 s.
Integration runs were 35.16, 33.02 and 34.00 s.
Tmux runs were 231.56, 230.05 and 238.12 s.
Test counts sum the `TESTS` column of the final run of each tier and cover top-level tests only.
The load average before is the machine idle apart from other sessions, taken before the first run; it rose to 10 by the end.

The integration tier's test set changed at the retag: tmux-starting files moved to the `tmux` tier and `smoke` was retired, so these numbers do not compare with the before and after-speed ones.
The `tmux` tier now carries the cost the integration tier shed, so its wall time is the long pole.
Every `-tags tmux` run failed on `internal/reedcli`'s `TestSmokeDotFillCrossClientControl`, which tmux 3.6 refuses with "no space for new pane" on this machine; it is counted in the wall time and was not caused by the retag.
Tier 1 gained tests since the before-state through the new untagged guards and unit tests.

The per-test redundancy of this tree is in [test-redundancy.md](test-redundancy.md).

The `-tags tmux` run's five slowest top-level tests (run 4):

| Test | Package | Elapsed |
|---|---|---|
| `TestSmokeRepaintCandidateMeasurement` | `internal/reedcli` | 57.69 s |
| `TestSmokeDotFillCrossClientControl` | `internal/reedcli` | 43.02 s (FAIL) |
| `TestSmokeDotFillFloorIsCleanOnASettledAttach` | `internal/reedcli` | 24.82 s |
| `TestSmokeDotFillResizeTreatment` | `internal/reedcli` | 22.97 s |
| `TestSmokeDotFillResizeControl` | `internal/reedcli` | 22.68 s |

## test-tiers: after the round-gate and link cuts

The after-state of the `test-tiers` task: the round gate narrowed, `websterengine` and `loomshed` off the code index, and the guards and budget in place.
This section is a measurement report: it records one run and is not meant to stay true.

### After state (2026-10-07)

```yaml
machine: AMD Ryzen AI 7 445 w/ Radeon 840M, 12 threads
os: Linux 7.0.0-31-generic (Ubuntu), bare metal
go: go1.26.0 linux/amd64
revision: 53d33c500
load_average_before: 2.89 8.93 9.81
load_average_after: 4.45 9.27 10.62
tier_1_top_level_tests: 2829
integration_top_level_tests: 3508
tmux_top_level_tests: 2894
tier_1_wall: 6.67 s
integration_wall: 34.46 s
tmux_wall: 101.52 s
```

Same method as `test-suite-measure`: `go build ./...` first, then four runs each of `go run ./cmd/testtiming`, `-tags integration` and `-tags tmux`, the first of each discarded as cold, and the median wall time of the other three.
Tier 1 runs were 6.44, 6.67 and 6.68 s.
Integration runs were 45.94, 34.46 and 30.35 s.
Tmux runs were 101.61, 98.57 and 101.52 s.
Test counts sum the `TESTS` column of the final run of each tier and cover top-level tests only.
The tiers are separate tag sets and do not nest, so each tier's count includes the untagged tests it recompiles with.
The load average before was taken right after `go build ./...`, while other sessions' load was still decaying from the previous minutes.

The `tmux` tier passed in all four runs, `TestSmokeDotFill`'s `CrossClientControl` subtest included.

The integration run's twenty slowest top-level tests (run 4):

| Test | Package | Elapsed |
|---|---|---|
| `TestBattenIntegration_Rows` | `internal/battencli` | 12.82 s |
| `TestRunCLI_MergeScenario` | `internal/fabriccli` | 5.86 s |
| `TestRunCLI_AdoptRemoteWeftScenario` | `internal/fabriccli` | 4.73 s |
| `TestLoomFailedReedUpRefuses` | `internal/loomcli` | 4.51 s |
| `TestLoomPreBootstrapPair` | `internal/loomcli` | 4.27 s |
| `TestCrossCompileLinux` | `cmd/lym` | 3.78 s |
| `TestLoomStatusAndPauseOnNeverBootstrappedPair` | `internal/loomcli` | 3.73 s |
| `TestResetCmd` | `internal/webstercli` | 3.60 s |
| `TestRunCLI_CleanupRemoveScenario` | `internal/fabriccli` | 3.54 s |
| `TestExitSweep_ConfiguredCountKeepsNewestSeededTraces` | `cmd/lym` | 3.30 s |
| `TestRootHookWritesTraceFileOnNonZeroExit` | `cmd/lym` | 3.24 s |
| `TestBuild_ProducesExecutableBinary` | `internal/testkit/lymbin` | 3.04 s |
| `TestExitSweep_InvalidConfigWarnsAndKeepsExitCode` | `cmd/lym` | 2.99 s |
| `TestFinalize_OverRealHub` | `internal/landingshed` | 2.86 s |
| `TestSpawnScenario` | `internal/ideengine` | 2.73 s |
| `TestRunCLI_HubMutationScenario` | `internal/fabriccli` | 2.56 s |
| `TestEnforcement_FabricVocabulary` | `internal/lymcwd` | 2.43 s |
| `TestVerbCases_CleanState` | `internal/fabricengine` | 2.32 s |
| `TestSync` | `internal/boardengine/boardtest` | 2.16 s |
| `TestConcurrentReadsDuringUpserts` | `internal/boardengine/boardtest` | 2.14 s |

The `-tags tmux` run's five slowest top-level tests (run 4):

| Test | Package | Elapsed |
|---|---|---|
| `TestSmokeRepaintCandidateMeasurement` | `internal/reedcli` | 17.48 s |
| `TestSmokeStatusStrandAcrossDriverSeeds` | `internal/loomcli` | 16.85 s |
| `TestSmokeDotFill` | `internal/reedcli` | 16.67 s |
| `TestSmokeDriverStrand_ReentrantAcrossThreeBootstraps` | `internal/loomcli` | 15.83 s |
| `TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning` | `internal/loomcli` | 14.50 s |

### Tree-sitter links

Test binaries linking `github.com/tree-sitter/go-tree-sitter`, counted by `go list -test -deps -tags integration,tmux,llm ./...`:

| State | Test binaries |
|---|---|
| Before (this plan's measurement on the before tree) | 44 |
| After | 8 |

The eight after-state binaries are the CLI packages that call the index (`cmd/lym`, `loomcli`, `shedcli`, `webstercli`, `quarrycli`), `planglyph`, and the test binaries of `websterengine` and `loomshed`, whose tests drive the real index.

Forced-relink `go test -c` per package, three runs each after one warm build, relinking through a changed `-ldflags` value:

| Package | Links tree-sitter | Relink runs |
|---|---|---|
| `internal/websterengine` | yes | 1.09, 1.10, 1.14 s |
| `internal/loomshed` | yes | 1.03, 1.03, 1.01 s |
| `internal/shedrecipe` | no | 0.60, 0.63, 0.64 s |
| `internal/configreg` | no | 0.47, 0.46, 0.44 s |
| `internal/fabricengine` | no | 0.46, 0.45, 0.47 s |
| `internal/hubgeom` | no | 0.42, 0.41, 0.41 s |
| `internal/verifytree` | no | 0.23, 0.22, 0.26 s |

On the before tree the same relink took about 1.3–1.4 s for a binary linking tree-sitter through `websterengine` (`configreg`, `hubgeom`) against 0.45–0.85 s for comparable binaries that do not (`verifytree`, `fabricengine`), and 3.0 s for `websterengine` itself.
Both seams now link it only where the tests drive the real index, and `configreg` and `hubgeom` fall to the cgo-free range.
The suite's top-level test count and the integration tier's wall time, before (`test-suite-measure`'s before-state: 5593 tests, 57.05 s) and after (3508 tests, 34.46 s), go into the PR description.
The counts do not compare one to one: the retag moved tmux-starting files out of the integration tier and the prune tasks removed redundant tests between the two states.

## test-time-audit: first recurring audit

The first run of the recurring audit that watches suite growth after the per-package test-count budget was deleted.
It compares against the `test-tiers` after-state above.
This section is a measurement report: it records one run and is not meant to stay true.

### Audit state (2026-10-28)

```yaml
machine: AMD Ryzen AI 7 445 w/ Radeon 840M, 12 threads
os: Linux 7.0.0-31-generic (Ubuntu), bare metal
go: go1.26.8 linux/amd64
revision: ba2a346b
load_average_before: 0.68 1.23 1.80
load_average_after: 4.96 5.90 4.43
tier0_top_level_tests: 3734
tier1_top_level_tests: 902
tier2_top_level_tests: 69
tier0_wall: 18.8 s
tier1_wall: 58.8 s
tier2_wall: 50.0 s
```

#### Tiers and how they map to the older columns

The gate's tags are disjoint and do not nest, and `lym config gate --print` declares what each one runs:

| Tag | Runs | Older name |
|---|---|---|
| `tier0` | untagged tests, offline | Tier 1 (`test-tiers`' `tier_1`) |
| `tier1` | integration tests, after `go build ./...` and `go vet` under each of the four tags | Tier 2 / `integration` |
| `tier2` | tmux tests | `tmux` |

The older integration and tmux counts include the untagged tests they recompiled with, so the tag-only counts below are the older count minus the older Tier 1: 3508 - 2829 = 679 integration-only and 2894 - 2829 = 65 tmux-only.
`tier3` (LLM tests, run by hand) is not measured.

#### Route and method

`go run ./cmd/testtiming` is refused to an agent's session: the hook names `lym gate test --tier N`, so every number here is the gate's summary-line wall for `lym gate test --tier N --all -- -count=1`, not `cmd/testtiming`'s.
That changes what is measured against the older sections:

- The gate runs `go test -json` under `GOFLAGS="-trimpath -p=4"`, where `cmd/testtiming` ran under the default package parallelism of 12 threads, so a wall here is not comparable one to one with an older one.
- The `tier1` wall includes `go build ./...` and four `go vet` runs before its `go test`.
- `-- -count=1` is part of every run: without it the gate's `go test` replays the Go test cache, and the first attempt at this audit, which omitted it, measured 7.1 to 11.3 s for `tier0` and 12.6 to 12.8 s for `tier2` and was discarded.
- A repeat of an unchanged green tree is carried by the gate's record and takes 0.0 s, so the worktree held one untracked file for the length of the measurement, which makes the tree dirty and the gate read no record; it was removed afterwards.
- Per-package counts and times are read from the `ok` lines and top-level `--- PASS|FAIL|SKIP` lines of the gate's logs.
  The counts come from the replayed (cached) runs, where each package's output is contiguous; the uncached runs interleave packages, so the totals agree and the per-package split would not.

Per tier: `go build ./...` ran inside the gate's first tier, then four `-count=1` runs, the first discarded as cold, and the median of the other three taken.
`tier0` runs were 19.0 (discarded), 18.8, 18.8 and 18.9 s.
`tier1` runs were 58.9 (discarded), 58.8, 58.7 and 58.9 s.
`tier2` runs were 49.0 (discarded), 50.3, 50.0 and 49.7 s.
All 12 runs passed: `tier0` 21021 discovered including subtests with 1 skipped, `tier1` 3574, `tier2` 265.

#### Machine state

`lym tender status` and `lym gate status` ran before the measurement.
No gate slot was held or awaited, and no other session was busy; the orch was idle awaiting input, and several other tasks' batten runners were registered with no test run in flight.
The load average before the first run was 0.68, rising to 8.49 during the `tier1` runs and 4.96 at the end; most of that is the gate's own `-p=4` load, since the spread within each tier's three runs is under 0.3 s.

### Wall time against the `test-tiers` after-state

| Tier | Older state | Older wall | Now | Wall now |
|---|---|---|---|---|
| `tier0` | `tier_1` (untagged) | 6.67 s | `tier0` | 18.8 s |
| `tier1` | `integration` | 34.46 s | `tier1` | 58.8 s |
| `tier2` | `tmux` | 101.52 s | `tier2` | 50.0 s |

The `tier2` fall is partly the retag: the older `tmux` run carried the whole untagged set, which `tier2` does not.
The `tier0` and `tier1` rises cannot be split between growth and the method differences above; the per-tier test counts below are the comparable signal.

The packages with the longest `ok` time in each tier:

| Tier | Package | Time |
|---|---|---|
| `tier0` | `cmd/lym` | 5.56 s |
| `tier0` | `internal/lymcwd` | 1.70 s |
| `tier1` | `internal/fabricengine` | 42.74 s |
| `tier1` | `internal/fabriccli` | 17.00 s |
| `tier1` | `internal/gatecli` | 7.75 s |
| `tier2` | `internal/loomcli` | 40.47 s |
| `tier2` | `internal/reedengine` | 40.00 s |
| `tier2` | `internal/reedcli` | 38.15 s |

`internal/reedengine` in `tier2` takes 40.00 s for 2 tests, a round figure that reads as a fixed timeout waited out; the `tier2` wall is bound by the three packages above running together.

### Test counts against the `test-tiers` after-state

| Tier | Older top-level tests | Now | Change |
|---|---|---|---|
| `tier0` | 2829 | 3734 | +905 |
| `tier1` (integration-only) | 679 | 902 | +223 |
| `tier2` (tmux-only) | 65 | 69 | +4 |

The older state holds per-tier totals only.
A per-package baseline is the per-package count in [test-redundancy.md](test-redundancy.md), regenerated on 2026-10-27 under the older tags `integration,tmux`; it totals 3506 against 4705 today for the three tiers together, so the comparison below is indicative and packages split or moved between the two states.

Packages that grew most in top-level tests, older to now across the three tiers:

| Package | Older | Now | Change |
|---|---|---|---|
| `internal/battenshed` | 76 | 160 | +84 |
| `internal/shuttleengine` | 129 | 207 | +78 |
| `internal/boardengine/boardtest` | 3 | 68 | +65 |
| `internal/loomcli` | 167 | 228 | +61 |
| `internal/shell` | 11 | 65 | +54 |
| `internal/fabricengine` | 610 | 661 | +51 |
| `internal/shedrun` | 22 | 72 | +50 |
| `internal/battenrecipe` | 8 | 57 | +49 |

The largest falls are `internal/shedrecipe` (-76), `internal/shedverbs` (-62), `internal/boardengine` (-57), `internal/shuttleengine/claudeengine` (-46) and `internal/websterengine` (-45); `internal/sheddriver` (99 tests), `internal/dobbyengine`, `internal/gateslot`, `internal/hubreconcile` and 58 more packages hold tests the older report has no section for.

### Prune-task candidates

Each line is a lead for a prune task under `PATTERN-test-speed`, not a verdict; `git log --since=2026-10-07` and `git diff --shortstat` over the package were read for new behavior, and none was filed.

- `internal/boardengine/boardtest` 3 to 68 tests: 6 commits since 2026-10-07 touching 5 files with +91/-76 lines, a size that does not explain 65 more tests; part of the rise may be tests the older report did not count because of its tags.
- `internal/shell` 11 to 65 tests: 7 commits and +148/-21 lines in 5 files, far more tests than changed code.
- `internal/battenrecipe` 8 to 57 tests: 10 commits, +196/-67 lines in 6 files.
- `internal/shedrun` 22 to 72 tests: +903/-209 lines in 11 files over 27 commits, a plausible match for new behavior (beams, dobby, arbiter) but worth a redundancy pass.
- `internal/battenshed` +84, `internal/shuttleengine` +78, `internal/loomcli` +61 and `internal/fabricengine` +51 track 49 to 142 commits and 9.7 to 34.9 thousand changed lines each, so their growth matches new behavior.
- `internal/reedengine` `tier2`: 2 tests spending 40.00 s, a candidate for an injectable duration under `PATTERN-test-speed`.

#### Correction (2026-10-29)

The per-package counts above for the three prune leads do not match the packages' sources, so the per-package split of the gate's logs is not reliable as a count.
Counted by `func Test` declarations, `internal/shell` went from 11 to 14 tests, `internal/boardengine/boardtest` stayed at 3 and `internal/battenrecipe` went from 8 to 9; a verbose run shows `internal/shell`'s 14 tests carrying 60 subtests, which neither count includes.
`internal/reedengine`'s `tier2` time was no waited-out duration: its 32 tests ran serially, with the slowest at 11 s, and their times summed to the package's wall.
A later audit counts per package from `go test -json` events keyed by package, or from a verbose run of the package alone.
