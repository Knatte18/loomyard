```yaml
lens: code-tests
outcome: reported
```

Behavior-to-test list:

- Over-cap test-file or line-count changes invoke the review with the fixer range, cap reason and fixer report: `TestFinalize_GateAndFixer`, “a fix over the caps goes to the fix review…” (`finalize_gate_test.go:323–367`).
- Approval lands: “the fix review's end decides…” approval row (`finalize_gate_test.go:380–382`).
- Rejection reroutes with findings and report path: the over-cap table (`finalize_gate_test.go:355`).
- Missing or invalid report reroutes: end-routing rows (`finalize_gate_test.go:385–392`).
- Dirty tree or changed HEAD halts: end-routing rows (`finalize_gate_test.go:395–415`).
- Previously approved matching HEAD skips another review: seeded-record row (`finalize_gate_test.go:443–456`).
- Approval is actually persisted before landing, nonapproval never persists it, and a changed HEAD invalidates it: none.
- No downstream review leaves the existing halt behavior and spawns no reviewer: `finalize_gate_test.go:459–476`; Code-budget and pawl wiring have existing tests.
- Spent failing fixer retains existing routing: existing spent-entry rows.
- Both verdicts, unknown verdict, empty rejected findings, missing file: `TestParseFixReviewReport`.
- Review model and timeout come from fixer configuration: none; `TestFixerTable` tests the separate fixer builder.
- Review timeout reroutes and startup failure is transient: none; `fixerrun_test.go` exercises the separate fixer runner.
- Stale review approval cannot satisfy a new review, and an invalid first report receives exactly one repair turn: none.
- No new sleeps or waits were added; parser tests run in parallel, and integration cases extend the existing fake-based Finalize test.

Findings:

1. **MEDIUM — scope — `internal/landingshed/finalize_gate_test.go:443`**
   **Summary:** The approval-cache test seeds the result instead of covering persistence and invalidation.
   **Evidence:** The test writes `Reviewed: "f1"` itself at line 448; the successful-approval row reaches `requireLanded`, whose lines 173–181 explicitly expect the record to have been deleted. Neither checks the new write in `finalize.go:740–741`. No test supplies a nonmatching nonempty `Reviewed` value to exercise the comparison at `finalize.go:722`.
   **Fix:** Extend Finalize's fake-based tests to approve a review, interrupt landing after approval, read the saved record, resume at the same HEAD without a new session, and resume at a changed HEAD with a new session. Assert rejected and unusable reviews do not save an approval.

2. **MEDIUM — scope — `internal/landingshed/finalize_gate_test.go:370`**
   **Summary:** The new review runner's timeout, failed-start and report-retry safeguards have no behavioral coverage.
   **Evidence:** The end-routing table includes approval, absent report, invalid verdict, dirty tree and moved HEAD only. New branches at `fixreview.go:234–239` and `264–266` are never exercised. `fakeFixerSeats` has timeout and startup-error seams (`fixerrun_test.go:34–36`), but their callers test `fixerRun`, not `runFixReview`. No review test seeds a stale approval for the archive at `fixreview.go:223`, or supplies a second corrected report for the retry configured at line 226.
   **Fix:** Add fake-driven Finalize cases for timeout, `ErrNotStarted` and `Chair.NotStarted`, stale approval followed by no fresh report, and invalid-then-valid reports with exactly one retry. Assert the transient classification, reroute causes, absence of landing where applicable, and bounded turn count.

3. **MEDIUM — scope — `internal/landingshed/finalize_gate_test.go:359`**
   **Summary:** The review table's fixer model and timeout wiring are untested.
   **Evidence:** Assertions at lines 360–364 inspect role, range, cap reason and fixer report only. None inspect the new model resolution and timeout assignments at `fixreview.go:149,164`. Existing assertions in `fixer_test.go:68–69` cover `FixerTable`, which the new `fixReviewTable` does not call.
   **Fix:** Extend the existing review-table assertions with a distinctive configured fixer model/effort and timeout, checking the captured review seat's resolved model choice and table timeout.

Tool-call manifest:

- Call 1: Searched all landing tests for reviewer, timeout, startup and cache assertions; read fixture setup and review-format rules, which were absent or truncated in inherited context.
- Call 2: Read Finalize assertion helpers and remaining resume cases, searched repository tests for direct review-builder/runner coverage, and read fixer-record tests to determine whether persistence and invalidation were covered elsewhere.
- Call 3: Searched retry/archive tests and obtained exact production line numbers for the uncovered behaviors.
