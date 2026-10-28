# Fixer-report — code round 1

## Changes

- **F1** (fixed with an own change to the first half): the review proposed reading the attempt record after "an approval lands", but a landing removes the record.
  The new test "an approval is kept in the attempt record as the HEAD it approved" refuses the push, which stops the landing after the merge and keeps the record, then asserts `Reviewed == "f1"`.
  The "pass at the approved HEAD" subtest is now a table with a second row: a seeded `Reviewed` of a different HEAD, a rejecting review, one fix review session and a reroute.
- **F2** (fixed as proposed): rows in "the fix review's end decides" for a review timeout (reroute naming `fixer_timeout_min`) and for an unparseable report followed by an approval (landing).
  A separate test pins a failed review start as a transient error with nothing landed, since the table's runner fails on any error.
  The fake gained `reviewTimeout` and `reviewStartErr`, which apply to the fix review role alone so the fixer session of the same run is unaffected.
- **F3** (fixed as proposed): the resumed-entry test asserts the exact session count, none for the rows that spawn no review and one for the row that scripts a review, besides the role check.
- **F4** (fixed as proposed): the over-cap table row asserts the review table's `Timeout` equals `fixer_timeout_min` and its `ModelChoice` equals the fixer table's.
- **F5** (fixed as proposed): `dirtyWayForward` is now a constant in `verifygate.go` that `dirtyReason` uses, and `runFixReview` trims it from the cause of a changed review.
  The dirty-tree row asserts the halt reason lacks "commit or remove them".
- **F6** (fixed as proposed): `contracts/specs/llm-model-spec.md` and `docs/overview.md` name the fix review session among what `fixer` and `fixer_timeout_min` govern.
  The overview wording was reshaped after the self-check so the key list stays intact.

Commits: F6 docs, F5, F1–F4 together (the F5 absence assertion is in the test file and rides in that commit), and the overview wording fix.
`lym gate test ./internal/landingshed` is green.

## Self-check

- fix-consistency, LOW: the relative clause in `docs/overview.md` split the key list — fixed as proposed.
- fix-code-correctness: no findings.
- fix-proposal: no findings.
- fix-affected-set: no findings.

## Disputed

None.

## Deferred

None.
