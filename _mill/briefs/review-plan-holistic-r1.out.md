MILL_REVIEW_BEGIN
# Review: Shed-setup validity checker — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-21
```

## Findings

### [NIT:decision] Commit-body instruction for the Segment decision has no card home
**Location:** overview.md, `## Shared Decisions` — "`Segment` is never read, and no invariant is added" (applies to: all batches)
**Issue:** The decision's rationale requires "state the 'no new invariant' conclusion explicitly in the commit body rather than leaving it ambiguous," but no card's `Requirements:`/`Commit:` in batch 1 (or elsewhere) assigns this to a specific commit.
**Fix:** Add one sentence to Card 1 or Card 2's `Requirements:` instructing the commit body to state that no new `CONSTRAINTS.md` section/invariant is added and `loomshedAllowedImports`/`shedengine`'s allowlist are untouched.

## Verdict
Solid, internally consistent plan (verified loom's 13-row list is clean under the described algorithm) with one minor decision-traceability gap.
MILL_REVIEW_END
