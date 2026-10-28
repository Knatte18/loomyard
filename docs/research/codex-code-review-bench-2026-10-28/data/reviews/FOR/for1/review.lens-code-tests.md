```yaml
lens: code-tests
outcome: reported
```

Behavior-to-test list:

- Over-cap review uses the fixer range, cap reason, role, and fixer report: `TestFinalize_GateAndFixer`, over-cap rejection cases, `finalize_gate_test.go:322–368`.
- Approved review lands: approval row, `finalize_gate_test.go:379–383`.
- Rejection reroutes with findings, report path, and fixer commits: `finalize_gate_test.go:355`.
- Missing report reroutes: `finalize_gate_test.go:384–388`.
- Unknown verdict reroutes: `finalize_gate_test.go:389–393`.
- Dirty reviewer tree halts: `finalize_gate_test.go:394–404`.
- Reviewer moves HEAD and halts: `finalize_gate_test.go:405–415`.
- Previously approved identical HEAD skips review: `finalize_gate_test.go:443–457`.
- Approval is actually persisted before landing, survives an interrupted Finalize, and a changed HEAD invalidates it: none.
- Timeout reroutes with its cause: none for the new review runner.
- Failed reviewer start is transient: none for the new review runner.
- Reviewer uses configured fixer model and timeout: none.
- Malformed/missing report gets exactly one re-prompt and can recover: none; current malformed-report cases never recover or assert the turn count.
- Stale review report cannot authorize the next review: none for the new review caller; shared archive functionality has existing fixer coverage.
- No review after reroute means over-cap halt without a review session: `finalize_gate_test.go:459–475`.
- Spent fixer preserves its route: existing spent-entry cases, `finalize_gate_test.go:273–320`; these now permit any number of review sessions rather than asserting none.
- Parser accepts approval, approval with notes, rejection with findings; rejects unknown verdict and empty rejection; wraps a missing-file error: `TestParseFixReviewReport`, `fixreview_test.go:17–84`.
- Spec registration and deployed path: `contracts/specs/specs_test.go`, registry and relative-path tests.
- Added tests introduce no sleeps or waits; the fake drives turns synchronously.
- No substantiated duplicate-coverage finding.

Finding:

**MEDIUM — design — `internal/landingshed/finalize_gate_test.go:370–457` — The new review lifecycle lacks tests for several independent acceptance guarantees.**

The approval-resume test seeds `Reviewed: "f1"` directly at line 448, while the approval route immediately calls `requireLanded`, whose check only observes that the attempt record was removed after Done (`finalize_gate_test.go:173–180`).
Consequently, deleting the production approval write at `finalize.go:740–743`, or changing its same-HEAD comparison at line 722 to accept any nonempty approval, would leave these tests green.
There is likewise no test exercising the new runner’s timeout branch (`fixreview.go:264–266`), transient-start branch (`fixreview.go:234–239`), or checking its configured model and timeout (`fixreview.go:149,164`).
The existing timeout, start-error, and model tests exercise the separate merge-fixer implementation, not `runFixReview`.
Current missing/invalid-report rows also do not pin the promised one-re-prompt recovery.
These are uncovered behaviors under the rubric’s Test economy rule.
Extend the existing Finalize table and fake runner to cover interrupted approval persistence and changed-HEAD invalidation, review timeout and failed start, configured model/timeout, and successful bounded re-prompt recovery.

Tool-call manifest:

1. Searched landingshed tests for review, approval, timeout, start, model, and lock assertions; obtained exact production/test line references absent from the inherited excerpts.
2. Searched all repository test files for review entry points and approval records; read the fake’s outcome controls and existing fixer-only assertions to distinguish inherited coverage from coverage of the new runner.
