```yaml
lens: code-tests
outcome: reported
```

Behavior-to-test list:

- Wrapped ETXTBSY retry, then successful child execution: `TestLoop_ChildStartRetriesABusyExecutable/BusyThenStarted`.
- Five total attempts and 100/200/400/800 ms waits: `BusyOnEveryTry`.
- Non-ETXTBSY fails immediately: `OtherStartErrorNotRetried`.
- Exhausted-start cause and loop trace reach hook and envelope: both interrupted rows in the new loop test.
- Child trace and nonempty stderr selection: existing `TestLoop_InterruptedChildFillsTheErrorForm` rows cover the original selection; they do not arm `LoopTraceFile`.
- Retry warnings: no assertion covers them.
- Interrupted retry sleep ends attempts: no test returns an error from the injected retry sleep.
- Successful atomic replacement and preservation of an open old file: `TestInstallBinary_RenamesTheBuildOverTheTarget/BuildSucceeds`.
- Build failure preserves target and removes temporary output: `/BuildFails`.
- Rename failure removes temporary output and preserves destination: no test.
- Added tests sit out no real retry waits. The loop test follows its file-level process-global logger exception to parallel execution; deploy cases run in parallel.
- No duplicated coverage identified.

Findings:

1. **MEDIUM · design · `tools/deploy/main_test.go:289` — Rename-failure cleanup lacks coverage.**  
   The only cases are successful build/install and build failure (`main_test.go:290–291`), so neither reaches the new cleanup/error branch at `tools/deploy/main.go:128–130`. Removing that cleanup would leave every added test green despite leaking a built binary on a failed deployment. Add a deterministic rename-failure case, for example a nonempty directory at `dest`, and assert an installation error, unchanged destination contents and absence of the temporary binary.

2. **MEDIUM · design · `internal/shedverbs/loop_test.go:726` — Interrupted retry waits lack coverage.**  
   The new sleep stub always returns nil (`loop_test.go:726–729`), leaving `loopchild.go:64–65` untested. Add a public-loop case whose injected sleep returns cancellation after the first ETXTBSY, asserting no second runner call and the resulting interrupted cause/trace. This should also pin the actual number of attempts reported after interruption.

3. **LOW · design · `internal/shedverbs/loop_test.go:738` — Retry logging lacks coverage.**  
   No assertion reads the logs written by the new retry branch at `loopchild.go:63`. Deleting the warning, lowering its level, or losing its attempt/error fields leaves all tests green. Extend the existing retry test using the logger capture seam to assert the expected retry records and their levels; do not add a separate helper-level test.

4. **LOW · design · `internal/shedverbs/loop_test.go:730` — New fallback precedence lacks coverage.**  
   The new cases arm `LoopTraceFile` only when neither child trace nor stderr exists, while existing child-trace/stderr tests leave it nil. Consequently an implementation that unconditionally replaces child evidence with the loop trace on start failure would pass. Add start-failure cases with a child trace or nonempty stderr and an armed loop fallback, asserting that the child evidence wins; also include an empty fallback result.

Tool calls:
- Searched loop/deploy/proc tests, read later interrupted-stop cases and review format because inherited excerpts did not establish all existing coverage or finding format.
- Read the remaining interrupted-child cases, deploy test header, trace-related tests across arming packages, and numbered changed test/implementation lines to distinguish existing coverage from gaps and supply precise evidence.
