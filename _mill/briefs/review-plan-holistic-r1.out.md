MILL_REVIEW_BEGIN
# Review: Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-08
```

## Findings

### [BLOCKING] Card 5 cites a Shared Decision that does not exist
**Location:** batch 01-location-threading, Card 5 (`resolveLocation`'s doc-comment requirement)
**Issue:** Requirements say `resolveLocation`'s doc comment "must state the limit from the `out-of-hub-synthesized-location` decision," but no such decision appears in the overview's `## Shared Decisions` and the batch's own "Batch-local decisions … none" line confirms none was added there either — it is an orphaned reference, unlike every other named decision in this batch (`anchor-stays-worktreepath`, `no-nil-layout-check`, `stale-citation-left-alone`, all of which do resolve).
**Fix:** Either add an `### Decision: out-of-hub-synthesized-location` entry to `## Shared Decisions` capturing the limit prose Card 5 already inlines, or drop the decision-name citation and keep only the inline prose.

## Verdict
REQUEST_CHANGES
Fix the one orphaned decision citation in Card 5 before implementation; everything else verified sound against source.
MILL_REVIEW_END
