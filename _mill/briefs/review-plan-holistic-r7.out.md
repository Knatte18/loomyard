MILL_REVIEW_BEGIN
# Review: Introduce warp: the host↔weft-coordinated git module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-25
```

## Findings

### [BLOCKING] Card 12 misses dormant/adopt updates in weftwiring_test.go
**Location:** Batch 4 / Card 12 (vs Card 10)
**Issue:** Card 10 moves `worktree/weft_test.go` → `warp/weftwiring_test.go`, which contains `TestWeftSpawnCreatesJunction`, `TestWeftSpawnSeedsExclude`, `TestSeederParity` (all assert a junction/exclude exist *after* `Add`), `TestWeftPrechecks/RejectExistingWeftBranch` (asserts Add aborts on an existing weft branch), and `TestWeftMissingParentBranch`; Card 12 makes `Add` dormant + adopt-or-create but lists only `add.go`, `add_test.go`, `configcli_integration_test.go` in Edits — these `weftwiring_test.go` tests will fail or contradict the new behaviour, failing batch-4 verify.
**Fix:** Add `internal/warp/weftwiring_test.go` to Card 12's Edits and specify which junction-after-add / abort-on-existing-weft-branch cases to update or move (junction assertions now belong with the `WireJunctions`/init activation path).

### [BLOCKING] rollbackAdd no longer references removeHostJunction — test churn
**Location:** Batch 4 / Card 12
**Issue:** Card 12 drops the `removeHostJunction` step from the create-rollback path, but `weftwiring_test.go`'s `TestWeftRollbackOnPostHostCreateFailure` manually wires state and exercises `rollbackAdd`; the requirement does not state whether `removeHostJunction` is deleted (now unused → Go compile error if unreferenced and unexported) or retained for `Remove`.
**Fix:** State in Card 12 that `removeHostJunction` is retained (still used by `Remove`/teardown) and update the rollback test's expectations; confirm no symbol becomes unused.

### [NIT] Card 9 ConfigTemplate qualifier across packages
**Location:** Batch 3 / Card 9
**Issue:** `configreg.go` replaces `{"worktree", worktree.ConfigTemplate}` with `{"warp", warp.ConfigTemplate}`; the requirement names this correctly, but `warp.ConfigTemplate` must be exported and reside in `warp/template.go` (moved same card) — fine, just confirm `template.go` is created before `configreg.go` edit within the card.
**Fix:** Note intra-card ordering (create `template.go` then edit `configreg.go`) to keep the build green mid-card.

### [NIT] TestRunDispatchesToWorktree name left stale
**Location:** Batch 3 / Card 10
**Issue:** Card 10 retargets the invocation `{"worktree","list"}` → `{"warp","list"}` but does not rename the test `TestRunDispatchesToWorktree`, leaving a misleading name.
**Fix:** Optionally rename to `TestRunDispatchesToWarp` for accuracy.

## Verdict

REQUEST_CHANGES
Card 12's dormant/adopt change breaks junction-asserting tests it does not edit.
MILL_REVIEW_END
