MILL_REVIEW_BEGIN
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-29
```

## Findings

### [GAP] warp clone split omits exporting cloneHub
**Section:** Decisions › ambiguous-file-placement; Technical context › warp; Q&A (r4)
**Issue:** `runClone` is assigned to `warpcli` but calls `cloneHub` (clone.go:95), and the discussion explicitly keeps `cloneHub` engine-internal ("deriveBoardURL/cloneRepo/cloneHub stay engine-internal unless a cli-half caller needs them") — yet runClone is exactly such a cli-half caller, so the listed export set (`DeriveHostName`, `HubSuffix`, `RemoveAll`) is incomplete and the split would not compile.
**Fix:** Add `cloneHub` → `warpengine.CloneHub` to the warp export list (or, alternatively, keep `runClone` in `warpengine`), resolving the contradiction the r4 export decision left open.

### [NOTE] clone_integration_test.go must be physically split across packages
**Section:** Testing › warp split
**Issue:** `clone_integration_test.go` contains both `cloneHub` domain tests (engine) and the reset-swap test at 309–353 (cli); a single `_test.go` file cannot belong to two packages, so it must be split, not merely "moved."
**Fix:** State that the file is divided — cloneHub-driving scenarios to `warpengine`, the reset-swap test to `warpcli`.

## Verdict
GAPS_FOUND
The warp clone export set omits cloneHub, which the cli-half runClone calls.
MILL_REVIEW_END
