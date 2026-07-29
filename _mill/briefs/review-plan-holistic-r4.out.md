MILL_REVIEW_BEGIN
# Review: codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] Card 32 Context omits symbol.go and errors.go
**Location:** batch 8 (definition-and-symbol-engine), card 32
**Issue:** Requirements call `symbolFromClient` (defined in `symbol.go`, card 30) and construct/assert against `ErrAmbiguousSymbolSentinel`/`ErrSymbolNotFoundSentinel`/`ErrResolverUnsupportedSentinel` (all defined in `errors.go`), but Context lists only `refs_test.go`/`lspclient_test.go` and Edits is `none` — neither file contains `symbolFromClient`, `SymbolMatch`'s field names, or any of those three sentinel spellings (verified against the real `refs_test.go`/`lspclient_test.go`, which only reference `ErrServerNotFoundSentinel`/`ErrServerTimeoutSentinel`).
**Fix:** Add `internal/codeintelengine/symbol.go` and `internal/codeintelengine/errors.go` to card 32's Context.

### [BLOCKING] Card 25 Context omits errors.go
**Location:** batch 6 (ensure-server-supervised), card 25
**Issue:** `supervised_test.go`'s retry-exhaustion sub-test asserts `errors.Is(err, ErrServerSpawnTimeoutSentinel)` — that sentinel is defined in `errors.go` (card 23) — but card 25's Context (`ensureserver.go`, `daemonstate.go`, `lspclient_test.go`, `refs_integration_test.go`) never includes `errors.go`; `ensureserver.go` will only show the struct construction `&ErrServerSpawnTimeout{Lang: lang}`, not the sentinel var itself.
**Fix:** Add `internal/codeintelengine/errors.go` to card 25's Context.

### [NIT] `ensureSupervised` has no recovery from a live-but-wedged daemon
**Location:** batch 6, card 24
**Issue:** `daemonStale` only checks PID liveness + protocol version; a daemon process that is alive but has hung/never bound its listen socket will never be classified stale, so every later caller's dial-then-finalize fails, a losing caller never restarts it (state reads healthy at step 3), and the call spins through the bounded retry to `ErrServerSpawnTimeout` forever until the process actually dies or an operator intervenes.
**Fix:** Note this as a known limitation in the doc comment (card 43 already covers restart-on-staleness elsewhere), or have step (3)'s double-check treat a dial failure against an already-"healthy" state as a restart trigger too — not blocking since `supervised` has no live V1 dispatch path.

## Verdict

REQUEST_CHANGES
Two cards (25, 32) omit Context files their own Requirements construct/assert types from — fix before proceeding.
MILL_REVIEW_END
