MILL_REVIEW_BEGIN
# Review: Fix stale .mhgo/ config-layer docs + cross-cutting stale-docs sweep — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-14
```

## Findings

### [NIT] board.md data-flow comment still says "_mhgo/ layers"
**Location:** `C:\Code\mhgo\wts\docs-stale-sweep\docs\modules\board.md:299`
**Issue:** The `## Data flow: upsert` diagram comment reads "resolve from _mhgo/ layers" — "layers" (plural) echoes the removed multi-layer framing; the card's corrections did not reach this inline comment.
**Fix:** Change to "resolve from `_mhgo/board.yaml` + defaults" or similar single-layer wording.

### [NIT] muxpoc.md omits the `%CLAUDE%` substitution from expandTpl description
**Location:** `C:\Code\mhgo\wts\docs-stale-sweep\docs\modules\muxpoc.md:144-145`
**Issue:** The configuration flags section lists `-launch` and `-resume` template flags and mentions `expandTpl` replaces `%SID%` and `%TASK%`, but the code also replaces `%CLAUDE%` with the resolved claude path at the call sites in `up.go` and `review.go`; the doc omits this substitution.
**Fix:** Add `%CLAUDE%` to the template-expansion description so the full set of substitutions is documented.

## Verdict

APPROVE
All thirteen cards are faithfully realised; shared decisions applied consistently across all batches; no blocking issues found.
MILL_REVIEW_END
