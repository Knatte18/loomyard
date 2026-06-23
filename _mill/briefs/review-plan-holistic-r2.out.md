This grep is decisive. Two new findings emerge:

**`cmd/lyx/main_test.go:42` and `:76`** use `t.Setenv("BOARD_SKIP_GIT", "1")`. These exercise the `lyx board` command via `RunCLI` (the cmd/lyx dispatcher). Since RunCLI keeps reading env via `applySkipEnv` (card 4), these remain correct — env is still honored at the CLI entry. No regression. (Consistent with the decision.)

**`internal/ide/menu_test.go`** uses `t.Setenv("BOARD_SKIP_GIT", "1")` at 4 sites. The r1 review noted the IDE Menu only calls read paths (HealthCheck/GetTask), never writeOp, so its env is vestigial and removing the read does not regress it. Fine — not a finding.

Now the critical confirmation for finding 1: `concurrency_test.go:150` (`b.Setenv` in `BenchmarkGetDuringUpsert`) is the missed site. Card 9 explicitly lists only lines 33 and 109 (the two tests) for concurrency_test.go. The benchmark at line 150 builds the facade Board and writes — it meets card 9's own conversion criterion but is omitted. This confirms the BLOCKING finding.

MILL_REVIEW_BEGIN
# Review: Speed up and stabilize the integration test tier — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-23
```

## Findings

### [BLOCKING] Card 9 misses BenchmarkGetDuringUpsert facade write
**Location:** Batch 2, Card 9 (concurrency_test.go)
**Issue:** `BenchmarkGetDuringUpsert` (concurrency_test.go:149-150) sets `b.Setenv("BOARD_SKIP_GIT","1")`, builds the Board via `board.New(cfg)` (line 155), and writes via `w.UpsertTask` (line 170); after card 3 `writeOp` keys off `b.skipGit` (false here), so it spawns real `spawnSync` against a non-repo temp dir — the exact regression card 9 exists to prevent. Card 9 converts only lines 33 and 109, omitting line 150 (the same omission r1 caught for `BenchmarkUpsertFacade`, since fixed).
**Fix:** Add `BenchmarkGetDuringUpsert` to card 9's concurrency_test.go bullet: set `cfg.SkipGit = true` on the Config it passes to `board.New` instead of `b.Setenv`.

### [NIT] Card 9 rationale "do not build a Board" is wrong for Upsert/Get/List
**Location:** Batch 2, Card 9 (bench_test.go bullet)
**Issue:** Card 9 says leave `BenchmarkUpsert/Get/List` untouched because "they do not build a Board" — but `BenchmarkUpsert` calls `board.RunCLI` (bench_test.go:128), which after card 4 builds a Board and folds env via `applySkipEnv`. They stay correct (RunCLI still reads env), so leaving them is fine, but the stated reason is inaccurate.
**Fix:** Reword: they go through `RunCLI`, which still honors `BOARD_SKIP_GIT` env via `applySkipEnv`, so their `b.Setenv` remains live and correct.

### [NIT] Card 2 leaves dangling SyncGit refs in board-performance.md
**Location:** Batch 1, Card 2 (board-performance.md bullet)
**Issue:** Card 2 removes only the "## Push access" section, but board-performance.md also references the deleted `BenchmarkSyncGit`/`BenchmarkSyncGitNoPush` in the "How to run" block (lines 17-20: `-bench SyncGit` + "Network + push access...required") and the "Background sync" results table (lines 86-91). These dangle after batch 1 deletes both benchmarks.
**Fix:** Extend card 2 to also drop/annotate the `-bench SyncGit` invocation and the SyncGit results table in board-performance.md, or defer them explicitly to batch 4.

## Verdict

REQUEST_CHANGES
One benchmark missed in card 9 spawns real sync against a non-repo; two doc nits.
MILL_REVIEW_END