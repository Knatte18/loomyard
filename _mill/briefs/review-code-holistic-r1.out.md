MILL_REVIEW_BEGIN
# Review: codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-29
```

## Findings

### [BLOCKING] `ensureSupervised`'s retry loop can spin forever with no deadline check
**Location:** `internal/codeintelengine/ensureserver.go:285-304` (step 3, the "acquired the lock, but state already healthy" branch)
**Issue:** When the spawn-race lock is uncontended (no other caller racing) and the recorded daemon reads as healthy but is not actually dialable/answering (the exact "wedged daemon" scenario the function's own doc comment names), the loop: step 1 dial/finalize fails → step 2 acquires the free lock immediately → step 3 re-reads healthy state, releases the lock, sleeps 100ms, and `continue`s back to step 1 — with **no `time.Now().After(deadline)` check anywhere in this path**. The only deadline check in the whole function is in the `!acquired` branch (line 278). This directly contradicts the function's own doc comment ("The whole call is bounded by `deadline`...") and the "Known limitation" paragraph's explicit claim that "the bounded retry in step (2)/(3) still returns `ErrServerSpawnTimeout` per call rather than hanging" — step (3) has no such bound. `internal/codeintelengine/doc.go:228-233` repeats the same now-inaccurate claim. This traces back to the plan itself (`06-ensure-server-supervised.md` card 24, step 3) never specifying a deadline check for this branch — the implementer followed the plan faithfully, but the plan has the gap.
**Fix:** Add the same `if time.Now().After(deadline) { return nil, &ErrServerSpawnTimeout{Lang: lang} }` check in step 3 before (or instead of) the unconditional sleep-and-continue, mirroring step 2's guard; update the doc comments in `ensureserver.go` and `doc.go` if the semantics change. `supervised_test.go`'s retry-exhaustion test only exercises the `!acquired` path (it pre-holds the lock itself), so this gap is currently untested — add a sub-test with an uncontended lock and a permanently-undialable-but-PID-alive daemon to pin the fix.

## Verdict

REQUEST_CHANGES
`ensureSupervised`'s step-3 branch has no deadline check, contradicting its own doc comment's bounded-retry guarantee.
MILL_REVIEW_END
