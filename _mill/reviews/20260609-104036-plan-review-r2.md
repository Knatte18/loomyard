I've read all files. The plan is sound, well-grounded in the discussion. Let me verify the key claims against source.

The plan's Requirements reference `const writes` = `300`, `const readers = 8`, `seedWiki(t, 100)`, `len(tasks) != 100`, `task.Slug != "task-50"` — all confirmed in concurrency_test.go lines 26, 33-34, 84, 74. The `Board.writeOp` 3 AtomicWrite claim is consistent with board.go (Save + RenderToDisk) and git.go. The `t.Setenv("BOARD_SKIP_GIT", "1")` is at line 25. All decisions are faithfully implemented. Context completeness: Requirements mention `Board.writeOp` (board.go in Context), `AtomicWrite` (git.go in Context), `seedWiki` (bench_test.go in Context). All good.

One minor note: the `verify:` runs the full package which also runs `TestConcurrentUpsertsDoNotLoseWrites` — acceptable and noted. No blocking issues.

MILL_REVIEW_BEGIN
# Review: Cut boardtest concurrency test run time — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-09
```

## Findings

### [NIT] Comment-only "writes if it differs" guidance is soft
**Location:** Batch 1 / Card 1, Requirements bullet 4
**Issue:** The card says "Record the chosen value in the comment if it differs from 50," but the empirical-tune step can land on a non-50 value while the commit message hard-codes `300->50`; the two could silently diverge.
**Fix:** State that if tuning yields a value other than 50, both the comment and the commit subject must reflect the final number.

### [NIT] verify runs the second test too; scope wording slightly loose
**Location:** overview Decision go-native-verify / Batch 1 Batch Tests
**Issue:** `verify: go test ./internal/board/boardtest/` also exercises `TestConcurrentUpsertsDoNotLoseWrites`, not only the changed test; the Decision phrases this as "exactly the affected tests."
**Fix:** None required — running the sibling test is harmless and intended (validation step 4); wording is just imprecise.

## Verdict

APPROVE
Single-card plan faithfully implements all decisions; claims verified against source; only nits.
MILL_REVIEW_END
