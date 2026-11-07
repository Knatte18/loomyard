# PATTERN-sole-parsers

Each on-disk format has one parser; consumers read only its model.

## Formats and their sole parsers

- The plan: `internal/planparser`.
- The parts file: `internal/partsparser`.
- The discussion: `internal/discussionparser`.
- The summary: `internal/summaryparser`.
- The recipe: `internal/shedbuild`.
- The halt note: `internal/haltnote`.
- The drift expectation and judgement: `internal/driftengine`.
- The raddle proposal: `internal/raddleparser`.
- The lens report's outcome: `internal/lensreport`.

A format another task adds joins this list with its sole parser.

## Plan format (`internal/planparser`)

`internal/planparser` is the sole production parser and writer of the on-disk plan format (`_lym/plan/`).

- Consumers read only from the `planparser.Plan` model.
- These are the write paths, and no others:
  - `SetApproved` (approval).
  - `RewriteRefs` (ref substitution across the plan).
  - `AppendAmendment` (the append-only amendment log).
  - `InsertCard` and `RemoveCard` (one card added or dropped, the cards after it renumbered under a journal).
  - `RestoreRenumber` (the journal's only reader, which undoes an interrupted renumber).
  - `JoinParts` (a plan joined from the plans of a split task's parts).
- In tests, `internal/testkit/plankit` is the one writer of valid plans,
  and a test may write raw plan bytes only to test rejection of a malformed plan.
  That admits a second, test-only writer of the plan format.
  The testkit importer rule makes it reachable only from `_test.go` files.
- It renders through `planparser.RecognizedFormat`, so a format bump fails at one site,
  and `TestValidate_GoldenFixture_ZeroFindings` stays the parser-side anchor.

### Ref shapes

`internal/planparser` is the sole declarer of ref-shape vocabulary:
classification (`classifyRef` and `refKind`) and the `plan:` handle grammar (`HandlePrefix` and the exported helpers).

- Every ref-shape decision in `internal/planparser` and `internal/planglyph` routes through one of two places.
  They are the kind-policy ledger in `internal/planparser/shape.go` and the exported handle vocabulary.
- Enforced by these checks:
  - The `refKind` enum to `allRefKinds` sync meta-test.
  - The `refGate` constants to `ledger` keys sync meta-test.
  - The ledger completeness meta-test.
  - The AST-based boundary-enforcement scans, each with a package-qualified exempt set.
    The `refKind` scan exempts `{internal/planparser/classify.go, internal/planparser/shape.go}`.
    The `plan:`-op scan also exempts `internal/planparser/handle.go`.
    No planglyph file is exempt.
- The two sync meta-tests are separate obligations,
  and both parse a const block from the AST because Go cannot reflect over constants.
  Ledger completeness ranges `ledger`'s own keys and therefore cannot see a gate missing from it entirely.

## Parts format (`internal/partsparser`)

`internal/partsparser` is the sole parser of `_lym/parts/parts.md` and sole declarer of the parts directory's paths.
It reads decision ids only through `discussionparser.DecisionIDs`.
A part directory holds a plan in the plan format, which only `internal/planparser` reads and writes.

## Discussion format (`internal/discussionparser`)

`internal/discussionparser` is the sole reader of `_lym/discussion/`'s on-disk format.
It imports the standard library only.

## Summary format (`internal/summaryparser`)

In production code, `internal/summaryparser` is the sole declarer of the final-summary artifact's filename.
It is also the sole parser of the artifact's format.
It imports the standard library only.

## Halt-note format (`internal/haltnote`)

`internal/haltnote` is the sole renderer and parser of the Go-authored halt note.
`loomcli` writes that note into the loom friction directory.
It imports the standard library only.

## Recipe format (`internal/shedbuild`)

`internal/shedbuild` is the sole parser of the recipe file format, and declares no on-disk location for recipe files.

## Drift expectation and judgement (`internal/driftengine`)

`ParseExpectation` is the only reader of `_lym/drift/expected.yaml`.
`ParseJudgement` is the only reader of `_lym/drift/judgement.yaml`.
Consumers read the `Expectation` and `Judgement` models they return.
Both files are agent-written, so both parsers are strict and refuse an unknown key.

## Raddle proposal (`internal/raddleparser`)

`internal/raddleparser` is the sole parser of the raddle proposal, `_lym/raddle/proposal.yaml`.
It reads the format `contracts/specs/raddle-proposal-spec.md` pins and is the sole home of the checks that judge it.
It imports the standard library and `internal/discussionparser` only.
It reads the Sketch's decision record through `internal/discussionparser` alone.
Consumers read only the `raddleparser.Proposal` model and the findings its checks return.

## Lens report outcome (`internal/lensreport`)

`internal/lensreport`'s `Returned` is the sole Go reader of a lens report.
It reads the `outcome` key alone, in the format `contracts/specs/twill-lens-report-spec.md` pins.

## Fixer report format (`internal/landingshed`)

`internal/landingshed`'s `ParseFixerReport` is the sole parser of the merge fixer's report format.
`contracts/specs/landing-fixer-report-spec.md` defines that format.
Parent-Align and Finalize read only the `FixerReport` it returns.

## Fix review report format (`internal/landingshed`)

`internal/landingshed`'s `ParseFixReviewReport` is the sole parser of the fix review's report format.
`contracts/specs/landing-fix-review-spec.md` defines that format.
Finalize reads only the `FixReviewReport` it returns.
