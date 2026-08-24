MILL_REVIEW_BEGIN
# Review: loom: Discussion-Write producer

```yaml
duration_s: 226.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [NIT:scope] Stencil self-check line assigned here, never dispositioned
**Demoted-from:** BLOCKING
**Section:** Scope (In: stencil rewrite bullet) / Decisions
**Issue:** `manifest/roadmap.md:355` states, in a shipped `## Done` entry, that "instructing the writer agent to call these verbs belongs to the Planned `loom: Discussion-Write producer` … item", and `docs/overview.md:316` says `validate-discussion` exists "so a writer agent can self-check before handing off" — but the discussion's stencil-rewrite bullet lists only Step 0, the Fix 1 bound, the Step 3 category, and the HTML comment, and neither Scope Out nor any Decision mentions `lyx loom validate-discussion` at all (grep: zero occurrences in `_mill/discussion.md`).
**Fix:** Decide explicitly whether the rewritten stencil gains a self-check step calling `lyx loom validate-discussion` before the agent ends its turn, or record the deferral and name the item that inherits it; note that `blind-revalidate-bounce`'s "a fresh agent has an independent chance of not repeating it" rationale changes materially if the writer self-checks.

### [NIT:consistency] "One legitimate bounce path" is false against the recipe
**Section:** Decisions → `no-on-stuck` (and `blind-revalidate-bounce`)
**Issue:** `contracts/recipes/loom-recipe.yaml:35-37` already declares `Discussion-Review` with `on_stuck: Discussion-Write`, so two rows route into this one; the claim is true only because row 5 is still a `Stub` that never returns `Stuck`.
**Fix:** Qualify the sentence to "the one *live* bounce path today, with `Discussion-Review`'s already-declared `on_stuck` inert while that row is a Stub".

### [NIT:consistency] Sibling timeout comment goes stale by the same standard
**Section:** Decisions → `timeout-comment-only`
**Issue:** `internal/loomengine/template.yaml:4`'s `plan_timeout_min` comment reads "autonomous, shorter than the interview", a contrast that stops holding once the discussion run is itself autonomous — the same class of shipped inaccuracy the `--auto` correction is being made for.
**Fix:** Either extend the decision to that adjacent comment or state in one clause why it is left alone.

## Verdict

APPROVE
One committed-elsewhere stencil obligation has no stated disposition here.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
