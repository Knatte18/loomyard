# Running the tests

How to run Loomyard's Go test suite, what the four tiers mean,
and the timing harness that produces the tables in [test-suite-timing.md](test-suite-timing.md).
For the recorded numbers themselves, see that file — this one is the "how", not the "how fast".

## The four tiers

The suite is split into four tiers, and which tier a test belongs to is decided by one rule: **a test earns an opt-in build tag for touching a real substrate, never merely for being slow.**
"Substrate" means one of a fixed set of categories a hermetic, in-process unit test cannot fake: real `git` subprocess spawning, real filesystem junction/symlink creation, real `tmux` sessions, and real cross-compilation.
A test that is merely slow — a big table-driven case, a large in-memory fixture — stays untagged in `tier0`.

The hub declares its tiers in `gate.yaml`, numbered `tier0` through `tier3`, and `lyx gate test --tier <n>` runs one over a scope lyx computes or `--all`.

- **`tier0` — the default offline loop** (`lyx gate test --tier 0`): pure-unit and static-guard tests only.
  No `git init` / `git worktree add` / fixture-tree copies anywhere in an untagged test — that is the tier's **premise** (a cheap, expected-to-fail `git rev-parse` on an error path, e.g. via `hubgeometry.Resolve`, is still allowed and does not violate it).
  Machine- enforced by `cmd/lyx/tierpurity_test.go` (`TestTierPurity_UntaggedTestsSpawnNothing`).
  Fast again: measured median ~29 s on Windows (Cortex XDR), ~1 s on Linux.
  This is what you run constantly and what must stay fast.
