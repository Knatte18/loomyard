MILL_REVIEW_BEGIN
# Review: loom: Plan-Review producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] Fixer edits to `_lyx/plan` are never re-validated
**Section:** "Recipe routing" / "`Plan-Burler`'s `fix-scope` is `overlay`"
**Issue:** `Plan-Burler` (fix-scope `overlay`, write surface = `_lyx/plan`) rewrites card files, then `Plan-Bouncer`'s approval routes straight to `Batchifier`, which only calls `batcher.Active` (`internal/loomshed/batchifier.go:44`) and never parses the plan — while rubric "Do not flag" item 1 explicitly forbids the judge from checking the sixteen `planparser` checks. A fixer-introduced format regression (bad card numbering, missing `Intent:`, malformed target path) therefore reaches `Webster`, whose row carries no `on_stuck`, and blocks the run with a human as the only recovery.
**Fix:** Decide and record the routing for post-fix mechanical re-validation — e.g. `Plan-Bouncer.on_done: Plan-Validate` with `Plan-Validate.on_done` unchanged, or an explicit rationale for why the gap is accepted — instead of leaving `on_done: Batchifier` unexamined.

### [BLOCKING:consistency] Docs "In" list contradicts the discussion's own stale-text scan
**Section:** "Scope → In → Docs" vs "Stale text is found by a scan"
**Issue:** The Docs bullet enumerates only `designs/loom.md`, `designs/shed-recipe.md`, `manifest/roadmap.md`, attaching "and the stale text the sweep finds" to `loom.md` alone — yet the scan decision itself names `manifest/designs/shed.md:91`/`:148` (both carrying the now-stale "`Plan-Review`'s stuck routes back to `Plan-Write`" routing example, verified present) and `manifest/designs/review-finding-classification.md:47`, neither of which is in the In-scope doc list. A plan writer building cards from the Docs bullet leaves both files stale.
**Fix:** State that the scan output is the doc inventory and the Docs bullet is illustrative, or extend the Docs bullet to name `designs/shed.md` and `designs/review-finding-classification.md`.

### [NIT:scope] `docs/overview.md` is Out, but the repo-wide scan hits it
**Section:** "Scope → Out" / stale-text scan pattern 1
**Issue:** `docs/overview.md:399` ("its Plan-Review segment's `Bouncer`") matches pattern 1 while the file is declared Out; the discussion gives no classification for that hit, so a plan writer must guess whether it is a deliberate no-op.
**Fix:** Add one line classifying the hit — the mention names the segment and becomes correct on landing, so no edit.

## Verdict

REQUEST_CHANGES
Post-fix plan re-validation is undecided; doc scope list contradicts the discussion's own scan.
MILL_REVIEW_END
