MILL_REVIEW_BEGIN
# Review: shedengine: per-producer bounce budget + explicit OnDone routing

```yaml
duration_s: 189.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [NIT:consistency] Budget test arithmetic is wrong: both use A↔B
**Demoted-from:** BLOCKING
**Section:** "Ordering gotcha in the `Stuck` arm" (:223-225) and Testing (:292, :294)
**Issue:** `TestRun_BounceBudgetExhaustion` (`run_routing_test.go:203-240`) and `TestRun_MaxBouncesZeroResolvesToDefault` (:242-267) both wire **two** producers, `A OnStuck: "B"` / `B OnStuck: "A"`, and assert `a.calls + b.calls == MaxBounces+1` (4) and `defaultMaxBounces+1` (11); under a per-producer budget each producer carries its own count, so the totals become `2×MaxBounces+1` (7) and `2×defaultMaxBounces+1` (21) — the discussion states the pre-append read preserves "`TestRun_BounceBudgetExhaustion`'s current expectation" and describes the second test as merely "now two-level", both false.
**Fix:** State that both engine budget tests change their asserted call totals under per-producer semantics (or must be re-wired to a single bouncing producer), and reserve the pre-append/post-append note for the single-producer boundary it actually pins.

### [BLOCKING:design] No disposition for the lost total-spend bound
**Section:** "Per-producer bounce budget" rationale (:79-82) and the `shed.md` doc inventory (:244)
**Issue:** `shed.md:90` grounds the single counter in "total wasted spend before a human is pulled in" and rejects per-producer budgets precisely because an A↔B cycle runs `2×budget`; the discussion instructs that this "argument is now overridden and the doc must say so" but never states what overrides it — the offered reasons (roadmap wording, lockstep Bouncer/Burler) address the segment-vs-producer unit, not the aggregate, and episode scoping makes a task's lifetime total unbounded since every `Done` resets. A plan writer rewriting that paragraph has no position to write.
**Fix:** Record an explicit decision on the aggregate spend bound — either that no run-wide cap exists after this task and why that is acceptable, or that one is deliberately deferred — and make that the sentence `shed.md:90` is rewritten to.

### [NIT:design] Runaway `Done` cycle grows the status file quadratically
**Section:** "Runtime disposition for a `Done` cycle" (:155-161)
**Issue:** The accept-as-unbounded rationale names only interruptibility, but each iteration appends one entry and `persist` rewrites the whole `history` slice (`run.go:305-320`), so an unattended `Done` cycle is O(n²) status-file writes and unbounded file growth, not merely a spinning loop.
**Fix:** Add one clause to the accepted-risk statement noting the growth cost so `shed.md`'s wording does not read as "harmless until someone notices".

## Verdict

REQUEST_CHANGES
Two blocking issues: false test-arithmetic claim, and no stated position on aggregate bounce spend.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
