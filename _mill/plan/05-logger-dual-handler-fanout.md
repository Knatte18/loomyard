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
  Change `SetOutput` (`logger.go:140-143`) so it rebuilds only the stderr handler inside the composite from Card 17, leaving the durable handler (and its `ensureDurableSink`/`writeDurable` wiring) untouched. Update `SetOutput`'s doc comment to state this explicitly — today it silently discards nothing because there is no durable handler yet, but after this change a caller invoking `SetOutput` to redirect stderr output (`configureFromEnv`'s `LYX_LOG_FILE` handling at `logger.go:86-94` is exactly such a caller) must not also silently detach the durable sink, per discussion.md's `dual-handler-fan-out` decision: "`SetOutput` binds the stderr handler only... After this change `SetOutput` rebinds the stderr half and leaves the durable half untouched, and its doc comment says so." Also document the `LYX_LOG_FILE` duplication as intentional (per the same decision's "`LYX_LOG_FILE` duplication is intended" bullet) so a future reader does not "fix" the apparent double-write.
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
- **Commit:** `fix(logger): guard SetOutput/stderr-handler state with the shared sink mutex`

### Card 21: Dual-handler fan-out tests

- **Context:** none
- **Edits:**
  - `internal/logger/logger_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/logger/logger_test.go` (using `SetDurableSinkDir(t.TempDir())` from batch 4's Card 12 so no geometry resolution runs):
  - A `Debug` call under `-vv` (`SetVerbosity(2)`) reaches the stderr buffer and does **not** reach the durable sink (assert no file, or the durable file if one exists from a prior Info in the same test has no Debug line — structure the test to check file absence for a Debug-only sequence).
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
  Add a test to `internal/logger/logger_test.go` driving concurrent `Warn` calls from several goroutines (e.g. 20 goroutines each emitting enough bytes to approach the 8 MiB cap) with `SetDurableSinkDir(t.TempDir())`, run under `go test -race`: assert a single sink file is created (not one per goroutine — pins `sinkOnce`), and that crossing the cap under concurrency still yields exactly one truncation-marker line in the file (pins `writeDurable`'s mutex-guarded counter+cap-check+marker+write critical section from batch 4's Card 11). This is the test discussion.md's Testing section calls out as "what pins `concurrency-contract`; without `-race` in CI for this package the mutex is unverified" — hence this batch's `verify:` command uses `go test -race`, not plain `go test`.
- **Commit:** `test(logger): add -race concurrency test for the sink write path`

## Batch Tests

`verify: go test -race ./internal/logger/...` — the `-race` flag is required for Card 22's concurrency assertion to be meaningful; running the whole package's suite under `-race` at this batch boundary also re-validates every earlier batch's logger tests race-clean now that concurrent write paths exist.
</content>
