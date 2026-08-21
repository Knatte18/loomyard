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
| **Two-tier split, machine-enforced** (`//go:build integration` + `cmd/lyx/tierpurity_test.go`) | The single ~82 s loop became ~3.5 s / ~42 s | Tier 1 spawns no `git init` / `git worktree add` / fixture-tree copies repo-wide. Not "zero processes" — untagged tests reaching `hubgeometry.Resolve` on error paths still spawn one cheap, expected-to-fail `git rev-parse`, which the guard deliberately permits. |

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

The 2026-07-13 mousetrap block corrected two earlier causal claims: `cmd/lyx`'s guard tests cost ~0.25 s combined in isolation (not the AST-walk cost earlier blocks attributed to them), and 44 of a since-retired module's 45 tests summed to under 1 s (its earlier 12–19 s was contention attribution plus the one lingering-child test).
Both were parallel-contention artifacts, which is the standing hazard when reading per-package numbers.
