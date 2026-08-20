MILL_REVIEW_BEGIN
# Review: Bouncer: the generic review-gate producer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Card 6's ResolveRound hard-error path skips cancelErr
**Location:** batch 3 (`03-bouncer-producer.md`), Card 6, `Call`'s sequence, the `ResolveRound` step.
**Issue:** Every other non-success exit in this design explicitly "consults `cancelErr` first" (seed, re-bounce, every judge degradation), matching `ctx.go`'s own doc comment ("every non-success return path... consults cancelErr first," in Card 6's own Context) and the sibling `perch.go`/`singlellm.go` pattern (e.g. `resolveRunID`'s error path). `_mill/discussion.md`'s "Cancellation and the output pointer" section states the rule with no carve-out: "cancelErr replaces every result except a genuinely parsed verdict." Card 6's text for the `ResolveRound` hard-error branch — "a non-nil `err` is returned wrapped as a hard error" — never mentions checking `cancelErr`, so a ctx cancelled during the `ResolveRound` stat scan would surface as the RunDir error instead of `context.Canceled`, diverging from every other path in the same producer and from the discussion's absolute rule.
**Fix:** Add "consulting `cancelErr` first" to the `ResolveRound`-error branch of Card 6's Requirements, exactly as every other branch already states.

### [NIT:consistency] Stencils.go family-ordering rationale is factually wrong
**Location:** batch 2 (`02-stencils.md`), Card 4, the registration paragraph.
**Issue:** Card 4 justifies placing the new `Bouncer*` vars "after the burler vars and before the treadle vars so the file's family grouping stays alphabetical-by-family as it reads today." Verified against `contracts/stencils/stencils.go`: the on-disk order is landing, loom, burler, treadle, webster, pattern — not alphabetical (pattern sits last, after webster; landing/loom precede burler). The placement instruction itself is harmless (it matches the file's actual existing physical layout), but the stated rationale misdescribes the codebase.
**Fix:** Drop or correct the "alphabetical-by-family" claim; state instead that the new vars simply follow the file's existing physical ordering.

## Verdict

REQUEST_CHANGES
Card 6 must add the missing cancelErr check on the ResolveRound hard-error path before this plan is otherwise sound.
MILL_REVIEW_END
