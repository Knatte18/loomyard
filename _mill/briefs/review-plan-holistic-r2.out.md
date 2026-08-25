MILL_REVIEW_BEGIN
# Review: loom: Discussion-Review producer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-4.5 (self-assessed; I cannot verify this with certainty)
reviewed_file: plan/
date: 2026-08-25
```

## Findings

### [BLOCKING:scope] Card 12 Context omits webstergeom.go; its claim is also wrong
**Location:** batch 4 (loomcli-wiring), card 12
**Issue:** Requirements state "BurlerGeometry uses l.WorktreePath() ... while WebsterGeometry uses l.AnchorPath(), a deliberate divergence documented in both files," but `internal/hubgeom/webstergeom.go` (where `WebsterGeometry` lives) is not in Card 12's Context, and the claim is factually wrong: `BurlerGeometry`'s own doc comment in `hubgeom.go` says nothing about `WebsterGeometry`, and `WebsterGeometry`'s doc comment documents its divergence against `ReedGeometry`, not `BurlerGeometry`. An implementer following only the listed Context cannot find this "documentation" and may waste time hunting for it.
**Fix:** Add `internal/hubgeom/webstergeom.go` to Card 12's Context, and correct the sentence to state the divergence is observed directly from each function's own code (WorktreePath vs AnchorPath) rather than "documented in both files."

### [NIT:consistency] Card 6 doesn't update burlerRoundProfile's stale key-count doc comment
**Location:** batch 2 (shedrecipe-entries), card 6
**Issue:** `burlerRoundProfile`'s doc comment in `entries_burler.go` currently reads "recognising exactly six keys -- target, fasit, rubric, fix-scope, tool-use, and cluster-fan." Card 6 adds a seventh key, `rubric_stencil`, with a mutual-exclusivity rule against `rubric`, but the requirements never mention updating this doc comment, so it silently becomes false.
**Fix:** Add a line to Card 6 requiring the doc comment be updated to seven keys and to note the `rubric`/`rubric_stencil` mutual exclusivity.

### [NIT:consistency] Card 12 miscounts the fields it fills as "six"
**Location:** batch 4 (loomcli-wiring), card 12
**Issue:** "Fill six fields on the shedrecipe.Env literal" is followed by an enumeration of eight: `StencilsDir`, `RunRoot`, `Burler`, `Now`, `ReviewModel`, `ReviewEffort`, `ReviewVersion`, `ReviewTimeout`. The count doesn't match the list (harmless since the list itself is complete and correct, but a specificity slip).
**Fix:** Change "six fields" to "eight fields."

### [NIT:consistency] Card 22 leaves a "two producers not yet built" claim stale
**Location:** batch 6 (docs), card 22
**Issue:** `manifest/designs/loom.md` line 76 says the discussion-detail section "carries the detail that belongs to `Discussion-Validate` and `Discussion-Review` instead — two producers not yet built." `Discussion-Validate` was already real before this task, and after this task `Discussion-Review` is real too, making the "not yet built" clause fully false. Card 22's requirements list the producer table row, the "thirteen rows" prose, and the two rubric subsections' opening lines, but never this line.
**Fix:** Add a line to Card 22 requiring line 76's "two producers not yet built" clause be corrected or dropped.

## Verdict

REQUEST_CHANGES
One Context-completeness gap (card 12) plus three low-cost doc/prose accuracy misses; core wiring, sequencing, and constraint compliance are sound.
MILL_REVIEW_END
