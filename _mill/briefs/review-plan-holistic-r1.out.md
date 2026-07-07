I have reviewed the complete plan and verified its claims against the source files.

MILL_REVIEW_BEGIN
# Review: Build internal/stencil: fill markdown prompt templates — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-07
```

## Findings

### [NIT] Empty-value guard only covers top-level markers
**Location:** Batch 1 / Card 1 (step 3) + Card 3
**Issue:** The pre-execute walk catches absent-or-empty only at depth-0; a branch-internal marker that is *present-but-empty* (`values["fasit"] == ""` inside a taken `{{if}}`) renders a silent blank, since `missingkey=error` fires only on absent keys, not empty values — a residual hole in the "no silently-blank fasit" guarantee if fasit ever lives in a branch.
**Fix:** Acceptable as the deliberately-pinned scope; Card 3 already documents it honestly ("do not over-promise"). Confirm no consumer places fasit/target inside a conditional branch.

### [NIT] Nil-Tree guard could also cover Root
**Location:** Batch 1 / Card 1 (step 3)
**Issue:** Requirement guards `tmpl.Tree` nil but dereferences `tmpl.Tree.Root.Nodes`; a defensive `Root != nil` check is cheap insurance for empty/comment-only parses.
**Fix:** Nil-check `tmpl.Tree.Root` before ranging `.Nodes` (or rely on empty ListNode — verify behaviour).

## Verdict

APPROVE
Sound, minimal, constraint-clean leaf plan; findings are optional polish.
MILL_REVIEW_END