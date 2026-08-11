MILL_REVIEW_BEGIN
# Review: gitexec: decide whether RunGit should return a typed error carrying stderr — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [NIT:consistency] predicate-sites rejects not routed to any section
**Location:** Batch 1 / Card 1, item 5 (`## Rejected alternatives`) and item 7 (`## How the migration goes`).
**Issue:** The discussion's `predicate-sites-are-real-and-must-stay-expressible` decision carries two `Rejected:` bullets (treating mixed tri-states as unmigrated debt; folding them into "raw, permanently correct") that explain why `ancestry.go`/`diff --cached --quiet` get the checked form instead of staying raw — item 5's explicit enumerated list omits both, and item 7's mixed-tri-state sub-part doesn't call them out either, so a literal-minded implementer could transcribe the disposition without the "why not raw" framing.
**Fix:** Name these two rejects inside item 7's mixed-tri-state sub-part (they belong there, not in item 5's shape-decision list).

## Verdict

APPROVE
Plan faithfully implements every Shared Decision and discussion decision; only one minor editorial-completeness NIT found.
MILL_REVIEW_END
