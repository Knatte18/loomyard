MILL_REVIEW_BEGIN
# Review: Prune and consolidate the test suite (board first) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-22
```

## Findings

### [BLOCKING] TestRenderOrphanDetection split uses slash in table-row name

**Location:** `internal/board/render_test.go:306-313`
**Issue:** The two table rows use names `"TestRenderOrphanDetection/WithBody"` and `"TestRenderOrphanDetection/WithoutBody"`. Go's `t.Run` treats `/` as a subtest separator, producing three-level nesting (`TestRenderSingleTask/TestRenderOrphanDetection/WithBody`) rather than the plan-required single-level subtest `TestRenderSingleTask/TestRenderOrphanDetection`. The `preserve-names-as-subtests` shared decision and Card 1(c) both require the original func name as the subtest name; the slash in the `name` field breaks that contract and the doc's name-map claim (`TestRenderOrphanDetection → TestRenderSingleTask/OrphanDetection`) matches neither variant.
**Fix:** Rename the two rows to `"TestRenderOrphanDetection"` (the original func name) and fold both body/no-body assertions into that single subtest, matching the plan intent and the doc's name-map entry.

### [NIT] Doc name-map omits TestSyncIntegration_EventuallyPushed

**Location:** `docs/benchmarks/test-suite-timing.md` (weft name-map block, line ~175)
**Issue:** `TestSyncIntegration_EventuallyPushed` is in the weft baseline (`_mill/plan/baseline/weft.txt:20`) and is now a row within `TestPushIntegration`, but is entirely absent from the equivalence guardrail name-map. Card 17 requires every removed top-level name to appear in the map.
**Fix:** Add `TestSyncIntegration_EventuallyPushed → TestPushIntegration/TestSyncIntegration_EventuallyPushed` to the weft section of the equivalence guardrail.

### [NIT] Doc name-map subtest paths strip "Test" prefix incorrectly

**Location:** `docs/benchmarks/test-suite-timing.md` (board name-map block, lines ~135-153)
**Issue:** The doc writes e.g. `TestInitCreatesStructure → TestInitFirstRun/CreatesStructure`, `TestOutputsFromConfig → TestOutputs/OutputsFromConfig`, and all `TestLoadConfig` entries with suffix-only paths. The actual `t.Run` call names preserve the full `Test` prefix, so the real `go test -run` path is `TestInitFirstRun/TestInitCreatesStructure`, not `TestInitFirstRun/CreatesStructure`.
**Fix:** Use the exact `t.Run` argument (full name including `Test` prefix) in each map entry.

### [NIT] Unplanned test case TestRenderDeferredTask added without plan coverage

**Location:** `internal/board/render_test.go:145-153`
**Issue:** `TestRenderDeferredTask` is a new test case inside `TestRenderProposalAndShapesHomepage` that does not appear in Card 1's fold list and has no prior baseline name. The plan's `function-consolidation-only` shared decision limits this task to folding and dropping existing tests.
**Fix:** Either add a plan note to Card 1 acknowledging the new case or move it to a separate task.

## Verdict

REQUEST_CHANGES
One blocking slash-naming defect breaks the `preserve-names-as-subtests` contract; three nits.
MILL_REVIEW_END