MILL_REVIEW_BEGIN
# Review: Dedicated sandbox suite for mux — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-03
```

## Findings

### [NIT] Card 6 omits test-side updates forced by Card 5's signature/const changes
**Location:** Batch 2, Card 5 + Card 6
**Issue:** Card 5 changes `renderScheme` to take the doc body as a 2nd param and prefers removing the `suiteFileName`/`defaultInstruction` consts, but those are referenced in `suite_test.go` (line 73 `renderScheme(info)`, lines 275–276 `defaultInstruction`, line 432 `suiteFileName`); Card 6 owns `suite_test.go` yet never names these call sites and even says the mainSuite assertions "must remain otherwise unchanged", which contradicts the required edits and would break compilation mid-batch.
**Fix:** Add to Card 6's Requirements: update `TestRenderScheme_ContainsHeaderAndBody` to `renderScheme(info, sandboxSuiteMD)` and swap `defaultInstruction`/`suiteFileName` references to `mainSuite.instruction`/`mainSuite.fileName` if Card 5 removes the consts.

## Verdict

APPROVE
Well-formed, DAG-clean, decisions faithfully implemented; one non-blocking test-update gap the batch verify would catch.
MILL_REVIEW_END
