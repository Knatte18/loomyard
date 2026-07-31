# Batch: burler-fabric-shuttle-adoption

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: burler-fabric-shuttle-adoption
number: 11
cards: 1
verify: go test ./internal/burlerengine/... ./internal/fabricengine/... ./internal/shuttleengine/...
depends-on: [5]
```

## Batch Scope

`internal/burlerengine`, `internal/fabricengine`, and `internal/shuttleengine` each already carry exactly one `logger` call (per discussion.md's adoption-baseline table) and are otherwise small packages — this batch audits all three against the `adoption-scope` done-criterion in one card rather than three separate batches, since their combined `Context:`/`Edits:` footprint is small enough to share.

## Cards

### Card 38: Audit and adopt across burler/fabric/shuttle

- **Context:**
  - `internal/fabricengine/coalesce.go`
- **Edits:**
  - `internal/burlerengine/engine.go`
  - `internal/fabricengine/spawn.go`
  - `internal/shuttleengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Apply the `adoption-scope` done-criterion to each package:

  - **`internal/burlerengine/engine.go`** — two research-verified candidate sites right beside the existing `logger.Info("burler: round starting", ...)` call at line 108: `engine.go:120-122` (`os.MkdirAll(burlerDir, 0o755)` failure, currently wrapped and returned with no log call) and `engine.go:123-126` (`os.MkdirTemp(burlerDir, "round-")` failure, same shape). Add `logger.Warn` at both, naming `burlerDir`/`opts.Round` and the error.
  - **internal/fabricengine** — audit `spawn.go` (the `SpawnDetachedPush` re-exec at line 62, `cmd.Start()`'s own error path — confirm whether it is already logged or silently returned) and `coalesce.go` (already has one `logger.Warn` at line 86 for the push-rejected/diverged-remote case) against the done-criterion; add a call only to a site that genuinely qualifies (discarded/retried error, or missing identifying context, on a process/lock/file/subprocess path) — do not force a call onto `spawn.go:62`'s intentional "not `Wait()`ed" detached-start pattern if its own `cmd.Start()` error is already propagated with adequate context.
  - **`internal/shuttleengine/run.go`** — confirmed candidate: `saveRunState(runDir, state)`'s own failure branch (`run.go:152-165`, immediately before the existing `logger.Info("shuttle: run started", ...)` call at line 167) wraps and returns `fmt.Errorf("shuttle: save run state: %w", err)` with no internal/logger call on the `saveRunState` failure itself — the branch's own nested `RemoveStrand`-failure diagnostic uses the stdlib `log.Printf`, not internal/logger, so it does not satisfy the obligation either. Add `logger.Warn` naming `runDir`, `strand.GUID`, and the `saveRunState` error before the branch's cleanup/return.

  Do not add filler calls to sites that already wrap and propagate identifying context, or to sites outside a process/lock/file/subprocess boundary. Existing suites must stay green (instrumentation only).
- **Commit:** `feat(burlerengine,fabricengine): adopt logger.Warn at unlogged mkdir/spawn error sites`

## Batch Tests

`verify: go test ./internal/burlerengine/... ./internal/fabricengine/... ./internal/shuttleengine/...` — all three suites must stay green.
</content>
