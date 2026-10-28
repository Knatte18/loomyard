---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: internal/shedverbs/loop.go:432-440
    summary: >-
      A failed child start does not read start-failed after <n> tries as D4 and its acceptance criteria require; only the ETXTBSY case carries a suffix and no case names the tries in the decided form.
    fix: >-
      In runChild build the cause as fmt.Sprintf("start-failed after %d tries: %s", tries, err.Error()) for every start error, with tries the attempts startChild actually made (1 for a non-ETXTBSY error), and for proc.IsSpawnBusy(err) append "; the executable was open for writing, as while a deploy replaces it in place". Make startChild return the tries made alongside the child and error. Update the loop.go and doc.go wording and the wantCause strings in TestLoop_ChildStartRetriesABusyExecutable to the new form.
    origin: lens:code-correctness
  - id: F2
    severity: MEDIUM
    class: design
    location: internal/shedverbs/loop.go:311-313
    summary: >-
      The loop-trace fallback also requires res.startFailed, so a child that started and died leaving no trace and no non-empty stderr still gets an empty trace_file, which D4 and its acceptance criterion say the loop's own trace file replaces.
    fix: >-
      In runLoop drop the res.startFailed condition so traceFile falls back to loop.LoopTraceFile() whenever interruptedTraceFile returns an empty string and LoopTraceFile is non-nil, and remove the childResult.startFailed field and its set in runChild if nothing else reads it. Add a test row where a started child ends interrupted with no trace file and empty stderr and assert the loop's own trace file.
    origin: lens:code-correctness
  - id: F3
    severity: LOW
    class: design
    location: internal/shedverbs/loopchild.go:65-67
    summary: >-
      When the context ends during a backoff, startChild returns the busy error after fewer than five tries, and runChild still reports that the executable stayed open across five tries.
    fix: >-
      Report the tries startChild actually made in the cause, as F1's fix does, so a cancelled wait names the real count. Add a test row where LoopSpec.Sleep returns an error and assert one try and a cause naming one try.
    origin: lens:code-correctness
  - id: F4
    severity: LOW
    class: scope
    location: internal/shedverbs/loop_test.go:677-757
    summary: >-
      The fallback's empty-path cases have no row, LoopTraceFile nil and LoopTraceFile returning an empty string.
    fix: >-
      Add rows to TestLoop_ChildStartRetriesABusyExecutable, or beside it, that leave LoopTraceFile nil and that return an empty string, and assert the interrupted stop's trace_file stays empty without a panic.
    origin: lens:code-tests
  - id: F5
    severity: LOW
    class: scope
    location: tools/deploy/main_test.go:279-332
    summary: >-
      The rename-failure branch of installBinary, which D3 says removes the temporary file and leaves the target as it was, has no test row.
    fix: >-
      Add a row to TestInstallBinary_RenamesTheBuildOverTheTarget whose build callback creates the temporary path as a non-empty directory so the rename over the target file fails, and assert an error, the target content unchanged and no temporary entry left in the directory.
    origin: lens:code-tests
---

### [BLOCKING] Start-failed cause is not the decided form

**Location:** internal/shedverbs/loop.go:432-440, with internal/shedverbs/doc.go:40-41 and internal/shedverbs/loop_test.go:702-710 pinning the divergent text.
**Issue:** D4 and the focus directive give the cause `start-failed after <n> tries: <error>`, with the ETXTBSY meaning added.
The code writes `start-failed: <error>` for every error and appends `; the executable stayed open for writing across N tries ...` only for ETXTBSY.
A non-ETXTBSY failure never names its tries, and the ETXTBSY case is not in the decided form.
The doc and test encode the code's form, so no check catches the difference.
The code-affected-set lens raised the same defect as MEDIUM; the decision record's explicit text and acceptance criterion make it a failure to make the change asked for.

### [MEDIUM] Trace fallback needs a start failure, not just an empty trace

**Location:** internal/shedverbs/loop.go:311-313
**Issue:** D4's acceptance criterion says an interrupted stop whose child left no trace and no stderr records the loop's own trace file.
The condition `traceFile == "" && res.startFailed` limits that to a child that never started, so a child that started, died and left nothing still ends with an empty `trace_file`.
The loop's trace holds the interrupted stop's context in that case as well.
The code-tests lens raised the missing row as LOW; it is folded into this finding's fix.

### [LOW] A cancelled backoff reports tries that never happened

**Location:** internal/shedverbs/loopchild.go:65-67
**Issue:** `startChild` returns `nil, err` when `sleep` fails, after fewer than `childStartBusyAttempts` tries, while `runChild` states the executable stayed open across all of them.
The cause then says something false about the host.

### [LOW] Empty-path fallback cases are untested

**Location:** internal/shedverbs/loop_test.go:677-757
**Issue:** The focus directive names the empty-path case.
The test always supplies a non-empty `LoopTraceFile`, so a nil field and an empty return have no row.

### [LOW] Rename failure of installBinary is untested

**Location:** tools/deploy/main_test.go:279-332
**Issue:** D3 asks the temporary file be removed when the build or rename fails.
Only the build-failure row exists, so the rename branch's cleanup could regress unseen.

## Rejected

- code-affected-set F2, doc.go says "the lym binary" while the retry covers any child executable: the loop's child is the lym binary in every production spec, and the sentence states the reason for the retry, not its reach.
- code-tests 3, cancellation path untested: folded into F3's fix, which adds that row.
- code-tests 5, no test asserts `LoopTraceFile` is set in `looparm.go` and `arm.go`: a constructor wiring assertion duplicates what the loop test pins, and the lens could not name an existing spec-construction test to extend.
- code-tests 6, Warn log on each retry unpinned: logging is not a behavior the rubric requires a test for, and the log call is a single statement beside the retry.
- code-tests 7, no `t.Parallel()` in the new test: the file's header states no test in it calls `t.Parallel` because the loop logs through the process-global logger, which is the reason the golang-testing rule asks for.
- code-tests 1, the started-child fallback row: kept as part of F2's fix, not a separate finding.
- Both commits carry a `Co-Authored-By: Claude Opus 5.5` trailer: the commit trailers are outside the diff's code and the rubric's commit-message check concerns `Pattern-Departure:` trailers, so no finding.
