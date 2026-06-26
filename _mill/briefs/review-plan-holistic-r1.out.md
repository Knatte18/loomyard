MILL_REVIEW_BEGIN
# Review: Speed up internal/warp integration tests — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-26
```

## Findings

### [BLOCKING] Card 8 deletion drops mispointed-junction coverage
**Location:** Batch 3, Card 8
**Issue:** `TestPairInSync_JunctionPointsElsewhere` is the only test of PairInSync's "junction points to wrong target" branch (link exists, `PointsTo` mismatch); `BrokenJunction` covers the *missing*-junction branch and `TestStatus_JunctionHealth` exercises `Status`, not `PairInSync` — so the plan's claim "No production path loses its only coverage" is inaccurate.
**Fix:** Fold the wrong-target assertion into `TestPairInSync_BrokenJunction` as a second sequential step (repoint to a decoy, re-check), instead of deleting outright.

### [BLOCKING] Card 10 deletion drops report-only pollution coverage
**Location:** Batch 3, Card 10
**Issue:** `TestStatus_CodeguidePollutionReportOnly` uniquely covers the `ReportOnly=true` / empty-`Remedy` branch; `TestStatus_LyxPollutionDetected` asserts the opposite (`ReportOnly=false`, non-empty `Remedy`). The file header documents both `_lyx` (remediable) and `_codeguide` (report-only) as required behaviors, so this is a distinct production branch losing its only test.
**Fix:** Fold the `_codeguide` report-only assertions onto the same fixture in a sibling (e.g. extend `TestStatus_LyxPollutionDetected`) rather than deleting.

### [NIT] Card 4 mischaracterizes a table row as a test function
**Location:** Batch 2, Card 4
**Issue:** `TestWeftPrechecksHardRequireWeftRepo` is a `name` field of a table row inside `TestWeftPrechecks` (weftwiring_test.go:209), not a standalone func; the card frames it like the deletable top-level tests.
**Fix:** State that the row is removed from the `TestWeftPrechecks` table, leaving the single `RejectExistingWeftWorktree` case.

### [NIT] Card 6 drops the only missing-parent rollback trigger
**Location:** Batch 2, Card 6
**Issue:** `TestWeftMissingParentBranch` is the only test exercising Add's live paired rollback triggered by a missing parent weft branch; `TestAddRollback` triggers via portal-clobber and `TestWeftRollbackOnPostHostCreateFailure` calls `rollbackAdd` white-box — neither covers the missing-parent error path.
**Fix:** Confirm this delete is intended, or fold the missing-parent error assertion into a kept test.

### [NIT] Card 14 merge does not fit the single-call table body
**Location:** Batch 5, Card 14
**Issue:** `HostDirty`/`WeftDirty` need two sequential `Remove` calls each, but the `TestRemove` table body issues exactly one `Remove`; the card omits whether these become bespoke `t.Run` subtests beside the table.
**Fix:** Specify that the merged cases are standalone sequential subtests within `TestRemove`, leaving `HappyPath`/`NonexistentSlug` as table rows.

### [NIT] Card 21 drops subpath-collision assertion
**Location:** Batch 5, Card 21
**Issue:** `TestCreatePortalMultipleSubpaths` uniquely asserts two distinct subpaths yield non-colliding portal links for one slug; "structurally guaranteed" is plausible but unverified by a kept test.
**Fix:** Confirm intentional, or keep the `link1 != link2` assertion.

## Verdict

REQUEST_CHANGES
Two deletions drop the only coverage of distinct production branches; fold instead of delete.
MILL_REVIEW_END
