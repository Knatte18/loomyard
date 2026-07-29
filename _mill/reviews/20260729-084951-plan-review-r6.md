MILL_REVIEW_BEGIN
# Review: board: move storage to weft:main — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [NIT] Card 19's "calling UpsertTask directly" is ambiguous between two same-named methods
**Location:** batch 3 / Card 19 (crash-between-saves PromoteNote test)
**Issue:** The requirement says to simulate the mid-crash state "by calling UpsertTask directly with the note's same slug," but both `Board.UpsertTask` and `Store.UpsertTask` exist in scope; `Board.UpsertTask` would actually be rejected by Card 15's own new cross-store slug check (the slug already exists in `notesStore`), so only `Store.UpsertTask` (via `boardengine.NewStore` + `Load`/`Save`, mirroring Card 16's own bypass pattern) works.
**Fix:** Name `Store.UpsertTask` (via `NewStore(tasks.json path)`) explicitly in the requirement text to remove the ambiguity — an implementer choosing `Board.UpsertTask` will hit an immediate, obvious setup-step error, so this is self-correcting but worth tightening.

## Verdict

APPROVE — extensive source-grounding across all six batches confirms every checked claim (signatures, line numbers, existing test literals, guard patterns) is accurate.
MILL_REVIEW_END
