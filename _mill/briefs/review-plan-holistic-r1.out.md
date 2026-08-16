MILL_REVIEW_BEGIN
# Review: PATTERN directives: move from Go constants to stencil files — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5 / "claude-sonnet-5")
reviewed_file: plan/
date: 2026-08-16
```

## Findings

### [BLOCKING:scope] `stencils/stencils.go` omitted from Context on cards 5, 7, 8, 9
**Location:** Batch 2 (directive-read-path), cards 5, 7, 8, 9.
**Issue:** Each of these cards edits a test-stencil-seeding helper to reference the new `stencils.PatternDirectiveImplementer`/`PatternDirectiveReviewFix`/`PatternDirectiveOrchestrator` vars (declared in `stencils/stencils.go`, added by batch 1 card 2), but none lists `stencils/stencils.go` in `Context:`. Card 5 spells the three identifiers out verbatim in `Requirements:` but still omits the declaring file from `Context:`; cards 7, 8, and 9 neither list the file nor name the identifiers, instead saying only "the three new files from the `stencils` package's embedded defaults" — each edits a *different* file (`internal/loomengine/prompt_test.go`, `internal/burlerengine/prompt_test.go`, `internal/websterengine/template_test.go` respectively) that does not itself already contain these new names, so the exact identifiers are not derivable from any file in that card's own `Context:`/`Edits:`.
**Fix:** Add `stencils/stencils.go` to `Context:` on all four cards, and/or name the three exact identifiers explicitly in `Requirements:` for cards 7, 8, and 9 the way card 5 already does.

## Verdict

REQUEST_CHANGES
One Context-completeness gap in batch 2 (cards 5/7/8/9); everything else — decisions, sequencing, DAG, files-touched, moves — checks out.
MILL_REVIEW_END
