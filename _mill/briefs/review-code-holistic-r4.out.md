MILL_REVIEW_BEGIN
# Review: fabric: cutover -- rewire consumers onto fabric, delete warp/weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-26
```

## Findings

### [BLOCKING] Out-of-plan file: add_rollback_adopt_test.go
**Location:** `internal/fabricengine/add_rollback_adopt_test.go` (whole file, new)
**Issue:** This test file exists in the source tree but is not referenced by any batch's Context/Edits/Creates list, is not in the brief's "Files included" manifest, and is not in "All Files Touched" in `_mill/plan/00-overview.md`. Its own doc comment states "A live review round reproduced the pre-fix behavior" — confirming it was added during implementation/fix cycles without the corresponding batch file (04-delete-modules.md, card 15, or a new card) being updated first, per the "Out-of-plan files" review criterion.
**Fix:** Either fold this file's Creates entry into the relevant batch (card 15, since it lives beside the other fabricengine test-backfill work) and record it in `00-overview.md`'s "All Files Touched", or move/justify it through a proper plan amendment before it ships. This is a discipline gap even though the test itself (verifying `Add`'s rollback preserves an adopted pre-existing weft branch) looks correct and valuable.

### [NIT] config.go retains stale warp.yaml/weft.yaml provenance phrasing
**Location:** `internal/fabricengine/config.go:3-4,20-21`
**Issue:** The package comment and the `Config` struct doc still say "warp.yaml's BranchPrefix equivalent" and "weft.yaml's Pathspec equivalent." `config.go` is explicitly named in batch D3 card 23's provenance-comment sweep list, but this phrasing survived (it doesn't match the Tier-2 `warpengine|weftengine|warpcli|weftcli` grep, so the mechanical gate passes).
**Fix:** Reword to state the field provenance without citing the deleted config-module filenames, e.g. "the host branch prefix and the weft-sync pathspec, unified from fabric's two predecessor config schemas into one file."

### [NIT] Pre-existing docs still cite internal/warpengine as live
**Location:** `manifest/designs/loom.md:243`, `docs/shared-libs/hubgeometry.md:100`
**Issue:** Both cite `internal/warpengine` / `warpengine/prune.go` as if the package still exists ("builds on ... internal/warpengine"; "Used by warpengine/prune.go"). Neither file is in any batch's Edits list, so this is a pre-existing gap the plan's scope never covered, but it contradicts the task's own stated Shared-Decision rationale ("the user directive is 'update ALL references to old warp/weft to fabric'").
**Fix:** Out of this plan's card scope; flag for a follow-up doc sweep of `manifest/designs/loom.md` and `docs/shared-libs/hubgeometry.md` repointing the `internal/warpengine` mentions to `internal/fabricengine`.

## Verdict

REQUEST_CHANGES
One out-of-plan test file (add_rollback_adopt_test.go) needs its batch/overview record backfilled before merge.
MILL_REVIEW_END
