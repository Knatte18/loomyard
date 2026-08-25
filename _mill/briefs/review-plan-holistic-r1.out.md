MILL_REVIEW_BEGIN
# Review: loom: Discussion-Review producer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude (Sonnet 5, per environment metadata)
reviewed_file: plan/
date: 2026-08-25
```

## Findings

### [BLOCKING:scope] shape_test.go's TestNew_ProducerTable breaks on the new Segment/MaxBounces
**Location:** Batch 5, Card 18. **Issue:** `internal/loomrecipe/shape_test.go`'s `TestNew_ProducerTable` unconditionally asserts every row's `Segment == ""` and `MaxBounces == 0` (its current body, lines ~141-146). Once card 14 lands, `Discussion-Bouncer`/`Discussion-Burler` legitimately carry `Segment: "Discussion-Review"` and `MaxBounces: 5`, so this test fails. Card 18 converts the identical hardcoded check in `recipe_test.go`'s `TestNew_ShapeMatchesRecipe` to per-row comparisons, but explicitly says "Leave `TestNew_ProducerTable` in shape_test.go … it needs no count edit" — true for the length check, false for these two zero-value assertions, which it never mentions. **Fix:** extend card 18 to also convert `TestNew_ProducerTable`'s Segment/MaxBounces checks to per-row comparisons against the new `want.segment`/`want.maxBounces` fields, same as the recipe_test.go fix.

### [BLOCKING:design] Card 11 cites a template/struct agreement mechanism that doesn't exist
**Location:** Batch 3, Card 11. **Issue:** Card 11 says to "Follow whatever mechanism [`internal/loomengine/config_test.go`] already uses to assert template/struct agreement rather than inventing a second one." Verified: that file has no such mechanism today — only literal-value round-trip checks per field (`TestLoadConfig_WellFormed`). The only repo-wide precedent for a template/struct key-set agreement check lives in `internal/websterengine/config_test.go`, which this card does not list in Context. The instruction rests on a false premise and leaves the implementer with nothing to "follow." **Fix:** either point at the actual precedent (adding its file to Context) or instruct adding a new key-set agreement assertion explicitly.

### [NIT:consistency] stub.go's stale row-count prose left untouched
**Location:** Batch 5, Card 14. **Issue:** Card 14 updates "thirteen"→"fourteen" in `loomshed.go`'s and `doc.go`'s comments, and asks `stub.go`'s stubbed-row list to drop `Discussion-Review`, but `internal/loomshed/stub.go` itself still says "13-row producer list" (twice) and "backs three rows" — neither is covered by the card's explicit instruction, so both go stale post-edit. **Fix:** extend card 14 to update stub.go's "13-row"→"14-row" and "three rows"→"two rows" in the same pass.

### [NIT:consistency] Card 24's rationale mischaracterizes an already-stale figure
**Location:** Batch 6, Card 24. **Issue:** Card 24 says both numbers in smoke_test.go's "backs five of its thirteen rows" comment are "wrong after batch 5," implying "five" was accurate before. Verified against `internal/loomrecipe/coverage_guard_test.go`'s `loomRowEngines`: only 3 rows use engine `Stub` today (Discussion-Review, Plan-Review, Webster-Review), so "five" is already wrong pre-task, not a regression this task introduces. **Fix:** note this in the card; the corrected end-state text (fourteen rows, two Stub) is unaffected either way.

## Verdict

REQUEST_CHANGES
Two blocking test-coverage/consistency gaps in batches 3 and 5 must be closed before landing.
MILL_REVIEW_END
