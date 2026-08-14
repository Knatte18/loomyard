MILL_REVIEW_BEGIN
# Review: Relocate producer prompt files into a stencils/ directory — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-14
```

## Findings

### [NIT:design] runTriage/runTargeting new param position unspecified
**Location:** batch 5 (treadle-runtime-read), card 22
**Issue:** `runTriage` and `runTargeting` each "gain the parameter" for `stencilsDir`, but unlike every other signature change in this plan (card 18's leading param, card 26's trailing param), neither card states whether it is leading or trailing.
**Fix:** State the parameter's position explicitly (leading, matching `composePrompt`'s established convention) so both loose-scalar functions and their call sites land unambiguously.

### [NIT:consistency] render.go's new fabricengine import not called out
**Location:** batch 6 (webster-runtime-read), card 26
**Issue:** `RenderForkPrompt`/`RenderRecoveryPrompt`/`RenderMasterPrompt` must now call `fabricengine.StencilsDir(l.HubPath)`, requiring a new `internal/fabricengine` import in `render.go` — but this card is the only signature/import-changing card in batches 4-7 that omits the explicit "add the X import" instruction other cards (18, 23, 24, 27) all carry.
**Fix:** Add the same explicit import-addition line used elsewhere: "Add the `internal/fabricengine` import to `render.go` if not already present."

### [NIT:consistency] seam_enforcement_test.go's prose goes stale after the allowlist edit
**Location:** batch 5 (treadle-runtime-read), card 23
**Issue:** The card only asks for a new `allowedImports` map entry, but the same file's header comment (lines 1-5, "imports ONLY the standard library, internal/lock, internal/logger, internal/state, internal/stencil, internal/shuttleengine, and gopkg.in/yaml.v3") and the assertion's error-message string (line 86, same enumeration) both go stale the moment `internal/stencilstore` is added, since neither mentions it.
**Fix:** Extend card 23 to update both prose spots in the same file/commit, alongside the CONSTRAINTS.md bullet it already amends.

## Verdict
REQUEST_CHANGES
Three NIT-level specificity/consistency gaps; no BLOCKING findings — the plan is otherwise precisely grounded in the current source.
MILL_REVIEW_END
