I have enough to complete the review. I verified every plan claim against source, the DAG, global card numbering, decisions, and searched the full blast radius for references to the removed/renamed symbols.

Key finding: the plan's "All Files Touched" and cards convert `cli_test.go`, `store_test.go`, and `helptree_test.go`, but two other test files reference the removed `id_or_slug` key and neither is enumerated in any card: `internal/board/boardtest/bench_test.go` and `cmd/lyx/main_test.go`.

MILL_REVIEW_BEGIN
# Review: Board fixes from sandbox run — payload keys, help, rerender — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-28
```

## Findings

### [BLOCKING] Benchmark still uses removed `id_or_slug`, never converted
**Location:** Batch 1, Cards 1-2 (and overview "All Files Touched")
**Issue:** `internal/board/boardtest/bench_test.go:145` calls `get '{"id_or_slug":"task-0"}'` and `b.Fatalf` on exit!=0; after Card 2's strict-key rejection this fails under `-bench`. The file is in no card's Edits, and the batch verify `go test ./internal/board/ ./cmd/lyx/` (no `...`) never compiles/runs the `boardtest` package, so the rot is silent — contradicting the task's "remove `id_or_slug` entirely."
**Fix:** Add `internal/board/boardtest/bench_test.go` to Card 2's Edits and convert the `BenchmarkGet` payload to `{"slug":"task-0"}`.

### [NIT] `main_test.go` keeps `id_or_slug` payload, passes only incidentally
**Location:** Batch 1, Card 2
**Issue:** `cmd/lyx/main_test.go:114` uses `remove '{"id_or_slug":"nope"}'` and asserts exit 1; post-Card-2 it still exits 1, but now via unknown-key rejection rather than not-found, leaving a stale removed-key reference.
**Fix:** Convert to `{"slug":"nope"}` and list `cmd/lyx/main_test.go` in Card 2 Edits so it genuinely exercises the not-found path.

### [NIT] Card 8 commit leaves an existing render test red
**Location:** Batch 3, Cards 8/9
**Issue:** Card 8 deletes `removeOrphanProposals`; the existing single-pass `TestRenderToDisk` ghost-removal assertion goes red because the seed render removes nothing, and the fix lives in Card 9 — so Card 8's commit is non-atomic.
**Fix:** Fold the `TestRenderToDisk` ghost-test restructure into Card 8 (or state that intermediate-red within the batch is accepted).

### [NIT] roadmap/overview confirm dropped silently
**Location:** Overview "All Files Touched"
**Issue:** The discussion flags "confirm whether roadmap/overview need a note in the same commit" for the observable `set-phase`→`set-status` rename, but no card or file addresses it.
**Fix:** Add an explicit confirm-or-skip note (Card 1 or 6) per the task-completion docs discipline.

### [NIT] Card 1 cites `TestMergeTasks` as a `SetPhase` call site
**Location:** Batch 1, Card 1
**Issue:** Card 1 lists `TestMergeTasks` among `s.SetPhase(...)` call sites to rename, but it calls `MergeTasks`, not `SetPhase` — the only direct call sites are in `TestSetPhase` (`TestSetPhaseNil`/`TestSetPhaseMissing`).
**Fix:** Drop `TestMergeTasks` from that example list.

## Verdict

REQUEST_CHANGES
One blast-radius test file with the removed `id_or_slug` key is unconverted and uncaught by verify.
MILL_REVIEW_END
