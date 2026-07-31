# Batch: logger-durable-sink

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: logger-durable-sink
number: 4
cards: 8
verify: go test ./internal/logger/...
depends-on: [1, 2, 3]
```

## Batch Scope

Implements the durable second sink in a new `internal/logger/sink.go`: lazy geometry-resolved open, the two-phase header record (static fields at arm time, worktree-root at open — discussion.md's `sink-open-triggers`), the two open triggers (first Info+ record; non-zero process exit), the size cap + truncation marker (part of the write path, guarded by the shared mutex from `concurrency-contract`), the test seam, and the `LYX_TRACE=1` test-entry-activation opt-in. Depends on batch 1 (`hubgeometry.WorktreeLogsDir()`), batch 2 (`TraceID()` for the header and filename), and batch 3 (`retention.Sweep` called on open). This batch does **not** wire `Debug`/`Info`/`Warn` to call the sink — that fan-out wiring is batch 5; this batch exposes the unexported `ensureDurableSink()` entry point batch 5's handler calls.

**Batch-local naming (fixed, used by later batches):** `sinkHeader` (struct: `Command string`, `Argv []string`, `TraceID string`, `PID int`, `WorktreeRoot string`), `armHeader()` (unexported, `sync.Once`-guarded static-field capture), `Arm()` (exported, calls `armHeader()` — the function cmd/lyx's root hook calls, batch 7), `ensureDurableSink() (io.Writer, bool)` (unexported, `sinkOnce`-guarded open — the function batch 5's fan-out handler calls on an Info+/Warn record), `NotifyExit(code int)` (exported — cmd/lyx's `run()`/`main()` calls this after capturing `clihelp.RunRoot`'s result, batch 7), `SetDurableSinkDir(dir string)` (exported test seam), `writeDurable(p []byte) (int, error)` (unexported, mutex-guarded write path with the size cap and truncation marker).

## Cards

### Card 9: Header static-field composition

- **Context:**
  - `internal/logger/trace.go`
- **Edits:** none
- **Creates:**
  - `internal/logger/sink.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/logger/sink.go` implementing the header half of discussion.md's `sink-open-triggers` decision:

  ```go
  type sinkHeader struct {
  	Command      string
  	Argv         []string
  	TraceID      string
  	PID          int
  	WorktreeRoot string
  }
  ```

  Package-level `headerOnce sync.Once` and `header sinkHeader`. Unexported `armHeader()` captures the **static** fields only — `Command: os.Args[0]`, `Argv: append([]string(nil), os.Args[1:]...)` (copy, not alias `os.Args`), `TraceID: TraceID()` (batch 2's accessor), `PID: os.Getpid()` — inside `headerOnce.Do(...)`. `WorktreeRoot` is deliberately left zero-value here; it is filled in later, at `ensureDurableSink()`'s open (Card 10), never by `armHeader()`. Exported `func Arm() { armHeader() }` — the entry point cmd/lyx's root hook calls (batch 7); it is a thin wrapper so the identical unexported `armHeader()` is also reachable from `ensureDurableSink()` itself (Card 10) for the case where nothing called `Arm()` first (the `LYX_TRACE=1`/test-seam path, per `no-arming-under-test`'s "header composition is unaffected by which path arms the sink" bullet).
- **Commit:** `feat(logger): add durable-sink header static-field composition`

### Card 10: Lazy sink open — geometry, retention sweep, header write

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/logger/retention.go`
- **Edits:**
  - `internal/logger/sink.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/logger/sink.go`: package-level `sinkOnce sync.Once` (distinct from `traceOnce` per `concurrency-contract` — never merge them) and the durable writer state (`sinkWriter io.Writer`, `sinkOK bool`, plus whatever byte-counter/cap state Card 11 adds under the same critical section).

  `func ensureDurableSink() (io.Writer, bool)`:
  1. `sinkOnce.Do(func() { ... })` body:
     - If `testing.Testing()` is true and `os.Getenv("LYX_TRACE") != "1"`, leave `sinkOK = false` and return (no filesystem or geometry access at all — this is the `LYX_TRACE=1` opt-in gate from `test-entry-activation`, checked here rather than only at the cmd/lyx hook, since this function itself is what the fan-out calls on every Info+/Warn regardless of which entry point is running).
     - Call `armHeader()` (idempotent — a no-op if `Arm()` already ran).
     - Resolve the target directory: if `SetDurableSinkDir` (Card 12) has been called, use that override directory and leave `header.WorktreeRoot` empty (test-seam path). Otherwise call `hubgeometry.Resolve(...)` (via `hubgeometry.Getwd()` for the starting cwd) and `layout.WorktreeLogsDir()` (batch 1); on any resolution error, set `sinkOK = false` and return — **never** fail the calling operation (per `lazy-sink-open`'s "failure to open a diagnostic sink must never break the operation being diagnosed"). On success, set `header.WorktreeRoot = layout.WorktreeRoot`.
     - `os.MkdirAll(dir, 0o755)`; on error, `sinkOK = false`, return.
     - Call `retention.Sweep(dir)` (batch 3) — a sweep failure does not abort the open (log nothing about it; the sweep itself already tolerates per-file failures internally).
     - Build the filename `trace-<YYYYMMDDTHHMMSSZ>-<16-hex-id>-<pid>.log` using a UTC timestamp taken now, `header.TraceID`, and `header.PID` — this is the naming grammar `retention.go`'s `Sweep` and `discussion.md`'s `one-file-per-process` decision both key on.
     - Open the file (`os.OpenFile` with `O_CREATE|O_WRONLY|O_APPEND`, matching `configureFromEnv`'s existing `LYX_LOG_FILE` open flags at `logger.go:86-94` for consistency), write the header record as the file's first line (a `slog`-shaped or plain text line naming `header.Command`, `header.Argv`, `header.TraceID`, `header.PID`, `header.WorktreeRoot`), set `sinkWriter` to the opened file, `sinkOK = true`.
  2. Return `sinkWriter, sinkOK`.

  This function is called by batch 5's dual-handler on every Info+/Warn record (trigger (a)) and by `NotifyExit` (Card 14, trigger (b)) — both call sites go through the same `sinkOnce.Do`, so the open logic runs at most once per process regardless of which trigger fires first.
- **Commit:** `feat(logger): implement lazy durable-sink open with geometry resolution and header write`

### Card 11: Size cap, truncation marker, and the shared mutex

- **Context:** none
- **Edits:**
  - `internal/logger/sink.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/logger/sink.go` the write-path implementing discussion.md's `retention` decision's size cap and `concurrency-contract`'s mutex scope:

  - Package-level `sinkMu sync.Mutex` guarding, as **one critical section**: the byte counter, the 8 MB cap check, the truncation-marker flag, and the actual write. (`concurrency-contract` requires these four be atomic together — an atomic counter alone would let two goroutines both observe the crossing and each emit a marker.)
  - `func writeDurable(p []byte) (int, error)`: under `sinkMu.Lock()`/`defer Unlock()`, if the running byte count plus `len(p)` would cross 8 MiB and the truncation marker has not yet been written, write one terminal record noting the truncation, set the marker-written flag, and stop accepting further writes to this file (subsequent calls become no-ops returning `len(p), nil` so callers do not error). Otherwise write `p` to `sinkWriter` and add `len(p)` to the running counter.
  - This is the function batch 5's durable handler calls for every Info+/Warn record once `ensureDurableSink()` has returned `sinkOK == true`.
- **Commit:** `feat(logger): add size-capped write path with a single truncation marker under the shared mutex`

### Card 12: Test seam — durable-sink directory override

- **Context:** none
- **Edits:**
  - `internal/logger/sink.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func SetDurableSinkDir(dir string)` to `internal/logger/sink.go`, following `SetOutput`'s existing precedent (`logger.go:140-143`) as the model for "a seam that lets tests point at a controlled location without going through production resolution." Setting a non-empty `dir` makes `ensureDurableSink()` (Card 10) use it directly instead of calling `hubgeometry.Resolve`, and leaves `header.WorktreeRoot` empty (no geometry ran) — this is the seam the "Sink naming and lazy open" unit tests (Card 15) and the header's documented seam-path behavior depend on.

  **`SetDurableSinkDir` is also the full sink-state reset point every test must use between cases.** `sinkOnce`, `sinkWriter`, `sinkOK` (Card 10), `header`, `headerOnce` (Card 9), and the write-path's byte counter and truncation-marker flag (Card 11) are all package-level and, being guarded by `sync.Once`/plain package vars, otherwise persist for the lifetime of the test binary — so without an explicit reset, only the first test in the whole internal/logger package that ever triggers `ensureDurableSink()` actually opens a file, and every later test's own `SetDurableSinkDir(t.TempDir())` call silently no-ops against a `sinkOnce` that already fired. Every call to `SetDurableSinkDir` (including with a non-empty `dir`, not only `SetDurableSinkDir("")`) must therefore reset ALL of: the override dir, `sinkOnce` (reassign a fresh `sync.Once`), `sinkWriter`, `sinkOK`, `header` (zero value), `headerOnce` (fresh `sync.Once`), the byte counter, and the truncation-marker flag — under `sinkMu` (Card 11/20). This mirrors Card 4's equivalent white-box reset requirement for `traceOnce`/`traceID`, extended to cover every piece of sink-side `sync.Once`-guarded state, not just the directory override.
- **Commit:** `feat(logger): add SetDurableSinkDir test seam for the durable sink`

### Card 13: `LYX_TRACE=1` test-entry-activation documentation note

- **Context:** none
- **Edits:**
  - `internal/logger/sink.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Card 10 already implements the `LYX_TRACE=1` gate inside `ensureDurableSink()`'s `sinkOnce.Do` body. This card adds the doc comment above `ensureDurableSink()` explaining the gate per discussion.md's `test-entry-activation` decision: unset means no durable sink and no filesystem or geometry access at all; `LYX_TRACE=1` is read once per process (via the same `sinkOnce` — no separate `sync.Once` needed since the gate check and the open both happen inside the one `Do` call), matching the existing outside-the-process activation model `LYX_LOG_LEVEL`/`LYX_LOG_FILE` already use (`logger.go:6-38`'s package doc, "Activation outside the lyx CLI" section). State explicitly in the comment that this env var is independent of `testing.Testing()` for the trace-ID/mint side (batch 2's `TraceID()` always lazily resolves regardless of `LYX_TRACE`) — `LYX_TRACE=1` gates only whether the **durable sink** opens, not whether `trace=`/`span=` fields are computed and stamped on stderr output.
- **Commit:** `docs(logger): document the LYX_TRACE=1 test-entry-activation gate`

### Card 14: Trigger (b) — force-open on non-zero exit

- **Context:** none
- **Edits:**
  - `internal/logger/sink.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func NotifyExit(code int)` to `internal/logger/sink.go`, implementing discussion.md's `sink-open-triggers` trigger (b): "the process exiting with a non-zero exit code." When `code == 0`, this is a no-op. When `code != 0`, call `ensureDurableSink()` (Card 10) — forcing the `sinkOnce`-guarded open (and header write) to run even if no Info+/Warn record was ever emitted, so a run that fails having logged nothing above Debug still leaves a reconstructable trace file. This is the function cmd/lyx's `run()`/`main()` calls (batch 7) right after capturing `clihelp.RunRoot`'s exit code and before returning it / calling `os.Exit`.
- **Commit:** `feat(logger): add NotifyExit to force the durable sink open on non-zero exit`

### Card 15: Sink naming and lazy-open tests

- **Context:** none
- **Edits:** none
- **Creates:**
  - `internal/logger/sink_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Untagged unit tests in `internal/logger/sink_test.go`. Every case below calls `SetDurableSinkDir(t.TempDir())` (Card 12) at its own start — never sharing one call across cases — so no `hubgeometry.Resolve` runs (Test Tier Purity) and, per Card 12, every case's `sinkOnce`/`sinkWriter`/`sinkOK`/`header`/`headerOnce`/byte-counter/marker-flag state is fully reset before that case runs, regardless of what an earlier case in this file (or in `logger_test.go`/`span_test.go`, added by later batches) already triggered:
  - No file exists in the sink directory after only `Debug`-level activity (this proves `traceOnce`/`sinkOnce` are not merged — a Debug-only run never triggers `ensureDurableSink`). Note: this test calls `ensureDurableSink()`/the write path directly or via whatever minimal surface exists at this point in the DAG; batch 5 is what wires `Debug` itself to call these — if this batch lands before batch 5, phrase the assertion in terms of `ensureDurableSink()` never having been invoked by anything at Debug level, using a manual call sequence that mirrors what batch 5 will later wire, OR (preferred) treat this specific "Debug never opens the file" assertion as re-verified by batch 5's own dual-handler fan-out test (Card 21) once the real call wiring exists, and scope this card's tests to what `sink.go` alone can prove: calling `ensureDurableSink()` directly creates exactly one file, whose first line is the header record.
  - The created filename matches the `trace-<YYYYMMDDTHHMMSSZ>-<16-hex-id>-<pid>.log` grammar and carries this process's `TraceID()` value and `os.Getpid()`.
  - The header's `WorktreeRoot` field is empty when opened via `SetDurableSinkDir` (the seam path never resolves geometry) — per `sink-open-triggers`'s note on the test seam.
  - `Arm()` called explicitly before `ensureDurableSink()`, versus never calling `Arm()` at all before `ensureDurableSink()` runs (the `LYX_TRACE=1`/test-seam path) — both produce an identical header shape for the static fields (`Command`/`Argv`/`TraceID`/`PID`), proving `armHeader()`'s `sync.Once` guard makes the two entry points equivalent.
- **Commit:** `test(logger): cover durable-sink naming and header composition`

### Card 16: Sink-open trigger and size-cap tests

- **Context:** none
- **Edits:**
  - `internal/logger/sink_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/logger/sink_test.go` (created by Card 15). Every test case below calls `SetDurableSinkDir(t.TempDir())` at its own start (never shares one call across cases) — per Card 12, this fully resets `sinkOnce`/`sinkWriter`/`sinkOK`/`header`/`headerOnce`/the byte counter/the marker flag, so each case starts from a clean, unarmed sink regardless of what earlier cases (in this file or `logger_test.go`/`span_test.go`) already triggered:
  - Calling `NotifyExit(0)` never opens the sink (no file created).
  - Calling `NotifyExit(1)` (or any non-zero code) with `SetDurableSinkDir` pointed at an empty `t.TempDir()` and no prior `Info`/`Warn` activity creates exactly one file whose first line is the header record — this is trigger (b) in isolation.
  - Writing past the 8 MiB cap via `writeDurable` (Card 11) stops accepting further byte writes and appends exactly one truncation-marker line; a second write attempted past the cap does not append a second marker (assert the file's marker-line count is exactly 1 after multiple post-cap write attempts).
- **Commit:** `test(logger): cover sink-open triggers and the size-cap truncation marker`

## Batch Tests

`verify: go test ./internal/logger/...` runs `sink_test.go` (Cards 15+16) alongside `trace_test.go` and `retention_test.go` from the two prerequisite batches, plus the existing `logger_test.go`.
</content>
