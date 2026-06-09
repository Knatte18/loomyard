Now I have the complete picture. Let me compile the findings:

1. **`board.go:35,37`** — Two stale `wiki` references in comments: `` `mhgo wiki sync` `` and `"wiki sync" commits`. The shared decision `rename-surface` requires all doc-comment references to the old name to be updated. Card 3 specifically requires updating "doc-comment references to these env vars and to `mhgo wiki sync`".

2. **`boardtest/bench_test.go:183`** — `BenchmarkUpsertFacade` sets `cfg.Path = dir` where `dir` is the cwd parent returned by `seedWiki`, but the seeded `tasks.json` is at `<dir>/board/tasks.json`. The facade will operate on an empty board, not the N-task seed, defeating the benchmark's purpose. The concurrency tests (lines 29, 104) correctly use `filepath.Join(cwd, "board")`. Card 11 says use `cfg.Path = dir` from `seedWiki(b, n)` — but the card was written when `seedWiki` returned the board dir directly; the final `seedWiki` returns the cwd. This is a functional bug.

3. **`main_test.go:18`** — Stale comment references `internal/wiki`. Minor documentation issue.

4. **`benchmarks.md:93`** — Background sync table note says `WIKI_SKIP_PUSH=1` instead of `BOARD_SKIP_PUSH=1`.

5. **`init_test.go`** — `runInit` helper uses manual `os.Chdir`/restore instead of `t.Chdir`. The plan card 20 says "using a temp cwd (`cwd := t.TempDir(); t.Chdir(cwd)`)". The discussion's `testability` decision says "no `os.Chdir`". While non-blocking (tests are non-parallel), it contradicts the explicit decision.

Now I'll assess severity: Finding #2 (`BenchmarkUpsertFacade` wrong board path) is functionally a bug — the benchmark silently measures the wrong workload. This is a behavioral correctness issue for a test, not a production code path, but it defeats the benchmark's stated purpose. I'll rate it BLOCKING per the criterion "Integration correctness — the pieces work together". Finding #1 is a rename-surface miss (doc comments), which is a Shared Decision violation — that is BLOCKING per "Shared-decisions alignment — the Shared Decisions subsections are applied consistently across all batches; deviation is BLOCKING."

MILL_REVIEW_BEGIN
# Review: board-modul (rename fra wiki) + _mhgo-konfigurasjon — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-09
```

## Findings

### [BLOCKING] Stale "wiki" references in board.go doc comments

**Location:** `C:\Code\mhgo\wts\config-layer\internal\board\board.go:35-37`
**Issue:** The `writeOp` doc comment still reads "launches a detached `mhgo wiki sync`" and "batched \"wiki sync\" commits" — both violate the `rename-surface` Shared Decision (card 3 explicitly requires updating doc-comment references to `mhgo wiki sync`).
**Fix:** Replace both occurrences with `mhgo board sync` and `"board sync" commits` respectively.

### [BLOCKING] BenchmarkUpsertFacade points at wrong board directory

**Location:** `C:\Code\mhgo\wts\config-layer\internal\board\boardtest\bench_test.go:183`
**Issue:** `cfg.Path = dir` assigns the cwd returned by `seedWiki` (which puts `tasks.json` at `<dir>/board/tasks.json`), so the facade operates on an empty board rather than the N-task seed; the benchmark measures a cold 1-task upsert, not the re-render-over-N-tasks cost it claims to measure. The concurrency tests in the same package correctly use `filepath.Join(cwd, "board")` (lines 29, 104).
**Fix:** Change `cfg.Path = dir` to `cfg.Path = filepath.Join(dir, "board")` in `BenchmarkUpsertFacade`.

### [NIT] Stale comment in main_test.go references internal/wiki

**Location:** `C:\Code\mhgo\wts\config-layer\cmd\mhgo\main_test.go:18`
**Issue:** Comment still says "not the wiki behaviour itself (that lives in `internal/wiki`)" after the full rename.
**Fix:** Update to reference `internal/board`.

### [NIT] benchmarks.md background-sync table cites old env var name

**Location:** `C:\Code\mhgo\wts\config-layer\docs\benchmarks.md:93`
**Issue:** The `SyncGitNoPush` row note reads `WIKI_SKIP_PUSH=1` — a missed rename in the docs.
**Fix:** Change to `BOARD_SKIP_PUSH=1`.

### [NIT] init_test.go uses os.Chdir instead of t.Chdir

**Location:** `C:\Code\mhgo\wts\config-layer\internal\board\init_test.go:26-42`
**Issue:** The `runInit` helper manually saves/restores cwd with `os.Getwd()` + `defer os.Chdir(origCwd)` instead of `t.Chdir(cwd)`, contradicting the `testability` discussion decision ("no `os.Chdir`") and card 20's explicit instruction ("using a temp cwd via `t.Chdir`").
**Fix:** Replace the manual save/restore with `t.Chdir(cwd)` inside `TestInit*` callers, consistent with how `cli_test.go` and `main_test.go` handle this.

## Verdict

REQUEST_CHANGES
Two blocking issues: stale `mhgo wiki sync` doc-comment (rename-surface decision violation) and `BenchmarkUpsertFacade` targeting the wrong directory (benchmark silently measures the wrong workload).
MILL_REVIEW_END
