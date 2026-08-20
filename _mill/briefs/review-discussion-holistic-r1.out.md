MILL_REVIEW_BEGIN
# Review: shedadapters: Burler-round producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Retry token breaks round resolution and seed/judge
**Section:** "Round resolution from disk" + "Always `Stuck`, never `Done`"
**Issue:** A round whose attempt 1 died writes its review at the retry token (`round-3b-review.md`, per `treadleengine/roundfiles.go:18,42`), so the disk scan for the highest `N` in `round-<N>-review.md` still resolves 2 and the next `Call` re-runs round 3; the same gap makes the `Bouncer`'s stated seed-vs-judge discriminator ("the round's review artifact exists") read a completed round as a seed call.
**Fix:** State a round-resolution rule that recognises suffixed tokens (or record round identity in a token-independent artifact), and state it as the two-sided contract the `Bouncer` reproduces.

### [BLOCKING:consistency] Fail-safe focus parse feeds fail-loud `validate`
**Section:** "The next-round focus file" + "Cluster fan-out trimming"
**Issue:** Reading is "fail-safe end to end… never an error", yet a well-formed focus file whose `exclude_lenses` names every fan lens — or that arrives against a template `Profile` with an empty `ClusterFan` — is a `validate` hard error, so an LLM-authored directive does take the unattended round down, contradicting the stated rationale.
**Fix:** Decide the producer-side treatment of these two cases explicitly (drop/clamp the directive with a `logger.Warn`, or accept the hard error and drop the fail-safe claim).

### [BLOCKING:decision] Stale round artifact has no stated disposition
**Section:** Scope / "Round resolution from disk"
**Issue:** `treadle`'s stale-artifact move-aside is named as not ported and `shedadapters/archive.go`'s `archiveStaleOutputs` is listed only as a file to read; a leftover `round-<token>-review.md` at the computed token (crash, partial write, operator copy) would be read by `burlerengine.Run` (`engine.go:183`) as this round's output.
**Fix:** State whether `BurlerProducer` archives stale outputs at the round's paths before calling the runner, or explicitly why it need not.

### [NIT:design] Retry vs cancellation unspecified
**Section:** "Non-done outcomes: one deterministic retry" + "Cancellation"
**Issue:** The shared rule covers `Call` entry and exit, but not whether attempt 2 starts when the context was cancelled during attempt 1 — an expensive LLM round on an already-cancelled run.
**Fix:** Say the retry re-checks `ctx.Err()` before attempt 2 (or that it does not, with a reason).

### [NIT:decision] Unknown-lens exclusion decided only in Testing
**Section:** "Cluster fan-out trimming" vs Testing
**Issue:** "an exclusion naming a lens not in the fan is a no-op rather than an error" appears only as a test scenario, and sits against `ResolveFan`'s otherwise fail-loud posture on unknown fan/lens names (`config.go:91`).
**Fix:** Move that call into the `ClusterExclude` decision with its rationale.

### [NIT:design] `MaxBounces` now bounds review rounds
**Section:** "Always `Stuck`, never `Done`"
**Issue:** Every completed round returns `Stuck`, so `ProducerDef.MaxBounces` (default 10, persisted per producer per `shedengine/shed.go:21-30`) silently caps how many review rounds a segment can run; the discussion never says so.
**Fix:** Note the coupling in the decision and in the producer's doc comment.

## Verdict

REQUEST_CHANGES
Retry-token round identity, fail-safe/fail-loud focus contradiction, and stale artifacts need resolution.
MILL_REVIEW_END
