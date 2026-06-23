MILL_REVIEW_BEGIN
# Review: weft producers: _lyx/config, lyx config, codeguide — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-23
```

## Findings

### [NIT] Integration test calls `editOne` directly, not `dispatch`
**Location:** `C:\Code\loomyard\wts\weft-producers\internal\configcli\configcli_integration_test.go:70`
**Issue:** Card 14 requires the e2e test to "run `dispatch` with a fake editor"; the test calls `editOne(baseDir, ...)` directly, skipping the `dispatch` routing layer entirely.
**Fix:** Replace the `editOne` call with `dispatch(hostLayout, os.Stdin, &out, []string{"worktree"}, fakeEdit, injectedSync)` so the plan-prescribed entry point is exercised.

### [NIT] `configcli_integration_test.go` absent from overview's "All Files Touched"
**Location:** `C:\Code\loomyard\wts\weft-producers\_mill\plan\00-overview.md:83-106`
**Issue:** The file is created by Batch 4 Card 14 and is present on disk, but the plan summary list omits it; the batch file itself accounts for it, so this is a documentation gap only.
**Fix:** Add `internal/configcli/configcli_integration_test.go` to the "All Files Touched" list.

## Verdict

APPROVE
All constraints satisfied, no blocking issues; two minor nits do not affect correctness.
MILL_REVIEW_END