# Benchmarks: board command performance

Tracks how fast the board commands run, and how that changes across revisions.
The benchmark suite lives in [`internal/boardengine/boardtest`](../../internal/boardengine/boardtest).

Since the async-sync change (see [overview.md#modules](../overview.md#modules)) a write only
touches the filesystem and returns; the git round-trip happens in a detached
background `Sync`. So the suites split into the **hot path** (writes + reads, no
git) and the **background sync** (git).

## How to run

```sh
# Hot path (default): writes + reads, git skipped (BOARD_SKIP_GIT=1).
go test -run '^$' -bench . -benchmem ./internal/boardengine/boardtest

```

Board size (number of tasks already in `tasks.json`) is swept over 10 / 100 / 1000
to show how cost scales — every write re-renders all tasks.

## What the suites mean

- **Hot path** — what a `lyx board` command actually waits for: file I/O for a
  write, JSON load for a read, plus configuration loading (os.Getwd() + config
  resolution from defaults + the module's `_lyx/config/<module>.yaml`). This is the stable signal for catching
  logic/allocation regressions.
- **Background sync** — the git commit + push the detached pusher does. It never
  blocks a command, so its seconds-scale cost is latency the user does not feel.
- Reads (`get`, `list`) never touch git but include the configuration-load cost
  from the cwd model.

## Results

Numbers are wall-clock per op and **noisy**, so treat them as order-of-magnitude,
not precise. Each dated block names its OS in the `Machine:` line — Windows blocks
were measured with Cortex XDR live (file I/O + AV + GC); the
[Linux baseline](#2026-07-13--linux-baseline-ubuntu-2604) has no such tax, so do
not compare a Windows row against a Linux one. Record a new block per revision (or
per OS) rather than editing the old one, so each trend stays visible.

### 2026-06-08 — async git sync

- Machine: Intel Core Ultra 7 155U, Windows 11 Enterprise, `windows/amd64`, 14 logical CPUs, Go default GC
- Endpoint security active (≈30 ms process-creation tax — see below)

Hot path, in-process (`go test -bench . -benchmem`, default benchtime):

| Benchmark            | n=10    | n=100   | n=1000   |
|----------------------|---------|---------|----------|
| Render (pure)        | 0.03 ms | 0.28 ms | 3.5 ms   |
| Upsert (CLI)         | 10.4 ms | 18.2 ms | 30.6 ms  |
| UpsertFacade         | 10.8 ms | 11.8 ms | 27.8 ms  |
| Get                  | 0.77 ms | 1.52 ms | 4.59 ms  |
| List                 | 0.45 ms | 1.20 ms | 7.91 ms  |
| GetDuringUpsert*     | —       | 0.78 ms | —        |

`Render` (tasks → markdown, no I/O) runs once inside every write; at a fraction of
a millisecond for realistic board sizes it is a small part of an `Upsert`.

**Note:** The CLI-driven benchmarks (`Upsert`/`Get`/`List`) were re-architected
when the module moved to the cwd-authoritative configuration model. Previously
they used `--wiki-path` to inject a board directory; with the new `LoadConfig`
resolver, the CLI-path benchmarks now run in a temp cwd seeded with
`_lyx/config/board.yaml` and include the `os.Getwd()` + configuration-load cost
(defaults + the module's `_lyx/config/board.yaml` + environment expansion). The historical numbers
below show the pre-config baseline for comparison.

\* `GetDuringUpsert` reads (seed n=100) while a writer upserts continuously in the
background. At 0.78 ms vs 1.52 ms single-threaded `Get`, reads stay fast under
write load — the swap lock fences readers out only for the rename's microseconds.

Write latency end-to-end (warm binary, git-bash wall-clock, includes process
startup and configuration load):

| Path                                    | wall-clock |
|-----------------------------------------|------------|
| file-only write, no sync (BOARD_SKIP_GIT)| ~205 ms    |
| file-only write + detached sync spawn   | ~235 ms    |

The spawn adds only ~30 ms; a *cold* (just-built) binary's first spawn costs ~1 s
while endpoint security scans the image, then warms up. The ~200 ms floor is
process startup (git-bash launch + `CreateProcess`), not wiki work — the
in-process write is the ~10–18 ms `Upsert` row above.

**Note:** The integration tier no longer benchmarks against a remote — all git tests
are now local and deterministic. Historical network benchmarks (SyncGit, SyncGitNoPush)
have been removed.

### 2026-07-13 — Linux baseline (Ubuntu 26.04)

First Linux measurement, recorded in parallel with the Windows block above.
**Compare down each OS's column, not across** — the Windows box ran Cortex XDR
(file I/O + AV throttling), the Linux box has no equivalent, so faster Linux
numbers mostly measure the absent AV tax. See
[linux-portability-survey.md](../research/linux-portability-survey.md) for the
portability pass that made the suite runnable on Linux.

- Machine: AMD Ryzen AI 7 445 w/ Radeon 840M, Ubuntu 26.04 LTS, `linux/amd64`, 12 logical CPUs, Go 1.26.0, default GC

Hot path — `Render` (pure, tasks → markdown, no I/O), `go test -bench . -benchmem`:

| Benchmark     | n=10     | n=100    | n=1000   |
|---------------|----------|----------|----------|
| Render (pure) | 0.016 ms | 0.089 ms | 1.05 ms  |

Windows measured 0.03 / 0.28 / 3.5 ms for the same rows — Linux is ~2–3× faster
even on this pure-CPU path (no I/O involved; the delta is CPU/allocator, not AV).

CLI-driven commands and the Board facade — CLI rows via
`go test -tags integration -run '^$' -bench 'Upsert|Get|List' -benchmem` (they
drive `boardcli.RunCLI`, whose config resolution spawns `git rev-parse`, so they
now live behind `//go:build integration`; see `bench_cli_test.go`), facade rows
via the untagged `-bench UpsertFacade`:

| Benchmark      | n=10    | n=100   | n=1000  |
|----------------|---------|---------|---------|
| Upsert (CLI)   | 2.56 ms | 2.30 ms | 2.29 ms |
| Get (CLI)      | 2.11 ms | 2.11 ms | 2.33 ms |
| List (CLI)     | 2.09 ms | 2.22 ms | 2.25 ms |
| UpsertFacade   | 0.12 ms | 0.41 ms | 3.91 ms |

Two things to read here, both Linux-specific:

- **The CLI rows are flat across board size** (~2.3 ms regardless of n), unlike
  Windows (Upsert scaled 10.4 → 18.2 → 30.6 ms). On Linux the per-command cost is
  dominated by a fixed ~2 ms floor — the `git rev-parse` config resolution
  (`hubgeometry.Resolve`) plus CLI/config-load overhead paid once per command —
  which swamps the sub-millisecond render work even at n=1000. The
  **`UpsertFacade`** row (same write, but the facade bypasses CLI + config
  resolution) is the one that scales with board size (0.12 → 3.91 ms), because it
  is pure board logic with no per-call git.
- **Do not compare these against the historical Windows CLI rows.** Those numbers
  (Get 0.77 ms, Upsert 10–30 ms) predate the current git-requiring config
  resolution — a Windows `git rev-parse` alone is ~72 ms (see
  [fixture-copy.md](fixture-copy.md#process-spawn-cost-the-real-floor)), so those
  sub-5 ms Get rows cannot have included it. Treat the Linux block as its own
  baseline; the facade row is the only apples-to-apples cross-OS write cost.

Process startup floor (native no-op Go exe, 50 sequential spawns, bash `for`):

| Launcher            | ms / process |
|---------------------|--------------|
| bash (`for`), Linux | ~0.6         |

vs Windows ~30 ms (cmd) / ~78 ms (git-bash) for the same no-op exe — a ~50–130×
gap that is the Windows `CreateProcess`-interception/AV-scan tax, not a Go cost
(a Go binary starts in single-digit ms on a clean machine, as
[the Windows block](#process-startup-context) already noted). With git off the
hot path, this near-zero Linux spawn floor means a board write command's cost on
Linux is essentially just the in-process `Render` + file write.

### Pre-config baseline — synchronous writes (historic reference)

Kept for history. At this earlier revision every write did `pull → commit → push`
synchronously, so the command waited the full git round-trip. Benchmarks measured
`UpsertGit` (the whole write incl. push). This is also before the
cwd-authoritative configuration model, so no config-load cost.

| Benchmark            | n=10    | n=100   | n=1000   |
|----------------------|---------|---------|----------|
| Upsert (no-git)      | 10.7 ms | 10.2 ms | 21.0 ms  |
| Get                  | 0.40 ms | 0.76 ms | 4.47 ms  |
| List                 | 0.63 ms | 1.18 ms | 7.99 ms  |

| Git-backed write  | ns/op   | Notes                                |
|-------------------|---------|--------------------------------------|
| UpsertGitNoPush   | ~1.35 s | pull + local commit, no push         |
| UpsertGit (push)  | ~4.42 s | full pull + commit + push (synchronous, blocked the command) |

## Process startup context

Every `lyx` invocation is a fresh process. Measured startup on this machine
(50× a no-op `lyx`, by launcher):

| Launcher            | ms / process |
|---------------------|--------------|
| cmd (`for /l`)      | ~30          |
| PowerShell 5.1      | ~43          |
| PowerShell 7.6      | ~46          |
| git-bash (MSYS)     | ~78          |

A comparable Go binary starts in ~2–5 ms on a clean machine, so ~30 ms here is
the OS + endpoint-security (`CreateProcess` interception/scan) floor, paid by
native exes too — not a Go cost. With git off the hot path, this startup floor is
now the dominant cost of a write command.