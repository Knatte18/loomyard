MILL_REVIEW_BEGIN
# Review: fabric: warp-side commit lock + push coalescing — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: plan/
date: 2026-07-30
```

## Findings

### [NIT] CoalescePushBothAt homes lock at weftPath unconditionally
**Location:** Batch 2 card 4 / Batch 3 card 7
**Issue:** `CoalescePushBothAt` always builds the absorbing lock via `ensureWeftLockDirAt(weftPath)`; a warp-only bypass invocation (`lyx fabric --warp-path X push`, empty `weftPath`) — which the CLI still accepts (bypass=true, push allowed) and which the replaced `if warpPath != ""` handler tolerated — would `mkdir .weft` and run `git rev-parse` relative to cwd. Production never triggers it (Fabric.Commit always passes both paths), so the gap is narrow.
**Fix:** Guard `CoalescePushBothAt` against an empty `weftPath` (fall back to `warpPath` as lock home, or return early/error), or have card 7 note the CLI rejects a warp-only bypass push.

## Verdict

APPROVE
Plan is well-formed, decisions faithfully implemented, no constraint violations; one narrow non-production edge as a NIT.
MILL_REVIEW_END
