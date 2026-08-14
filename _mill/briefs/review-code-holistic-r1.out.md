MILL_REVIEW_BEGIN
# Review: Relocate producer prompt files into a stencils/ directory — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-14
```

## Findings

### [BLOCKING:scope] promote never errors on a board copy with no matching source file
**Location:** `internal/stencilcli/promote.go:51-74`
**Issue:** Batch 8's scope paragraph and card 33 both explicitly require: "With no `stencils/` source tree, or with a board-copy name matching no source file, exit with an error naming what is missing" — and the scope paragraph states "Neither [promote nor diff --all] ever creates a `stencils/` directory and neither silently no-ops." The implementation only guards the top-level tree-absent case (`resolveSourceDir(l) == ""`); when the tree exists but the specific stencil's family subfile is missing, `promote` unconditionally `os.MkdirAll`s the family dir and writes the file rather than erroring — it silently creates rather than reporting the missing file.
**Fix:** Before writing, `os.Stat(targetPath)` and return `output.Err` naming the missing source file when absent, per the stated design; add a test for this path (currently uncovered — `cli_integration_test.go` only tests the whole-tree-absent case).

### [BLOCKING:scope] Webster's relocated banners still name the pre-move filename
**Location:** `stencils/webster/webster-prefix-fork.md:1,7`, `stencils/webster/webster-prefix-recovery.md:1`
**Issue:** Both banners still read "composed with implementer-body.md" — the pre-relocation filename (`internal/websterengine/implementer-body.md`) rather than the new `webster-body-implementer.md`. Batches 2/4/5 each carried an explicit banner-rewrite step for their relocated families (card 8, card 20, card 21's own paragraph) with the stated rationale "a banner naming a file that no longer exists is read by a human constantly ... which is why this is not cosmetic" — batch 6 (card 25/26/27/28) has no equivalent step, so webster's five banners were never swept, and this repo-wide grep confirms these two files are the only remaining stale hits outside `_mill/`.
**Fix:** Rewrite `implementer-body.md` to `webster-body-implementer.md` in both banners (3 occurrences total), matching the pattern already applied to loom/burler/treadle.

## Verdict

REQUEST_CHANGES
Two scope gaps: promote's missing-source-file error path is unimplemented, and webster's banners retain a stale pre-move filename.
MILL_REVIEW_END
