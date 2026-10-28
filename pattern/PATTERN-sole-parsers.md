# PATTERN-sole-parsers

Each on-disk format has one parser; consumers read only its model.

## Plan format (`internal/planparser`)

`internal/planparser` is the sole production parser and writer of the on-disk plan format (`_lym/plan/`).

- Consumers read only from the `planparser.Plan` model.
- `SetApproved` (approval), `RewriteRefs` (ref substitution across the plan), `AppendAmendment` (the append-only amendment log), `InsertCard` and `RemoveCard` (one card added or dropped, the cards after it renumbered under a journal) and `RestoreRenumber` (the journal's only reader, which undoes an interrupted renumber) are the write paths, and no others.
- In tests, `internal/testkit/plankit` is the one writer of valid plans, and a test may write raw plan bytes only to test rejection of a malformed plan.
  That admits a second, test-only writer of the plan format, reachable only from `_test.go` files by the testkit importer rule.
- It renders through `planparser.RecognizedFormat`, so a format bump fails at one site, and `TestValidate_GoldenFixture_ZeroFindings` stays the parser-side anchor.

### Ref shapes

`internal/planparser` is the sole declarer of ref-shape vocabulary: classification (`classifyRef` and `refKind`) and the `plan:` handle grammar (`HandlePrefix` and the exported handle helpers).

- Every ref-shape decision in `internal/planparser` and `internal/planglyph` routes through the kind-policy ledger in `internal/planparser/shape.go` or the exported handle vocabulary.
- Enforced by the `refKind` enum to `allRefKinds` sync meta-test, the `refGate` constants to `ledger` keys sync meta-test, the ledger completeness meta-test, and the AST-based boundary-enforcement scans with per-scan package-qualified exempt sets: `{internal/planparser/classify.go, internal/planparser/shape.go}` for the `refKind` scan, plus `internal/planparser/handle.go` for the `plan:`-op scan.
  No planglyph file is exempt.
- The two sync meta-tests are separate obligations, and both parse a const block from the AST because Go cannot reflect over constants.
  Ledger completeness ranges `ledger`'s own keys and therefore cannot see a gate missing from it entirely.

## Discussion format (`internal/discussionparser`)

`internal/discussionparser` is the sole reader of `_lym/discussion/`'s on-disk format.
It imports the standard library only.

## Summary format (`internal/summaryparser`)

In production code, `internal/summaryparser` is the sole declarer of the final-summary artifact's filename and the sole parser of its format.
It imports the standard library only.

## Halt-note format (`internal/haltnote`)

`internal/haltnote` is the sole renderer and parser of the Go-authored halt note `loomcli` writes into the loom friction directory.
It imports the standard library only.

## Recipe format (`internal/shedbuild`)

`internal/shedbuild` is the sole parser of the recipe file format, and declares no on-disk location for recipe files.

## Fixer report format (`internal/landingshed`)

`internal/landingshed`'s `ParseFixerReport` is the sole parser of the merge fixer's report format, which `contracts/specs/landing-fixer-report-spec.md` defines.
Parent-Align and Finalize read only the `FixerReport` it returns.

## Fix review report format (`internal/landingshed`)

`internal/landingshed`'s `ParseFixReviewReport` is the sole parser of the fix review's report format, which `contracts/specs/landing-fix-review-spec.md` defines.
Finalize reads only the `FixReviewReport` it returns.
