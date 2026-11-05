```yaml
lens: code-correctness
outcome: reported
```

Procedure list:

- `proc.IsSpawnBusy`: nil and ENOENT return false; direct ETXTBSY and wrapped `fs.PathError` return true through `errors.Is`.
- `startChild`: immediate success returns without sleeping; two ETXTBSY refusals then success perform three starts and 100/200 ms waits; persistent ETXTBSY performs five starts and four waits totaling 1.5 seconds; another error stops immediately.
- `startChild` cancellation: a sleep interrupted after the first refusal returns that refusal after one actual start; downstream reporting incorrectly claims five tries.
- `runChild`: failed starts become interrupted results, but their cause does not follow D4’s required format or report actual attempts.
- `runLoop` trace selection: a failed start with no child trace or non-empty stderr gets the loop trace; existing child evidence takes precedence; nil or empty loop trace remains empty.
- `runLoop` trace selection after successful spawn followed by an interruption: no child evidence leaves an empty trace despite an available loop trace, contrary to D4.
- `procChildRunner` and `proc.StartTied`: each retry creates a fresh command; stderr is closed on return; Linux start errors propagate before child execution.
- `loopSpecFor` and `battenLoopSpec`: both supply the `logger.TraceFile` callback; the logger’s unavailable-sink result remains empty.
- `installBinary`: successful build and rename replace the target and consume the temporary name; build failure and rename failure attempt temporary-file removal and propagate the primary error.
- `run`: the build callback keeps cgo and channel flags and builds at the install helper’s supplied path.
- Windows deployment: the destination name remains `lym.exe`; replacement failure follows the rename-error cleanup path.

Findings:

1. **MEDIUM — design — `internal/shedverbs/loop.go:435`**  
   **Summary:** Failed-start causes omit the required attempt format and can falsely report five attempts.  
   **Evidence:** D4 requires `start-failed after <n> tries: <error>` at `_lym/discussion/decision-record.md:39`.
   The implementation retains `start-failed: ...`, gives non-ETXTBSY errors no attempt count, and appends `childStartBusyAttempts` for every ETXTBSY failure at `loop.go:437`.
   `startChild` returns early when the injected or real sleep fails at `internal/shedverbs/loopchild.go:64`, so cancellation after the first refusal produces a halt note claiming five attempts although only one occurred.
   **Fix:** Return the actual number of attempts from `startChild`, use it to construct D4’s required cause for every failed start, and append the executable-open-for-writing explanation only for ETXTBSY.
   Adjust the existing cause assertions accordingly.

2. **MEDIUM — design — `internal/shedverbs/loop.go:311`**  
   **Summary:** Loop-trace fallback excludes interrupted children that successfully started but left no evidence.  
   **Evidence:** D4 and its acceptance criterion require the loop trace whenever an interrupted child left neither a trace nor non-empty stderr (`_lym/discussion/decision-record.md:40,64`).
   The new fallback requires `res.startFailed`.
   A successfully spawned child that exits without an envelope, trace, or stderr returns a result with `startFailed == false` at `internal/shedverbs/loop.go:480`, so `interruptedTraceFile` returns empty and the available loop trace is discarded.
   `loomAfterInterrupt` then copies that empty trace directly into the halt note at `internal/loomcli/looparm.go:192`.
   **Fix:** Put the loop-trace fallback in `interruptedTraceFile` after child trace and non-empty stderr selection, including its no-`StopFiles` branch, and remove the `startFailed` restriction.
   Preserve the separate status-read-stop trace handling.

Tool-call record:

1. Read `proc`’s Linux/Windows tied-start implementations, the interrupt-hook portion missing from truncated inherited output, the loop declarations, caller locations, and review format to trace production errors and reporting.
2. Read step dispatch and interrupted envelope generation, obtained exact changed-code line numbers, and attempted to inspect Go platform/build sources; those assumed standard-library paths were absent.
3. Located the installed Go command, searched cause/fallback test references, and obtained decision-record and child-exit line numbers to ground the two concrete reporting failures.
