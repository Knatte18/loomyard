MILL_REVIEW_BEGIN
# Review: Fix Bouncer anchor-path and run-dir clearing

```yaml
duration_s: 218.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Re-entry does not give the segment a fresh budget
**Section:** Stale-assertion inventory — defect 2, row `bouncer.go:75-79`
**Issue:** The rewritten budget rule ("the second generation starts on a fresh budget") is true for the Bouncer row only — `BurlerProducer.Call` never returns `Done` (`internal/shedadapters/burler.go:34`), so `episodeStuckCount` never resets for the Burler row and generation 2 runs on whatever is left of generation 1's `max_bounces: 5`, blocking the run with "bounce budget exhausted" on the Burler row rather than after a full second review.
**Fix:** State the two-row budget consequence explicitly and decide its disposition (accept the shrinking second generation, or change the budget), rather than asserting a fresh budget that only one of the two rows gets.

### [NIT:scope] Defect-2 enumeration reads three hand-named files
**Demoted-from:** BLOCKING
**Section:** Stale-assertion inventory — defect 2, method paragraph
**Issue:** The method is "read every comment in `bouncer.go`, `doc.go`, and `loom-recipe.yaml`", a hand-named file list, and it misses `internal/shedadapters/burler.go:43-50`, whose "the Bouncer row has the same unresetting property … the Bouncer's normally binds" paragraph is falsified by the same change; `burler.go` is absent from the Scope In-list too.
**Fix:** Restate the method as a claim-class scan over the whole `internal/shedadapters` package (plus the recipe) rather than three named files, and give the newly reached sites dispositions.

### [BLOCKING:design] A resume at the Bouncer also fires the clear
**Section:** Decision `clearing-trigger-is-the-approved-verdict-already-on-disk` (rationale, "as opposed to a *resume*, which … never re-enters from the top")
**Issue:** `shedengine.Run` documents that `StateFailed`/`StateBlocked` do not short-circuit and re-call `current_producer` (`internal/shedengine/run.go:94-96`), and `settle`'s only hard error is a `Commit` failure — which leaves the Bouncer as `current_producer` with an APPROVED verdict on disk, so the resume now archives the approved generation and re-reviews instead of retrying the commit as it does today. The rationale's fire/non-fire enumeration omits this case and the crash-window Decision covers only a *successful* `Commit`.
**Fix:** Add the commit-failure resume to the enumerated fire set with an explicit disposition (re-review vs. preserve-and-retry-commit), and add a test for it to the `shedadapters` list.

### [NIT:design] Cost of the newly-live Plan route unstated
**Section:** Problem, "Why now" / Decision `plan-revalidate-on-stuck-stays-plan-write`
**Issue:** With the live-lock removed, each `Plan-Revalidate` Stuck → `Plan-Write` → `Plan-Bouncer` pass now costs a complete LLM re-review generation instead of an instant replay, up to `Plan-Revalidate`'s own budget; the discussion states the removal but never states the cost it replaces the live-lock with.
**Fix:** One sentence bounding the worst case, beside the existing accepted-cost statement.

## Verdict

REQUEST_CHANGES
Budget-reset claim, defect-2 enumeration, and the commit-failure resume case need resolution.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
