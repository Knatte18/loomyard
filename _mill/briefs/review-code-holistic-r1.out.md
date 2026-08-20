MILL_REVIEW_BEGIN
# Review: shedengine: per-producer bounce budget + explicit OnDone routing — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-20
```

## Findings

### [BLOCKING:consistency] roadmap.md: stale "three" flattening-group count after the Done move
**Location:** `manifest/roadmap.md:39,48,53`
**Issue:** Card 19 moved the shedengine item out of the "Perch → Shed flattening" Planned group into Done, shrinking that group from 3 items to 2 (`shedadapters: Burler-round producer`, `Bouncer: the generic review-gate producer`). All three downstream items (`loom: Discussion-Review producer`, `loom: Plan-Review producer`, `loom: Webster-Review producer`) still read "Depends on the three 'Perch → Shed flattening' items above" — now factually wrong, since only two items remain in that group.
**Fix:** Update all three occurrences to "the two" (or otherwise reflect the post-move count), in the same commit that moved the roadmap item.

## Verdict

REQUEST_CHANGES
Implementation matches the plan exceptionally closely; one stale cross-reference count in roadmap.md needs fixing.
MILL_REVIEW_END
