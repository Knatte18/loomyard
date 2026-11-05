```yaml
lens: code-correctness
outcome: reported
```

Task: code-correctness lens on the ETXTBSY retry, start-failed cause, trace fallback, `IsSpawnBusy` and `installBinary`.

**Per function and branch (case run, verdict)**

- `startChild`, first try accepted: returns at once, no sleep. Correct.
- `startChild`, busy twice then accepted: waits are 100 ms then 200 ms and the third try runs. Correct.
- `startChild`, busy on all 5 tries: waits are 100, 200, 400 and 800 ms, four in all. The fifth refusal returns with no sleep, so it makes exactly 5 tries. Correct.
- `startChild`, non-busy error: returns on the first try. Correct.
- `startChild`, `ctx` cancelled during a wait: returns `nil, err` with the last refusal. The error is kept, but `runChild` then reports the wrong try count (F1).
- `startChild`, `Sleep` nil: falls back to `sleepFor`. Correct.
- `procChildRunner` on retry: `os.Create` truncates the stderr file and `defer` closes it, so no descriptor leaks and no stale stderr carries over. Correct.
- `IsSpawnBusy`: `errors.Is` on a `*fs.PathError` wrapping `syscall.ETXTBSY` matches, and nil gives false. Correct. `syscall.ETXTBSY` is defined on Windows too, like the EAGAIN predicate beside it. `StartTied` returns `startPlatform`'s error unchanged, so the `PathError` chain is intact.
- `runChild` cause, ETXTBSY: gives `start-failed: <err>; the executable stayed open for writing across 5 tries, ...`. It is internally consistent, but it does not match D4's form (F2).
- `runChild` cause, other error: gives `start-failed: <err>` with no try count. D4 says `start-failed after <n> tries: <error>` for every start that still fails (F2).
- `runLoop` trace fallback, the start-failed case with `LoopTraceFile` returning a path: that path is used. Correct.
- `runLoop` trace fallback, `LoopTraceFile` nil or returning `""`: `traceFile` stays `""`, the same as before the diff. Correct.
- `runLoop` trace fallback, a child that started and died leaving no trace and no stderr: `res.startFailed` is false, so `traceFile` stays `""` (F3).
- `installBinary`, build succeeds: the temp file is `.lym.deploy-<pid>` beside the target, and the rename replaces the directory entry. A held old inode stays intact. Correct.
- `installBinary`, build fails: the temp file is removed and the target is untouched. Correct.
- `installBinary`, rename fails: the temp file is removed and the error is wrapped. Correct.
- `installBinary`, Windows: the name becomes `.lym.exe.deploy-<pid>`, which `go build -o` accepts. A rename over a running `lym.exe` fails, as the record says it did before. Correct.
- `installBinary`, a temp file left by a killed deploy with another pid: it stays in the directory. Only `kill -9` or power loss causes this, so I did not raise it.

**Findings**

- F1 — LOW, design — `internal/shedverbs/loop.go` (`runChild`, the `IsSpawnBusy` branch), with `internal/shedverbs/loopchild.go:65-67` as evidence.
  - When `ctx` ends during a backoff, `startChild` returns the busy error after fewer than 5 tries.
  - `runChild` still writes "stayed open for writing across 5 tries", which is false.
  - Fix: have `startChild` return the number of tries made and use it in the cause.
- F2 — MEDIUM, design — `internal/shedverbs/loop.go` (`runChild`, the start-failed cause).
  - D4 and the focus directive require `start-failed after <n> tries: <error>`, with the ETXTBSY meaning added.
  - The code emits `start-failed: <err>` for non-busy errors, with no try count.
  - For ETXTBSY it appends a suffix instead of using the `after <n> tries` form.
  - `shedverbs/doc.go` and `loop_test.go` (`wantCause`) pin the divergent text.
  - Fix: build the cause as `start-failed after <n> tries: <error>`, add the ETXTBSY meaning for busy errors, and update the doc and test.
- F3 — MEDIUM, design — `internal/shedverbs/loop.go` (the `traceFile == "" && res.startFailed` condition in `runLoop`).
  - D4 and the acceptance criterion say the loop's own trace file is used when the child left no trace and no non-empty stderr.
  - The code also requires `startFailed`, so a started child that died silently still gets an empty `trace_file`.
  - Fix: drop the `res.startFailed` condition, and with it the `startFailed` field if nothing else reads it.

**Tool calls**
1. Read `internal/shedverbs/loopchild.go` lines 1-42. My context held only the diff hunks, and I needed `procChildRunner`'s stderr handling on a retry.
2. Ran a grep of `StartTied` in `internal/proc/tied.go`, because I needed to know whether it wraps the start error and so keeps the ETXTBSY chain.
