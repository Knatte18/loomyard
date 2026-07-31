MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic), self-assessed
reviewed_file: plan/
date: 2026-07-31
```

## Findings

### [BLOCKING] sinkOnce/headerOnce test state never reset between cases
**Location:** batches 4/5/6 — Cards 15, 16, 21, 22, 25, 26
**Issue:** `sinkOnce`, `sinkWriter`, `sinkOK`, `header`, `headerOnce`, and the size-cap byte-counter/truncation-marker are package-level and fire once per `internal/logger` test-binary process. Card 4 explicitly requires white-box resets of `traceOnce`/`traceID` between test cases, but no equivalent instruction exists for this analogous sink state — so only the first test across sink_test.go/logger_test.go/span_test.go that triggers `ensureDurableSink()` actually opens a file; every later test's own `SetDurableSinkDir(t.TempDir())` silently does nothing (the cached writer still targets the first test's now-cleaned-up TempDir).
**Fix:** Add the same reset instruction Card 4 gives traceOnce/traceID to sinkOnce/sinkWriter/sinkOK/header/headerOnce and the cap counter/marker flag, in each affected card.

### [BLOCKING] Composite Handle() per-inner-handler gating unspecified
**Location:** batch 5, Card 17
**Issue:** Card 17 states the composite's `Enabled()` ORs both inner gates, but never states `Handle()` must re-check each inner handler's own `Enabled(record.Level)` before delegating. `slog.Handler.Handle` implementations trust the caller already gated — a naive Handle() forwarding unconditionally to both inner handlers would print every durable-triggered Info line to stderr even at default Warn threshold, contradicting Card 21's own assertion ("Info reaches stderr only at -v or above").
**Fix:** State explicitly that Handle() must check each inner handler's Enabled(level) itself before calling its Handle.

### [BLOCKING] Shared sinkMu risks self-deadlock across Handle()/writeDurable
**Location:** batch 5, Card 20 (interacts with Card 11)
**Issue:** Card 20 puts the composite's "read of the current stderr handler" under the same `sinkMu` that `writeDurable` (Card 11) locks. A natural single Lock/defer-Unlock spanning the whole `Handle()` call (which then invokes the durable branch's writeDurable) self-deadlocks — `sync.Mutex` is non-reentrant. Card 22's `-race` test would hang rather than fail cleanly.
**Fix:** Specify the stderr-handler-state read locks/unlocks in its own narrow section, released before any call into writeDurable.

### [BLOCKING] Card 38 omits internal/shuttleengine/run.go from Edits
**Location:** batch 11, Card 38
**Issue:** Card 38 directs auditing shuttleengine/run.go's `saveRunState` error path (near the line-167 `logger.Info`) and adding a call "if the audit confirms it qualifies," but run.go is listed only under Context, not Edits, and is absent from `## All Files Touched`. Source confirms `saveRunState`'s failure branch (run.go:152-165) is a bare, unlogged error return, so the audit will very likely require editing this file.
**Fix:** Add `internal/shuttleengine/run.go` to Card 38's Edits and to All Files Touched.

### [BLOCKING] CONSTRAINTS.md addition misdescribes perchengine as a spawn site
**Location:** batch 14, Card 46
**Issue:** Card 46 adds `internal/perchengine/engine.go` to Live-Substrate Spawn Observability's "Known instrumented call sites" list, but perch's own added call (batch 12 Card 39) is a generic error-passthrough Warn on `te.Run()`'s result — not a spawn/teardown log naming session/socket/PID. Perch never itself spawns an OS process (treadleengine owns the gate subprocess); listing it there is inaccurate.
**Fix:** Drop `internal/perchengine/engine.go` from that list, or reword to make clear it's an adoption-pass call, not a spawn/teardown observability call.

### [NIT] Batch 2 frontmatter card-count mismatch
**Location:** batch 2 (02-logger-trace-identity.md)
**Issue:** Header states `cards: 4` but only three cards (2, 3, 4) exist in the file.
**Fix:** Correct the frontmatter to `cards: 3`.

### [NIT] SetOutput doc comment's stale claim left uncorrected
**Location:** batch 5, Card 19 / internal/logger/logger.go:137-139
**Issue:** The existing comment says "production code never calls it," but `configureFromEnv` (production init path) already calls `SetOutput(f)` for LYX_LOG_FILE (logger.go:93) today. Card 19 rewrites this same doc comment for the stderr-only-rebind note but doesn't fix the pre-existing false claim.
**Fix:** While rewriting SetOutput's doc comment, also correct "production code never calls it."

## Verdict

REQUEST_CHANGES
Missing test-state-reset instructions and underspecified composite-handler locking/gating risk correctness and deadlocks.
MILL_REVIEW_END
