Just one stale comment in a test function doc (not code). Now let me produce the final review.

MILL_REVIEW_BEGIN
# Review: Extract internal/vscode; keep ide IDE-generic — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-22
```

## Findings

### [NIT] Stale comment in spawn_test.go names deleted symbol

**Location:** `C:\Code\loomyard\wts\extract-internal-vscode\internal\ide\spawn_test.go:165`
**Issue:** The test function doc says "Spawn picks a color via pickColor" — `pickColor` was deleted; the seam now calls `vscode.PickColor`.
**Fix:** Update the comment to reference `vscode.PickColor`.

## Verdict

APPROVE
One stale comment in an unchanged test file; implementation is correct and plan-complete.
MILL_REVIEW_END
