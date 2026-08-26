MILL_REVIEW_BEGIN
# Review: Fix Bouncer anchor-path and run-dir clearing — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5
reviewed_file: plan/
date: 2026-08-26
```

## Findings

### [BLOCKING:scope] Card 10's test-breakage enumeration misses a verdict-identical test
**Location:** batch 2 (rundir-clear), card 10
**Issue:** `TestBouncer_Judged_IgnoresFocusFile` in `internal/shedadapters/bouncer_replay_test.go` lays out round 1 with a report plus an APPROVED verdict and ledger, then asserts `Call()` returns `shedengine.Done` with the shuttle never invoked — the identical fixture shape to `TestBouncer_Replay_Approved`, which card 10 explicitly names and rewrites. After card 10's clear-and-reseed trigger lands, this exact disk state also fires the trigger, so `Call()` instead archives, reseeds, invokes the shuttle, and returns `Stuck` — contradicting both of the test's existing assertions (`outcome != shedengine.Done` and `shuttle.called`). Card 10's own closing sentence ("`TestBouncer_Replay_Blocking`, `TestBouncer_Cancellation_DuringRun_ParsedVerdictSurvives`, and the remaining pointer-discipline subtests reach `settle` through the judge path or a BLOCKING verdict and are unaffected") is factually false for this test: it is not a pointer-discipline subtest, and it reaches replay through an APPROVED verdict already on disk at `Call` entry, not through the judge path or a BLOCKING verdict. Card 9 even flags this same test by name as the load-bearing proof that the `judged`/`judgedVerdict` split is behavior-preserving, which makes its silent breakage one card later easy to miss.
**Fix:** Add `TestBouncer_Judged_IgnoresFocusFile` to card 10's named list of tests to repair — either rewrite it to assert the new clear-and-reseed outcome (mirroring the `TestBouncer_Replay_Approved` rewrite already specified), or change its fixture to a BLOCKING verdict so it keeps isolating "judged ignores the absent focus file" without colliding with the new APPROVED-triggered clear.

## Verdict

REQUEST_CHANGES
Card 10's own enumeration of tests broken by the clear-trigger change is demonstrably incomplete.
MILL_REVIEW_END
