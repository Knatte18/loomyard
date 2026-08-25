MILL_REVIEW_BEGIN
# Review: loom: Webster-Review producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] cluster-fan failure is at run time, not construction
**Section:** `### the-round-runs-a-cluster-fan` (open risk) + `## Testing`
**Issue:** The risk's mitigation rests on a false premise: `burlerRoundEntry` passes `cluster-fan` through unvalidated (`internal/shedrecipe/entries_burler.go:163-167,219`), and `ResolveFan` runs only inside `Profile.validate`, called from `burlerengine.Engine.Run` (`internal/burlerengine/engine.go:98`, `profile.go:107-113`) — so a deleted `standard` fan fails the Webster-Burler *round*, mid-run at the end of the whole loom run, never `loomrecipe.New`, and the analogy to `NewBouncer`'s eager `rubric_stencil` probe does not hold.
**Fix:** Restate the risk against the real failure point, and decide whether an eager fan check belongs in this task (which would touch `shedrecipe`, currently listed Out).

### [BLOCKING:consistency] Testing claims a construction failure that cannot occur
**Section:** `## Testing` → `internal/shedrecipe/entries_burler_test.go`
**Issue:** "assert an unknown fan name fails construction with a named error" contradicts the code above — `burlerRoundProfile` accepts any `cluster-fan` string and never resolves it, so no such error exists to assert.
**Fix:** Reduce the new case to the key→`Profile.ClusterFan` mapping, or make the eager-validation change explicit and in scope.

### [BLOCKING:decision] `profile.fasit` for Webster-Burler is never decided
**Section:** `### diff-derivation-lives-in-the-rubric-not-in-go`
**Issue:** Only `profile.target` is decided (instructions, no paths); `Profile.validate` hard-errors when Fasit carries neither Paths nor Instructions (`internal/burlerengine/profile.go:77-79`), and both shipped Burler rows set one, so the row as specified fails at its first round.
**Fix:** Decide Fasit explicitly — most plausibly `paths: [_lyx/plan]` plus instructions naming the plan as the answer key — in the same decision.

### [BLOCKING:design] Lowest-batch `startSha` is overwritten by recovery
**Section:** `### diff-derivation-lives-in-the-rubric-not-in-go`
**Issue:** `recoverSpawn` writes a fresh `BatchState` with `StartSHA` = current HEAD into the *same* `State.Batches[batchNumber]` slot (`internal/websterengine/recoverbatch.go:190-225`, pinned by `recoverbatch_test.go:372`), so a recovered batch 1 yields a base that already contains its own committed work — the diff silently under-scopes, and the stated fallback only fires on an unreadable/empty `state.json`.
**Fix:** State the derivation over a recovery-safe value (e.g. the minimum `startSha` across all batches is equally unsafe — say what is used) or accept and record it as a named risk with its consequence.

### [BLOCKING:consistency] Round artifacts are under `.lyx`, not `_lyx`
**Section:** `### fix-scope-is-source-and-there-is-no-commit-seam`
**Issue:** The segment's run root is `loomengine.LoomReviewsDir` = `LoomScratchDir(l)/reviews` (`internal/loomengine/config.go:157-158`), i.e. under `.lyx`, whose doc states it is deliberately ephemeral — the discussion says `_lyx/loom/reviews/webster/`, and the rubric's do-not-flag list would transcribe that wrong path; the `run_subdir` value itself is also never explicitly decided.
**Fix:** Correct the path to the `.lyx` tree and state the `run_subdir` value the two new rows share.

### [BLOCKING:scope] The `Webster` row's outbound edge is not covered
**Section:** `### perch-row-names-and-routing` / `## Scope`
**Issue:** `Webster` currently carries `on_done: Webster-Review` (`contracts/recipes/loom-recipe.yaml:182-184`); the routing decision enumerates only the new pair's edges and the dropped `on_stuck: Webster`, leaving the inbound edge into the segment unstated.
**Fix:** State `Webster` → `on_done: Webster-Bouncer` as part of the routing decision.

### [NIT:decision] `loom-status-spec.md` example names the retired row
**Section:** `## Scope` (doc updates)
**Issue:** `contracts/specs/loom-status-spec.md:118-126` uses `Webster-Review` as an example `current_producer`/history value ("stuck with no OnStuck target"); the discussion gives it no disposition.
**Fix:** Say whether the example is updated to `Webster-Bouncer` or deliberately left alone.

## Verdict

REQUEST_CHANGES
Two decisions rest on false premises about where validation runs; Fasit and one routing edge undecided.
MILL_REVIEW_END
