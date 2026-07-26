MILL_REVIEW_BEGIN
# Review: fabric: cutover -- rewire consumers onto fabric, delete warp/weft — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-26
```

## Findings

### [NIT] `.gitattributes` still names deleted-module file paths
**Location:** `.gitattributes:18-20`
**Issue:** Three `text eol=lf` rules still target `internal/warpengine/post-checkout.sh`, `internal/warpengine/template.yaml`, and `internal/weftengine/template.yaml`, all deleted in card 16; no batch card covers `.gitattributes`, so these are dead/inert path rules that no longer match anything.
**Fix:** Delete the three stale lines (or repoint the two `post-checkout.sh`/`template.yaml` rules to their `internal/fabricengine` equivalents if such files exist there).

### [NIT] Stale cross-batch comment references a deleted test helper
**Location:** `internal/loomengine/preflight_integration_test.go:24-27`
**Issue:** The `setupPreflightFixture` doc comment cites "`buildDiffPair`'s identical fixup in fabricengine's own differential tests" as a cross-reference; `buildDiffPair` and the differential test files it lived in were deleted in batch D1 (card 15), so the reference now dangles (comment-only, no code/behaviour impact, not matched by card 27's `warpengine|weftengine|warpcli|weftcli` grep since it names neither word).
**Fix:** Reword to point at a still-existing standalone fixture (e.g. `newFabricFixture` in `internal/fabricengine/reconcile_stale_registration_test.go`) or drop the cross-reference.

## Verdict

APPROVE
All six batches are correctly implemented, cross-batch contracts hold, and the grep-clean gate is genuinely clean in source.
MILL_REVIEW_END
