MILL_REVIEW_BEGIN
# Review: Spike: structured Go reference/call-graph lookup (go/packages / gopls) — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-17
```

## Findings

### [NIT] Stale card reference "card 9" in batch 3
**Location:** 03-measure-and-writeup.md / Card 6
**Issue:** Card 6 says the CC-native characterization is written "for card 9 to fold into the doc", but the folding card is Card 7 (batch scope itself says Card 7 folds it in); no card 9 exists.
**Fix:** Change "card 9" to "card 7".

### [NIT] Stale card reference "card 10" in batch 4
**Location:** 04-revert-and-verify.md / Batch Tests
**Issue:** Batch Tests names "The doc-only diff assertion in card 10", but the only card in this batch (and the one doing the doc-only diff assertion) is Card 8; no card 10 exists.
**Fix:** Change "card 10" to "card 8".

## Verdict

APPROVE
Plan is complete and constraint-clean; only two stale prose card-number references to correct.
MILL_REVIEW_END
