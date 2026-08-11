MILL_REVIEW_BEGIN
# Review: format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-11
```

## Findings

### [NIT:consistency] Producer-and-contract sections formatted differently across the two contract files
**Location:** `docs/reference/discussion-format.md:7` vs `docs/reference/plan-format.md:7-9`
**Issue:** `plan-format.md`'s new `## Producer and contract` section splits produced/validated/reviewed into three separate sentences (one per line), while `discussion-format.md`'s equivalent section states all three as one compound sentence on a single line — the same pinned content, two different applications of the semantic-line-breaks shared decision within the same batch.
**Fix:** Split `discussion-format.md:7` into three one-sentence lines to match `plan-format.md`'s pattern, or note the divergence is intentional.

### [NIT:consistency] Long compound sentence with a coordinated subject+verb clause not broken
**Location:** `docs/reference/discussion-format.md:25`
**Issue:** The sentence "This follows the same principle ... and that `docs/overview.md` states architecturally: ..." coordinates with a comma+"and" clause that has its own subject (`docs/overview.md`) and verb (`states`), which `CLAUDE.md`'s semantic-line-break rule calls out as a break point; the line runs on as one very long line instead.
**Fix:** Break the line at the comma before "and that `docs/overview.md` states architecturally".

## Verdict

APPROVE
All cards across both batches are realized correctly, cross-batch anchors resolve, and both grep acceptance gates pass cleanly.
MILL_REVIEW_END
