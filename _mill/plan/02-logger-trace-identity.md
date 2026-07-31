# Batch: logger-trace-identity

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: logger-trace-identity
number: 2
cards: 4
verify: go test ./internal/logger/...
depends-on: []
```

## Batch Scope

Adds the process trace-identity primitive in a new `internal/logger/trace.go`: mint-or-adopt-and-export, plus a lazily-resolved `TraceID()` accessor with the three-way precedence discussion.md's `trace-id-mint-and-propagate` and second-mint-site paragraph require (root-hook value → `LYX_TRACE_ID` env → fresh mint). This batch is stdlib-only (no `hubgeometry`, no `proc`) and has no dependency on any other batch — the durable sink (batch 4) and dual-handler fan-out (batch 5) both consume `TraceID()` but this batch does not consume anything from them. Stamping `trace=` onto emitted lines is explicitly **not** this batch's job — that is batch 5's `Debug`/`Info`/`Warn` rewrite, since `trace=` must appear on every emitted line regardless of sink/verbosity state, and that wiring lives in the fan-out change.

## Cards

### Card 2: Trace-ID resolution state and precedence

- **Context:**
  - `internal/logger/logger.go`
- **Edits:** none
- **Creates:**
  - `internal/logger/trace.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/logger/trace.go` implementing the process trace-identity primitive described in discussion.md's `trace-id-mint-and-propagate` decision:

  - `mintTraceID() string` — generates 8 random bytes via `crypto/rand.Read` and renders them as 16 lowercase hex characters via `encoding/hex.EncodeToString`.
  - Package-level `traceOnce sync.Once` and `traceID string`, **distinct from `sinkOnce`** (batch 4) per the `concurrency-contract` decision — `traceOnce` fires on **any** emit, including a `Debug` call that never reaches the durable sink, because `trace=` is stamped on every line at every level (batch 5 wires the call site).
  - A resolver, invoked lazily inside `traceOnce.Do(...)`, with this exact precedence: (1) if a value has already been set by `MintOrAdoptAndExport` (card 3) — i.e. `traceID` is already non-empty when `traceOnce.Do` runs, which happens when the root hook called `MintOrAdoptAndExport` before any emit occurred — use it; (2) otherwise adopt `os.Getenv("LYX_TRACE_ID")` when set and non-empty after `strings.TrimSpace`; (3) otherwise call `mintTraceID()`. Treat whitespace-only as unset (same `TrimSpace`-then-empty-check idiom `configureFromEnv` already uses in `logger.go:78-85` for `LYX_LOG_LEVEL`).
  - `func TraceID() string` — the exported lazy accessor: calls `traceOnce.Do(resolve)` then returns `traceID`. This is what batch 5's `Debug`/`Info`/`Warn` will call to stamp `trace=`.

  This design means a `go test` binary driving live substrate (never running cmd/lyx's root hook) still resolves a real per-process trace-ID the first time anything logs, with no code change at the call site, matching discussion.md's "second mint/adopt site, for entry points that never reach the root hook" paragraph.
- **Commit:** `feat(logger): add lazy trace-ID resolution with root-hook/env/mint precedence`

### Card 3: Root-hook mint/adopt/export entry point

- **Context:**
  - `cmd/lyx/main.go`
- **Edits:**
  - `internal/logger/trace.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/logger/trace.go`:

  ```go
  func MintOrAdoptAndExport() string
  ```

  This is the **only** function in the package that calls `os.Setenv`. It is called exclusively by `cmd/lyx/main.go`'s root `PersistentPreRunE` (batch 7) — never by the lazy `TraceID()` path in Card 2. Behavior: adopt `os.Getenv("LYX_TRACE_ID")` when set and non-empty (same trim-and-check as Card 2's resolver); otherwise mint via `mintTraceID()`. Store the result into the same package-level `traceID` var Card 2's `traceOnce` guards — call `traceOnce.Do(func() { traceID = <resolved value> })` so a subsequent `TraceID()` call (from any package-level `Debug`/`Info`/`Warn` in the same process) sees this value via step (1) of Card 2's precedence rather than re-resolving from the environment. Then `os.Setenv("LYX_TRACE_ID", traceID)` so every spawned child inherits it (children inherit `os.Environ()` by default at every non-`cmd.Env`-touching spawn site per discussion.md's `trace-id-mint-and-propagate` "Sites the plan must verify" paragraph). Return the resolved ID.

  A test-binary path (which calls `TraceID()` directly without ever calling `MintOrAdoptAndExport`) must never call `os.Setenv` as a side effect of logging — this function's export call is deliberately gated to only the one call site.
- **Commit:** `feat(logger): add MintOrAdoptAndExport for the root command hook`

### Card 4: Trace-ID mint/adopt/export unit tests

- **Context:** none
- **Edits:** none
- **Creates:**
  - `internal/logger/trace_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Untagged unit tests (Test Tier Purity — no spawns, this package is stdlib-only so nothing here could spawn anyway) in `internal/logger/trace_test.go`:
  - A set `LYX_TRACE_ID` (via `t.Setenv`) is adopted verbatim by `MintOrAdoptAndExport()`.
  - Unset `LYX_TRACE_ID` causes `MintOrAdoptAndExport()` to mint a value matching a 16-lowercase-hex-character pattern (regex or manual char-class check).
  - An empty string and a whitespace-only string set for `LYX_TRACE_ID` are both treated as unset (mint path taken, not adopted verbatim).
  - `TraceID()` called with no prior `MintOrAdoptAndExport()` call and no `LYX_TRACE_ID` set also mints a 16-lowercase-hex value (the lazy/test-entry path from Card 2).
  - Each test that exercises `traceOnce`/`sync.Once` state must run in its own subprocess-free isolation: since `traceOnce` is a package-level `sync.Once`, tests that need a fresh resolution must reset the unexported `traceOnce`/`traceID` vars directly (white-box test, same package) between cases — follow `logger_test.go`'s existing white-box pattern (it is `package logger`, not `logger_test`).
  - Propagation check: after `MintOrAdoptAndExport()` returns a value, `os.Getenv("LYX_TRACE_ID")` returns that same value (this is the untagged, no-real-spawn version of discussion.md's Testing → "Propagation check" bullet — the real spawn sites (`internal/boardengine/spawn.go:27`, `internal/fabricengine/spawn.go:62`) inherit `os.Environ()` by default with no `cmd.Env` assignment, so asserting the exported env var is set correctly is sufficient without a real child process).
- **Commit:** `test(logger): cover trace-ID mint/adopt/export precedence and propagation`

## Batch Tests

`verify: go test ./internal/logger/...` runs `trace_test.go` alongside the existing `logger_test.go` suite (unaffected — `logger.go` is not edited by this batch).
</content>
