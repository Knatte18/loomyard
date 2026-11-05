```yaml
lens: code-tests
outcome: reported
```

Behavior-to-test list:

- Wrapped ETXTBSY followed by success: `TestLoop_ChildStartRetriesABusyExecutable/BusyThenStarted` pins three starts and waits of 100/200 ms.
- Persistent ETXTBSY: `.../BusyOnEveryTry` pins five starts, four doubling waits, interrupted stop, cause and loop trace.
- Other start error: `.../OtherStartErrorNotRetried` pins one start and no wait.
- Retry logging at Warn: no assertion.
- Sleep interrupted by cancellation: no test row.
- Start failure with no child trace/stderr: the two failure rows pin the loop trace fallback.
- Fallback with an empty callback result, child-trace precedence, and non-empty-stderr precedence while `LoopTraceFile` is configured: no test rows.
- Existing child trace and stderr selection: `TestLoop_InterruptedChildFillsTheErrorForm` covers these with `LoopTraceFile` unset.
- Deploy success, preserving an already-open old file, and removing the temporary name: `TestInstallBinary_RenamesTheBuildOverTheTarget/BuildSucceeds`.
- Deploy build failure preserves target and removes partial output: `.../BuildFails`.
- Deploy rename failure preserves target and removes temporary output: no test row.
- Retry waits are injected, so the new test sits out no production backoff.
- No duplicate coverage found: the existing interruption tests never produce a runner start error, and the previous deploy tests do not cover replacement.

Findings:

1. **MEDIUM, design — `tools/deploy/main_test.go:291` — Rename-failure cleanup has no coverage.**  
   Evidence: the only cases are `BuildSucceeds` and `BuildFails` at lines 291–292; both use a replaceable file destination, so neither reaches `tools/deploy/main.go:128`'s rename-failure branch. This leaves the newly required removal of the successfully built temporary binary after a failed rename untested. Add a case that makes the destination an incompatible directory, checks the rename error, verifies the destination remains intact, and checks that no temporary binary remains.

2. **MEDIUM, design — `internal/shedverbs/loop_test.go:677` — The retry test does not pin its Warn diagnostics or cancelled-wait exit.**  
   Evidence: the new test records delays and asserts starts, stop and cause, but its Sleep callback at lines 726–729 always succeeds and it makes no log assertions. Removing the retry Warn call in `loopchild.go:63`, or ignoring the Sleep error at lines 64–65, would leave these tests green. Extend the existing table to capture retry logs and assert their count and Warn level, and add an injected Sleep-error case proving no further start occurs.

3. **MEDIUM, design — `internal/shedverbs/loop_test.go:730` — Trace fallback precedence is untested with the new callback armed.**  
   Evidence: every new failure case installs a non-empty loop trace at lines 730–731 while supplying neither a child trace nor stderr. Existing child-trace and stderr cases at lines 358–450 leave `LoopTraceFile` unset. An implementation that unconditionally overwrote a start failure's trace with the loop path would pass. Add failure rows with a supplied child trace, non-empty stderr, and an empty loop callback result, checking the resulting hook and envelope trace.

Tool calls:
- Call 1: searched tests for all changed behavior and read the previously unseen existing interrupted-child rows to determine whether they cover fallback precedence; read the review format to use the required severity/class vocabulary.
