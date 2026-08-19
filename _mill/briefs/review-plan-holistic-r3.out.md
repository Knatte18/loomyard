MILL_REVIEW_BEGIN
# Review: landing: Publish + Finalize producers — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-19
```

## Findings

### [NIT:consistency] loom.md's producer table still claims Finalize does "teardown"
**Location:** batch 6, card 38 (`manifest/designs/loom.md`) **Issue:** `loom.md:41`'s Output cell reads "merge-back, teardown" for row 13. Batch 6's own scope statement calls this exact claim one of three the task overturns ("the teardown step it names, which names nothing in the codebase and was loose wording"), and card 39 correctly strips it from `roadmap.md`, with card 36 folding the corrected description into `landingshed/doc.go`. Card 38's loom.md work item only repoints the file's four `landing.md` links (rows 12/13, the Raddle-fold paragraph, the build-ordering paragraph) — it never touches the "teardown" word sitting in the same row-13 cell as one of those links, so the same superseded claim survives in loom.md after the task, contradicting the corrected version now living in `landingshed/doc.go` and `roadmap.md`. **Fix:** Add a line to card 38's loom.md item (or card 36) dropping "teardown" from row 13's Output cell, matching the correction already made everywhere else the claim appears.

## Verdict

APPROVE
Plan is thorough, internally consistent, and closely grounded in the actual source; one cosmetic doc wording gap noted above.
MILL_REVIEW_END