- **`tier1` — the opt-in integration loop** (`lyx gate test --tier 1`): `tier0` **plus** the gated tests that spawn one of the substrate categories above — real `git` (worktrees, commits, pushes, junctions), real filesystem junctions/symlinks, real cross-compilation, or real external-binary spawn; a test that drives real `tmux` sessions belongs to `tier2` instead.
  It is slow **by design** — it does far more work.
  Measured median ~128 s on Windows (Cortex XDR), ~5 s on Linux.
  Numbers across machines and operating systems: [test-suite-timing.md](test-suite-timing.md#all-environments).
  Every git-spawning test package runs under the **Hermetic Git Test Environment Invariant** (`PATTERN.md`): a `TestMain` wires in `gitkit.HermeticGitEnv()` before any test spawns git, which is what keeps this tier's git processes from inheriting the operator's global `~/.gitconfig` (and the `fsmonitor--daemon`/auto-`maintenance` spawns that config can trigger) — see [fixture-copy.md](fixture-copy.md) for the measured before/after.

> **`tier1` is not a regression of `tier0`.** The heavy git work used to run inside the default loop and made it slow (~82 s historically); the two-tier split moved that work behind `-tags integration`. Same work, now off the default path. When reading a timing table, compare _down_ a column (is this package fast in the loop I run?), never _across_ (`tier0` vs `tier1` are not comparable — `tier1` is the superset).

Two further opt-in tags exist alongside `integration`, each gating a distinct kind of live substrate rather than widening `integration` itself:

- **`tier2` — `tmux`** (`lyx gate test --tier 2`): tests that start a real tmux server and drive real panes.
  It needs `tmux` on `$PATH` and spends no tokens.
  A test that only probes `tmux has-session` stays in `integration`.
- **`tier3` — `llm`** (`go vet -tags llm ./...` to compile; `lyx gate test --tier 3 <package> -- -run <Test>` by hand to run): tests that spawn a real `claude` session.
  It needs a logged-in `claude` and costs tokens, wall-clock and RAM.
  No gate ever runs `llm`; the plan verify and the card gates only compile it.

The tags do not nest: `-tags tier1` does not run `tier0`'s tests and does not include `tier2` or `tier3`, and a run that wants several names them all (`-tags tier1,tier2`).
`cmd/lyx/tierpurity_test.go` holds the untagged tier to its premise and `cmd/lyx/llmtier_test.go` keeps LLM spawns inside `llm`.
The rules behind the tiers live in `PATTERN-test-speed` in `PATTERN.md`, which this page does not restate.

## Commands

```sh
# tier0 — default / offline loop. No build tag. (Premise: no `git init` /
# `git worktree add` / fixture-tree copies — see test-suite-timing.md.)
lyx gate test --tier 0 --all

# tier1 — gated integration loop. Real worktrees, commits, pushes, junctions.
lyx gate test --tier 1 --all

# tier2 — real tmux servers, no tokens.
lyx gate test --tier 2 --all

# tier3 — real LLM sessions: compile only; run one test by hand, never in a gate.
go vet -tags llm ./...
lyx gate test --tier 3 ./internal/<package> -- -run <TestName>

# One package of a tier, verbose, with per-test seconds.
lyx gate test --tier 0 ./internal/fabricengine -- -v
```

`-count=1` disables the test cache so every run is honest;
without it, unchanged packages report `(cached)` in ~0 s and the numbers lie.

## Timing harness — `cmd/testtiming`

The simplest way to get a sorted timing table is the bundled harness.
It runs the suite and prints per-package times, the measured wall-clock,
and the slowest top-level tests.
No arguments needed;
it works the same outside any editor.

```sh
# Fast: tier0 (offline), run as -tags tier0. Windows ~29 s / Linux ~1 s (median of 3, 2026-07-13).
go run ./cmd/testtiming

# Full: tier1 (real git), run as -tags tier1, which does not run tier0's tests. Windows ~128 s / Linux ~5 s (median of 3, 2026-07-13).
go run ./cmd/testtiming -full

# Show more (or fewer) of the slowest tests (default 15).
go run ./cmd/testtiming -full -top 30

# Any tag set, e.g. the tmux tier (refused together with -full).
go run ./cmd/testtiming -tags tier2
```

It shells out to `go test ./... -json -count=1 -tags <tags>` (`tier0` with no tags of its own, `tier1` in full mode), so it needs nothing beyond a working Go toolchain.
The tiers are disjoint, so a run that wants several names every tag.
A run under a tier above `tier0` builds `lyx` once and hands it to every test binary through the `LYX_PREBUILT_LYX` variable, as `lyx gate test --tags` does, and a `tier0` run builds nothing and strips an inherited value.
`lyx gate test --tags <tags> <packages>` is where one `lyx` build serves every package of the run.
Exit code mirrors `go test`: `0` on success, `1` if any package fails to build or any test fails (failing rows are marked `FAIL` in the table).

`go run ./cmd/testtiming -redundancy` writes the per-test coverage redundancy report; the committed report is [test-redundancy.md](test-redundancy.md).
The before and after measurement of the six-package prune is [test-suite-prune-core.md](test-suite-prune-core.md).
The before and after measurement of the `test-suite-prune-rest` task is [test-suite-prune-rest.md](test-suite-prune-rest.md).
The report judges a test unless it may run this module's code in another process.
What that excludes, and the hand check that backs up the static scan, are in `pattern/PATTERN-test-economy.md`.
A candidate the prune keeps carries `//testtiming:keep <reason>` on the line directly above its `func Test…` line.
The report lists it under "Kept" with the reason,
and an empty reason or a misplaced directive aborts the run.
`-pkg` takes comma-separated package patterns (`-pkg ./internal/a,./internal/b`).
A run over every package of the module, or with no report at `-out` yet, writes the whole report.
Any other run rewrites only the sections of the packages it measured and leaves the header and the other sections byte-identical.
A run whose `-tags` differ from the report's header refuses and writes nothing.

`go run ./cmd/testtiming -resources` measures what each package costs instead of how long it takes: wall time, CPU, peak memory, processes started, processes left behind and the git processes it ran, split into fixture setup and code under test.
It runs one package at a time inside one gate slot, so it needs no `lyx gate test` wrapper; every `cmd/testtiming` mode holds one slot for its whole run in a hub.
The committed report is [test-suite-weight.md](test-suite-weight.md), regenerated by three commands, one per measured tier:

```sh
go run ./cmd/testtiming -resources
go run ./cmd/testtiming -resources -tags tier1
go run ./cmd/testtiming -resources -tags tier2
```

CPU and peak memory come from the package's systemd scope cgroup when `systemd-run --user --scope` works, and from the child's rusage otherwise, where peak memory is the largest single process rather than the sum.
The process count is system-wide, so a measurement reads "quiet machine" as a precondition: run it with nothing else using the machine, and read the load averages at the end of each table.
The `llm` tier is not in the report, because a run spends tokens and its time and CPU are the model's; `go run ./cmd/testtiming -resources -tags tier3` measures it by hand.

Example (tier0):

```
Running Tier 0 (offline)  —  go test -tags tier0 ./... -count=1

PACKAGE                                   ELAPSED
----------------------------------------  --------
internal/boardengine/boardtest                1.49s
cmd/lyx                                       0.93s
...
internal/git                              (no test files)

Wall-clock: 2.78s   (sum of package times: 7.91s across 17 packages)

Slowest 15 top-level tests
...
RESULT: all packages passed
```

## `LYX_TRACE=1` and the durable sink

`internal/logger`'s durable trace sink (`ensureDurableSink`) short-circuits under `testing.Testing()` unless `LYX_TRACE=1` is set, and its `sinkOnce` is process-wide, so the first call to log in a test binary pins the trace directory for every subsequent call in that same process, for the rest of that binary's run.

Before the chdir-to-`RunCLIIn` migration (see [test-suite-timing.md](test-suite-timing.md#trend-log)'s 2026-08-13 row), a test process that chdir'd across several fixture hubs left the sink pinned against whichever hub happened to log first — an arbitrary answer, since it depended on test execution order within the binary.
After the migration, the process cwd itself never moves (every test drives its CLI seam at an explicit cwd instead), so the sink resolves against the repo worktree the test binary was launched from — a deterministic answer, and the same one every time.

This is a change from one arbitrary answer to one predictable answer, not a fix and not a regression: per-test, hub-accurate trace directories were never delivered before the migration and still are not after it, since the sink is still pinned once per process rather than re-resolved per test.
No code change was made to `internal/logger` as part of this migration — the disposition above is a side effect of the CLI test suite no longer moving the process, not a behavior change in the sink itself.

## Reducing wall-clock

If the suite feels slow locally, the highest-leverage levers, in order:

1. **Rely on the test cache** — a tier's command keeps the cache warm;
   only changed packages re-run, so a no-op tier run returns in ~1 s.
2. **Scope to the package you're editing** — `lyx gate test --tier 0 ./internal/fabricengine` beats the whole repo.
3. **Stay in the offline tier.**
   `tier0` spawns no `git init` / `git worktree add` / fixture-tree copies repo-wide (see [test-suite-timing.md](test-suite-timing.md#levers-that-moved-the-numbers)).
   Only reach for `tier1` when you are changing fabric / hubgeometry / board / ide git behaviour — and budget ~128 s (~2 min) for that tier.
