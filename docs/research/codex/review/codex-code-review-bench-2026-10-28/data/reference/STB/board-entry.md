# Board entry `spawn-text-file-busy`: Friction-Reflect producer fails to start with 'text file busy' when the lym binary is replaced mid-run

Imported from [issue #893](https://github.com/Knatte18/loomyard/issues/893).

## What happened

A loom run halted `failed` at the `Friction-Reflect` producer with anomaly `interrupted-stop`:

```
start-failed: fork/exec <go bin dir>/lym: text file busy
```

Every earlier producer in the run (Preflight, Darn-Write, Code-Bouncer, PR-Review, Finalize) finished or was skipped normally.
The halt note carried an empty `trace_file`, so the failure left no trace to inspect.

## Why it is abnormal

`text file busy` (ETXTBSY) on `fork/exec` of the `lym` binary means another process was writing to that executable at the moment of the spawn, most likely a deploy or rebuild replacing the binary in place.
The last producer of an otherwise clean run then failed for a reason unrelated to the task, and the run ended `failed` instead of finishing.
The classification as `interrupted-stop` also hides the cause: it is a transient spawn error, not an interrupted run.

## What to do

- Retry the spawn a few times with a short backoff when `fork/exec` returns ETXTBSY, before declaring `start-failed`.
- Have the deploy path (`update-plugins.sh`, `deploy-dev`, the Land row's `deploy` command) install the binary by writing a temp file and renaming it over the target, so a running or spawning process never sees a file open for writing.
- Write a trace file, or the spawn error's detail, into the halt note when `start-failed` occurs, so the note is self-explanatory.

Provenance: task slug `codex-hook-spike`; notes `loom-halt.md`.
