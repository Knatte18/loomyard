MILL_REVIEW_BEGIN
# Review: Fix Bouncer anchor-path and run-dir clearing

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class; exact version self-assessed as uncertain
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:scope] Stale-assertion inventory covers defect 1 only
**Section:** Scope → Stale-assertion inventory (and Scope's `shedadapters/bouncer.go` bullet)
**Issue:** The inventory's enumeration method greps for *root* claims, so it catches nothing falsified by defect 2, yet the clearing change falsifies four verified assertions in `internal/shedadapters/bouncer.go` that the In-list never mentions (that bullet names only "a clear-and-re-seed step at `Call` entry"): `NewBouncer`'s budget paragraph ("the Bouncer's only `Done` exits the segment and its episode therefore never resets" — `shedengine.episodeStuckCount` counts since the producer's own most recent `Done`, so a re-entered segment now *does* reset), `Call`'s doc ("branch into one of four modes — seed, re-bounce, judge, or replay" and "`shedengine.Done` … reachable only through harvest or replay"), `settle`'s doc ("would … re-approve and re-attempt the commit every pass … since `judged(n)` stays true on re-entry"), and the inline comment at `bouncer.go:308-310` ("an APPROVED replay is not a warning condition").
**Fix:** Add a second inventory (or extend the existing one) with a stated enumeration method and a per-site disposition for the assertions defect 2's fix falsifies, at minimum the four `bouncer.go` sites above.

### [NIT:consistency] Scope's yaml bullet excludes a yaml edit the task makes
**Section:** Scope → In (`contracts/recipes/loom-recipe.yaml` bullet) vs `plan-revalidate-on-stuck-stays-plan-write`
**Issue:** The bullet limits yaml work to "comment-only edits at the sites the inventory names", but the inventory names only lines 118-121 and 201-203; the `Plan-Revalidate` `on_stuck` comment (loom-recipe.yaml:178-181) that the decision says must be rewritten is not an inventory row and is not reachable by the inventory's grep.
**Fix:** Either add loom-recipe.yaml:178-181 as an inventory row with a **Rewrite** disposition, or reword the bullet to "the sites the inventory names, plus the `Plan-Revalidate` comment".

### [NIT:scope] Cross-invocation fire case has no stated disposition
**Section:** `clearing-trigger-is-the-approved-verdict-already-on-disk` (fire/non-fire enumeration) and Testing
**Issue:** The run directory is per-worktree and per-segment under `LoomReviewsDir` and is never cleaned between loom runs, so a run that re-enters a segment from the top after an earlier run left it APPROVED also fires the clear and re-reviews an unchanged artifact from round 1; the enumeration lists only in-run non-fire cases and the test list has no row for it.
**Fix:** State this case explicitly as intended (or not) in the decision, and add it to the non-triggering/triggering test list accordingly.

## Verdict

REQUEST_CHANGES
Defect-2 stale assertions in `bouncer.go` are unenumerated and outside the stated scope.
MILL_REVIEW_END
