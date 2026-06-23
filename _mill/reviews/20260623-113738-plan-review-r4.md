Card 14's claims verified: line 54 contains "incl. one real GitHub push" in the "Where the Tier 2 cost is" cell for `internal/board/boardtest`. The history is append-only. Card 14 correctly says to append a new block and update the "Current best times" section. All accurate.

I have now verified all major plan claims against source.

Key observations:
1. All line numbers in cards are accurate (verified board.go:83, sync.go:32/103, git.go:69, cli.go:83, the 10 CopyPaired call sites, board_test.go/concurrency_test.go/bench_test.go env-setters).
2. The Shared Decisions are faithfully implemented in cards 3-10.
3. The "single production env read" decision is sound: env folded once at RunCLI, all consumption sites take params.
4. Card 9 correctly identifies all facade-write-path regressions and the intentionally-untouched env-setters (main_test.go, menu_test.go).
5. Batch DAG is acyclic, all files exist, global step numbering 1-14 sequential.
6. Integration test reachability: batch 3 adds `TestWeftSpawnPushesWeftBranch` (tagged integration) and verify runs `-tags integration ./internal/worktree`.

MILL_REVIEW_BEGIN
# Review: Speed up and stabilize the integration test tier — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-23
```

## Findings

### [NIT] Batch 2 verify rationale cites a nonexistent cli_test.go case
**Location:** 02-board-skip-seam-parallelize.md — Batch Tests
**Issue:** The verify prose says the test leg runs "the new `cli_test.go` case", but no card adds a case to `cli_test.go`; card 10 creates a separate white-box file `skipenv_internal_test.go`.
**Fix:** Reword to reference `skipenv_internal_test.go` (the actual new test surface); the verify command itself is correct and unchanged.

## Verdict

APPROVE
Plan is constraint-clean, line-accurate, DAG-valid, and decisions are faithfully implemented; only one cosmetic prose nit.
MILL_REVIEW_END