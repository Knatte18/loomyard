MILL_REVIEW_BEGIN
# Review: fabric: cutover -- rewire consumers onto fabric, delete warp/weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-26
```

## Findings

### [BLOCKING] Card 27 Tier-2 grep gate unreachable — unswept _test.go comments
**Location:** Batch D3 / cards 23, 27 (also 17)
**Issue:** Card 27 Tier-2 greps ALL `.go` files tree-wide (`--include='*.go' .`) for `warpengine|weftengine|warpcli|weftcli` expecting zero matches, but card 23's own sweep grep excludes `_test.go`, and many surviving test files carry "Adapted from warpengine's…"/"mirroring weftengine…" provenance comments that NO card edits (so the "sweep files you edit" Shared Decision never reaches them): `fabricengine/{clone,config,hook,ancestors,template,launcher_content,fabric}_test.go`, `fabriccli/cli_test.go`, `initengine/undo_test.go`, `loomengine/testmain_test.go`, `hubgeometry/siblinglayout_test.go`, `reedengine/config_test.go`, `perchcli/run_integration_test.go` (card 17 also leaves `lyxtest/leaf_enforcement_test.go` doc-comment lines, sweeping only its import list). These aren't weft repo/role terms — they name deleted modules — so the gate finds hits with no owning card to route them to, and the "update ALL references" directive is unmet.
**Fix:** Add a D3 card sweeping deleted-module provenance comments from all surviving `_test.go` files, or scope card 27 Tier-2's grep to exclude `_test.go` to match card 23.

### [BLOCKING] Card 12 misses second test using the removed `warp` module
**Location:** Batch B / cards 9 + 12
**Issue:** Card 9 removes `warp` from the registry, but `configcli_integration_test.go`'s second test `TestDispatchSet_PreservedKeyDetectedByReconcile` (lines ~160-235) seeds, dispatches, and asserts the `"warp"` config module — `dispatch(...,[]string{"warp"},...)` → `setModule` → `configreg.Template("warp")` returns not-found → exit 1 → `t.Fatalf`, reddening the batch-B integration verify. Card 12 only describes the first test's fixture, and its "Preserve every assertion" is impossible for that test's `module=="warp"` assertion.
**Fix:** Card 12 must also rewrite `TestDispatchSet_PreservedKeyDetectedByReconcile`'s seed/dispatch/assert onto a still-registered module (`fabric`).

### [NIT] Card 11 import guidance imprecise
**Location:** Batch B / card 11
**Issue:** Card 11 says "Drop the `warpengine`/`weftengine` imports," but `configreg_test.go` imports only `weftengine` (no `warpengine`), and the card omits that the `fabricengine` import must be added for the new `fabricengine.ConfigTemplate()` assertion.
**Fix:** Reword to "drop the `weftengine` import and add `fabricengine`."

## Verdict

REQUEST_CHANGES
Card 27's tree-wide grep gate is unreachable (unswept _test.go comments) and card 12 breaks batch-B tests.
MILL_REVIEW_END
