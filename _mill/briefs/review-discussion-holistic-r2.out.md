MILL_REVIEW_BEGIN
# Review: Bump quarry to v0.2.0 and adopt Status.Known()/Rejected()

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-09
```

## Findings

### [BLOCKING:design] Completeness test as specified cannot fail
**Section:** §Testing "TDD candidate — vocabulary completeness" vs §Decisions `statuses-completeness-test`
**Issue:** The three spelled-out assertions do not detect a widened vocabulary: a hypothetical fifth status admitted by `Known()` yields `resolved=false, stillExists=true`, producing `create-not-done`/`rename-not-done` and never `glyph-rejected`, so "every value in `quarry.Statuses` … never a `glyph-rejected` finding" passes, and "keyed off `len(quarry.Statuses)`" merely sizes a range loop rather than asserting anything — while the Decision section says each value must receive "an explicit, named disposition", which requires a locally declared per-status expectation table a new status has no entry in.
**Fix:** State the failing mechanism concretely — a locally declared `map[quarry.Status]<expected disposition>` (or equivalent) that the test asserts is exhaustive against `quarry.Statuses`, so an unlisted new status fails — and drop or restate the weaker "never `glyph-rejected`" / `len()` phrasings that contradict it.

### [NIT:scope] Rename directions absent from the test spec
**Section:** §Testing, completeness-test bullets
**Issue:** `doneCheckVerdicts` has four `checkID` arms (`create-not-done`, `delete-not-done`, `rename-not-done-old`, `rename-not-done-new`, `donecheck.go:183-220`), but the completeness spec names only "the create/delete rules"; the two rename arms read the same booleans and are equally exposed to a widened vocabulary.
**Fix:** Name all four `checkID` arms in the completeness test's coverage statement, or say explicitly that the rename arms are covered transitively by the shared booleans.

## Verdict

REQUEST_CHANGES
The completeness test's failure mechanism is underspecified and, as written, would not fail.
MILL_REVIEW_END
