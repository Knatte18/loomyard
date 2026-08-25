MILL_REVIEW_BEGIN
# Review: loom: Webster-Review producer

```yaml
duration_s: 236.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [NIT:scope] Digit-spelled row counts escape the stated sweep
**Section:** Technical context → "Row-count knock-on" / Scope → "stub-stays"
**Issue:** The sweep is given as `git grep -n sixteen`, but `internal/loomshed/stub.go` spells it numerically at **two** sites — line 2 (file-header comment) and line 12 (`stubProducer` doc) — both reading "loom's 16-row producer list … that no task has built for real yet", and the Stub decision names only "stubProducer's doc comment" (singular).
**Fix:** Name both stub.go comment sites explicitly, and state that the sweep is `sixteen` *plus* the digit spellings (`16-row`/`16 rows`), since the criterion is row-counting prose, not one token.

### [NIT:scope] Fixture stencil-seeding helper's own comment goes stale
**Section:** Testing → `internal/loomrecipe`
**Issue:** The discussion flags that the fixture must seed `loom-rubric-webster-review` (correct — `NewBouncer` probes the rubric eagerly, `bouncer.go:136`), but `fixture_test.go:71-78`'s `seedBouncerStencils` doc comment enumerates "the four stencils a live Discussion-Review or Plan-Review segment reads" and names both rubrics; that becomes five stencils across three segments.
**Fix:** Add the helper's own doc comment to the test-update inventory alongside the map entry.

### [NIT:decision] No stated disposition for an in-flight run parked on `Webster-Review`
**Section:** Scope / Open risks
**Issue:** `contracts/recipes/loom-recipe.yaml:2-4` warns that a row rename "breaks resume for any in-flight task"; retiring the `Webster-Review` row leaves a `status.json` whose `current_producer` names a row the new list has no entry for, and the discussion never says whether that is accepted (as both sibling perches implicitly did) or handled.
**Fix:** Record it in Open risks as accepted-with-precedent, naming the two shipped perches as the precedent.

## Verdict

APPROVE
Every load-bearing mechanism claim verified against source; three recordable NITs, no blockers.
MILL_REVIEW_END
