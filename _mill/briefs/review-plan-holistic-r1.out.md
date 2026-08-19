MILL_REVIEW_BEGIN
# Review: invariants and docs for the told-geometry rule — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (model id claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-19
```

## Findings

### [BLOCKING:design] Batch 3 forward-references batch 1's new heading with no `depends-on`
**Location:** batch 3 (`03-docgo-audit.md`), cards 7-11; `00-overview.md`'s Batch Index; the "the new invariant's anchor slug" Shared Decision.
**Issue:** All five batch-3 cards instruct "Point at `CONSTRAINTS.md`'s Told-Geometry Invariant by name" — a heading only batch 1 creates — yet batch 3 declares `depends-on: []`, and the anchor-slug Shared Decision's "Applies to" line is scoped to batches 1, 4, 5 only, on the stated premise that batch 3 has no such reference. That premise is false against the batch's own card text: verified against every one of cards 7-11, each carries the identical instruction. If batch 3 lands before batch 1 (nothing in the DAG forbids it), five `doc.go` files cite an invariant section that does not yet exist in the tree; this is not machine-checked (Go doc comments are outside both `docslink_test.go`'s scan sources and the Fabric Vocabulary `.md` walk), so it would land silently rather than failing a build.
**Fix:** Add `depends-on: [1]` to batch 3 in both `00-overview.md` and `03-docgo-audit.md`'s own frontmatter, and add batch 3 to the anchor-slug decision's "Applies to" list alongside 1, 4, 5.

## Verdict

REQUEST_CHANGES
Fix batch 3's missing `depends-on: [1]`; every other cross-file, source-grounded, and register claim I checked verified accurate.
MILL_REVIEW_END
