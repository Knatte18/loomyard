Now I have all the information needed for the review.

MILL_REVIEW_BEGIN
# Review: Local lyx sandbox for manual experimentation — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-26
```

## Findings

### [NIT] Unused `expectHub` field in relative-path subtest

**Location:** `C:\Code\loomyard\wts\lyx-sandbox\tools\sandbox\main_test.go:25-26`
**Issue:** The table entry for "relative parent path" sets `expectHub: "relative/path"` but the field is never read inside the subtest body; the assertion verifies only `IsAbs` and `Base`, so the listed value is dead and misleading.
**Fix:** Either assert `hubPath` contains the expected suffix, or drop the `expectHub` field from the relative case and add a comment explaining what the subtest actually validates.

## Verdict

APPROVE
All plan cards fully realised; constraints, shared decisions, and path-invariant rules satisfied.
MILL_REVIEW_END
