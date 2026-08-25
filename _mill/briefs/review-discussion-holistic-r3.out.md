MILL_REVIEW_BEGIN
# Review: loom: Plan-Review producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class model, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] Commit-failure outcome left to the plan writer
**Section:** "`Plan-Burler`'s `fix-scope` is `overlay`…" + Technical context (`shedadapters/bouncer.go`) + Testing.
**Issue:** The failure semantics of a non-nil `Commit` returning an error are explicitly deferred ("think about whether a commit belongs before or after that guarantee, and say so in the plan"), while Testing pre-commits to `degrade`; verified at `bouncer.go:218-228`, `degrade` consults `cancelErr` first and returns `Stuck` with an empty pointer — so it breaks `settle`'s own documented contract that a parsed verdict is the one exception `cancelErr` never applies to, routes an APPROVED plan to `on_stuck: Plan-Burler` for a fixer round with no BLOCKING findings and no `ensureFocus(n+1)` call, and on re-entry `judged(n)` is still true so `settle` re-approves and re-commits until `max_bounces` is spent.
**Fix:** Decide in the discussion: the ordering relative to the cancellation guarantee, and whether a commit failure is `degrade`/`Stuck`, a hard error, or `Done`-with-warning — and state the resulting bounce behaviour on an already-APPROVED verdict.

### [BLOCKING:consistency] `_lyx/plan` resolves under two different roots
**Section:** "`artifact_paths` is the plan **directory**" and the commit-seam decision.
**Issue:** `entries_bouncer.go:90` resolves `artifact_paths` via `resolveUnderRoot(..., env.WorktreeRoot, p)` and `wiring.go:121` fills `WorktreeRoot: location.WorktreePath()`, while `Env.CommitPlan` (`wiring.go:170`) commits `planparser.PlanDirRel()` anchored at `AnchorPath()` — the root `planparser`'s own invariant requires. With a non-`"."` `AnchorRel` the row would judge `<worktreeRoot>/_lyx/plan` and commit `<anchorPath>/_lyx/plan`; the discussion never states which root the recipe entry resolves against.
**Fix:** State the resolution root for both recipe rows explicitly and record what happens when `AnchorRel != "."` (accepted, or resolved via `Env.AnchorPath`), noting the shipped Discussion pair carries the same shape.

### [BLOCKING:scope] Stale-text scan's three patterns miss commit-seam claims
**Section:** "Stale text is found by a scan, not by a hand-written list".
**Issue:** The scan covers `Plan-Review`, `NamePlanReview`, and the fourteen-count only; `internal/loomengine/config.go:152-154` (`LoomReviewsDir`'s doc) asserts "there is no commit seam for a Bouncer row" as the reason the reviews tree is ephemeral — this change makes that literally false, and the file is listed under **Out** so a plan writer would skip it.
**Fix:** Add a fourth scan pattern for commit-seam/Bouncer-commits claims (`commit seam`, `Bouncer row`) and say whether the `Out` list excludes only production behaviour in that file, not its doc comments.

### [NIT:design] Rubric's decision-record path form and the judge's cwd
**Section:** "How the judge reaches the decision record".
**Issue:** The rubric names `_lyx/discussion/decision-record.md`, but the judge's other reading-order entry (`{{.artifacts}}`, `bouncer-template-judge.md:21`) is explicitly absolute; nothing states the working directory the judge session resolves a relative path from.
**Fix:** State that the path is worktree-anchor-relative and that the judge session runs at the anchor, so the relative form resolves.

### [NIT:consistency] Post-review commit reuses Plan-Write's commit message
**Section:** the commit-seam decision.
**Issue:** `Env.CommitPlan` hard-codes `"loom: plan artifacts for <slug>"` (`wiring.go:171`), so the post-approval commit is indistinguishable in history from `Plan-Write`'s own.
**Fix:** Note the reuse as accepted, or say the message stays generic on purpose.

## Verdict

REQUEST_CHANGES
Commit-failure semantics, the plan-dir resolution root, and one scan-pattern gap need deciding.
MILL_REVIEW_END
