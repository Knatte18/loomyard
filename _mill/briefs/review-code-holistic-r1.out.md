MILL_REVIEW_BEGIN
# Review: loom: Discussion-Review producer — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-25
```

## Findings

### [NIT:consistency] Stale "thirteen rows" comment survives in the file this task edited
**Location:** `internal/loomrecipe/shape_test.go:232` (`TestNew_ProducerTableOrderUnchangedByWiring`'s doc comment)
**Issue:** The comment says "the thirteen rows stay in their existing table order... regardless of what backs rows 12 and 13," but this same file's row count moved from thirteen to fourteen in this task (card 18 touched `wantProducerTable` and the file's header comment), and `Publish`/`Finalize` are now rows 13/14, not 12/13.
**Fix:** Update the sentence to "fourteen rows" and "rows 13 and 14" in the same pass that touched this file's other row-count prose.

### [NIT:consistency] manifest/designs/shed-recipe.md still says "thirteen-row"
**Location:** `manifest/designs/shed-recipe.md:9` and `:83`
**Issue:** Both lines describe `internal/loomrecipe.New()`'s output and the tests that build it as a "thirteen-row" list; this task's own recipe and `internal/loomrecipe` list are now fourteen rows, and this file was not in the plan's file list or any batch's `Edits:`, so a grep for "thirteen" was evidently not run against the whole repo.
**Fix:** Update both occurrences to "fourteen-row" in a follow-up (or fold into card 22's scope next time a `manifest/designs/shed-recipe.md` sweep lands).

## Verdict

APPROVE
All six batches match the plan precisely; only two pre-existing stale-comment NITs found, no functional or constraint issues.
MILL_REVIEW_END
