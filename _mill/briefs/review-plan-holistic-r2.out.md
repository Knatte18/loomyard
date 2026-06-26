MILL_REVIEW_BEGIN
# Review: Speed up internal/warp integration tests — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-26
```

## Findings

### [BLOCKING] Card 17 drops the backup-content assertion
**Location:** Batch 5 / Card 17 (hook ChainsExistingHook → ChainIdempotent)
**Issue:** `TestInstallPostCheckoutHook_ChainIdempotent` (hook_test.go:169-211) never reads `post-checkout.user`; it only asserts first==second install content. The card's premise that it "already validates ... user hook backed up" is wrong — deleting `ChainsExistingHook` loses its unique assertion `backupContent == userHookContent` (the non-clobber/backup-preservation check), violating the `preserve-coverage` decision ("a dropped assertion is a plan defect").
**Fix:** Also port the backup-content assertion into `ChainIdempotent` after its first install: read `hookPath + ".user"` and compare to the planted `"#!/bin/sh\necho user\n"`; keep the post-checkout.user-reference port too.

### [NIT] Card 13 leaves the merged Cleanup BranchPrefix unspecified
**Location:** Batch 4 / Card 13 (combine Cleanup live-branch cases)
**Issue:** The two source tests call `Cleanup` with different configs (empty vs `"hanf/"`). The merged single `Cleanup(true,true)` must run on `New(Config{BranchPrefix:"hanf/"})` for the `hanf/feature-prefix-live` branch to be recognized as live; an empty-prefix config would report/delete it and break the regression coverage. The card does not state which prefix to use.
**Fix:** Specify the merged Cleanup uses `BranchPrefix:"hanf/"`, and name the two distinct slugs for the no-prefix and prefixed pairs.

### [NIT] Card 10 omits the explicit second Status() call
**Location:** Batch 3 / Card 10 (fold CodeguidePollutionReportOnly)
**Issue:** The card says "create the `_codeguide` pollution ... and assert ReportOnly=true" but the first `Status()` (for `_lyx`) cannot observe a pollution created afterward; a second `Status()` call is required and only implied.
**Fix:** State that `Status()` is re-invoked after the `_codeguide` force-add+commit before the report-only assertions.

## Verdict

REQUEST_CHANGES
Card 17 drops a unique backup-preservation assertion, violating the preserve-coverage decision.
MILL_REVIEW_END
