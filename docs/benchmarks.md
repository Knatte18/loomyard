# Benchmarks: wiki command performance

Tracks how fast the wiki commands run, and how that changes across revisions.
The benchmark suite lives in [`internal/wiki/wikitest`](../internal/wiki/wikitest).

## How to run

```sh
# No-git suite (default): pure wiki logic + file I/O, git skipped.
go test -run '^$' -bench . -benchmem ./internal/wiki/wikitest

# Git-backed write suite: real pull/commit/push against the dummy wiki.
# Network + push access to github.com/Knatte18/mhgo-wiki-test required.
go test -tags integration -run '^$' -bench UpsertGit -benchmem -benchtime=10x ./internal/wiki/wikitest
```

The no-git benchmarks set `WIKI_SKIP_GIT=1`; the git ones clone the dummy wiki
fresh per run. Wiki size (number of tasks already in `tasks.json`) is swept over
10 / 100 / 1000 to show how cost scales — every write re-renders all tasks.

## Why two suites

A real write command's time is dominated by **git**, not by wiki logic. The
no-git suite is the stable signal for catching logic/allocation regressions; the
git suite shows the real-world latency a user feels. Reads (`get`, `list`) never
touch git, so they have no git variant.

## Results

Numbers are wall-clock per op on Windows; they are **noisy** (Windows file I/O +
Defender + GC), so treat them as order-of-magnitude, not precise. Record a new
block per revision rather than editing the old one, so the trend stays visible.

### 2026-06-08 — baseline (Go port + swap-lock concurrency fix)

- Commit: `69b02ef` + uncommitted swap-lock/bench work (this commit)
- Machine: Intel Core Ultra 7 155U, `windows/amd64`, Go default GC
- Endpoint security active (≈30 ms process-creation tax — see below)

No-git (`go test -bench . -benchmem`, default benchtime):

| Benchmark            | n=10    | n=100   | n=1000   |
|----------------------|---------|---------|----------|
| Upsert (CLI)         | 10.7 ms | 10.2 ms | 21.0 ms  |
| UpsertFacade         | 15.5 ms | 15.8 ms | 37.7 ms  |
| Get                  | 0.40 ms | 0.76 ms | 4.47 ms  |
| List                 | 0.63 ms | 1.18 ms | 7.99 ms  |
| GetDuringUpsert*     | —       | 0.85 ms | —        |

\* `GetDuringUpsert` reads (seed n=100) while a writer upserts continuously in the
background. At 0.85 ms vs 0.76 ms uncontended, reads stay fast under write load —
the swap lock fences readers out only for the microseconds of the rename.

Git-backed write (`-tags integration -bench UpsertGit -benchtime=10x`):

| Benchmark           | ns/op    | Notes                                       |
|---------------------|----------|---------------------------------------------|
| UpsertGitNoPush     | ~1.35 s  | pull (network) + local commit, no push      |
| UpsertGit (push)    | ~4.42 s  | full pull + commit + push to the remote     |

**Takeaways:**
- Git is essentially the entire write latency. Full push is ~550× the no-git
  path (≈4.42 s vs ≈8 ms at n=100); even without push, pull + commit is ≈1.35 s.
  The push leg alone is ≈3 s. The cost is the network round-trips plus several
  git subprocess spawns, each paying the process-creation tax.
- Reads scale with wiki size (JSON unmarshal of all tasks); writes scale with
  size too (re-render of all tasks) but are dwarfed by git when git is on.

## Process startup context

Every `mhgo` invocation is a fresh process. Measured startup on this machine
(50× a no-op `mhgo`, by launcher):

| Launcher            | ms / process |
|---------------------|--------------|
| cmd (`for /l`)      | ~30          |
| PowerShell 5.1      | ~43          |
| PowerShell 7.6      | ~46          |
| git-bash (MSYS)     | ~78          |

A comparable Go binary starts in ~2–5 ms on a clean machine, so ~30 ms here is
the OS + endpoint-security (`CreateProcess` interception/scan) floor, paid by
native exes too — not a Go cost. This ~30 ms floor exceeds most no-git wiki
operations, so per-command responsiveness is bounded by process startup, not by
anything in this code.

## Push access

`BenchmarkUpsertGit` (and `TestIntegrationCommitPush`) push to
`github.com/Knatte18/mhgo-wiki-test`, so the machine's git credential needs
write access to that repo (it is granted via collaborator access). Without it,
push returns HTTP 403 and only the no-push / no-git suites can run.
