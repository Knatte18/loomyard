---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    origin: handler
    location: internal/shedverbs/loopchild.go:63
    summary: Repeated busy refusals log Warn repeatedly within one unchanged failure episode
    fix: In startChild, log the first ETXTBSY refusal at Warn and subsequent refusals at Debug, with recovery at Info, following the logger episode policy; extend the existing retry table to capture and assert those levels. Update the retry-level description in the landing summary to match.
  - id: F2
    severity: MEDIUM
    class: design
    origin: lens:code-correctness
    location: internal/shedverbs/loop.go:437
    summary: Cancelling the first backoff reports five starts when only one occurred
    fix: Return the actual attempt count from startChild and use it in runChild's busy-start cause instead of childStartBusyAttempts. Add a row to TestLoop_ChildStartRetriesABusyExecutable whose injected Sleep returns context.Canceled after the first refusal, asserting one start, no further retry, and an accurate cause.
  - id: F3
    severity: MEDIUM
    class: design
    origin: lens:code-tests
    location: tools/deploy/main_test.go:291
    summary: The new rename-failure cleanup behavior has no test coverage
    fix: Extend TestInstallBinary_RenamesTheBuildOverTheTarget with an incompatible-directory destination that makes rename fail after a successful fake build; assert the returned error, unchanged destination contents, and removal of the temporary binary.
  - id: F4
    severity: MEDIUM
    class: design
    origin: lens:code-tests
    location: internal/shedverbs/loop_test.go:730
    summary: Tests do not protect child evidence from being overwritten by the new loop trace fallback
    fix: Extend the existing start-failure table with rows supplying a child trace and non-empty captured stderr while LoopTraceFile is armed; assert that the child trace takes precedence over stderr and that stderr takes precedence over the loop trace in both AfterInterrupt and the envelope.
---

### [BLOCKING] Repeated warnings violate the episode policy

**Location:** internal/shedverbs/loopchild.go:63
**Issue:** Persistent ETXTBSY produces four Warn retry records for the same executable-busy episode, without any successful start or other recovery between them.
`pattern/PATTERN-spawn-observability.md` requires logging loops to follow the logger policy and specifically says a failure repeating each tick logs Warn once per episode.
`internal/logger/logger.go:141` specifies Warn once, Debug while the failure persists, and Info at recovery.
The decision writer's D1 and the focus request for Warn on every retry conflict with this constraint; step 1 explicitly gives the constraint precedence.
The bounded count satisfies the spawn-count rule but does not exempt repeated failures from the episode rule.

### [MEDIUM] Cancellation produces an inaccurate halt cause

**Location:** internal/shedverbs/loop.go:437
**Issue:** After the first ETXTBSY refusal, cancellation during the injected or production sleep causes `startChild` to return immediately at `loopchild.go:64–65`.
Only one runner call occurred, yet `runChild` says the executable stayed open for writing across five tries because it formats the configured maximum rather than an observed count.
This is a concrete incorrect diagnostic in the self-explanatory halt note requested by the board entry.
The existing tests always return nil from Sleep, so they miss this path.
The cancellation coverage portion of the tests lens's second finding is consolidated here.

### [MEDIUM] Rename failure cleanup is untested

**Location:** tools/deploy/main_test.go:291
**Issue:** The table contains only successful build and failed build cases against a replaceable file.
Neither reaches the new rename-error cleanup branch at `main.go:128–130`.
Removing that cleanup would leave these tests green and leave a built temporary binary behind after a refused installation.
This is an uncovered new behavior under the rubric's test-economy rule.

### [MEDIUM] Trace precedence is untested with fallback enabled

**Location:** internal/shedverbs/loop_test.go:730
**Issue:** The new start-failure cases provide a loop trace but no child trace or non-empty stderr.
Existing interrupted-child cases exercise child evidence with the new callback unset.
Consequently, overwriting a start failure's available child evidence unconditionally with the loop trace would pass both sets of tests.
The fallback's conditional behavior needs coverage with the callback enabled and child evidence present.

## Rejected

- Exact `start-failed after <n> tries` spelling: the authoritative board snapshot asks for explanatory spawn detail, not this exact format; the decision writer's unamended D4 is not the measuring stick under the rubric.
- Extending fallback to every interrupted child: the board's requested diagnostic change concerns start failures, and no authoritative amendment expands that scope.
- A separate test demanding Warn on every retry: that expectation conflicts with the governing logging policy; logging coverage is included in F1 with the compliant levels instead.
- A separate empty-callback-result test: returning an empty path introduces no additional selection branch beyond the already exercised absent-evidence path; the material precedence coverage gap is retained as F4.

## Focus departures

- "retry only ETXTBSY, and log each retry at Warn through internal/logger": the retry predicate and count were reviewed, but the requested repeated Warn behavior conflicts with PATTERN-spawn-observability's episode policy, as recorded in F1.
- "does the start-failed cause read `start-failed after <n> tries: <error>`": it does not; the format-only discrepancy is not a finding because the rubric makes the board snapshot, amended only by Added after Discussion entries, authoritative.

The committed range reviewed was `161a4a7f..HEAD`.
No builds or tests were run under the read-only review instructions.
The predicate uses errors.Is and accepts wrapped ETXTBSY; the retry count and doubling waits are bounded and injected.
Both production arming sites pass logger.TraceFile lazily, and an unavailable logger path remains empty.
The deployment path builds beside its target, retains the Go build flags and cgo setting, and consumes the temporary pathname through rename on success.
The package documentation accompanies the changes; no declared PATTERN amendment was supplied.
