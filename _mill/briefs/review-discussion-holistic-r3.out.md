MILL_REVIEW_BEGIN
# Review: Bouncer: the generic review-gate producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] judged(N) can strand a parsed APPROVED verdict
**Section:** "Three modes, told apart by file existence only" + "Artifact naming and paths"
**Issue:** A judge spawn that writes `round-<N>-bouncer-verdict.md` and `round-<N>-bouncer-ledger.md` but not `round-<N+1>-focus.md` classifies non-`done` (`allOutputFilesExist`, `internal/shuttleengine/wait.go:222,273`), so the design's own degradation returns `Stuck` without ever reading the verdict; on the next `Call` `judged(N)` is now true, so the producer re-bounces forever and an `APPROVED` round never reaches `Done` — the same stranding the unconditional-`OutputFiles` note says must never happen.
**Fix:** State a disposition for "all `judged(N)` files present but the outcome was discarded as a degradation" — e.g. whether the re-bounce mode re-reads and acts on the existing parsed verdict, or whether that case is excluded from `judged(N)`.

### [BLOCKING:design] Missing round-N+1 focus file has no stated recovery
**Section:** "Scope / In" (focus-file writer) + "Three modes"
**Issue:** Scope promises a focus writer used by "any path that must synthesise an empty-exclusions focus file", but no decision names such a path beyond the seed call; after a `BLOCKING` judge whose focus write failed (or an unparseable ledger, which `judged(N)` does not require to parse), round `N+1` starts with no `round-<N+1>-focus.md` and the round producer's read path is broken exactly as the seed fallback's rationale says must be avoided.
**Fix:** Name explicitly which non-seed paths synthesise the focus file, and whether `judged(N)` requires the ledger to parse as the verdict does.

### [BLOCKING:consistency] Stale r1 answer contradicts the pinned budget rule
**Section:** Q&A log, `[review r1]` bounce-budget entry
**Issue:** That entry's **A:** still asserts "`MaxBounces: N` yields `N-1` judged rounds", contradicting the body's pinned rule (verified correct against `run.go:197` reading `st.History` pre-append and `episodeStuckCount` at `run.go:275`) and the later r2 entry; a plan writer quoting the r1 line documents the wrong operator-facing rule.
**Fix:** Mark the r1 answer superseded in place, or restate it as `N` judged rounds.

### [NIT:design] Rubric stencil existence unvalidated and unregistered
**Section:** "Stencils — two generic templates, rubric injected by name"
**Issue:** `stencilstore.Read` never falls back to a shipped default (`reconcile.go:28`) and `Reconcile` seeds only registry names, so an unregistered or mistyped `RubricStencil` degrades every judge call to `Stuck` and burns the budget silently — the exact mid-run wiring-typo failure the constructor's eager-validation rationale rejects.
**Fix:** State whether rubric stencils must be registry-registered and whether the constructor probes the rubric's readability.

### [NIT:consistency] Pointer rule stated three ways
**Section:** "Cancellation and the output pointer" + Q&A pointer entry
**Issue:** The Q&A answer says "ledger path on `Done` and `Stuck` from a judge call, empty from a seed call" with no mention of degraded or re-bounce paths, and the decision sentence lists the re-bounce exception inside the "every other path reports an empty pointer" enumeration.
**Fix:** Restate the exists-or-empty rule once, as one list of outcomes, and align the Q&A answer to it.

## Verdict

REQUEST_CHANGES
Judged-round predicate can strand an approval; focus-file recovery and a stale budget claim unresolved.
MILL_REVIEW_END
