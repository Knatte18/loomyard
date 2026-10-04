# PATTERN-sole-parsers

Each on-disk format has one parser; consumers read only its model.

## Plan format (`internal/planparser`)

`internal/planparser` is the sole production parser and writer of the on-disk plan format (`_lyx/plan/`).

- Consumers read only from the `planparser.Plan` model.
- `SetApproved` (approval), `RewriteRefs` (ref substitution across the plan) and `AppendAmendment` (the append-only amendment log) are the three write paths, and no others.
- In tests, `internal/testkit/plankit` is the one writer of valid plans, and a test may write raw plan bytes only to test rejection of a malformed plan.
  That admits a second, test-only writer of the plan format, reachable only from `_test.go` files by the testkit importer rule.
- It renders through `planparser.RecognizedFormat`, so a format bump fails at one site, and `TestValidate_GoldenFixture_ZeroFindings` stays the parser-side anchor.

## Discussion format (`internal/discussionparser`)

`internal/discussionparser` is the sole reader of `_lyx/discussion/`'s on-disk format.
It imports the standard library only.

## Summary format (`internal/summaryparser`)

In production code, `internal/summaryparser` is the sole declarer of the final-summary artifact's filename and the sole parser of its format.
It imports the standard library only.

## Recipe format (`internal/shedbuild`)

`internal/shedbuild` is the sole parser of the recipe file format, and declares no on-disk location for recipe files.
