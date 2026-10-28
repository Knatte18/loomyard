```yaml
lens: code-tests
outcome: reported
```

Behavior-to-test list:

- Over-cap changes with a review after reroute spawn one review with the fixer’s range, cap reason and report path: covered by `TestFinalize_GateAndFixer/a fix over the caps goes to the fix review...` (`finalize_gate_test.go:323–368`).
- Approval permits landing: covered by `TestFinalize_GateAndFixer/the fix review's end decides.../an approval lands` (`finalize_gate_test.go:379–382`).
- Rejection reroutes with findings, report path and fixer commits: covered by the over-cap table (`finalize_gate_test.go:355`).
- Missing and unparseable reports reroute: covered by the end-routing table (`finalize_gate_test.go:384–393`).
- Dirty tree and changed HEAD halt without merging: covered by the end-routing table (`finalize_gate_test.go:395–433`).
- Review timeout reroutes, including when an approval report exists: no test.
- Review startup failure becomes a transient error: no test.
- Approval writes `Reviewed` before landing so an interrupted Finalize can reuse it: no test.
- A matching, preseeded `Reviewed` skips review: covered by `a pass at the HEAD the fix review approved lands with no second review` (`finalize_gate_test.go:443–457`).
- A changed HEAD invalidates a previous approval and requires review: no test.
- Only approval stores a new reviewed HEAD: no test.
- Review uses the configured fixer model-spec and timeout: no test.
- Review retries a malformed report once and accepts its corrected replacement: no test asserts the retry count or successful second turn.
- A previous approval report is archived before a new review: no review-path test; `TestFixerRun` covers the shared archiving operation through the merge-fixer path.
- No review after reroute keeps the prior halt behavior and spawns no review: covered by `with no review after the reroute...` (`finalize_gate_test.go:459–477`), with existing loom wiring tests covering Code budget and recipe eligibility.
- Spent fixer keeps its previous route: covered by the resumed-entry table (`finalize_gate_test.go:284–285`); that table prohibits fixer roles but does not explicitly prohibit review roles.
- Review executes while the parent lock is held: no review callback checks the lock; existing lock tests exercise the merge fixer.
- Parser accepts both verdicts and findings, rejects unknown verdicts and an empty rejected Findings section, and preserves missing-file identity: covered by `TestParseFixReviewReport`.
- New spec registration and deployed relative path: covered by `contracts/specs/specs_test.go:24,62`.
- New test timing: no sleeps, production-duration waits or subprocesses were added.
- Test redundancy: no concrete redundant new test identified; parser tests pin the file contract and the Finalize tests pin routing.

Finding:

**BLOCKING — design — `internal/landingshed/finalize_gate_test.go:370–457` — The review tests leave several explicit acceptance criteria and the approval persistence boundary untested.**

The new end-routing table exercises approval, missing/malformed reports and branch mutations, but never produces a review timeout or review startup failure.
The existing timeout and transient-start tests in `fixerrun_test.go:210–245,370–388` call `fixerRun.run`, not the new `runFixReview`, so they do not cover `fixreview.go:232–242,264–266`.
Likewise, the resume case seeds `Reviewed: "f1"` directly (`finalize_gate_test.go:448`) and the approval case immediately completes landing, whose helper requires the record to have been removed (`finalize_gate_test.go:174–185`).
Consequently, deleting the approval write in `finalize.go:740–743` would leave these tests passing, as would incorrectly honoring any nonempty reviewed SHA instead of matching the current HEAD.
No assertion on the review table checks `ModelChoice` or `Timeout`, despite D5’s explicit acceptance criterion; the existing model/timeout assertions target `FixerTable`, a separate builder.

This is one recurring coverage-enumeration gap under the rubric’s test-economy requirement and `PATTERN-test-speed`.
Extend the existing Finalize table to cover the review-specific timeout/start branches, configuration propagation, and a real approval followed by an interrupted landing and resumed Finalize at both the same and a different HEAD.
Also pin the bounded retry and stale-report behavior through that public path rather than adding tests for each helper.
The evidence is the tests above and the unexercised production branches in `fixreview.go:147–168,223–242,264–266` and `finalize.go:722,739–744`.

Tool-call manifest:

1. Searched all landingshed tests for review, record, timeout, model and startup assertions because inherited context contained changed hunks but not the complete existing coverage.
2. Read the Finalize fixture and unchanged test rows, the existing fixer startup/configuration tests and attempt-record tests, and searched all repository tests for review identifiers to establish whether coverage existed outside the changed files.
3. Read the remainder of the test-speed background, searched Finalize tests for parallelism and lock coverage, and read the fake seat runner’s full behavior because those details were absent or truncated in inherited context.
