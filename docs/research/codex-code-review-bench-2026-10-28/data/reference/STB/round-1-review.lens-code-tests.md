```yaml
lens: code-tests
outcome: reported
```

Task: the code-tests lens. For each behaviour the diff adds or changes, I name the test row that would fail if it broke.

**Per behaviour: pinning test**
- **`proc.IsSpawnBusy` matches a wrapped ETXTBSY and rejects other errors.** `TestLoop_ChildStartRetriesABusyExecutable` pins it indirectly. `BusyThenStarted` and `BusyOnEveryTry` use a `fs.PathError` wrapping ETXTBSY, and `OtherStartErrorNotRetried` uses ENOENT. There is no direct test in `internal/proc`, which the public-surface test covers, so no finding.
- **Five tries in all.** The `BusyOnEveryTry` row checks `wantTries == childStartBusyAttempts`.
- **Waits start at 100 ms and double through `LoopSpec.Sleep`.** `BusyThenStarted` and `BusyOnEveryTry` check the recorded waits, and `Sleep` is injected, so no real waiting.
- **A refused start followed by an accepted one runs the step.** `BusyThenStarted` expects `LoopStopHalted`.
- **Another start error is not retried.** `OtherStartErrorNotRetried` expects 1 try and no waits.
- **Cause text for ETXTBSY and for another error.** `BusyOnEveryTry` and `OtherStartErrorNotRetried` check the exact `AfterInterrupt` cause. These rows pin the current wording, `start-failed: <err>; the executable stayed open ... across 5 tries`. The decision record's D4 wording, `start-failed after <n> tries: <error>`, differs from it. The code-correctness lens owns that mismatch.
- **Interrupted stop with a start failure records the loop's own trace file.** The two interrupted rows check the hook trace and the envelope `trace_file` against `loopTrace`.
- **The fallback fires only when the child left no trace and no non-empty stderr.** None. The code applies it only when `startFailed` is set, and the test covers only a start failure.
- **Warn log on each retry.** None. This is logging only, so I make it a NIT finding.
- **A ctx-cancelled wait returns the last refusal (`startChild`, `sleep(...) != nil`).** None.
- **`LoopTraceFile` wiring in `looparm.go` and `arm.go`.** None.
- **`installBinary` success.** `TestInstallBinary_RenamesTheBuildOverTheTarget/BuildSucceeds` checks the new content at the target, that a reader holding the old file still sees "old", and that the install directory holds only the target.
- **`installBinary` build failure.** The `BuildFails` row checks the error is returned, the target is unchanged and no temp file is left.
- **`installBinary` rename failure.** None.

**Findings**

1. LOW, scope, `internal/shedverbs/loop_test.go:677-757`, with the code at `loop.go:311-313`. No row covers D4's acceptance criterion that an interrupted stop whose child left no trace and no stderr records the loop's own trace file. The new rows exercise only `startFailed`. A child that started, died and left nothing is untested, and the code never applies the fallback to it. Evidence: `loop.go:311` requires `res.startFailed`. Add a row with a started child that exits interrupted with no trace and empty stderr, and decide the behaviour with the code-correctness lens.
2. LOW, scope, `internal/shedverbs/loop_test.go:677-757`. The fallback's guards are untested: `LoopTraceFile` nil, and `LoopTraceFile()` returning "" (the empty-path case the focus names). The test always sets a non-empty path. A negative row would pin that the stop's trace file stays empty without a panic.
3. LOW, scope, `internal/shedverbs/loopchild.go:66-68`. The retry's cancellation path has no test. When `sleep` returns an error, `startChild` returns the last refusal with a nil child and does not try again. Add a row where `Sleep` returns `ctx.Err()` and assert exactly one try and a `start-failed` stop.
4. LOW, scope, `tools/deploy/main_test.go:279-332`, with the code at `tools/deploy/main.go:125-128`. The rename-failure branch of `installBinary`, which removes the temp file and leaves dest unchanged, has no row. The build callback could, for example, create the temp as a directory so the rename over a file fails. D3 asks that the temp file be removed "when the build or rename fails".
5. NIT, scope, `internal/loomcli/looparm.go:51` and `internal/battencli/arm.go:537`. No test asserts that either production `LoopSpec` sets `LoopTraceFile`. Without it, a `start-failed` stop in production would silently write an empty `trace_file`, which is the issue's original symptom. If a spec-construction test already exists beside these, one assertion would cover it.
6. NIT, consistency, `internal/shedverbs/loop_test.go:677-757`. The Warn log on each retry (D1, `PATTERN-spawn-observability`) is unpinned. Logging is conventionally untested, so this is minor.
7. NIT, consistency, `internal/shedverbs/loop_test.go:677-757`. The new test and its subtests do not call `t.Parallel()`, and no comment says why. `golang-testing` requires `t.Parallel()` unless a reason is given. I could not see the neighbouring tests, so the file's convention is unverified.

Tool calls: none; the diff and test code in my context were enough.
