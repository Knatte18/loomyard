```yaml
lens: code-affected-set
outcome: reported
```

Affected-set list:

- `IsSpawnBusy`: callers are `startChild` and `runChild`; the new retry table exercises the wrapped `fs.PathError` case, and `internal/proc/doc.go` documents the predicate.
  Covered.
- Retry constants and `startChild`: `runChild` is the production caller; `LoopSpec.Sleep`, `procChildRunner`, `proc.StartTied`, and `sleepFor` supply the existing process and timing seams.
  The retry table asserts retry limits and doubling waits.
  Covered.
- Start-failure cause: `runLoop` passes it to `interruptedError` and `AfterInterrupt`; `loomAfterInterrupt` preserves it as the halt-note reason.
  The new test and package documentation adopt the implementation’s old prefix, contrary to D4.
  Not covered correctly.
- `LoopTraceFile`: both production constructors, `loopSpecFor` and `battenLoopSpec`, supply `logger.TraceFile`; test constructors may omit the optional callback.
  `runLoop` applies the fallback only to `startFailed` results, leaving another D4-required interruption case uncovered.
- Atomic installation: `run` is the only production caller of `installBinary`; the production and development shell/Windows launchers all call `tools/deploy`.
  Batten invokes its configured deploy command.
  The helper’s tests cover successful replacement and build failure.
  Callers and documentation are consistent with the new installation path.
- Repository rules: the loop test file explicitly documents its process-global logger reason for omitting `t.Parallel`; new waits use the injected seam.
  Package documentation changes accompany the implementation.
  No stencil changes, declared PATTERN changes, or `Pattern-Departure:` trailers occur in this range.

Findings:

1. **MEDIUM — design — `internal/shedverbs/loop.go:435–437`**
   **The start-failure cause does not implement D4’s required attempt-count format.**
   D4 requires `start-failed after <n> tries: <error>`.
   The implementation retains `start-failed: <error>` and adds an attempt count only for ETXTBSY.
   Consequently an ordinary failed start has no attempt count, and a busy failure has a different format.
   This reaches the actual halt-note reason because `internal/loomcli/looparm.go:187` copies `cause` into `haltNote.Reason`.
   `internal/shedverbs/loop_test.go:702,710` asserts the divergent format instead of the settled contract.
   Fix the cause construction and its existing assertions, and align `internal/shedverbs/doc.go:41`.

2. **MEDIUM — design — `internal/shedverbs/loop.go:311`**
   **The fallback omits an interrupted child that started but left neither trace nor stderr.**
   D4 says `interruptedTraceFile` falls back to the loop trace whenever the child left neither trace nor non-empty stderr; its acceptance criterion likewise describes an interrupted stop without restricting it to spawn failures.
   `interruptedTraceFile` still returns `""` for that case (`internal/shedverbs/loop.go:538`), and the new caller fallback additionally requires `res.startFailed`.
   A child that starts and exits before writing either output therefore leaves `trace_file` empty even when `LoopTraceFile` supplies a usable path.
   Apply the fallback after child-trace and non-empty-stderr selection for all such interrupted-child stops.

Tool-call record:

1. Searched all repository Go/Markdown readers of the changed symbols; read the loop test’s previously truncated parallelism explanation, `loomAfterInterrupt`, and the refusal background to verify downstream consumers and rules.
2. Read logger’s level policy, searched contract/plugin/documentation readers, read deploy launchers and Batten’s deploy invocation to check logging constraints and entry-point coverage; attempted platform-source paths were absent.
