MILL_REVIEW_BEGIN
# Review: Custom-typed plan cards skip path-missing checks — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-26
```

## Findings

### [NIT:consistency] Garbled sentence in plan-card-format.md's Custom bullet
**Location:** `manifest/designs/plan-card-format.md:96`
**Issue:** "a card carrying a `Custom` group could instead be expressed as a multi-label combination is a defect" is missing a relative pronoun ("that"/"whose targets"), making the sentence read as a run-on/garbled clause; the rubric's equivalent sentence ("A `Custom` card whose targets could instead be expressed as a multi-label combination of the other six is a finding") is grammatically clean by contrast.
**Fix:** Insert "whose targets" (or "that") before "could instead be expressed", matching the rubric's phrasing.

## Verdict

APPROVE
All three batches are fully realized, cross-batch contracts hold, and every doc/test sweep is consistent; one cosmetic prose NIT only.
MILL_REVIEW_END
