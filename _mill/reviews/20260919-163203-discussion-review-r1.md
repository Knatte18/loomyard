# Review: Seeded Shed core: run addressing, seed contract, batten

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewer_self_id: claude-fable-5
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [NIT:consistency] Run-Shed escalation rests on "Stuck with no self-route", which cannot exist
**Demoted-from:** BLOCKING
**Section:** `run-shed-re-entrancy-via-self-pointing-on-stuck` + the `internal/battenshed` testing bullet
**Issue:** `OnStuck` is static per `ProducerDef` (`shedengine/producer.go:43`), and a producer's `Call` returns only `(Outcome, OutputPointer, error)` — with `on_stuck: Run-Shed` in the recipe, EVERY Stuck routes back to Run-Shed (`run.go:251-255`); a blocked/paused/failed child cannot escalate "unchanged" as the decision text and the test table ("Stuck with no self-route") both claim. The Q&A's last entry states the opposite (escalation = bounce exhaustion), so the artefact contradicts itself on its own hardest point.
**Suggested fix:** Rewrite the decision against a mechanism that exists: either terminal-blocked child → hard error (StateFailed, message carrying state/error/current_producer — matches `innerRunProducer`'s existing error-vs-verdict split), or explicitly accept exhaustion-based escalation with its delay/spin cost stated; update the battenshed test table to match, and note the blocked-child bounce has no sleep, so it burns the remaining budget in a tight loop.

### [BLOCKING:design] Re-entrancy cost accounting omits one weft commit+push per bounce
**Section:** `run-shed-re-entrancy-via-self-pointing-on-stuck`, "Known cost" paragraph
**Issue:** Every `persist` invokes `CommitStatus` unconditionally (`shedengine/run.go:473-476`), and the seam the discussion says to model (`loomcli/wiring.go newCommitStatusSeam`) commits AND pushes per transition; with batten's status durable and `CommitStatus` non-nil (both decided here), a 12-hour child at 30s is ~1440 weft commits and pushes on prime's pair — the stated cost ("~1440 history entries") undercounts by the expensive part, and this materially strengthens the rejected alternatives.
**Suggested fix:** State the commit+push-per-bounce cost and decide: accept it explicitly, make batten's seam skip the self-bounce case (producer and state unchanged — history churn rides along on the next real transition), or revisit the third-Outcome alternative with the true cost on the table.

### [NIT:consistency] "max_bounces set to today's attempt cap" is ambiguous after the 5s→30s interval change
**Section:** `run-shed-re-entrancy-via-self-pointing-on-stuck`
**Issue:** Today's cap expresses 12 hours at 5s (`entries_lifecycle.go`: 8640 attempts); reusing the attempt count at a 30s interval silently 6×es the watch window to 72h, while reusing the 12h wall-clock means 1440 — the plan writer cannot tell which is meant.
**Suggested fix:** Name the intended wall-clock budget (e.g. 12h) and derive the bounce count from it and the new interval.

## Verdict

REQUEST_CHANGES
The re-entrancy decision contradicts the engine's static OnStuck routing and understates its per-bounce commit+push cost.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
