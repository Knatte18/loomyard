# Running the tests

How to run Loomyard's Go test suite, what the two tiers mean, and the timing
harness that produces the tables in
[test-suite-timing.md](test-suite-timing.md). For the recorded numbers
themselves, see that file — this one is the "how", not the "how fast".

## The two tiers

The suite is split into two tiers. **They are different test sets, not the same
tests run twice.**

- **Tier 1 — the default offline loop** (`go test ./...`): pure-unit and
  static-guard tests only. Zero `git` subprocesses, no network. This is what you
  run constantly and what must stay fast (~3.5 s).
- **Tier 2 — the opt-in integration loop** (`go test -tags integration ./...`):
  Tier 1 **plus** the gated tests that spawn real `git` (worktrees, commits,
  pushes, junctions). It is slow **by design** — it does far more work
  (~a minute).

> **Tier 2 is not a regression of Tier 1.** The heavy git work used to run inside
> the default loop and made it slow (~82 s historically); the two-tier split moved
> that work behind `-tags integration`. Same work, now off the default path. When
> reading a timing table, compare _down_ a column (is this package fast in the loop
> I run?), never _across_ (Tier 1 vs Tier 2 are not comparable — Tier 2 is the
> superset).

## Commands

```sh
# Tier 1 — default / offline loop. No build tag. Spawns zero git subprocesses.
go test ./... -count=1

# Tier 2 — gated integration loop. Real worktrees, commits, pushes, junctions.
go test -tags integration ./... -count=1

# Per-test timing, structured (parse Elapsed from the JSON stream).
go test ./... -count=1 -json

# One package, verbose, with per-test seconds.
go test ./internal/weftengine -count=1 -v
```

`-count=1` disables the test cache so every run is honest; without it, unchanged
packages report `(cached)` in ~0 s and the numbers lie.

## Timing harness — `cmd/testtiming`

The simplest way to get a sorted timing table is the bundled harness. It runs the
suite and prints per-package times, the measured wall-clock, and the slowest
top-level tests. No arguments needed; it works the same outside any editor.

```sh
# Fast: Tier 1 (offline). Takes a few seconds.
go run ./cmd/testtiming

# Full: Tier 2 (integration, real git). Takes ~a minute.
go run ./cmd/testtiming -full

# Show more (or fewer) of the slowest tests (default 15).
go run ./cmd/testtiming -full -top 30
```

It shells out to `go test ./... -json -count=1` (adding `-tags integration` in
full mode), so it needs nothing beyond a working Go toolchain. Exit code mirrors
`go test`: `0` on success, `1` if any package fails to build or any test fails
(failing rows are marked `FAIL` in the table).

Example (Tier 1):

```
Running Tier 1 (offline)  —  go test ./... -count=1

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

## Reducing wall-clock

If the suite feels slow locally, the highest-leverage levers, in order:

1. **Rely on the test cache** — drop `-count=1` for iterative runs; only changed
   packages re-run, so a no-op `go test ./...` returns in ~1 s.
2. **Scope to the package you're editing** — `go test ./internal/weftengine` beats the
   whole repo.
3. **Stay in the offline tier.** Tier 1 (`go test ./...`) spawns zero git
   subprocesses repo-wide. Only reach for `-tags integration` when you are
   changing worktree / weft / paths / board / ide git behaviour — and budget
   ~a minute for that tier.
