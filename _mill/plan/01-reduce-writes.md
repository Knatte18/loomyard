# Batch: reduce-writes

```yaml
task: "Cut boardtest concurrency test run time"
batch: "reduce-writes"
number: 1
cards: 1
verify: go test ./internal/board/boardtest/
depends-on: []
```

## Batch Scope

This batch delivers the entire task: cut the runtime of
`TestConcurrentReadsDuringUpserts` by reducing its writer-loop iteration count,
and update the test's comments to record why. It is one batch with one card
because the change is confined to a single constant and the surrounding doc
comment in one test file. No external interface is produced or consumed.

Batch-local note: the implementer must *empirically tune* the final `writes`
value, not blindly hard-code 50 — see Card 1 Requirements and `## Batch Tests`.

## Cards

### Card 1: Reduce the writer iteration count and document why

- **Context:**
  - `internal/board/boardtest/bench_test.go`
  - `internal/board/board.go`
  - `internal/board/git.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/board/boardtest/concurrency_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In function `TestConcurrentReadsDuringUpserts`, change the `const writes` value from `300` to `50`. Leave `const readers = 8` unchanged. Leave `seedWiki(t, 100)` and the `len(tasks) != 100` and `task.Slug != "task-50"` assertions unchanged.
  - Update the doc comment block on `TestConcurrentReadsDuringUpserts` (and/or an inline comment on the `const ( readers ...; writes ... )` block) to explain: the test is filesystem-bound, not CPU-bound — each write goes through `Board.writeOp` and performs 3 `AtomicWrite` temp-create+rename ops (`tasks.json`, `Home.md`, `_Sidebar.md`), each scanned by endpoint AV; the readers loop continuously until the writer stops, so read-under-write coverage is governed by how long the writer runs, not by the number of writes; therefore `writes` is kept small to bound the FS-op stream while preserving the race window.
  - Do NOT add a `testing.Short()` branch, do NOT change `seedWiki`, and do NOT modify any production file. Keep `t.Setenv("BOARD_SKIP_GIT", "1")` in place.
  - Empirically tune the final `writes` value: run the verification (see `## Batch Tests`) and confirm the isolated wall-clock is ~1–1.5s. If it lands materially outside that band, adjust `writes` within the closed range [40, 75] — never above 75 — and re-run. Record the chosen value in the comment if it differs from 50.
- **Commit:** `test(board): cut TestConcurrentReadsDuringUpserts writes 300->50`

## Batch Tests

`verify: go test ./internal/board/boardtest/` runs the two `Test*` functions in
the `boardtest` package (`TestConcurrentReadsDuringUpserts` and
`TestConcurrentUpsertsDoNotLoseWrites`); benchmarks require `-bench` and the
git integration tests require `-tags integration`, so neither runs here. Both
tests must pass.

Beyond the `verify:` gate, the implementer performs these one-time validations
(none committed into the test):

1. `go test ./internal/board/boardtest/ -run TestConcurrentReadsDuringUpserts -count=1 -v` — confirm PASS and that the reported time is ~1–1.5s isolated; tune `writes` within [40, 75] if needed.
2. `go test ./internal/board/boardtest/ -run TestConcurrentReadsDuringUpserts -count=3` — confirm it passes reliably (no flakiness from the shorter window).
3. `go test ./internal/board/boardtest/ -run TestConcurrentReadsDuringUpserts -race -count=1` once — confirm no data race is detected (coverage preserved, only wall-clock shrank).
4. Confirm `TestConcurrentUpsertsDoNotLoseWrites` still passes unchanged (covered by the package-level `verify:`).
