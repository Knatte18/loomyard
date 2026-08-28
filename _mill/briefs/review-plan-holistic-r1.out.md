MILL_REVIEW_BEGIN
# Review: Audit internal/logger coverage across spawn/hard-error paths — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-28
```

## Findings

### [BLOCKING:scope] Card 1's Context omits three files its own Requirements assert a fact about
**Location:** batch 1 (`01-audit-doc-and-constraints.md`), Card 1, Requirements bullet 2. **Issue:** The card instructs the audit doc to "name the three files that hit the `exec.Command` substring with zero real calls: `internal/githubclient/doc.go`, `internal/reedengine/doc.go`, and `internal/reedengine/attach.go`" — a factual claim about those files' content — but none of the three appears in Card 1's `Context:` list (verified against the manifest: they are absent from that card, though all three are correctly present in Card 14's `Context:`). Per the Context-completeness rule, the implementer may only read files in `Context:`, so this claim cannot be independently re-verified and must be copied on faith from `_mill/discussion.md`, contradicting the plan's own `selector-reruns-are-the-authority` decision that tables are a snapshot to re-verify, not trust. **Fix:** Add `internal/githubclient/doc.go`, `internal/reedengine/doc.go`, and `internal/reedengine/attach.go` to Card 1's `Context:` list.

### [BLOCKING:consistency] Card 7 says "four" outcomes but enumerates only three, omitting the most-hit path
**Location:** batch 3 (`03-spawn-site-log-lines.md`), Card 7, Requirements bullet 2. **Issue:** The card requires "a `logger.Info` teardown line on each of the four post-`CombinedOutput` outcomes" but names only three: the clean-exit path (`err == nil`), the `context.DeadlineExceeded` path, and the `exec.ErrWaitDelay` path. Verified against `internal/treadleengine/gate.go`: `execGateCommand` has five returns total — those three, plus the ordinary non-zero-exit path (`errors.As(err, &exitErr)` → `return output, false, nil`) — which is uncounted in the "four" but also unnamed in the list — plus the final failed-to-start `Warn` path. The omitted branch is the ordinary "gate command ran and failed" case, the one `converged()` checks on every round; as written an implementer could plausibly leave it unlogged while believing all four required sites are covered. **Fix:** Explicitly name the fourth branch (`errors.As(err, &exitErr)`, the ordinary non-zero-exit return) in the enumerated list alongside the other three.

## Verdict

REQUEST_CHANGES
Two concrete, source-verified gaps: a missing Context entry and an under-enumerated branch list in Card 7.
MILL_REVIEW_END
