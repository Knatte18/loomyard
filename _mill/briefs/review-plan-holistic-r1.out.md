MILL_REVIEW_BEGIN
# Review: loom's status file can conflict on the landing merge — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-26
```

## Findings

### [BLOCKING:scope] Card 17 omits `internal/state` from Context despite naming its functions
**Location:** batch 4 / card 17. **Issue:** Requirements name `state.ReadJSONStrict[shedengine.Status]` and `state.UpdateJSON` by exact qualified name (the read-modify-write rig for diverging the two tasks' status files), but no file in package `internal/state` is listed in Card 17's `Context:`. `UpdateJSON`'s call shape is visible via `loomshed/seed.go` (in Context), but `ReadJSONStrict`'s signature/return shape (`(T, bool, error)`) is not shown in any listed file, forcing cold-start exploration. **Fix:** Add `internal/state/state.go` to Card 17's `Context:`.

### [BLOCKING:consistency] `constructoranchoring_test.go`'s header comment goes stale after Card 1's move
**Location:** batch 1 / card 1. **Issue:** The file's header comment (lines 22–24) enumerates ".lyx group in full" as `loomengine.LoomStatusLock, loomengine.LoomDriverLog, loomengine.LoomBootstrapLock, websterengine.PromptsDir/ScratchDir, logger.LogsDir`; Card 1 moves `loomengine.LoomStatusFile`'s `assertPath` into that same group but never updates this enumeration, leaving it incomplete/false the moment the card lands. **Fix:** Add `loomengine.LoomStatusFile` to the header comment's ".lyx group in full" list alongside the other five names.

### [BLOCKING:consistency] Card 13's resume-across-machines paragraph rewrite may leave a now-false middle sentence
**Location:** batch 3 / card 13. **Issue:** The card quotes only the opening sentence of `loom.md`'s resume-across-machines paragraph ("The status file lives in the weft repo...") and the closing sentence to keep, but the paragraph's middle sentence — "`lyx loom run` commits the seed once, and the only other commit of that file is the checkpoint the landing rows make... every persist in between leaves it as an uncommitted working-tree modification" — is not called out. After batch 1, the status file is never committed at all (not even the seed), so if an implementer edits only the named opening/closing sentences and leaves the middle one intact, the doc ships a false claim. **Fix:** Explicitly state in Card 13 that the middle sentence (the "commits the seed once... checkpoint" claim) is deleted along with the opening sentence, not merely the two.

## Verdict

REQUEST_CHANGES
Two Context/comment-completeness gaps and one under-specified doc rewrite risk a stale/false claim shipping; otherwise the plan is sound.
MILL_REVIEW_END
