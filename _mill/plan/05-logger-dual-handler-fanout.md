# Batch: logger-dual-handler-fanout

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: logger-dual-handler-fanout
number: 5
cards: 6
verify: go test -race ./internal/logger/...
depends-on: [2, 4]
```

## Batch Scope

Rewrites `internal/logger/logger.go`'s `Debug`/`Info`/`Warn` to fan out to two independently-gated handlers per discussion.md's `dual-handler-fan-out` decision: the existing stderr handler (gated by the existing verbosity threshold) and the new durable-sink handler (batch 4's `ensureDurableSink()`/`writeDurable`, pinned Info+ regardless of verbosity). Stamps `trace=` (batch 2's `TraceID()`) on every emitted line at every level. Changes `SetOutput`'s contract to rebind only the stderr half. This is the batch every adoption-pass batch (8-13) implicitly depends on, since it is what makes their existing/new `logger.Info`/`Warn` calls actually reach the durable sink.

## Cards

### Card 17: Composite handler wrapping stderr + durable

- **Context:**
  - `internal/logger/sink.go`
- **Edits:**
  - `internal/logger/logger.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/logger/logger.go`, add a composite `slog.Handler` implementation (`Enabled`/`Handle`/`WithAttrs`/`WithGroup`) that wraps two inner handlers: the existing stderr `slog.TextHandler` (currently built by `newLogger`, `logger.go:100-102`, gated by the existing `levelVar`) and a new durable handler whose `Enabled` reports true for `slog.LevelInfo` and above **unconditionally** (never gated by `levelVar`) and whose `Handle` calls `ensureDurableSink()` (batch 4) then, if `sinkOK`, formats the record as `slog.NewTextHandler` would and calls `writeDurable` (batch 4) with the bytes. The composite's own `Enabled(level)` is the **OR** of the two inner gates — this is the property discussion.md's Testing section calls "the case the motivating incident depends on": an `Info` record must reach the durable sink at the default (Warn) verbosity even though the stderr handler alone would report `Enabled(Info) == false` at that threshold.

  **`Handle()` must independently re-check each inner handler's own `Enabled(record.Level)` before delegating to it — it must never forward unconditionally just because the composite's own `Enabled` already returned true.** `slog.Handler` implementations trust that a logging call site already checked `Enabled` before calling `Handle`, so the composite's `Handle(ctx, record)` body must read: if the stderr handler's `Enabled(ctx, record.Level)` is true, call its `Handle`; if the durable handler's `Enabled(ctx, record.Level)` is true, call its `Handle`; do both independently, never one gated by the other's result. Without this, an `Info` record at the default Warn verbosity — which the composite's OR-gated `Enabled` correctly lets through so the durable handler can see it — would also get forwarded to the stderr handler's `Handle` unconditionally, printing it to stderr even though the stderr handler's own `Enabled(Info)` is false at that threshold. This is exactly what Card 21's assertion ("`Info` reaches stderr only at `-v` or above") depends on being false; get this wrong and that assertion fails.

  Rebuild `log` (`logger.go:66`) to use this composite instead of the bare `slog.NewTextHandler(out, ...)` `newLogger` currently returns.
- **Commit:** `feat(logger): add composite handler fanning out to stderr and the durable sink`

### Card 18: Stamp `trace=` on every emitted line

- **Context:**
  - `internal/logger/trace.go`
- **Edits:**
  - `internal/logger/logger.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `Debug`, `Info`, and `Warn` (`logger.go:106-120`) so every call stamps a `trace` key with `TraceID()`'s current value (batch 2). The simplest correct shape: each function calls `log.With("trace", TraceID()).Debug(msg, args...)` (respectively `.Info`/`.Warn`) rather than `log.Debug(msg, args...)` directly — or, if that per-call `With` proves wasteful, cache a `*slog.Logger` already bound with the trace attribute the first time `TraceID()` resolves and rebuild it if `SetOutput`/composite rebuilding invalidates it. Either shape must satisfy: `trace=` appears on **every** level including `Debug` (which never reaches the durable sink — the field is on the record regardless of which handler ends up accepting it), and calling `TraceID()` here is what actually triggers `traceOnce`'s first resolution if nothing has resolved it yet (a `go test` binary that never calls `MintOrAdoptAndExport` still gets a real trace ID the first time anything logs, per batch 2's Card 2 design).
- **Commit:** `feat(logger): stamp trace= on every Debug/Info/Warn line`

### Card 19: `SetOutput` rebinds only the stderr half

- **Context:** none
- **Edits:**
  - `internal/logger/logger.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `SetOutput` (`logger.go:140-143`) so it rebuilds only the stderr handler inside the composite from Card 17, leaving the durable handler (and its `ensureDurableSink`/`writeDurable` wiring) untouched. Update `SetOutput`'s doc comment to state this explicitly — after this change a caller invoking `SetOutput` to redirect stderr output (`configureFromEnv`'s `LYX_LOG_FILE` handling at `logger.go:86-94` is exactly such a caller) must not also silently detach the durable sink, per discussion.md's `dual-handler-fan-out` decision: "`SetOutput` binds the stderr handler only... After this change `SetOutput` rebinds the stderr half and leaves the durable half untouched, and its doc comment says so." While rewriting this comment, also correct its existing, already-stale claim that "production code never calls it" (`logger.go:139`) — `configureFromEnv` already calls `SetOutput(f)` at `logger.go:93` for `LYX_LOG_FILE` today, independent of anything this task adds; reword to describe it as a seam both tests and `configureFromEnv` use, not one production code never reaches. Also document the `LYX_LOG_FILE` duplication as intentional (per the same decision's "`LYX_LOG_FILE` duplication is intended" bullet) so a future reader does not "fix" the apparent double-write.
- **Commit:** `fix(logger): SetOutput rebinds only the stderr handler, durable sink stays untouched`

### Card 20: Concurrency — one mutex covers sink write path and stderr rebinding

- **Context:**
  - `internal/logger/sink.go`
- **Edits:**
  - `internal/logger/logger.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Per discussion.md's `concurrency-contract` ("the non-sink globals are covered too... `SetOutput` and the stderr-handler read path go under the same mutex as the durable sink's state"), guard `SetOutput`'s write to `out`/`log` (Card 19) and the composite handler's read of the current stderr handler with the **same** `sinkMu sync.Mutex` batch 4's Card 11 defined in `sink.go` — do not introduce a second mutex. `traceOnce`/`headerOnce`/`sinkOnce` need no lock of their own (`sync.Once` already serializes their bodies); this card's lock is specifically for the mutable `out`/`log` package vars that `SetOutput` can rebind while emits are concurrently in flight, which is the scenario a test calling `SetOutput` mid-emit exercises.

  **Self-deadlock hazard — `sync.Mutex` is non-reentrant, and `writeDurable` (Card 11) independently locks `sinkMu`.** Card 17's composite `Handle()` calls into the durable handler's `Handle`, which calls `writeDurable`, which locks `sinkMu` for its own byte-counter/cap-check/marker/write critical section. If `Handle()`'s read of the current stderr handler holds `sinkMu` for the whole call (a naive single `Lock()`/`defer Unlock()` spanning the entire `Handle()` body), the nested `writeDurable` call deadlocks against itself on the second `Lock()`. The stderr-handler-state read must therefore be its **own narrow critical section**: `sinkMu.Lock()`, copy the current stderr handler reference into a local variable, `sinkMu.Unlock()` — all before `Handle()` calls either inner handler's `Handle` method. Neither inner handler's `Handle` call (stderr or durable) may execute while `sinkMu` is still held from this read; `writeDurable`'s own lock/unlock (Card 11) is what protects the durable write itself, and it must be free to acquire `sinkMu` on its own. Card 22's `-race` test exercises this path under concurrency — get this wrong and that test hangs instead of failing cleanly.
- **Commit:** `fix(logger): guard SetOutput/stderr-handler state with the shared sink mutex`

### Card 21: Dual-handler fan-out tests

- **Context:** none
- **Edits:**
  - `internal/logger/logger_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/logger/logger_test.go`. Each case below calls `SetDurableSinkDir(t.TempDir())` (batch 4's Card 12) at its own start, never sharing one call across cases — this both keeps every case off `hubgeometry.Resolve` and, per Card 12, fully resets `sinkOnce`/`sinkWriter`/`sinkOK`/`header`/`headerOnce`/the byte-counter/marker-flag before the case runs, so an earlier case's sink-open (in this file or in `sink_test.go`/`span_test.go`) cannot leak into this one:
  - A `Debug` call under `-vv` (`SetVerbosity(2)`) reaches the stderr buffer and does **not** reach the durable sink (assert no file exists at all for a Debug-only sequence in a freshly-reset sink directory).
  - An `Info` call reaches the durable sink at **every** verbosity including the default (`SetVerbosity(0)`, Warn threshold) — the composite's `Enabled` OR-gate assertion — and reaches stderr only at `-v` (`SetVerbosity(1)`) or above.
  - A `Warn` call reaches both stderr and the durable sink at every verbosity.
  - A `Warn` call with the durable sink unarmed (e.g. `testing.Testing()` true and `LYX_TRACE` unset, so `ensureDurableSink`'s gate keeps `sinkOK` false) reaches stderr only, no error, no panic.
  - Every emitted line at every level carries `trace=` matching `TraceID()`'s current value (Card 18's assertion, deferred here since this is where the dual-handler wiring that actually calls `Debug`/`Info`/`Warn` end-to-end exists).
- **Commit:** `test(logger): cover dual-handler fan-out and trace= stamping across all three levels`

### Card 22: Concurrency race test

- **Context:** none
- **Edits:**
  - `internal/logger/logger_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a test to `internal/logger/logger_test.go`, starting with its own `SetDurableSinkDir(t.TempDir())` call (batch 4's Card 12 — this resets all sink-side state so the goroutine fan-out below starts from a genuinely unarmed sink, not whatever an earlier case in this file left behind), driving concurrent `Warn` calls from several goroutines (e.g. 20 goroutines each emitting enough bytes to approach the 8 MiB cap), run under `go test -race`: assert a single sink file is created (not one per goroutine — pins `sinkOnce`), and that crossing the cap under concurrency still yields exactly one truncation-marker line in the file (pins `writeDurable`'s mutex-guarded counter+cap-check+marker+write critical section from batch 4's Card 11). This is the test discussion.md's Testing section calls out as "what pins `concurrency-contract`; without `-race` in CI for this package the mutex is unverified" — hence this batch's `verify:` command uses `go test -race`, not plain `go test`.
- **Commit:** `test(logger): add -race concurrency test for the sink write path`

## Batch Tests

`verify: go test -race ./internal/logger/...` — the `-race` flag is required for Card 22's concurrency assertion to be meaningful; running the whole package's suite under `-race` at this batch boundary also re-validates every earlier batch's logger tests race-clean now that concurrent write paths exist.
</content>
