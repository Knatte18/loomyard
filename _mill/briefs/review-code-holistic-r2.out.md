MILL_REVIEW_BEGIN
# Review: Prune and consolidate the test suite (board first) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-22
```

## Findings

### [BLOCKING] `TestRenderOrphanDetection` not folded as required by Card 1

**Location:** `internal/board/render_test.go:347`
**Issue:** Card 1(c) requires folding `TestRenderSingleTask` and `TestRenderOrphanDetection` into one proposal-key table. `TestRenderOrphanDetection` still exists as a standalone top-level function (with subtests `WithBody`/`WithoutBody`), not as a subtest of `TestRenderSingleTask`. This causes two problems: (a) the doc name-map entry `TestRenderOrphanDetection → TestRenderSingleTask/OrphanDetection` is false, and (b) the board "after" count in the timing doc is reported as 38 but is actually 39 (one too many).
**Fix:** Either merge `TestRenderOrphanDetection`'s cases (`WithBody`/`WithoutBody`) into `TestRenderSingleTask` as the plan required, or update the doc to reflect that the fold was intentionally skipped with justification and correct the "after" count to 39.

### [BLOCKING] Doc name-map for worktree is severely incomplete

**Location:** `docs/benchmarks/test-suite-timing.md:162-163`
**Issue:** The "worktree (3 dropped)" section lists only `TestWeftPrechecksHardRequireWeftRepo`, but the implementation folded four baseline names into `TestWeftPrechecks` subtests: `TestWeftPrechecksRejectExistingWeftWorktree`, `TestWeftPrechecksRejectExistingWeftBranch`, `TestWeftHostPristineEnforced`, and `TestWeftPrechecksHardRequireWeftRepo`. Three of the four folded names are entirely absent from the name-map, breaking the auditable diff requirement of the `preserve-names-as-subtests` and `coverage-guardrail-with-frozen-baselines` shared decisions.
**Fix:** Add the three missing folded names to the worktree name-map section: `TestWeftPrechecksRejectExistingWeftWorktree → TestWeftPrechecks/TestWeftPrechecksRejectExistingWeftWorktree`, `TestWeftPrechecksRejectExistingWeftBranch → TestWeftPrechecks/TestWeftPrechecksRejectExistingWeftBranch`, `TestWeftHostPristineEnforced → TestWeftPrechecks/TestWeftHostPristineEnforced`.

### [NIT] `TestRenderDeferredTask` case added without plan coverage or doc entry

**Location:** `internal/board/render_test.go:143-153`
**Issue:** `TestRenderDeferredTask` is a new subtest case inside `TestRenderProposalAndShapesHomepage` that does not appear in the baseline `board.txt` and is not mentioned in Card 1's fold list.
**Fix:** Add a note to the equivalence guardrail that `TestRenderDeferredTask` is a new subtest row added during the fold for the previously uncovered deferred-task bucket path.

### [NIT] Weft doc name-map entry for `TestPullIntegration_FastForward` is contradictory

**Location:** `docs/benchmarks/test-suite-timing.md:169`
**Issue:** The entry reads "kept within suite; subsumed by TestPull_FastForward …" but the function was dropped. "Kept within suite" contradicts the actual outcome.
**Fix:** Change the entry to "dropped — strict subset of `sync_test.go:TestPull_FastForward`".

### [NIT] `TestCLIContract` subtests doc maps use truncated names

**Location:** `docs/benchmarks/test-suite-timing.md:125-133` vs `internal/board/cli_test.go`
**Issue:** Code preserves full original names (e.g. `name: "TestCLIUpsertTask"`) but the doc name-map drops the "TestCLI" prefix. The actual subtest path is `TestCLIErrorAndEdgeCases/TestCLIGetNonexistentTask`.
**Fix:** Update the doc entries to use the full subtest names as they appear in the code.

## Verdict

REQUEST_CHANGES
One plan deviation (Card 1(c) fold not completed) plus an incomplete worktree name-map block in the doc; nits are doc-only inaccuracies with no functional impact.
MILL_REVIEW_END