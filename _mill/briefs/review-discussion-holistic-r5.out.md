MILL_REVIEW_BEGIN
# Review: pattern told-geometry

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [NIT:consistency] Tautology comment cannot just widen its count
**Section:** Decisions → *anchoring-proof-relocates-to-the-call-site*
**Issue:** The block at `cmd/lyx/constructoranchoring_test.go:73-78`/`129-134` opens "The two planparser rows below…" and is immediately followed by the two planparser rows (79-80/135-136); the `pattern.FileHere` row (88/144) is separated from them by six rows (81-87/137-143) that are **not** tautological (they take `l`, not `l.AnchorPath()`). Literally "widening the count" to "three rows below" would misdescribe those six.
**Fix:** Say the amended comment must name the covered rows explicitly (the two `planparser` rows plus the `pattern.File` row), not restate a contiguous count.

### [NIT:scope] Stencils-helper hedge is already resolvable
**Section:** Testing → `internal/loomengine`
**Issue:** "confirm it seeds the three `pattern-directive-*` stencils, and extend it or seed them alongside it if it does not" leaves an open branch a plan writer must re-derive; `internal/loomengine/prompt_test.go:36-48` and `internal/burlerengine/prompt_test.go:43-56` both already seed all three from the embedded defaults.
**Fix:** State the seeding as an established fact for both packages and drop the extend-if-not branch.

## Verdict

APPROVE
Claims verified against source; decisions complete, no blocking gaps.
MILL_REVIEW_END
