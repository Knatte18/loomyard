MILL_REVIEW_BEGIN
# Review: planparser owns the plan-directory path — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude (Sonnet-class; environment reports "Sonnet 5" / claude-sonnet-5, unverifiable from inside the model itself)
reviewed_file: plan/
date: 2026-08-17
```

## Findings

### [BLOCKING:scope] Context omits the package two cards' Requirements name
**Location:** batch 2, card 4 (`internal/loomengine/plan_test.go`) and card 8 (`internal/webstercli/verbs_test.go`)
**Issue:** Card 4's Requirements name `modelspec.LoadRegistry(t.TempDir())`; card 8's name `batcher.ConfigTemplate()`. Neither `internal/modelspec` nor `internal/batcher` is listed in that card's `Context:`, and neither is an `Edits:` target — both calls are copy-pasted verbatim from existing code already inside each card's own `Edits:` file, so the practical risk is low, but it is a literal Context-completeness gap per the stated criterion.
**Fix:** Add `internal/modelspec` to card 4's `Context:` and `internal/batcher` to card 8's `Context:`.

## Verdict
REQUEST_CHANGES
One mechanical Context-completeness gap (cards 4 and 8); every other verified claim matched source exactly.
MILL_REVIEW_END
