The IDE `Menu` calls `board.New(cfg)` then only `b.HealthCheck()` and `b.GetTask()` — both are READ paths that never spawn sync (GetTask/HealthCheck don't go through writeOp). So `menu_test.go`'s `BOARD_SKIP_GIT=1` is actually a no-op already (the menu never writes), so removing the env read does NOT regress menu_test.go. Good — that one is a false alarm; the env var there is vestigial.

But `internal/board/board_test.go` `TestUpsertTask` and `TestRerender` DO go through `writeOp` (UpsertTask/Rerender are write ops). Those are the real regression. And `bench_test.go`'s `BenchmarkUpsertFacade` (facade path) similarly, but benchmarks don't run under `go test` without `-bench`, so not a test-failure concern, though `seedWiki` is shared.

This is a genuine BLOCKING gap: the plan's Shared Decision asserts tests never need env because "no test invokes RunCLI for these paths," but `board_test.go` proves that assumption false — it relies on env-suppression via the facade and is not updated by any card. I have enough to write the review.

Let me do a final quick check: does the plan's Card 3 note that `writeOp`'s behavior change affects `board_test.go`? No card lists `internal/board/board_test.go` in Edits. Confirmed gap.

MILL_REVIEW_BEGIN
# Review: Speed up and stabilize the integration test tier — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-23
```

## Findings

### [BLOCKING] board_test.go facade tests rely on BOARD_SKIP_GIT env
**Location:** Batch 2, Card 3 (and the overview "Single production env read" Decision)
**Issue:** `internal/board/board_test.go` `TestUpsertTask`/`TestRerender` set `t.Setenv("BOARD_SKIP_GIT","1")` and build the Board via the facade `board.New(cfg)` (not `RunCLI`); once Card 3 makes `writeOp` key off `b.skipGit` instead of env (board.go:83), these write-path tests no longer suppress the detached sync and `spawnSync` fires against a non-repo temp dir — contradicting the Decision's claim that "no test invokes RunCLI for these paths, tests never read env at all." This file is in batch 2's verify scope (`./internal/board/...`) but in no card's Edits.
**Fix:** Add a card to set `cfg.SkipGit = true` (drop the `t.Setenv`) in `board_test.go`'s `TestUpsertTask`/`TestRerender`/`TestHealthCheckPasses`, and list it in Edits/Context.

### [BLOCKING] Facade benchmarks in bench_test.go also lose env suppression
**Location:** Batch 2 (unscoped)
**Issue:** `boardtest/bench_test.go` `BenchmarkUpsertFacade` builds the Board via `board.New(cfg)` with only `b.Setenv("BOARD_SKIP_GIT","1")`; after Card 3 the env is ignored, so the facade benchmark spawns real syncs. `seedWiki` is shared with `concurrency_test.go` (Card 9). No card touches `bench_test.go`.
**Fix:** Either convert the facade benchmark to `cfg.SkipGit = true` in a card, or document that benchmarks are out of scope and confirm `seedWiki`'s contract is unaffected.

### [NIT] Stale doc references to deleted tests/benchmarks
**Location:** Batch 1, Card 1/Card 2
**Issue:** Deleting `integration_test.go`/`bench_git_test.go` leaves dangling references in `boardtest/doc.go:9` ("see integration_test.go and bench_git_test.go") and `docs/benchmarks/board-performance.md:142` (`BenchmarkSyncGit`/`TestIntegrationCommitPush`); neither is updated. `cmd/testtiming/main.go:93` also still prints "real git + network".
**Fix:** Add doc.go and board-performance.md edits to batch 1 (or batch 4), and refresh the testtiming "network" string.

### [NIT] Card 12 mischaracterizes one swapped call site
**Location:** Batch 3, Card 12
**Issue:** `weft_test.go:254` (`TestWeftRollbackOnPostHostCreateFailure`) is listed as a `SkipPush:true` site that "calls Add(...)", but it never calls `Add` — it invokes `rollbackAdd` directly. The lean-fixture swap is still safe (no weft push), but the justification is inaccurate.
**Fix:** Reword Card 12 to note line 254 uses `rollbackAdd` and is safe because it never pushes the weft branch.

### [NIT] Card 8 should explicitly drop the helper's t.Setenv
**Location:** Batch 3→ Batch 2, Card 8
**Issue:** `sync_test.go`'s `newSyncRepo` helper sets `t.Setenv("BOARD_SKIP_GIT","")` (line 29); unless removed, no caller can `t.Parallel()`. The card's "replace every t.Setenv" covers it implicitly but does not name the helper.
**Fix:** Name `newSyncRepo`'s line-29 `t.Setenv` explicitly as a deletion in Card 8.

## Verdict

REQUEST_CHANGES
Facade tests/benchmarks depending on BOARD_SKIP_GIT env are not migrated; they regress after Card 3.
MILL_REVIEW_END