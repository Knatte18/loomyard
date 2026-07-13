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

Numbers here are wall-clock and **noisy** — treat them as order-of-magnitude.
Each dated block is tagged with its OS in the `Machine:` line: the Windows blocks
were measured with Cortex XDR live (file I/O + AV process-creation tax — see
[board-performance.md](board-performance.md#process-startup-context)); the
[Linux baseline](#linux-baseline) has no such tax. **Never compare a Windows
number against a Linux one** — the AV delta dwarfs the code delta; compare down
a single OS's column.

## Current best times

As of **2026-07-13** (restore-tier1-floor: mousetrap disabled +
lingering-child test re-tiered).

- Machine: Intel Core Ultra 7 155U, Windows 11 Enterprise, `windows/amd64`, 14 logical CPUs
- Go 1.26.4, default GC, `GOMAXPROCS` = NumCPU (14)
- Method: median of 3 warm runs per tier via `go run ./cmd/testtiming[ -full]`
  (`-count=1` set by the harness; `go build ./...` run first to warm the build
  cache)

### Headline

| Loop | Command | Wall-clock | vs. previous (2026-07-13 hermetic-git-env block) |
|------|---------|-----------|----------------------------|
| **Tier 1** — offline, default | `go test ./... -count=1` | **~9.95 s** (spread 9.33–12.84 s) | was ~29 s — **~66 % faster** |
| **Tier 2** — integration, opt-in | `go test -tags integration ./... -count=1` | **~131.7 s** (spread 124.0–132.5 s) | was ~128 s — essentially flat; the levers below do not touch Tier 2's `internal/warpengine` floor (see Cause) |

All 3 Tier 1 runs and all 3 Tier 2 runs recorded `RESULT: all packages
passed`.

### Cause

Two levers drove the Tier 1 drop, plus one smaller contribution kept from
card 3:

- **(a) cobra's Windows mousetrap check disabled** at the shared
  `internal/clihelp` seam (`cobra.MousetrapHelpText = ""` in `exec.go`'s
  `init()`). Every `cobra.Command.Execute()` on Windows previously called
  `mousetrap.StartedByExplorer()` — a `CreateToolhelp32Snapshot` walk of the
  entire OS process table, done only to detect launch-by-double-click from
  Explorer. A CPU profile of `internal/clihelp` showed 99% of samples inside
  that syscall. Measured package effect: `internal/clihelp` 8.0 s → 0.46 s
  (this run's isolated Tier 1 elapsed) — the dominant lever, since every
  one of the ~15 `*cli` packages pays the syscall once per test that
  drives a command through `Execute`/`RunCLI`.
- **(b) `TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay`
  re-tiered to Tier 2** (`internal/perchengine/gate_lingering_test.go`,
  `//go:build integration`). Its two parallel subtests each sit in the
  production `gateWaitDelay = 10 s` pipe-abandon grace window (measured
  this run: 10.12 s and 12.00 s) — ~12 s of `internal/perchengine`'s prior
  ~14.6 s isolated Tier 1 time. Moving it out drops
  `internal/perchengine`'s Tier 1 elapsed to 3.21 s (median run); Tier 2
  absorbs the ~12 s invisibly inside its own ~131.7 s wall-clock
  (perchengine's Tier 2 package elapsed this run: 22.69 s, combining the
  fast untagged suite re-run under `-tags integration` plus the now-tagged
  lingering test).
- **(c) boardtest `writes` shrink, kept** (card 3's bounded attempt):
  `TestConcurrentReadsDuringUpserts`'s writer-iteration constant in
  `internal/boardengine/boardtest/concurrency_test.go` dropped from 50 to
  10, preserving the reader/writer overlap shape (1 writer, 8 readers,
  non-mutating upserts so the task-count assertion still holds). Measured
  effect this run: the test's own elapsed fell from ~8.1 s (2026-07-13
  hermetic-git-env block) to 0.45 s; the package's Tier 1 elapsed fell to
  1.16 s. Kept — it won well over the ~1 s bar.

Tier 2's wall-clock stays essentially flat (~128 s → ~131.7 s, both within
this suite's ~124–132 s noise band) because none of these levers touch
Tier 2's floor, `internal/warpengine`'s real git-worktree I/O (~96 s this
run); the ~12 s the re-tiered lingering-child test adds to
`internal/perchengine` is absorbed inside warpengine's own slack under
~50-package parallelism.

**Supersession note:** the 2026-07-12 and 2026-07-13 (hermetic-git-env)
blocks' causal claims for Tier 1's ~29–37 s floor — "cmd/lyx guard tests
AST-parse/walk the repo" and "perchengine's cost is its large table-driven
suite" — are corrected by this block. The four `cmd/lyx` guards
(`tierpurity`, `hermeticenv`, `registration`, `sandbox_coverage`) cost
0.00–0.11 s each in isolation (~0.25 s combined); `cmd/lyx`'s ~3.74 s in
this run's Tier 1 table (below) is still contention attribution, not
AST-walk cost. 44 of `internal/perchengine`'s 45 tests sum to under 1 s —
the 12–19 s attributed to it in earlier blocks was parallel-contention
attribution noise plus (unmeasured at the time) the one lingering-child
test now re-tiered above. The frozen blocks below are left unedited per
append-only discipline; only this new block corrects the record.

The lingering-child test had evaded the `tierpurity` guard by spawning
through the production `execGateCommand` wrapper rather than a banned
token the guard greps for (`gitexec.RunGit`, `exec.Command`,
`lyxtest.Copy`); no guard change was made — the guard's narrowness (a
deliberately narrow raw-substring match, not "spawn no processes") is by
design, see `cmd/lyx/tierpurity_test.go`.

### Tier 1: where the time goes

Tier 1 is still offline: no `git init` / `git worktree add` / fixture-tree
copies remain in any untagged test file, machine-enforced by
`cmd/lyx/tierpurity_test.go` (`TestTierPurity_UntaggedTestsSpawnNothing`).
This is **not** "zero processes spawned" — untagged tests reaching
`hubgeometry.Resolve` on their error paths (e.g. `boardcli`'s `RunCLI` seam
tests) still spawn one cheap, expected-to-fail `git rev-parse`; the guard
deliberately permits that.

| Package | Tier 1 elapsed (median run) | Cause |
|---------|------------------------------|-------|
| `cmd/lyx` | ~3.74 s | repo-wide guard tests plus cross-compile/help-tree checks — still the largest single package by elapsed, but the guards cost ~0.25 s combined in isolation; this is contention attribution, not AST-walk cost (see the supersession note above) |
| `internal/perchengine` | ~3.21 s | the remaining 44-test run-loop/gate/judge/state-machine suite, now with the one real-time lingering-child test moved to Tier 2 |
| `internal/builderengine` | ~3.00 s | builder-module facade tests (new since 2026-07-12) |
| `internal/perchcli`, `internal/buildercli`, `internal/burlerengine`, `internal/configcli` | ~1.8–2.2 s each | contention-inflated CLI/facade suites |
| everything else | < 1.8 s each, noisy | scheduler contention across 52 parallel test binaries, not a stable per-package cost |

**Attribution noise:** per-package elapsed is inflated by CPU contention
(`go test` runs 52 packages in parallel on 14 logical CPUs; the sum of
package times is ~56.9 s against a ~9.95 s wall-clock — a ~5.7× overlap).
No package does real git spawning in Tier 1, so this contention remains the
dominant source of per-package variance. Trust the wall-clock; treat
per-package numbers as attribution, not absolute cost.

### Tier 2: where the time goes

| Package | Tier 2 elapsed (median run) | Where the cost is |
|---------|------------------------------|--------------------|
| `internal/warpengine`            | **~96.0 s** | real `git worktree` add/remove, junctions, host↔weft topology — still the Tier 2 floor |
| `internal/builderengine`         | ~75.9 s | new module (batch-implementation loop): facade-level git-spawning tests |
| `internal/buildercli`            | ~73.0 s | same new module, CLI tests over real git scratch repos/host-hub fixtures |
| `internal/boardcli`              | ~54.8 s | its own git-spawning CLI tests (`seedCwd` `git init`s + `RunCLI`'s `hubgeometry.Resolve` `git rev-parse`) |
| `internal/initengine`            | ~54.6 s | paired-fixture copies per test |
| `internal/perchcli`              | ~38.5 s | `lyxtest.CopyPaired[Local]` fixture copies + weft-sync git assertions |
| `internal/boardengine/boardtest` | ~30.7 s | real local git commit/push, parallelized |
| `internal/configcli`             | ~28.4 s | `git init` tests, including `TestE2ESyncIntegration` |
| `internal/warpcli`               | ~26.2 s | CLI wrapper over warpengine clone/teardown paths |
| `internal/muxcli`                | ~24.6 s | real `tmux`/`psmux` contract-integration tests |
| `internal/perchengine`           | ~22.7 s | the same untagged run-loop suite re-run under `-tags integration`, **plus** the now-relocated `TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay` (measured this run: 10.12 s + 12.00 s parallel subtests) |

The floor is still `internal/warpengine`, now at ~96.0 s in the median run
(roughly comparable to the 2026-07-13 hermetic-git-env block's ~84 s, both
well inside Tier 2's overall noise band — see the wall-clock spread above):
its slowest single tests this run cost 15–17 s each (`TestRemove` 17.39 s,
`TestCloneHub_HappyPath` 15.97 s, `TestCleanup_LiveBranchNeverDeleted`
15.61 s) — the same fixture-copy-plus-real-git shape as before. This task's
levers do not touch `internal/warpengine`; Tier 2's wall-clock stays
essentially flat because the ~12 s the re-tiered lingering-child test adds
to `internal/perchengine` is absorbed inside `internal/warpengine`'s own
slack under ~50-package parallelism.

### Slowest 10 top-level tests (Tier 1, median run)

| Test | Package | Elapsed |
|------|---------|---------|
| `TestRun_Resume` | `internal/perchengine` | 0.66 s |
| `TestRunCLI_Run_FlagValidation` | `internal/shuttlecli` | 0.55 s |
| `TestProfile_Validate` | `internal/burlerengine` | 0.53 s |
| `TestConcurrentReadsDuringUpserts` | `internal/boardengine/boardtest` | 0.45 s |
| `TestParseReport_Rejections` | `internal/builderengine` | 0.38 s |
| `TestRun_OutcomeMapping` | `internal/builderengine` | 0.31 s |
| `TestRunDispatchesToConfig` | `cmd/lyx` | 0.29 s |
| `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` | `cmd/lyx` | 0.28 s |
| `TestConcurrentUpsertsDoNotLoseWrites` | `internal/boardengine/boardtest` | 0.25 s |
| `TestRunCLI_NotAGitRepo` | `internal/muxcli` | 0.25 s |

`TestConcurrentReadsDuringUpserts` dropping from 8.1 s (2026-07-13
hermetic-git-env block) to 0.45 s is the boardtest `writes` shrink (lever
c); it no longer dominates the Tier 1 top-10, unlike every prior block.

### Slowest 10 top-level tests (Tier 2, median run)

| Test | Package | Elapsed |
|------|---------|---------|
| `TestCLIStrictPayloadShapes` | `internal/boardcli` | 22.94 s |
| `TestCLILookupContract` | `internal/boardcli` | 21.75 s |
| `TestE2ESyncIntegration` | `internal/configcli` | 21.59 s |
| `TestRemove` | `internal/warpengine` | 17.39 s |
| `TestCloneHub_HappyPath` | `internal/warpengine` | 15.97 s |
| `TestCleanup_LiveBranchNeverDeleted` | `internal/warpengine` | 15.61 s |
| `TestSpawnBatch_RoleSelectionMatrix` | `internal/builderengine` | 15.12 s |
| `TestRunCLI_ResolvesLayoutAndConfig` | `internal/muxcli` | 14.48 s |
| `TestPollCmd_TerminalCleanupMatrix` | `internal/buildercli` | 14.36 s |
| `TestReconcile_MissingWeftWorktreeRecreated` | `internal/warpengine` | 13.93 s |

## Linux baseline

First Linux run of the suite (2026-07-13), recorded in parallel with the Windows
numbers above — **compare down each OS's own column, never across OSes** (the two
machines differ in CPU, core count, and, decisively, endpoint-AV load). The
Windows numbers were measured with **Cortex XDR** live, which throttles every
file-heavy operation; the Linux box has no equivalent tax, so a faster Linux
number is *expected* and mostly measures the absence of AV, while a *slower*
Linux number would flag a genuine Linux pathology.

Getting the suite green on Linux first required a portability pass (see
[linux-portability-survey.md](../research/linux-portability-survey.md)): the
first run had 6 failing tests (Windows path/shell/FS assumptions) plus a
concurrency test that self-starved to ~231 s without AV to throttle its readers.

- Machine: AMD Ryzen AI 7 445 w/ Radeon 840M, Ubuntu 26.04 LTS, `linux/amd64`, 12 logical CPUs
- Go 1.26.0, default GC, `GOMAXPROCS` = NumCPU (12)
- Method: median of 3 warm runs per tier via `go run ./cmd/testtiming[ -full]`
  (`-count=1` set by the harness; `go build ./...` run first to warm the cache)

### Headline

| Loop | Command | Linux wall-clock | Windows (2026-07-13, Cortex XDR) |
|------|---------|------------------|----------------------------------|
| **Tier 1** — offline, default | `go test ./... -count=1` | **~1.03 s** (spread 1.00–1.12 s) | ~9.95 s |
| **Tier 2** — integration, opt-in | `go test -tags integration ./... -count=1` | **~4.97 s** (spread 4.82–5.01 s) | ~131.7 s |

All 3 + 3 runs recorded `RESULT: all packages passed`. Tier 2 is ~26× faster on
Linux — almost entirely the AV tax on `internal/warpengine`'s real git-worktree
I/O that dominated Windows (~96 s there) evaporating (0.67 s here).

### Where the time goes (the profile inverts)

The Tier 2 floor moves OS to OS. On Windows it was **I/O-bound**
(`internal/warpengine` git worktrees, throttled by AV). On Linux that work is
nearly free, so the floor becomes **time-bound** — tests that sit in real
wall-clock grace/deadline windows and so do not shrink without AV:

| Package | Linux Tier 2 (median) | Note |
|---------|------------------------|------|
| `internal/buildercli` | ~4.31 s | poll-deadline/grace tests (`TestPollCmd_*`, ~1 s each) — real-time waits, the new Linux floor |
| `cmd/lyx` | ~0.74 s | includes `TestCrossCompileLinux` (0.62 s) |
| `internal/builderengine` | ~0.72 s | |
| `internal/warpengine` | ~0.67 s | the Windows floor (~96 s) — now negligible without AV |
| `internal/muxengine` | ~0.59 s | `TestMultiplexerContract` real-tmux probe |
| `internal/boardengine/boardtest` | ~0.53 s | real local git commit/push |

Tier 1 on Linux is dominated by `internal/boardengine/boardtest` (~0.33 s) and
`internal/muxengine` (~0.16 s) — pure-CPU suites; no package does git in Tier 1.

## Windows clean-CPU baseline (Ryzen 7 9800X3D, Defender A/B)

A third machine (2026-07-13), run specifically to **isolate the antivirus cost**:
the same box measured twice, once with Microsoft Defender real-time protection
active and once with the repo + `%TEMP%` excluded. Same CPU, same OS, only AV
differs — so the A→B delta is the pure Defender tax, and Run B is effectively
"clean Windows." This box has **no Cortex XDR** (unlike the 155U above), so the
comparison is single-variable.

- Machine: AMD Ryzen 7 9800X3D, Windows 11 (10.0.26200), 16 logical CPUs, Go 1.26.3
- Method: median of 3 warm runs per tier via `go run ./cmd/testtiming[ -full]`

| Loop | Defender ACTIVE | Defender EXCLUDED (clean) | Defender tax |
|------|-----------------|---------------------------|--------------|
| **Tier 1** | 3.29 s | **1.53 s** | ~54 % |
| **Tier 2** | 18.67 s | **16.09 s** | ~14 % |

### What this settles about AV vs CPU vs OS

Three machines side by side (median wall-clock):

| | Tier 1 | Tier 2 | AV | CPU class |
|---|--------|--------|-----|-----------|
| Intel 155U | ~9.95 s | ~131.7 s | **Cortex XDR** | 15 W ultrabook |
| Ryzen 9800X3D, Defender on | 3.29 s | 18.67 s | Defender | flagship desktop |
| Ryzen 9800X3D, clean | 1.53 s | 16.09 s | none | flagship desktop |
| Linux (Ryzen AI 7 445) | 1.03 s | 4.97 s | none | mobile |

- **Defender is a real but modest tax, and it lands on in-process work, not
  spawning.** Tier 1 (compile + in-process test execution + small-file I/O) drops
  ~54 % without Defender; Tier 2 (dominated by git-subprocess spawns) drops only
  ~14 %. The AV scanner spends its time on file reads/writes and allocation-heavy
  in-process work, not on process creation, on this box.
- **The 155U's huge numbers were mostly Cortex XDR + a weak CPU, not Defender.**
  Even with Defender *on*, the 9800X3D runs Tier 1 in a third of the 155U's time
  (3.29 s vs 9.95 s) — that gap is CPU + Cortex, since Defender itself only
  accounts for the 3.29 → 1.53 s part. Do not read the 155U↔9800X3D gap as "AV."
- **Clean Windows is still ~3× slower than Linux on Tier 2** (16.09 s vs 4.97 s)
  with no AV on either side — the irreducible cost of Windows process-spawn +
  NTFS + junctions vs POSIX `fork` + ext4 + symlinks. That floor is not AV and
  does not go away.

## History (trend log)

### 2026-07-13 — hermetic git test environment (was "Current best times")

As of **2026-07-13** (hermetic git test environment landed — see
[fixture-copy.md](fixture-copy.md) for the full analysis behind this
change). Superseded by the 2026-07-13 restore-tier1-floor block above;
frozen here for the trend log.

- Machine: Intel Core Ultra 7 155U, Windows 11 Enterprise, `windows/amd64`, 14 logical CPUs
- Go 1.26.4, default GC, `GOMAXPROCS` = NumCPU (14)
- Method: median of 3 warm runs per tier via `go run ./cmd/testtiming[ -full]`
  (`-count=1` set by the harness; `go build ./...` run first to warm the build
  cache)

#### Headline

| Loop | Command | Wall-clock | vs. 2026-07-12 (previous "Current best times") |
|------|---------|-----------|----------------------------|
| **Tier 1** — offline, default | `go test ./... -count=1` | **~29 s** (spread 28.5–29.4 s) | was ~36 s — modest improvement; Tier 1 never spawned real git, so this task's lever does not directly apply here (see below) |
| **Tier 2** — integration, opt-in | `go test -tags integration ./... -count=1` | **~128 s** (spread 125–128 s) | was ~208 s — **~38 % faster**; `internal/warpengine`'s floor fell from ~152 s to ~84 s |

All 3 Tier 1 runs and all 3 Tier 2 runs recorded `RESULT: all packages
passed`. Two new packages appear in this run that were not in the
2026-07-12 table at all: `internal/buildercli` and `internal/builderengine`
(the batch-implementation-loop module landed mid-task, on the same branch
this task built on).

#### Cause

The lever is the **hermetic git test environment**
(`lyxtest.HermeticGitEnv()`, wired via `TestMain` into every git-spawning
test package): it stops every git process a test spawns from reading the
operator's global/system gitconfig, which was silently carrying
`core.fsmonitor=true` and causing hundreds of `fsmonitor--daemon` +
auto-`maintenance` background spawns per Tier 2 run (measured: 308 daemon
spawns in one `internal/warpengine` run alone, 60 % of its git
process-seconds). See [fixture-copy.md](fixture-copy.md) for the full
measurement trail, including the original hardlink-objects hypothesis this
task started with and refuted (fixture-copy cost turned out to be ~1–2 % of
the tier, not the floor).

Two supporting fixes landed alongside the lever, both preconditions for a
green suite to measure against:

- **Builder re-tier fix** (batch 1): four untagged test files from the
  freshly-merged builder module (`internal/buildercli/spawnbatch_test.go`,
  `internal/buildercli/validate_test.go`,
  `internal/builderengine/config_test.go`,
  `internal/builderengine/template_test.go`) tripped
  `TestTierPurity_UntaggedTestsSpawnNothing`; tagged `//go:build
  integration`, mechanically, per the guard's own error message.
- **buildercli Tier 1 compile fix** (batch 4): the re-tiering above hid
  helper functions that other untagged sibling test files in the same
  package still referenced, breaking `go test ./...` — invisible to every
  batch's own `-tags integration` verify command, only caught by this
  card's official untagged run. See
  [fixture-copy.md](fixture-copy.md#pre-existing-red-discovered) for detail.

#### Tier 1: where the time goes

Tier 1 is still offline: no `git init` / `git worktree add` / fixture-tree
copies remain in any untagged test file, machine-enforced by
`cmd/lyx/tierpurity_test.go` (`TestTierPurity_UntaggedTestsSpawnNothing`).
This is **not** "zero processes spawned" — untagged tests reaching
`hubgeometry.Resolve` on their error paths (e.g. `boardcli`'s `RunCLI` seam
tests) still spawn one cheap, expected-to-fail `git rev-parse`; the guard
deliberately permits that. Since Tier 1 never spawned real git fixtures,
the hermetic-env lever does not move its floor — the modest ~36 s → ~29 s
shift here is within this suite's run-to-run noise band, not a hermetic-env
effect.

| Package | Tier 1 elapsed (median run) | Cause |
|---------|------------------------------|-------|
| `internal/perchengine` | ~14.8 s | the largest untagged test suite by volume (run-loop / gate / judge / state-machine table-driven unit tests) — real in-process CPU cost, no spawns |
| `cmd/lyx`              | ~12.2 s | repo-wide guard tests (`registration_test.go`, `sandbox_coverage_test.go`, `tierpurity_test.go`, `hermeticenv_test.go`) AST-parse or walk every package's source under the module root — real in-process CPU/disk cost, no process spawns |
| `internal/burlerengine`, `internal/boardengine/boardtest`, `internal/envsource`, `internal/clihelp` | ~8–9 s each | mostly contention-inflated in-memory tests (`clihelp`'s `TestExecute_*`; see the noise note below) |
| everything else        | < 7 s each, noisy | scheduler contention across ~50 parallel test binaries, not a stable per-package cost |

**Attribution noise:** per-package elapsed is inflated by CPU contention (`go
test` runs ~50 packages in parallel on 14 logical CPUs; the sum of package
times is ~173 s against a ~29 s wall-clock — a ~6× overlap). No package does
real git spawning, so this contention remains the dominant source of
per-package variance. Trust the wall-clock; treat per-package numbers as
attribution, not absolute cost.

#### Tier 2: where the time goes

| Package | Tier 2 elapsed (median run) | Where the cost is |
|---------|------------------------------|--------------------|
| `internal/warpengine`            | **~84 s** | real `git worktree` add/remove, junctions, host↔weft topology — floor fell from ~152 s under the hermetic env, matching the ~87 s measured in isolation (see fixture-copy.md) |
| `internal/buildercli`            | ~76 s  | new module (batch-implementation loop): CLI tests over real git scratch repos/host-hub fixtures |
| `internal/builderengine`         | ~75 s  | same new module, facade-level git-spawning tests |
| `internal/boardcli`              | ~59 s  | its own git-spawning CLI tests (`seedCwd` `git init`s + `RunCLI`'s `hubgeometry.Resolve` `git rev-parse`) |
| `internal/initengine`            | ~48 s  | paired-fixture copies per test |
| `internal/boardengine/boardtest` | ~42 s  | real local git commit/push, parallelized |
| `internal/perchcli`              | ~40 s  | `lyxtest.CopyPaired[Local]` fixture copies + weft-sync git assertions |
| `cmd/lyx`                        | ~36 s  | cross-compile + registration/help-tree over the full binary, plus real `git init` in `main_test.go` |
| `internal/perchengine`           | ~29 s  | same in-process run-loop suite as Tier 1 (the untagged tests run again under `-tags integration`; not additional fixture cost) |
| `internal/muxcli`                | ~24 s  | real `tmux`/`psmux` contract-integration tests |

The floor is still `internal/warpengine`, now at ~84 s (down from ~152 s):
its slowest single tests this run cost 11–15 s each
(`TestCleanup_LiveBranchNeverDeleted` 14.5 s, `TestRemove` 13.8 s,
`TestCloneHub_HappyPath` 13.4 s) — the same fixture-copy-plus-real-git shape
as before, just without the fsmonitor/maintenance tax on top.

#### Slowest 10 top-level tests (Tier 1, median run)

| Test | Package | Elapsed |
|------|---------|---------|
| `TestConcurrentReadsDuringUpserts` | `internal/boardengine/boardtest` | 8.1 s |
| `TestExecute_ConcurrentInvocationsDoNotCrossExitCodes` | `internal/clihelp` | 7.7 s¹ |
| `TestWrapRun_ShortCircuitsAfterAbort` | `internal/clihelp` | 6.7 s¹ |
| `TestExecute_UnknownSubcommandReturnsOneAndWritesUnknownCommand` | `internal/clihelp` | 5.8 s¹ |
| `TestExecute_SuccessHandlerReturnsZero` | `internal/clihelp` | 5.7 s¹ |
| `TestExecute_FailHandlerReturnsOne` | `internal/clihelp` | 4.8 s¹ |
| `TestConcurrentUpsertsDoNotLoseWrites` | `internal/boardengine/boardtest` | 2.0 s |
| `TestHelpTree_VerbModuleSubcommands` | `cmd/lyx` | 2.0 s |
| `TestParseReport_Rejections` | `internal/builderengine` | 1.8 s |
| `TestRenderToDiskManifestCleanup` | `internal/boardengine` | 1.7 s |

¹ Pure in-memory cobra-tree tests — their elapsed is contention artifact, not
real cost (see the attribution-noise note above).

#### Slowest 10 top-level tests (Tier 2, median run)

| Test | Package | Elapsed |
|------|---------|---------|
| `TestCLILookupContract` | `internal/boardcli` | 23.7 s |
| `TestCLIStrictPayloadShapes` | `internal/boardcli` | 23.4 s |
| `TestConcurrentReadsDuringUpserts` | `internal/boardengine/boardtest` | 19.1 s |
| `TestCleanup_LiveBranchNeverDeleted` | `internal/warpengine` | 14.5 s |
| `TestRemove` | `internal/warpengine` | 13.8 s |
| `TestMultiplexerContract` | `internal/muxengine` | 13.6 s |
| `TestCloneHub_HappyPath` | `internal/warpengine` | 13.4 s |
| `TestInstallPostCheckoutHook_WeftResolution_Child` | `internal/warpengine` | 12.3 s |
| `TestE2ESyncIntegration` | `internal/configcli` | 12.2 s |
| `TestRunCLI_ResolvesLayoutAndConfig` | `internal/muxcli` | 12.1 s |

### 2026-07-12 — post-fix baseline (superseded by 2026-07-13)

As of **2026-07-12** (post-fix baseline — both Tier 2 reds fixed, Tier 1
offline again). Superseded by the 2026-07-13 hermetic-git-env block above;
frozen here for the trend log.

- Machine: Intel Core Ultra 7 155U, Windows 11 Enterprise, `windows/amd64`, 14 logical CPUs
- Go 1.26.4, default GC, `GOMAXPROCS` = NumCPU (14)
- Method: median of 3 warm runs per tier via `go run ./cmd/testtiming[ -full]`
  (`-count=1` set by the harness; `go build ./...` run first to warm the build
  cache)

#### Headline

| Loop | Command | Wall-clock | vs. 2026-07-12 regression |
|------|---------|-----------|----------------------------|
| **Tier 1** — offline, default | `go test ./... -count=1` | **~36 s** (spread 29–38 s) | was ~44 s — improved, though still well above the 2026-06-23 ~3.5 s floor; see the noise note below |
| **Tier 2** — integration, opt-in | `go test -tags integration ./... -count=1` | **~208 s** (spread 164–236 s) | was ~181 s — but both previously-FAILing packages now pass |

Both Tier 2 reds are fixed: `internal/initengine`'s `TestInit_FirstRun` now derives
its expected module count from `configreg.Modules()` instead of a stale hardcoded
`3`, and `internal/ideengine`'s `Menu` now sets `cfg.Path = hubgeometry.BoardDir(l.Hub)`
before constructing the board, matching the board-dir-geometry migration `boardcli`
already followed. All 3 Tier 1 runs and all 3 Tier 2 runs recorded `RESULT: all
packages passed`.

#### Tier 1: where the time goes

Tier 1 is offline again: no `git init` / `git worktree add` / fixture-tree copies
remain in any untagged test file, machine-enforced by
`cmd/lyx/tierpurity_test.go` (`TestTierPurity_UntaggedTestsSpawnNothing`). This is
**not** "zero processes spawned" — untagged tests reaching `hubgeometry.Resolve` on
their error paths (e.g. `boardcli`'s `RunCLI` seam tests) still spawn one cheap,
expected-to-fail `git rev-parse`; the guard deliberately permits that (a single
failing `rev-parse` is cheap; the guard targets fixtures and loops, not every
subprocess).

| Package | Tier 1 elapsed (median run) | Cause |
|---------|------------------------------|-------|
| `internal/perchengine` | ~20.8 s | the largest untagged test suite by volume (~4,100 lines across 10 files: run-loop / gate / judge / state-machine table-driven unit tests) — real in-process CPU cost, no spawns |
| `cmd/lyx`              | ~19.2 s | three repo-wide guard tests (`registration_test.go`, `sandbox_coverage_test.go`, `tierpurity_test.go`) AST-parse or walk every package's source under the module root — real in-process CPU/disk cost, no process spawns |
| everything else        | < 12 s each, noisy | scheduler contention across ~50 parallel test binaries (see the noise note below), not a stable per-package cost |

**Attribution noise:** per-package elapsed is inflated by CPU contention (`go
test` runs ~50 packages in parallel on 14 logical CPUs; the sum of package
times is ~255 s against a ~36 s wall-clock — a ~7× overlap). Now that no
package does real git spawning, this contention is the *dominant* source of
per-package variance: package rankings shuffle significantly between runs
(e.g. `internal/lock` ranged 4–12 s across the 3 runs with no code-level
explanation). Trust the wall-clock; treat per-package numbers as attribution,
not absolute cost.

#### Tier 2: where the time goes

| Package | Tier 2 elapsed (median run) | Where the cost is |
|---------|------------------------------|--------------------|
| `internal/warpengine`            | **~152 s** | real `git worktree` add/remove, junctions, host↔weft topology — unchanged floor from the regression baseline |
| `internal/boardcli`              | ~63 s  | its own re-tiered git-spawning CLI tests (`seedCwd` `git init`s + `RunCLI`'s `hubgeometry.Resolve` `git rev-parse`), now fully in Tier 2 |
| `internal/perchcli`              | ~63 s  | re-tiered `lyxtest.CopyPaired[Local]` fixture copies + weft-sync git assertions |
| `internal/initengine`            | ~50 s  | paired-fixture copies per test (fixed — no longer FAILs) |
| `internal/boardengine/boardtest` | ~47 s  | real local git commit/push, parallelized |
| `internal/perchengine`           | ~46 s  | same in-process run-loop suite as Tier 1 (the untagged tests run again under `-tags integration`; not additional fixture cost) |
| `internal/configcli`             | ~40 s  | re-tiered `git init` tests |
| `internal/muxengine`             | ~38 s  | real `tmux`/`psmux` contract-integration tests |
| `internal/warpcli`               | ~37 s  | CLI wrapper over warpengine clone/teardown paths |
| `internal/ideengine`             | ~35 s  | fixed — board health-check tests over real board directories |

The floor is still `internal/warpengine`: its slowest single tests this run cost
25–39 s each (`TestCleanup_LiveBranchNeverDeleted` 39.4 s,
`TestWeftForkPointSubtaskIsolation` 30.9 s,
`TestWeftRollbackOnPostHostCreateFailure` 28.1 s) — the same deterministic
fixture-copy-plus-real-git floor the regression baseline recorded; re-tiering
does not touch it, since warpengine's cost was already gated.

#### Slowest 10 top-level tests (Tier 1, median run)

| Test | Package | Elapsed |
|------|---------|---------|
| `TestConcurrentReadsDuringUpserts` | `internal/boardengine/boardtest` | 9.7 s |
| `TestExecute_ConcurrentInvocationsDoNotCrossExitCodes` | `internal/clihelp` | 6.7 s¹ |
| `TestExecute_SuccessHandlerReturnsZero` | `internal/clihelp` | 6.0 s¹ |
| `TestExecute_FailHandlerReturnsOne` | `internal/clihelp` | 5.5 s¹ |
| `TestWrapRun_ShortCircuitsAfterAbort` | `internal/clihelp` | 5.4 s¹ |
| `TestExecute_UnknownSubcommandReturnsOneAndWritesUnknownCommand` | `internal/clihelp` | 4.3 s¹ |
| `TestHelpTree_VerbModuleSubcommands` | `cmd/lyx` | 1.6 s |
| `TestHelpSchema_LeafCommands` | `internal/boardcli` | 1.4 s |
| `TestRunCLI_Run_FlagValidation` | `internal/shuttlecli` | 1.2 s |
| `TestMountedUnknownSubcommand` | `cmd/lyx` | 1.0 s |

¹ Pure in-memory cobra-tree tests — their elapsed is contention artifact, not
real cost (see the attribution-noise note above).

### 2026-07-12 — regression baseline (pre-fix state)

As of **2026-07-12** (fresh baseline — **regression recorded, not yet fixed**).

- Machine: Intel Core Ultra 7 155U, Windows 11 Enterprise, `windows/amd64`, 14 logical CPUs
- Go 1.26.4, default GC, `GOMAXPROCS` = NumCPU (14)
- Method: median of 3 warm runs per tier via `go run ./cmd/testtiming[ -full]`
  (`-count=1` set by the harness)

#### Headline

| Loop | Command | Wall-clock | vs. 2026-06-23 |
|------|---------|-----------|----------------|
| **Tier 1** — offline, default | `go test ./... -count=1` | **~44 s** (spread 43–49 s) | was ~3.5 s — **~13× regression** |
| **Tier 2** — integration, opt-in | `go test -tags integration ./... -count=1` | **~181 s** (spread 173–238 s) | was ~65 s — **~2.8× regression**, plus **2 packages FAIL** |

The ~a-dozen modules landed since 2026-06-23 (mux/shuttle/burler/perch/warp/
stencil/selfreport/modelspec/…) each brought tests; the regression is real
execution time, not compilation.

#### Two RED packages (Tier 2)

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

#### Tier 1: where the time goes

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

#### Tier 2: where the time goes

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

#### Slowest 10 top-level tests (Tier 1, median run)

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

### 2026-06-23 — state after `optimize-integration-tier` (was "Current best times")

- Machine: Intel Core Ultra 7 155U, Windows 11 Enterprise, `windows/amd64`, 14 logical CPUs
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
