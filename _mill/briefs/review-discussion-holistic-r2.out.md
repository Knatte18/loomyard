MILL_REVIEW_BEGIN
# Review: loom: pin the spawn/handover status schema + discussion-format.md

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] Relative inbound links to moved docs uncovered
**Section:** Scope (line 44-51) / Testing (line 328-330)
**Issue:** Scope limits fixups to "**full-path** inbound reference" (`docs/modules/…`) and the verify greps only `docs/modules/plan-format.md`/`builder-contract.md`, but ~12 *relative* markdown links will break and are caught by neither: `docs/overview.md:269,272,375` (`modules/…`), `docs/roadmap.md:57,60,74,189,195,333` (`modules/…`), `docs/modules/loom.md:117` (sibling `(builder-contract.md)`), `docs/reference/model-spec.md:5` (`../modules/plan-format.md`) — all resolve to the moving files, none are "bare-filename," none match the full-path grep.
**Fix:** Extend scope + the link-integrity verify to relative links (`modules/…`, `../modules/…`, and same-folder siblings from loom.md), rewriting each to its new `reference/…`/`../reference/…` target; note loom.md, overview.md, and roadmap.md are already edited by this task so their inbound links must move in the same pass.

### [NOTE] `docs/reviews/builder-review-prompt.md` not enumerated
**Section:** Scope (line 44-48)
**Issue:** This file holds 8 full-path `docs/modules/{plan-format,builder-contract}.md` references (lines 47,50,61,62,68,150,244,452) — the largest single cluster — yet the Scope parenthetical (which reads as the exhaustive "wherever it appears" list) omits it; only the generic "docs" bucket + repo-wide grep cover it.
**Fix:** Name `docs/reviews/builder-review-prompt.md` explicitly in the Scope enumeration so the plan writer batches it, not just implicitly via the grep verify.

## Verdict

GAPS_FOUND
Relative-path inbound links to the relocated docs are excluded by both scope wording and verify grep.
MILL_REVIEW_END
