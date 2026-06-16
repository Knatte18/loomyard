# Benchmarks: board command performance

Tracks how fast the board commands run, and how that changes across revisions.
The benchmark suite lives in [`internal/board/boardtest`](../../internal/board/boardtest).

Since the async-sync change (see [board.md](../modules/board.md#background-sync)) a write only
touches the filesystem and returns; the git round-trip happens in a detached
background `Sync`. So the suites split into the **hot path** (writes + reads, no
git) and the **background sync** (git).

## How to run

```sh
# Hot path (default): writes + reads, git skipped (BOARD_SKIP_GIT=1).
go test -run '^$' -bench . -benchmem ./internal/board/boardtest

# Background sync suite: real commit/push against the dummy board.
# Network + push access to github.com/Knatte18/loomyard-test required.
go test -tags integration -run '^$' -bench SyncGit -benchmem -benchtime=10x ./internal/board/boardtest
```

Board size (number of tasks already in `tasks.json`) is swept over 10 / 100 / 1000
to show how cost scales — every write re-renders all tasks.

## What the suites mean

- **Hot path** — what a `lyx board` command actually waits for: file I/O for a
  write, JSON load for a read, plus configuration loading (os.Getwd() + config
  resolution from defaults + the module's `_lyx/<module>.yaml`). This is the stable signal for catching
  logic/allocation regressions.
- **Background sync** — the git commit + push the detached pusher does. It never
  blocks a command, so its seconds-scale cost is latency the user does not feel.
- Reads (`get`, `list`) never touch git but include the configuration-load cost
  from the cwd model.

## Results

Numbers are wall-clock per op on Windows; they are **noisy** (Windows file I/O +
Defender + GC), so treat them as order-of-magnitude, not precise. Record a new
block per revision rather than editing the old one, so the trend stays visible.

### 2026-06-08 — async git sync

- Machine: Intel Core Ultra 7 155U, `windows/amd64`, Go default GC
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
`_lyx/board.yaml` and include the `os.Getwd()` + configuration-load cost
(defaults + the module's `_lyx/board.yaml` + environment expansion). The historical numbers
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

Background sync (`-tags integration -bench SyncGit -benchtime=10x`):

| Benchmark         | ns/op   | Notes                              |
|-------------------|---------|------------------------------------|
| SyncGitNoPush     | ~0.7 s  | commit only (BOARD_SKIP_PUSH=1)    |
| SyncGit           | ~4.5 s  | commit + push to the remote        |

**Takeaways:**
- A write returns in ~0.2 s (startup-bound), versus the ~4.4 s it took when the
  push was synchronous (see the pre-async baseline below). Git is no longer on
  the hot path.
- The git cost did not go away — it moved to the background sync (~4.5 s with
  push). The user just never waits for it; a burst of writes coalesces into ~1
  push.
- Reads and writes still scale with board size (JSON unmarshal / re-render of all
  tasks), but at single-digit milliseconds that is dwarfed by process startup.
- Configuration loading (os.Getwd() + YAML merge + env expansion) is now part of
  every CLI invocation but remains sub-millisecond; the startup floor dominates.

### Pre-config baseline — synchronous writes (historic reference)

Kept for history. At this earlier revision every write did `pull → commit → push`
synchronously, so the command waited the full git round-trip. Benchmarks measured
`UpsertGit` (the whole write incl. push) rather than `SyncGit`. This is also
before the cwd-authoritative configuration model, so no config-load cost.

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

## Push access

`BenchmarkSyncGit` (and `TestIntegrationCommitPush`) push to
`github.com/Knatte18/loomyard-test`, so the machine's git credential needs write
access to that repo (granted via collaborator access). Without it, push returns
HTTP 403 and only the no-push / hot-path suites can run. The repo URL is unchanged
from earlier revisions and continues to serve as the integration test backend.
