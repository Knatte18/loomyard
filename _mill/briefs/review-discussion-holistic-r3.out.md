MILL_REVIEW_BEGIN
# Review: webster: rewrite for flat card list

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] Plan-level body-section parsing ownership unstated
**Section:** Scope → In (planparser); decision *fork-prompt-plan-level-context*; Constraints (new invariant)
**Issue:** planparser's scope lists path normalization + the 14 checks + per-card `verify:`, but never says it extracts/exposes the plan-level `## Shared Decisions`, `## Rename mechanic`, and `## verify:` sections — yet `RenderForkPrompt` and the integration fork need them, and the new "planparser is the sole parser of `_lyx/plan/`" invariant forbids websterengine from reading them itself.
**Fix:** State that planparser parses and exposes the three plan-level body sections on the `Plan` struct (so no other package reads `_lyx/plan/`), and add a planparser test for that extraction.

### [NOTE] Integration-suite fork prompt rendering unspecified
**Section:** decision *integration-suite-fork-with-bisect*; Technical context (render.go)
**Issue:** The final fork runs the plan-level `## verify:` suite, but the discussion names only `RenderForkPrompt`/`RenderMasterPrompt`/`RenderBatchIndex` — no render function or template is identified for the integration-suite fork's prompt.
**Fix:** Note whether the integration fork reuses `RenderForkPrompt` or needs a dedicated renderer/template (a plan-phase determination is fine, but flag that the surface exists).

## Verdict

GAPS_FOUND
One scope gap: plan-level section parsing ownership vs. the new sole-parser invariant is unstated.
MILL_REVIEW_END
