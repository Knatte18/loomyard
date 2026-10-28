---
verdict: APPROVED
findings:
  - id: F1
    severity: MEDIUM
    class: design
    origin: lens:code-correctness
    location: internal/shedverbs/loop.go:435
    summary: Failed-start causes omit the required format and can overstate the actual attempt count
    fix: 'Return the actual attempted-start count from startChild and construct every failed-start cause as start-failed after <n> tries: <error>, appending the executable-open-for-writing explanation for ETXTBSY only. Update the existing cause assertions in loop_test.go and the description in shedverbs/doc.go.'
  - id: F2
    severity: MEDIUM
    class: design
    origin: lens:code-affected-set
    location: internal/shedverbs/loop.go:311
    summary: The loop trace is discarded when a child starts but exits without trace or stderr
    fix: Move the optional LoopTraceFile fallback into interruptedTraceFile after child-trace and non-empty-stderr selection, including the nil StopFiles case. Remove the startFailed restriction and preserve the separate status-read-stop handling. Extend the existing interruption cases to cover a started child with neither output and an available loop trace, and update shedverbs/doc.go accordingly.
  - id: F3
    severity: MEDIUM
    class: scope
    origin: lens:code-tests
    location: internal/shedverbs/loop_test.go:677
    summary: Retry cancellation and warning emission have no behavioral coverage
    fix: Extend TestLoop_ChildStartRetriesABusyExecutable with an injected sleep failure and assert that no further start occurs and the cause names the actual attempt count. Capture the logger output in the existing retry cases and assert one Warn for each retry with its attempt and delay, without waiting in real time.
  - id: F4
    severity: MEDIUM
    class: scope
    origin: lens:code-tests
    location: internal/shedverbs/loop_test.go:731
    summary: Trace-fallback coverage omits source precedence and production callback wiring
    fix: Arm LoopTraceFile in the existing child-trace and stderr interruption cases and assert those sources retain precedence; add an empty callback-result case. Extend the existing loomcli and battencli arming tests to verify that their loop specifications expose the logger trace through LoopTraceFile, using the existing isolated logger facilities.
  - id: F5
    severity: MEDIUM
    class: scope
    origin: lens:code-tests
    location: tools/deploy/main_test.go:289
    summary: Atomic-install tests omit the required cleanup after rename failure
    fix: Add a rename-refusal row to TestInstallBinary_RenamesTheBuildOverTheTarget, using an existing nonempty directory as the destination and a successfully written temporary binary. Assert an installation error, preservation of the destination and its contents, and removal of the temporary build without spawning a process.
---

Reviewed `git diff 161a4a7f..HEAD` and both commit messages against the board snapshot, D1–D4, their acceptance criteria, the change description, and the relevant repository rules.
The verdict uses the format's distinction between MEDIUM findings and BLOCKING findings; the findings below still require correction.
No build, test, or deploy was run because this review permits read-only inspection only.

### [MEDIUM] Report actual start attempts in the settled cause format

**Location:** internal/shedverbs/loop.go:435
**Issue:** D4 requires `start-failed after <n> tries: <error>` for a failed start.
The code retains `start-failed: <error>` and names a count only for ETXTBSY, always using the maximum of five.
If the first ETXTBSY refusal is followed by cancellation during the injected or real sleep, `startChild` returns at loopchild.go:65 after one attempt, yet the halt reason claims five.
An ordinary non-ETXTBSY failure has no count at all, including one following earlier busy refusals.
The cause flows unchanged into `loomAfterInterrupt` and the halt note; the new test expectations at loop_test.go:702 and :710 encode the divergence rather than the decision.

### [MEDIUM] Apply the trace fallback to every evidence-free child interruption

**Location:** internal/shedverbs/loop.go:311
**Issue:** D4 and its acceptance criterion require an interrupted stop with neither child trace nor non-empty stderr to record the loop's own trace when available.
The new fallback requires `res.startFailed`, so a successfully spawned child that exits without an envelope or diagnostic output still leaves an empty trace.
That path returns a result without `startFailed` at loop.go:480, and `interruptedTraceFile` returns empty at loop.go:538 onward.
The empty path is then copied into the halt note despite an available loop trace.
Both production armers supply the callback correctly; the selection condition is the missing case.

### [MEDIUM] Exercise retry cancellation and retry diagnostics

**Location:** internal/shedverbs/loop_test.go:677
**Issue:** The new table covers eventual success, exhaustion, and a non-busy refusal, but its injected sleep always succeeds and it never inspects logging.
Ignoring a failed sleep or removing the Warn at loopchild.go:63 would leave those assertions passing.
These are added behaviors with no coverage under the rubric's test-economy rule.
An injected cancellation case also pins the actual-count failure described in F1 without sitting out a production delay.

### [MEDIUM] Cover fallback precedence and the real arming seams

**Location:** internal/shedverbs/loop_test.go:731
**Issue:** Every new interrupted case installs a non-empty fake loop trace and supplies no child evidence.
Existing child-trace and stderr cases leave the callback nil, so they do not establish precedence when both sources are available.
The new behavior's empty callback result is also untested.
The production assignments at loomcli/looparm.go:52 and battencli/arm.go:538 have no assertion in their existing arming tests, which exercise process registration instead.
Removing either assignment would preserve the new tests while losing the fallback for that production caller.

### [MEDIUM] Cover installation failure after a successful build

**Location:** tools/deploy/main_test.go:289
**Issue:** D3 explicitly requires temporary-file removal after rename failure.
The new table contains only successful installation and failed build, so neither row reaches main.go:128–130's error branch.
Removing its cleanup would leave the suite's new assertions passing while leaving a temporary binary behind after a refused replacement.

## Consolidation

The affected-set and correctness reports describe the same two D4 defects and are consolidated into F1 and F2.
The tests report's contradictory cause assertions are part of F1, not a separate defect.
No duplicate test coverage was identified.
The retry limit, doubling injected waits, ETXTBSY predicate, production trace callback assignments, and same-directory build/rename path are otherwise consistent with their intended behavior on inspection.
The docs land with the implementation, but their narrowed cause and fallback descriptions must change with F1 and F2.
No new cross-cutting invariant or declared PATTERN change was identified.

## Rejected

- Repeated Warn calls as an independent logging-policy violation: the logger's detailed policy explicitly admits retry warnings and its hard repetition bound is roughly ten iterations; this retry has at most five starts and D1 explicitly requires a Warn per retry.
- A separate helper-only test for IsSpawnBusy: the existing new loop table already exercises a wrapped ETXTBSY error through the consuming behavior, so another test of that same case would duplicate coverage.
