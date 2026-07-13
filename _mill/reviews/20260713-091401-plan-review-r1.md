MILL_REVIEW_BEGIN
# Review: Restore the Tier 1 floor: guards + perchengine — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-13
```

## Findings

### [NIT] New benchmarks block needs a distinguishing descriptor
**Location:** Batch 2 / Card 4
**Issue:** The card dates the new "Current best times" block 2026-07-13 and retitles the existing block to `### 2026-07-13 — hermetic git test environment`, producing two same-day blocks; the new one gets no descriptor, unlike the live doc's established `As of DATE (<descriptor>)` form (e.g. line 31 "hermetic git test environment landed").
**Fix:** Have Card 4 give the new block an "As of 2026-07-13 (restore-tier1-floor: mousetrap + re-tier)" descriptor so the two 2026-07-13 headings stay distinguishable once this block later freezes into History.

### [NIT] boardtest bounded-shrink scope absent from Shared Decisions
**Location:** Overview / `## Shared Decisions` (vs. Card 3)
**Issue:** Card 3 is a third code change with a nuanced conditional keep/revert policy, but the overview's `measurement-driven-scope` decision frames scope as exactly "two levers" and no `boardtest-bounded-shrink` decision is promoted from `_mill/discussion.md` into the overview; a reader of Shared Decisions alone would not see card 3's rationale.
**Fix:** Add the `boardtest-bounded-shrink` decision to the overview's `## Shared Decisions` (or amend `measurement-driven-scope` to acknowledge the bounded third change).

## Verdict

APPROVE
Scope, sequencing, DAG, numbering, and Context-completeness are sound; only two non-blocking NITs.
MILL_REVIEW_END
