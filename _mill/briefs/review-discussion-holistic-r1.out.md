MILL_REVIEW_BEGIN
# Review: Bouncer: the generic review-gate producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:consistency] `Shuttle` already exists in this package
**Section:** Technical context → "The judge spawn pattern"
**Issue:** The discussion instructs declaring "a package-local `Shuttle` interface ... with a `var _ Shuttle = (*shuttleengine.Runner)(nil)` compile-time proof, exactly as both `judge.go` and `singlellm.go` do" — but `internal/shedadapters/singlellm.go:23-27` already declares exactly that `Shuttle` interface and that `var _` in the same package, so a second declaration is a compile error.
**Fix:** State that the Bouncer reuses the package's existing `Shuttle` interface and existing compile-time proof, adding neither.

### [BLOCKING:design] Seed call silently consumes a bounce from the budget
**Section:** Decisions → "No round-cap ladder, no progress judge" / "Two modes"
**Issue:** `shedengine.episodeStuckCount` (run.go) counts every `Stuck` by a producer since that producer's own last `Done`; the Bouncer returns `Done` only on approval, which exits the segment, so the episode never resets and the unconditional seed-call `Stuck` permanently costs one unit — `MaxBounces: N` yields N−1 judged rounds, not N.
**Fix:** State the off-by-one explicitly as the segment's budget semantics, so the three `loom` wiring tasks and the constructor doc size `MaxBounces` against it.

### [BLOCKING:design] Exported round-resolution helper has no stated contract
**Section:** Decisions → "An exported round-resolution helper" / "Round resolution"
**Issue:** The helper exists precisely so the Burler-round producer cannot drift, yet its return shape is never pinned (the Bouncer needs "round to judge", the round producer needs "round to write", which differ by one on the same disk state), and the gap-in-sequence case (reports 1 and 3 present, 2 absent) is deferred to the test section as "pin and assert the chosen behavior explicitly" — an undecided item, not a decision.
**Fix:** Pin the exported signature and its semantics (including the seed case and the gap case) in the discussion, since both halves of the segment depend on the same answer.

### [BLOCKING:design] Re-judging an already-judged round is unaddressed
**Section:** Decisions → "Two modes, told apart by file existence only"
**Issue:** The discriminator is the *report* file alone; the roadmap pins the round producer to always return `Stuck` back to the Bouncer, so a round producer that bounces without writing a new report leaves the Bouncer seeing round N's report as highest and re-spawning a full judge call on a round whose `round-N-bouncer-verdict.md` already exists — archiving the prior verdict, paying for the session again, and looping until the budget is exhausted.
**Fix:** Decide and state the rule for "highest report already has a verdict" (e.g. also consult verdict presence or report mtime), rather than resolving mode from the report file alone.

### [NIT:decision] Live-Substrate obligation left as a question
**Section:** Constraints
**Issue:** The Live-Substrate Spawn Observability entry says "check whether this invariant imposes logging obligations on the spawning call site and honor them if so" — a TBD rather than a disposition, though the three sibling adapters resolve it (they log warnings only; the spawn/teardown lines live in `shuttleengine`).
**Fix:** State the disposition directly instead of deferring the check to the plan.

### [NIT:design] Ledger pointer on a degraded judge `Stuck`
**Section:** Decisions → "Cancellation and the output pointer"
**Issue:** The pointer is "the bouncer ledger path on both `Done` and `Stuck` from a judge call", but every degradation path also returns `Stuck` without a ledger having been written, so the recorded `history[].output` names a file that does not exist.
**Fix:** Say whether a degraded judge call reports an empty pointer or the (possibly absent) ledger path.

## Verdict

REQUEST_CHANGES
Four blocking gaps: duplicate `Shuttle`, budget off-by-one, unpinned helper contract, re-judge loop.
MILL_REVIEW_END
