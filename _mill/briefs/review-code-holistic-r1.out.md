MILL_REVIEW_BEGIN
# Review: shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-11
```

## Findings

### [NIT:consistency] shed.md: new prose skips required soft breaks at conjunction boundaries
**Location:** `manifest/designs/shed.md:28,31,32`
**Issue:** The `semantic-line-breaks` Shared Decision requires an extra soft break at "a comma followed by a coordinating conjunction... where what follows has its own subject and verb," but card 3's new prose keeps such boundaries on one physical line, e.g. line 28 "...nothing to re-read on resume, so a crashed `Discussion-Write` re-runs..." and line 31 "...with no artifact, and the resume-on-output-files rule degrades gracefully...".
**Fix:** Split at each `, and`/`, so` independent-clause boundary in lines 28, 31, and 32 into its own line, per the Shared Decision.

### [NIT:consistency] roadmap.md's new deferral record (card 14) skips the same required soft breaks
**Location:** `manifest/roadmap.md:62-63`
**Issue:** Line 62 ("...editing it, so rewriting it now would invent...") and line 63 ("...in both, so the enum is not stale...; realignment lands...") each contain a comma+`so` (and, on line 63, a semicolon) independent-clause boundary that the same Shared Decision requires to start a new line, but both stay on one line.
**Fix:** Break line 62 at the `, so` boundary and line 63 at both the `, so` and `; realignment` boundaries.

## Verdict

REQUEST_CHANGES
Substance and plan alignment are sound; only recurring semantic-line-break Shared-Decision violations in new prose need fixing.
MILL_REVIEW_END
