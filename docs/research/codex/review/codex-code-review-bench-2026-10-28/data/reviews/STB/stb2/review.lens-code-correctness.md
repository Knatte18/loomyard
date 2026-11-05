```yaml
lens: code-correctness
outcome: reported
```

Cases checked:

- `IsSpawnBusy`: nil and ENOENT return false; bare and `PathError`-wrapped ETXTBSY return true.
- `startChild`: immediate success returns once; ETXTBSY followed by success retries safely; persistent ETXTBSY starts five times with 100/200/400/800 ms waits; another error terminates immediately.
- `startChild` cancellation: cancellation during a retry wait terminates without another start, but the resulting diagnostic incorrectly reports five attempts.
- Trace selection: child trace wins over stderr; nonempty stderr wins over loop trace; absent child evidence falls back for start failures; nil callback or empty callback result safely leaves the trace empty.
- CLI wiring: loom and batten pass `logger.TraceFile` as a callback, preserving lazy sink resolution.
- Deployment: build success renames a sibling path over the target; build and rename errors attempt cleanup and retain the old target; the explicit `go build -o` filename is preserved on Windows despite lacking a final `.exe` suffix.
- Build mode: Go’s installer sets executable permissions before moving or copying the build artifact.
- Platform predicate: Windows’ syscall definitions include ETXTBSY.

Finding:

**MEDIUM — design — `internal/shedverbs/loop.go:437`: Report the actual number of attempted starts when a retry wait is interrupted.**

If the first start returns ETXTBSY and the context is cancelled during the first 100 ms wait, `startChild` returns that refusal immediately (`internal/shedverbs/loopchild.go:64–65`).
`runChild` nevertheless formats the diagnostic using the constant `childStartBusyAttempts`, claiming that the executable stayed open for writing “across 5 tries.”
Only one start happened, and the retry budget was never exhausted.
The same inaccurate count occurs when cancellation interrupts any earlier retry wait.
Return the actual attempt count from `startChild` and use it in the cause so the halt note accurately distinguishes exhausted retries from an interrupted retry sequence.

Tool calls:

1. Read numbered changed branches, Linux and Windows tied-start implementations, and local Windows syscall definitions to check exact locations, start-error propagation and platform support absent from the inherited excerpts.
2. Read Go’s local build output/install implementation and logger trace enumeration, and searched production readers, to verify explicit Windows output naming, installed executable modes and fallback dependencies absent from the manifest excerpts.
