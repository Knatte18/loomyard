MILL_REVIEW_BEGIN
# Review: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [BLOCKING:consistency] Card 18's Done-move leaves a stale "Planned `fabric`" cross-reference
**Location:** batch 6 / card 18 (`manifest/roadmap.md`)
**Issue:** Card 18 moves the whole `fabric: unified-repo view` bullet from `## Planned` to `## Done`, but the Someday section's `config: repo-wide default + per-worktree override` item (line 129) says `` `fabric.yaml` is the sole exception... — see the Planned `fabric` item's slices 7-10) ``. Once the move lands this becomes false — the item is no longer under Planned — and the card's requirements never mention this cross-reference.
**Fix:** Add a line to card 18 updating that Someday-section cross-reference (e.g. "see the `fabric` item in Done, slices 7-10") in the same edit.

## Verdict

REQUEST_CHANGES
Card 18's Planned→Done move for the fabric roadmap entry orphans a same-file cross-reference elsewhere in the Someday section.
MILL_REVIEW_END
