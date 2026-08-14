MILL_REVIEW_BEGIN
# Review: Move <hub>/.lyx into <hub>/_board — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-14
```

## Findings

### [NIT:consistency] Duplicate exclude-reading test helper in hubscratch_integration_test.go
**Location:** `internal/fabricengine/hubscratch_integration_test.go:117-138`
**Issue:** `readWeftExcludeLinesForHubScratch(t, worktreePath)` is line-for-line identical logic to the pre-existing `readWeftExcludeLines(t, weftPath)` in `internal/fabricengine/dotlyxjunction_integration_test.go:62-83` (same package, same test binary, same `git rev-parse --git-path info/exclude` + read pattern). Card 9 asked to reuse existing exclude-reading helpers rather than re-declare one; the signature mismatch with `readExcludeLines(t, l, slug)` (junction_pattern_integration_test.go, which needs a `*lyxcwd.Location`) justifies not using that one here, but `readWeftExcludeLines` already had the right bare-path signature and was not reused.
**Fix:** Call `readWeftExcludeLines` directly from the new test cases instead of declaring a third near-identical helper.

## Verdict

APPROVE
Both batches fully match the plan; board-junction deletion and hub-scratch relocation are complete and consistent across code and docs.
MILL_REVIEW_END
