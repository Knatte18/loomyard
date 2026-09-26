# Batch: logger-trace-accessors

```yaml
task: 'shed: the LLM driver as a generic stepper and mender'
batch: logger-trace-accessors
number: 1
cards: 2
verify: go test ./internal/logger/ -run 'TestTraceFile|TestTraceDir|TestEnsureDurableSink|TestNotifyExit' && go test -tags integration ./internal/logger/ -run 'TestDurableSink_CwdFallback'
depends-on: []
```

## Batch Scope

Adds the two exported, path-returning accessors the shed step and status envelopes report — `logger.TraceFile()` and `logger.TraceDir()` — and amends the logger's level-policy doc comment so `Info` covers a recorded durable state mutation and a shed step boundary, not only an OS-process spawn.
Batch 3 consumes both accessors from `internal/shedverbs`; batch 2's mutation logging relies on the amended policy.
One batch because both cards touch only `internal/logger` and share its sink state.

## Cards

### Card 1: TraceFile and TraceDir accessors

- **Context:**
  - `internal/logger/retention.go`
  - `internal/logger/trace.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/logger/sink.go`
  - `internal/logger/sink_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/logger/sink.go`, split the directory resolution out of `armDurableSinkLocked` into a new unexported `resolveSinkDirLocked() (dir, worktreeRoot string, ok bool)` (caller holds `sinkMu`).
    It returns `sinkDirOverride` with an empty `worktreeRoot` when the override is set;
    otherwise it returns `ok == false` under `testing.Testing()` unless `LYX_TRACE` is `"1"`, and otherwise resolves through `lyxcwd.Getwd`/`lyxcwd.Resolve`/`isLyxWorktree` exactly as today, returning `LogsDir(layout)` and `layout.WorktreePath()`.
    `armDurableSinkLocked` keeps its current behaviour and ordering: it calls `resolveSinkDirLocked`, calls `armHeader()` after the testing gate passes and before the cwd resolution result is used (moving `armHeader()` to just after a successful resolve is acceptable, since resolution reads no header field), sets `header.WorktreeRoot` only when the returned `worktreeRoot` is non-empty, then `MkdirAll`, `Sweep`, open and header-write as today.
  - Add exported `TraceFile() string`: calls `ensureDurableSink()` (the same lazy-arm path `NotifyExit` uses); returns `""` when that returns false, otherwise returns `sinkPath` read under `sinkMu`.
    Doc comment: it forces the lazy sink open, returns the absolute path of this process's trace file, `""` when no sink can arm, and the same path on every call within one sink generation.
  - Add exported `TraceDir() string`: takes `sinkMu`, calls `resolveSinkDirLocked`, and returns the directory, or `""` when `ok` is false.
    It never arms the sink, never creates a directory or a file, and never runs `Sweep`.
    Doc comment states that it returns the directory the sink would write to, so a caller can list trace files by trace id without arming a sink of its own, and that the directory need not exist yet.
  - Tests in `internal/logger/sink_test.go`, each using `SetDurableSinkDir` and a `t.Cleanup` reset to `""` per the overview's `trace-tests-use-sink-override` decision:
    `TestTraceFile_NoSinkReturnsEmpty` (`SetDurableSinkDir("")` under `go test` yields `""`);
    `TestTraceFile_ArmsAndReturnsPath` (override dir: the returned path is inside the dir, exists, and its first line is the header);
    `TestTraceFile_StableAcrossCalls` (two calls return the same path and the dir holds exactly one file);
    `TestTraceDir_MatchesOverrideAndCreatesNothing` (override set to a not-yet-existing subdirectory of `t.TempDir()`: `TraceDir()` returns it and the subdirectory still does not exist afterwards);
    `TestTraceDir_NoSinkReturnsEmpty` (no override under `go test` yields `""`).
- **Commit:** `feat(logger): add TraceFile and TraceDir accessors`

### Card 2: amend the level policy

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/logger/logger.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In the package doc comment's `# Level policy` section of `internal/logger/logger.go`, replace the single `Info` bullet with one naming three things: a real OS-process spawn or teardown lifecycle event (unchanged), one recorded durable state mutation, and a shed step boundary.
    Add one sentence stating why: the durable sink records `Info` and above only, and a mutation or step the trace does not record cannot be repaired from.
    Keep the `Warn`/`Debug` bullets and the hard rule about `Warn` in loops unchanged.
  - Do not change `CONSTRAINTS.md`; the Live-Substrate Spawn Observability constraint requires `Info` for spawns and does not reserve `Info` for them, so it needs no edit.
- **Commit:** `docs(logger): widen the Info level to mutations and step boundaries`

## Batch Tests

`verify:` runs the new `TestTraceFile*`/`TestTraceDir*` tests plus the existing `TestEnsureDurableSink*`/`TestNotifyExit*` sink tests, which pin that the `armDurableSinkLocked` refactor kept the arm behaviour (header, filename grammar, redirect ordering, concurrent redirect).
The tagged `TestDurableSink_CwdFallback*` cases (`sink_callsite_integration_test.go`) exercise the cwd-anchored resolution `resolveSinkDirLocked` now owns, in a plain checkout and a lyx-owned worktree.
Card 2 is a comment-only change covered by the package compiling.
