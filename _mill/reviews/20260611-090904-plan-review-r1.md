MILL_REVIEW_BEGIN
# Review: Extract shared primitives (paths, output) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-06-11
```

## Findings

### [NIT] output.Ok mutates caller-supplied map
**Location:** Batch 1 / Card 3 (Decision: output envelope shape); propagates to Batch 2 / Cards 4–5
**Issue:** `Ok` writes `fields["ok"] = true` into the map the caller passes; all current call sites use fresh literals so there is no bug today, but it is an undocumented destructive side-effect that will surprise any future caller who passes a reusable map.
**Fix:** Document the mutation in the function's godoc comment, or copy the map internally before injecting `"ok"`.

## Verdict

APPROVE
Plan is coherent, behaviour-preserving, and correctly sequenced; one minor API side-effect worth documenting.
MILL_REVIEW_END
