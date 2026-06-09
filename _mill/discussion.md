# Discussion: Cut boardtest concurrency test run time

```yaml
task: Cut boardtest concurrency test run time
slug: boardtest-concurrency-speed
status: discussing
parent: main
```

## Problem

`TestConcurrentReadsDuringUpserts` in `internal/board/boardtest/concurrency_test.go`
is slow. In isolation on the dev machine it runs in **~8.2s**; during a recent
merge-to-parent (`go test ./...` as part of merge verification) the test suite
took **several minutes**, and this test is the dominant cost.

**Why now:** the multi-minute test run blocked a merge and made the verify step
painful. The root cause is not Go compute — Go calls here are microseconds. The
test is **filesystem-bound**. Each write goes through `Board.writeOp`
(`internal/board/board.go:38`), which does, per `UpsertTask`:

- 1 `Store.Load` (read `tasks.json`)
- 3 × `AtomicWrite` (`internal/board/git.go:61`), each an `os.CreateTemp` (a
  fresh random `.tmp-` name) + `os.Rename`, for `tasks.json`, `Home.md`, and
  `_Sidebar.md` (seeded tasks have no body, so no proposal files).

That is ~6 filesystem operations per write × **300 writes** = ~1800 temp-create
+ rename operations, all under the OS temp dir (`t.TempDir()`). Three factors
multiply this into minutes:

1. **Cortex XDR** synchronously scans every `CreateTemp`/`Rename`. Each temp
   file has a new random name, so XDR sees an "unknown" file every time → full
   scan, no cache hit. Reads are barely taxed (the file is already scanned), so
   the cost is concentrated in the **writer's** loop.
2. **`go test ./...` runs packages in parallel** (GOMAXPROCS). During the merge
   this test competed with every other test package for disk + XDR at once;
   isolated it is 8.2s, but under full-suite load it is far worse.
3. **Lock contention**: 8 readers hold the swap lock (`Store.Load`/`Store.Save`)
   and delay the writer's `rename`, inflating each write from ~5.7ms
   (uncontended, measured via `BenchmarkUpsertFacade/n=100`) to ~27ms under the
   test's concurrency.

We cannot change Cortex XDR (managed endpoint security). The lever we control is
the **number of filesystem operations XDR must scan** — i.e. the writer's
iteration count.

## Scope

**In:**

- Reduce the `writes` constant in `TestConcurrentReadsDuringUpserts`
  (`internal/board/boardtest/concurrency_test.go`) from `300` to `50`.
- Update the test's comments to explain the value choice and that the test is
  FS-bound (each write = 3 XDR-scanned temp+rename ops), so the reader race
  window is governed by writer duration, not write count.

**Out:**

- `TestConcurrentUpsertsDoNotLoseWrites` (same file) — runs in 0.11s, untouched.
- The benchmarks in `bench_test.go` / `bench_git_test.go` — only run under
  `-bench`, not part of normal `go test`, untouched.
- The `seed` size (100 tasks) and `readers` count (8) — unchanged (see
  Decisions). The `len(tasks) != 100` assertion stays valid.
- Production code (`board.go`, `git.go`, `render.go`, `store.go`). No test-only
  seam (e.g. a "skip render" flag on `writeOp`) is added — rejected below.
- `testing.Short()` gating — rejected below.
- Any Cortex XDR / antivirus exclusion configuration (environment, not code, and
  not under our control).

## Decisions

### reduce-writes-constant

- Decision: Change `const writes` from `300` to `50` in
  `TestConcurrentReadsDuringUpserts`.
- Rationale: The writer's serial loop is the wall-clock driver and the source of
  the XDR-scanned temp+rename stream. Readers loop **continuously** until the
  writer closes `stop`, so the race window is bounded by how long the writer
  runs, not by the number of writes — over a ~1s window the 8 readers still
  execute on the order of hundreds of thousands of `GetTask` + `ListTasksBrief`
  iterations, preserving read-under-write coverage. Cutting writes 300→50
  shrinks the FS-op stream ~6× (expected ~1–1.5s isolated) while keeping a wide
  race window.
- Rejected: Keeping 300 (the 8.2s/minutes-under-load cost is the whole problem);
  dropping to ~25 (sub-second, but trims the overlap window more than needed for
  a "reasonable" target).

### keep-readers-8

- Decision: Leave `const readers = 8` unchanged.
- Rationale: Concurrent readers contending with the writer is the entire point
  of a read-under-write test. Reducing readers would speed each write (less
  swap-lock contention) but weaken exactly the property under test.
- Rejected: Dropping readers to 4 — faster, but reduces contention realism for
  marginal additional gain over the writes cut.

### keep-seed-100

- Decision: Keep `seedWiki(t, 100)` and the `len(tasks) != 100` assertion.
- Rationale: Minimal change. Board size is independent of the race property; the
  writes cut alone reaches the target time. Shrinking the board would also shrink
  the rendered files (smaller XDR scans) but is unnecessary and would churn the
  assertion and the test's documented intent.
- Rejected: Reducing seed to ~30 — extra lever not needed to hit the target.

### no-production-seam

