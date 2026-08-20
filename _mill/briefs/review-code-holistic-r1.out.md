MILL_REVIEW_BEGIN
# Review: shedadapters: Burler-round producer — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-20
```

## Findings

### [NIT:scope] roadmap.md group-one opening sentence lacks the doc pointer
**Location:** `manifest/roadmap.md:16`
**Issue:** Card 14 says every group-one/two cross-reference must both name the Burler-round producer as shipped and point at the `internal/shedadapters` package documentation; the opening sentence ("unlike the now-shipped Burler-round producer, genuinely domain-agnostic") fixes the stale "above" claim but carries no doc pointer, unlike the sibling cluster-fan sentence (line 20) and the Discussion-Review parenthetical (line 32), which both do.
**Fix:** Append a pointer to the `internal/shedadapters` package documentation to the opening sentence, matching the other two reworded sites.

## Verdict

APPROVE
All four batches match their cards, shared decisions, and cross-batch contracts; one cosmetic doc-pointer gap is non-blocking.
MILL_REVIEW_END
