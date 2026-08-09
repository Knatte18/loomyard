MILL_REVIEW_BEGIN
# Review: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-09
```

## Findings

### [NIT:consistency] Probe-dir-prefix literal still duplicated after export exists
**Location:** `internal/fabricengine/warpbinding_clone_integration_test.go:40` (`probeDirPrefixLiteral`) vs `internal/fabricengine/export_test.go:28` (`WarpProbeDirPrefixForTest`)
**Issue:** Card 11 duplicates the `.lyx-clone-probe-` literal because the export doesn't exist yet at that point in the plan's sequencing, but by the final state (after card 12 adds `WarpProbeDirPrefixForTest`) the same file still has two independent spellings of the same prefix — `noProbeResidueInParent` uses the local const while `TestCloneHub_HubExistsCheckPrecedesProbeInTwoArgForm` uses the export.
**Fix:** Retarget `noProbeResidueInParent` (and the const declaration) onto `fabricengine.WarpProbeDirPrefixForTest` now that it exists, so the prefix has one source of truth in the finished file.

### [NIT:consistency] Probe error fallback doesn't name the failing subcommand
**Location:** `internal/fabricengine/warpprobe.go:144-154` (`wrapProbeError`)
**Issue:** The plan specifies the hard-error fallback (when git's stderr is empty) should be "a description of the failing git subcommand"; the implementation instead falls back to the generic string `"git command failed"` with no indication of which git invocation (clone/rev-parse/ls-tree/show) produced it.
**Fix:** Thread a short operation label into `wrapProbeError`'s callers (e.g. `"clone"`, `"ls-tree"`) for the empty-stderr fallback branch, or accept the current generic text as sufficient and drop the plan's stronger wording in a future doc pass — either resolution is fine, just make the two agree.

## Verdict

APPROVE
Implementation matches the plan closely across all six batches; only two cosmetic NITs found, no blocking issues.
MILL_REVIEW_END
