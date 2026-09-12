MILL_REVIEW_BEGIN
# Review: lyx loom step + external supervisor skill — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-12
```

## Findings

### [NIT:consistency] loom-step.md's settled contract lists "stuck" as an example State value
**Location:** `manifest/designs/loom-step.md:33`
**Issue:** The `state` key's doc says "e.g. `running`, `stuck`, `blocked`, a terminal state," but `stuck` is never a legal `shedengine.State` value (the five are `running`/`paused`/`done`/`blocked`/`failed`) — `stuck` is an `Outcome` value, documented one line above as the `outcome` field's own vocabulary.
**Fix:** Replace `stuck` in that example list with a real `State` value (e.g. `paused` or `failed`), or drop it, so the settled contract's own example doesn't blur the two closed vocabularies it otherwise keeps carefully separate.

## Verdict

APPROVE
All seven batches faithfully realize the plan; cross-batch contracts, shared decisions, and constraints all hold; only one cosmetic doc nit found.
MILL_REVIEW_END
