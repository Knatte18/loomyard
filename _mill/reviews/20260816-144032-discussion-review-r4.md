MILL_REVIEW_BEGIN
# Review: PATTERN directives: move from Go constants to stencil files

```yaml
duration_s: 123.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-16
```

## Findings

### [NIT:consistency] Return matrix contradicts the banner-strip decision
**Demoted-from:** BLOCKING
**Section:** §Decisions → "Lazy read", row 4 **Issue:** The matrix states the success row as `(string(content), nil)` — the raw `Read` bytes — while the "`Directive` strips the leading banner" decision says the return is the `stencil.StripLeadingComment` body; verified `stencilstore.Read` (`reconcile.go:28-34`) is a bare `os.ReadFile` that strips nothing, so a plan writer following the matrix ships exactly the stamp-leak bug the other decision exists to prevent. **Fix:** Restate that row as the stripped body (e.g. `(stencil.StripLeadingComment(string(content)), nil)`), so the normative matrix and the strip decision agree.

### [NIT:scope] Design-doc correction list misses a fourth false claim
**Demoted-from:** BLOCKING
**Section:** §Decisions → "All five doc updates land in this commit" / §Q&A **Issue:** The discussion pins the design-doc edits at exactly three corrections (step 3, step 4, missing banner-strip step), but `manifest/designs/pattern-directive-stencils.md:57` also asserts `docs/shared-libs/stencil.md` is "the `Fill`/`FillOptional` contract these stencil files render through, unchanged by this task" — false by this task's own finding that these three are the first stencils never passing through `Fill` (they are injected as a values-map string). **Fix:** Add that "Related" bullet to the design-doc correction inventory, or state its disposition explicitly, so the shipped design doc carries no claim the code disproves.

## Verdict

APPROVE
Two contradictions: the return matrix drops the strip, and the design-doc correction list is short one false claim.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
