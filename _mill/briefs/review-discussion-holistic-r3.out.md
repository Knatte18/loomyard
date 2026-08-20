MILL_REVIEW_BEGIN
# Review: shedadapters: Burler-round producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] A died round can still leave round-N-review.md
**Section:** "Round resolution from disk…" / "Always `Stuck`, never `Done`"
**Issue:** The discussion's premise "a failed round returning `Stuck` with no review file written" is false: `shuttleengine` classifies `OutcomeDone` only when *all* `OutputFiles` exist (`internal/shuttleengine/wait.go:299,305`, `allOutputFilesExist`), so a round that wrote `ReviewPath` in phase A and died/timed out in phase B is `OutcomeDied` with `round-<N>-review.md` on disk — the most likely death window for an A-review→B-fix round. Attempt 2's pre-attempt archive covers the retry, but a *second* consecutive `died`/`timeout` (hard error), an `asking` hard error, and a cancellation between attempts all return while that partial review sits at the round's canonical path, so the next `Call` advances to `N+1` and the `Bouncer` judges a phase-A-only review as a completed round.
**Fix:** State the archive rule as "archive both round paths before returning on any exit that is not a completed, parsed round", not only on an error whose `Result.Outcome == OutcomeDone`, and add the double-`died`-leaves-a-partial-review case to the test list.

### [BLOCKING:design] MaxBounces cap is set by the Bouncer row, not this one
**Section:** "Always `Stuck`, never `Done`" — the "Consequence worth naming" bullet
**Issue:** The claim that this row's `effectiveMaxBounces` *is* the segment's hard cap (and must be raised on this row) rests on a false premise. The `Bouncer` also never returns `Done` until it approves, and per `manifest/roadmap.md:23` it is the segment's entry point, so its Stuck sequence is seed, then one per rejection — always one ahead of this producer's. With `episodeStuckCount`/`effectiveMaxBounces` as shipped (`internal/shedengine/run.go:197,275,295`), the Bouncer row exhausts its budget first and blocks the segment. Raising only this row's `MaxBounces` would not raise the round cap, and the discussion mandates putting that statement in the producer's doc comment.
**Fix:** Restate the consequence as "both rows in the segment carry an unresetting episode, and the segment's round cap is the smaller of the two budgets — the Bouncer's normally binds first", and say both rows must be raised together.

### [NIT:consistency] Off-by-one in "hard cap of ten rounds"
**Section:** same bullet, and "Prior-round hydration" (the eighteen-artifact estimate)
**Issue:** `internal/shedengine/run.go:197-199` blocks when the count *reaches* the budget, so a budget of ten performs ten bounce-backs and blocks on the eleventh Stuck — the row's own budget permits eleven of its calls, not ten.
**Fix:** Pin the boundary explicitly (budget N ⇒ N bounce-backs, blocked on the N+1th Stuck) wherever the round-cap number is quoted.

### [NIT:scope] Fourth doc site with a stale "three adapters" count
**Section:** Scope → In (doc updates)
**Issue:** The list names `manifest/designs/shed.md`'s Engine-adapters section plus `docs/overview.md:235,316,318`, but `manifest/designs/shed.md:324` ("The three engine adapters (`SingleLLMProducer`, `perch`, `Webster`) are a separate task") sits under "## Process" and also carries the count.
**Fix:** Either add that line to the enumerated sites or state explicitly that it is a historical task-decomposition sentence left standing.

## Verdict

REQUEST_CHANGES
Artifact discriminator and segment round-cap claims both rest on verified-false premises.
MILL_REVIEW_END
