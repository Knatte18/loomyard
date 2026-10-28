## Changes

- F1, fixed as proposed (`internal/shedverbs/loop.go`, `loopchild.go`, `doc.go`, `loop_test.go`): `startChild` returns the tries made, and `runChild` builds `start-failed after <n> tries: <error>` for every start error, with `; the executable was open for writing, as while a deploy replaces it in place` appended for ETXTBSY.
  Commit 548ae705.
- F2, fixed as proposed: the loop-trace fallback no longer needs `res.startFailed`, and the field is removed.
  The test gains rows for a started child that dies without a trace.
  Commit 548ae705.
- F3, fixed as proposed: the cause names the tries actually made, and the row `WaitEndedBeforeTheLastTry` pins one try and a cause naming one try.
  Commit 548ae705.
- F4, fixed as proposed: rows `NoLoopTraceFileNamed` and `LoopTraceFileNamedEmpty` assert an empty `trace_file` without a panic.
  Commit 548ae705.
- F5, fixed differently (`tools/deploy/main_test.go`): the proposed non-empty directory at the temporary path would make `os.Remove(tmp)` fail and leave a temporary entry behind, so the new test `TestInstallBinary_RenameFailureRemovesTheBuild` puts a non-empty directory at the target, which makes the rename fail.
  It asserts an error, the target untouched and no temporary entry.
  Commit a8ec64ca.
- Self-check follow-up: the `LoopSpec.LoopTraceFile` field comment and `doc.go` now name the broadened fallback.
  Commit 5d6a3354.

`lym gate test ./internal/shedverbs ./tools/deploy` is green.

## Self-check

- fix-consistency, LOW: stale `LoopTraceFile` field comment at `loop.go:57`, fixed.
- fix-consistency, LOW: `doc.go` trace-file sentence lacked the loop-trace fallback, fixed.
- fix-affected-set, LOW: the same stale `LoopTraceFile` comment, a duplicate, fixed.
- fix-code-correctness: no findings.
- fix-proposal: no findings.

## Disputed

None.

## Deferred

None.
