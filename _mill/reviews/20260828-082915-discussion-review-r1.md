# Review: reed: watchdog daemon

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Backoff attempt-count cap is asserted but never defined, and appears to conflict with "loop never exits"
**Section:** `watch-loop-failures-are-never-fatal` Decision; Constraints (Live-Substrate Spawn Observability); Testing (Tier 1, "The backoff" bullet)
**Issue:** The Decision commits to "backoff on repeated failure" and "the loop never exits," CONSTRAINTS.md's Live-Substrate Spawn Observability item is quoted verbatim as requiring "A retry/backoff loop must cap attempt count, not only elapsed time," and the Tier 1 test plan commits to asserting "attempt count is capped" — but no value or policy for that cap is stated anywhere, and it's unclear how a capped count coexists with a loop that never exits: the Q&A explicitly rejects "stopping the loop after N consecutive failures and falling back to `blockForever()`" as an alternative (self-heal silently stopping), which is the most natural reading of what a hard attempt-count cap would do. A plan writer has no way to derive whether the cap resets per resize signal, caps only backoff-interval growth (a ceiling on delay, not a stop), or something else.
**Suggested fix:** Add a short rationale clause to `watch-loop-failures-are-never-fatal` (or a new Decision) stating precisely what "capped attempt count" means here — e.g. the cap bounds a *bounded escalating backoff delay* per failure streak and resets on the next successful apply or next resize signal, never halting the loop — so the Tier 1 test has an actual contract to assert against, and the apparent tension with the rejected "stop after N failures" alternative is explicitly reconciled.

## Verdict

REQUEST_CHANGES
One well-grounded design gap (backoff/attempt-count-cap semantics undefined and in tension with the loop-never-exits decision); everything else — scope, the five live-Q&A decisions, constraint coverage, and testing — is unusually thorough and independently verified against source.
