```yaml
lens: code-correctness
outcome: reported
```

Cases reviewed:

- `proc.IsSpawnBusy`: nil and unrelated errors return false; direct ETXTBSY and a wrapping `fs.PathError` return true.
- `startChild`: immediate success returns the child; two busy refusals followed by success produce three starts and waits of 100 ms and 200 ms; persistent busy refusal produces five starts and four waits; a different error on any attempt stops immediately.
- `startChild` sleep failure: cancellation after the first busy refusal returns that refusal after one actual start.
- `runChild` failure reporting: ordinary start errors report their underlying error; exhausted busy retries report five attempts correctly; interrupted backoff incorrectly reports five attempts.
- `runLoop` trace selection: a child trace wins; otherwise nonempty captured stderr wins; a start failure with neither falls back to `LoopTraceFile`; a nil callback or callback returning an empty path leaves an empty trace.
- `loopSpecFor` and `battenLoopSpec`: the callback is passed without evaluating it early, so the loop asks its own logger for the path at failure time.
- `installBinary`: successful build and rename replace the destination and consume the temporary pathname; a build failure removes the temporary file and leaves the destination untouched; a rename failure attempts cleanup and returns the rename error.
- Deployment callers: both production and development pass the temporary pathname to `go build -o`, preserving the existing flags and cgo environment.
- Platform process startup: Linux returns the actual `cmd.Start` error; Windows returns a failed start or job setup error and cleans its job handle.

Finding:

**MEDIUM — design — `internal/shedverbs/loop.go:437`: Report the actual number of starts when retry backoff is interrupted.**

A concrete failing path is an initial ETXTBSY refusal followed by cancellation during the first 100 ms wait.
`startChild` returns that ETXTBSY error immediately at `internal/shedverbs/loopchild.go:64–65`, after only one call to the runner.
`runChild` then formats the halt cause using the constant `childStartBusyAttempts`, claiming that the executable stayed open for writing “across 5 tries.”
This misreports both the attempt count and what the retry loop established, undermining the self-explanatory failure record this change adds.
Return or otherwise preserve the actual attempt count and use it in the failure cause.

Tool-call record:

1. Read `internal/proc/tied_linux.go` and `tied_windows.go` to inspect the platform start implementations absent from inherited context; located destination helpers and numbered the retry/failure-reporting evidence; attempted to locate Go platform sources at `/usr/local/go`, which was absent.
2. Read `resolveDest`, `goBinDir`, and `goEnv`, whose definitions were absent from inherited context; searched for locally installed Go source; searched all Go readers of `IsSpawnBusy`, `childStartBusyAttempts`, `LoopTraceFile`, and `installBinary` to complete the direct consumer set.