- Decision: Do not add a "skip `.md` render" path (or any test-only flag) to
  `Board.writeOp`.
- Rationale: Skipping render would cut FS ops per write 3→1 (a further ~3× XDR
  win), but a simple constant change achieves the same wall-clock target without
  adding test-only surface to the production write path. Clean code over a
  premature optimization.
- Rejected: Functional option / `SkipRender bool` on the Board — YAGNI; pollutes
  production for a test-only benefit.

### no-short-gating

- Decision: Do not gate the counts behind `testing.Short()`.
- Rationale: `-short` only helps if every verify/merge invocation passes it,
  which they currently do not — so it would not fix the merge pain without also
  changing how `go test` is invoked across mill-go verify and mill-merge-in. An
  unconditional reduction fixes every run path with one change.
- Rejected: Two-tier full/short counts — adds a branch and a coverage story that
  no run path actually exercises today.

## Technical context

- **Test under change:** `internal/board/boardtest/concurrency_test.go`,
  function `TestConcurrentReadsDuringUpserts` (lines ~24–93). The constants live
  in a `const ( readers = 8; writes = 300 )` block at the top of the function.
  Only `writes` changes.
- **Write path:** `Board.writeOp` (`internal/board/board.go:38`) →
  `Store.Save` + `RenderToDisk` (`internal/board/render.go:21`) →
  `AtomicWrite` (`internal/board/git.go:61`, temp-create + rename). This is what
  makes each write FS-heavy; understanding it is why fewer writes is the right
  lever.
- **Seed helper:** `seedWiki(tb, n)` in `bench_test.go` (shared across the
  package) creates a `t.TempDir()` with `_mhgo/board.yaml` (`path: board`) and a
  `board/tasks.json` of `n` dependency-free tasks. Unchanged.
- **Why readers are cheap under XDR:** readers only `Load` (read) existing
  files; XDR scans a file once and caches, so repeated reads are not re-scanned.
  Writers create a *new* random temp file each time, defeating the cache. Hence
  the cost is on the writer loop.
- **Measured baselines (this machine, Intel Ultra 7 155U):**
  `TestConcurrentReadsDuringUpserts` ~8.19s; `TestConcurrentUpsertsDoNotLoseWrites`
  ~0.11s; `BenchmarkUpsertFacade/n=100` 5.7ms/op uncontended.

## Constraints

- `BOARD_SKIP_GIT=1` must stay set in the test (already via `t.Setenv`) so no
  detached `mhgo board sync` is spawned — the test measures board logic + file
  I/O only.
- The test must remain deterministic and pass reliably: the `writes` value must
  be large enough that readers and the writer demonstrably overlap (the writer
  must still be running while readers loop). 50 writes (~1s) satisfies this with
  wide margin.
- No behavior change to production board code; this is a test-only change.

## Testing

- This is a change *to* a test, not new production logic — no TDD cycle.
- Validation the implementer must perform:
  1. Run `go test ./internal/board/boardtest/ -run TestConcurrentReadsDuringUpserts -count=1 -v`
     and confirm it passes and the reported time is ~1–1.5s (isolated). If it
     lands materially outside that band, tune `writes` within [40, 75] to hit a
     "reasonable" ~1–1.5s; do not exceed 75.
  2. Run it a few times (e.g. `-count=3`) to confirm it passes reliably (no
     flakiness introduced by the shorter window).
  3. One-time only (not committed into the test): run with `-race` once to
     confirm the shorter loop still exercises the read-under-write path with no
     detected race — i.e. coverage is preserved, only wall-clock shrank.
  4. Confirm `TestConcurrentUpsertsDoNotLoseWrites` still passes unchanged.
- The `len(tasks) != 100` and `task.Slug != "task-50"` assertions are unaffected
  by the `writes` change and must remain.

## Q&A log

- **Q:** Strategy to cut runtime — unconditional count reduction vs `testing.Short()` gating vs a render-skip production seam? **A:** Unconditional count reduction. `-short` doesn't help unless every verify/merge run passes it (they don't); the production seam adds test-only surface for no wall-clock benefit over a constant change.
- **Q:** Go is fast — how could this take minutes? **A:** The test is filesystem-bound, not CPU-bound. ~1800 temp-create+rename ops under `t.TempDir()`, each synchronously scanned by Cortex XDR (new random temp names defeat its scan cache), amplified by parallel `go test ./...` package execution and reader/writer swap-lock contention.
- **Q:** Target wall-clock? **A:** Implementer's call to a "reasonable" time — chosen ~1–1.5s isolated (writes 300→50, ~6× faster).
- **Q:** Scope — which tests? **A:** Only the slow one (`TestConcurrentReadsDuringUpserts`). The 0.11s `TestConcurrentUpsertsDoNotLoseWrites` is fine; benchmarks untouched.
- **Q:** Can we configure Cortex XDR (e.g. exclude the temp dir)? **A:** No — managed endpoint security, out of our control. The fix must be pure code: reduce the FS-op count XDR has to scan.
- **Q:** Keep readers=8 and seed=100? **A:** Yes. Reader contention is the point of the test; seed size is independent of the race and keeping it minimizes churn and preserves the `len==100` assertion.
