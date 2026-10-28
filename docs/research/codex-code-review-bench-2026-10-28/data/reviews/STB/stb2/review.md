---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    origin: lens:code-affected-set
    location: internal/shedverbs/loopchild.go:63
    summary: Repeated busy refusals log Warn repeatedly within one failure episode
    fix: In startChild, log the first ETXTBSY refusal at Warn and subsequent refusals in the same retry sequence at Debug, retaining the attempt, delay and error fields; log recovery at Info when a retry succeeds. Update the shedverbs documentation and landing summary to describe the invariant-compliant levels.
  - id: F2
    severity: MEDIUM
    class: design
    origin: lens:code-correctness
    location: internal/shedverbs/loop.go:437
    summary: Cancellation during backoff falsely reports five attempted starts
    fix: Return the actual attempted-start count from startChild and use it in runChild's ETXTBSY cause instead of childStartBusyAttempts. Extend the public loop test table with an injected sleep cancellation after the first refusal and assert one runner call, the actual count in the cause, and the interrupted trace.
  - id: F3
    severity: MEDIUM
    class: design
    origin: lens:code-tests
    location: tools/deploy/main_test.go:289
    summary: Rename-failure cleanup has no behavioral coverage
    fix: Extend TestInstallBinary_RenamesTheBuildOverTheTarget with a deterministic rename failure, such as a nonempty directory at dest. Assert the installation error, unchanged destination contents and removal of the temporary build output.
  - id: F4
    severity: LOW
    class: design
    origin: lens:code-tests
    location: internal/shedverbs/loop_test.go:738
    summary: Retry observability is not asserted by the new loop tests
    fix: Capture logger output in the existing retry test and assert the first refusal warning, subsequent retry records at the levels required by PATTERN-spawn-observability, and their attempt and error fields. Restore logger state through cleanup.
---

### [BLOCKING] Repeated warnings violate the logging episode rule
**Location:** internal/shedverbs/loopchild.go:63
**Issue:** Four consecutive ETXTBSY refusals produce four warnings before the final failed-start warning.
`pattern/PATTERN-spawn-observability.md` requires repeated failures to warn once per episode, and `internal/logger/logger.go:141–142` requires Debug while the failure persists and Info on recovery.
The five-attempt cap satisfies the separate bounded-spawn rule but does not remove the episode rule.
The focus directive and the writer's D1 request Warn on every retry; the supplied constraints explicitly take precedence over that conflicting request.
The terminal did-not-start event can retain its distinct failure diagnostic.

### [MEDIUM] Interrupted backoff reports an exhausted retry budget
**Location:** internal/shedverbs/loop.go:437
**Issue:** If the first runner call returns ETXTBSY and cancellation ends the first sleep, `startChild` returns immediately at `loopchild.go:64–65`.
Only one start was attempted, yet the halt cause says the executable stayed open for writing across five tries.
This contradicts the board's requirement for a self-explanatory spawn failure.
The test lens independently found that no injected sleep returns an error; that coverage gap is consolidated here.

### [MEDIUM] Rename-error cleanup is untested
**Location:** tools/deploy/main_test.go:289
**Issue:** The new table covers a successful rename and a failed build, but neither case reaches `main.go:128–130`.
Removing cleanup from the rename-error branch would leave the suite's new assertions passing while failed installs leave temporary binaries behind.
This is a new behavioral coverage gap under the rubric's test-economy rule.

### [LOW] Retry logging can disappear without a failing assertion
**Location:** internal/shedverbs/loop_test.go:738
**Issue:** The retry tests assert attempts, waits, stop cause and trace path, but never inspect retry log records.
Deleting the retry logging leaves those assertions unchanged even though observability is part of the added behavior.
The assertions should follow the repository's episode policy identified in F1, rather than preserve the current repeated Warn behavior.

## Rejected

- Exact `start-failed after <n> tries: <error>` spelling: this is a claim in the writer's unamended decision record, not a requirement in the board snapshot or an Added after Discussion amendment.
  The current spelling still conveys the failure; the inaccurate cancellation count is separately retained in F2.
- Fallback for every interrupted child: the board asks for diagnostics when start fails, and the new start-failure fallback fulfills that scope.
  Extending it to children that actually ran is not required by the measuring stick.
- Test-lens fallback-precedence gap: existing tests cover child evidence selection, while a fresh trace ID and a child that never executes do not normally produce the proposed competing child trace or stderr evidence.
  No separate actionable production gap was established for those synthetic combinations.
- Predicate portability, executable mode and Windows output suffix: the predicate uses errors.Is with a syscall constant available on the supported platforms, and the Go build output path and executable mode handling support the sibling-file install.

## Focus departures

- “does the start-failed cause read `start-failed after <n> tries: <error>`”: checked, but did not treat this exact spelling as binding because the rubric makes only the board snapshot and explicit amendments authoritative.
- “log each retry at Warn through internal/logger”: checked, but F1 follows the higher-priority repository episode rule instead.

Review range: `161a4a7f..HEAD`.
No builds, tests, deploys or mutating git commands were run; validation was by source inspection.
