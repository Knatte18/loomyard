MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-12
```

## Findings

### [NIT:consistency] `CoalescePushBothAt` doesn't build an empty-hub-root recorder for empty warpPath
**Location:** `internal/fabricengine/coalesce.go:87`
**Issue:** Batch 3 card 11 requires `CoalescePushBothAt` to build its recorder with an empty hub root when `warpPath == ""` ("a true no-op today ... build the recorder with an empty hub root"). The code always does `NewMutations(filepath.Dir(warpPath))`; `filepath.Dir("")` returns `"."`, not `""`. Currently inert — `step()` only calls `AppendRef` (ref-based, no hub-root conversion) — but a future `Append` call added to this function would silently mis-convert its target against `"."` instead of falling back to the absolute path.
**Fix:** Guard the constructor, e.g. `hubRoot := ""; if warpPath != "" { hubRoot = filepath.Dir(warpPath) }`, matching the plan text and closing the latent trap for a future `Append` call in this function.

## Verdict

APPROVE
Implementation matches the plan precisely across all eight batches, with one inert, low-risk deviation.
MILL_REVIEW_END
